/*
Webapi is the executable for the main web server.
It builds a web server around APIs from `service/api`.
Webapi connects to external resources needed (database) and starts two web servers: the API web server, and the debug.
Everything is served via the API web server, except debug variables (/debug/vars) and profiler infos (pprof).

Usage:

	webapi [flags]

Flags and configurations are handled automatically by the code in `load-configuration.go`.

Return values (exit codes):

	0
		The program ended successfully (no errors, stopped by signal)

	> 0
		The program ended due to an error

Note that this program will update the schema of the database to the latest version available (embedded in the
executable during the build).
*/
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/gin-contrib/cors"
	_ "github.com/mattn/go-sqlite3"
	agentdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	feedomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/health"
	institutiondomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/institution"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	nameservice "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/name"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	rewarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/reward"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	wizarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/wizard"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/d3"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/embeddings"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/mapbox"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/qdrant"
	platformsqlite "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/sqlite"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/storage"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/trace"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// main is the program entry point. The only purpose of this function is to call run() and set the exit code if there is
// any error
func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error: ", err)
		os.Exit(1)
	}
}

// run executes the program. The body of this function should perform the following steps:
// * reads the configuration
// * creates and configure the logger
// * connects to any external resources (like databases, authenticators, etc.)
// * creates an instance of the service/api package
// * starts the principal web server (using the service/api.Router.Handler() for HTTP handlers)
// * waits for any termination event: SIGTERM signal (UNIX), non-recoverable server error, etc.
// * closes the principal web server
func run() error {
	// Load Configuration and defaults
	cfg, err := loadConfiguration()
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			return nil
		}
		return err
	}

	// Init logging
	var level slog.Level
	switch cfg.Logger.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return fmt.Errorf("invalid log level: %s", cfg.Logger.Level)
	}

	var logWriter io.Writer
	switch cfg.Logger.File {
	case "stderr":
		logWriter = os.Stderr
	case "stdout":
		logWriter = os.Stdout
	default:
		file, err := os.OpenFile(cfg.Logger.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("opening log file: %w", err)
		}
		defer func() {
			_ = file.Close()
		}()
		logWriter = file
	}

	logger := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug, // enable if in debug mode
	}))

	logger.Info("Initializing application...")

	// Start Database
	logger.Info("initializing database support")

	// 1. Establish GORM connection pool
	gormDB, err := gorm.Open(
		sqlite.Open(platformsqlite.DSN(cfg.DB.Filename)),
		&gorm.Config{
			SkipDefaultTransaction: true,
		},
	)
	if err != nil {
		logger.Error("Failed to connect to database: %v", slog.Any("err", err))
		return fmt.Errorf("connecting to database: %w", err)
	}

	// WAL improves concurrent read/write behaviour; busy_timeout avoids
	// sporadic "database is locked" errors under concurrent requests.
	if err := gormDB.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		logger.Warn("enabling SQLite WAL mode", slog.Any("err", err))
	}
	if err := gormDB.Exec("PRAGMA busy_timeout=5000;").Error; err != nil {
		logger.Warn("setting SQLite busy_timeout", slog.Any("err", err))
	}

	dbconn, err := gormDB.DB()
	if err != nil {
		logger.Error("error getting database connection", slog.Any("err", err))
		return fmt.Errorf("getting database connection: %w", err)
	}

	defer func(dbconn *sql.DB) {
		err := dbconn.Close()
		if err != nil {
			logger.Error("error closing database connection", slog.Any("err", err))
		}
	}(dbconn)

	// 2. Initialize concrete repository Database
	db, err := repository.NewDatabase(gormDB)
	if err != nil {
		logger.Error("error creating AppDatabase", slog.Any("err", err))
		return fmt.Errorf("creating AppDatabase: %w", err)
	}

	if err := dbconn.Ping(); err != nil {
		logger.Error("error pinging AppDatabase", slog.Any("err", err))
		return fmt.Errorf("pinging AppDatabase: %w", err)
	}

	// 3. Migrate the DB with goose
	if err := db.MigrateUp(os.DirFS("db" + string(os.PathSeparator) + "migrations")); err != nil {
		logger.Error("error migrating database", slog.Any("err", err))
		return fmt.Errorf("migrating database: %w", err)
	}

	// 4. Seed the demo accounts. Idempotent, so restarts are safe.
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		logger.Error("error seeding demo users", slog.Any("err", err))
		return fmt.Errorf("seeding demo users: %w", err)
	}

	// Start (main) API server
	logger.Info("initializing API server")

	// Wire domain services
	nameService := nameservice.New(db)
	platformService := platform.New(db)
	authService := auth.New(db, auth.Deps{Auditor: platformService})
	agentMode := "unavailable"
	if cfg.Features.DeepSeek && strings.TrimSpace(cfg.DeepSeek.Key) != "" {
		agentMode = "deepseek"
	}
	healthService := health.NewHealthService(dbconn, health.Checks{
		QdrantURL:         cfg.Qdrant.URL,
		QdrantAPIKey:      cfg.Qdrant.APIKey,
		QdrantEnabled:     cfg.Features.Qdrant,
		EmbeddingsURL:     cfg.Embeddings.URL,
		EmbeddingsModel:   cfg.Embeddings.Model,
		EmbeddingsKey:     cfg.Embeddings.Key,
		EmbeddingsEnabled: cfg.Embeddings.URL != "",
		D3Enabled:         cfg.Features.D3 && strings.TrimSpace(cfg.D3.URL) != "",
		AgentMode:         agentMode,
	})

	// Model outputs and tool calls are appended to a JSONL file under the
	// storage data directory (a bind mount in Docker Compose). Tracing is a
	// debugging aid: a failure to open the file only disables it.
	var tracer *trace.Recorder
	if tracePath := traceFilePath(cfg); tracePath != "" {
		opened, err := trace.Open(tracePath)
		if err != nil {
			logger.Warn("model trace disabled", slog.Any("err", err))
		} else {
			tracer = opened
			defer func() { _ = tracer.Close() }()
			logger.Info("model trace enabled", slog.String("file", tracePath))
		}
	}

	// D3 is optional: without a URL the catalog falls back to the form values.
	var cataloger itemdomain.Cataloger
	if cfg.Features.D3 && cfg.D3.URL != "" {
		cataloger = d3.New(cfg.D3.URL, cfg.D3.Key, cfg.D3.Model, cfg.D3.Timeout).WithTrace(tracer)
	}
	itemService := itemdomain.New(db, itemdomain.Deps{
		Storage:       storage.NewLocal(cfg.Storage.DataDir, cfg.Storage.MaxUploadMB),
		D3:            cataloger,
		AllowD3Images: cfg.D3.AllowImages,
	})
	custodyService := custodydomain.New(db, custodydomain.Deps{
		MaxDays:        cfg.Civic.CustodyDays,
		SensitiveHours: cfg.Sensitive.CustodyHours,
		Notifier:       platformService,
	})

	// The custody deadline is enforced by the domain checks, but the pickup
	// queue still needs to advance without an operator. A one-minute ticker
	// moves elapsed items to pickup_due; the admin tick remains available as a
	// manual override.
	tickerCtx, stopTicker := context.WithCancel(context.Background())
	defer stopTicker()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-tickerCtx.Done():
				return
			case ticked := <-ticker.C:
				if _, err := custodyService.MarkPickupDue(tickerCtx, ticked); err != nil {
					logger.Warn("marking pickup due", slog.Any("err", err))
				}
			}
		}
	}()
	// Ownership is judged by DeepSeek only: D3 stays for item classification
	// and the wizard. Without DeepSeek (or on a provider outage) the
	// deterministic local scorer answers, so a claim never blocks.
	var ownershipJudge claimdomain.Completer
	if cfg.Features.DeepSeek && strings.TrimSpace(cfg.DeepSeek.Key) != "" {
		ownershipJudge = deepseek.New(cfg.DeepSeek.URL, cfg.DeepSeek.Key, cfg.DeepSeek.Model, cfg.DeepSeek.Timeout).
			WithTrace(tracer, "verification")
	}
	ownershipEngine := claimdomain.DeepSeekEngine(ownershipJudge)
	claimService := claimdomain.New(db, claimdomain.Deps{
		Engine:   ownershipEngine,
		Notifier: platformService,
	})
	// PA fees use the seeded active tariff; the configured values are a
	// fallback for a fresh environment without tariff rows.
	feeService := feedomain.New(db, feedomain.Deps{
		Tariff: feedomain.Tariff{
			DailyRateCents: int64(cfg.PA.DailyRateCents),
			FixedFeeCents:  int64(cfg.PA.FixedFeeCents),
			Currency:       cfg.PA.Currency,
		},
		Notifier: platformService,
	})
	rewardService := rewarddomain.New(db)

	// Pickup passes are signed with the dedicated AUTH_PASS_SECRET. An empty
	// secret is a deliberate fail-closed mode: passes cannot be issued.
	passService := passdomain.New(db, passdomain.Deps{
		PassSecret: cfg.Auth.PassSecret,
		PassTTL:    cfg.Auth.PassTTL,
		Fees:       feeService,
		Notifier:   platformService,
	})

	// Vector search is optional: without Qdrant or embeddings the search
	// service falls back to SQLite, so the demo always works.
	embeddingsClient := embeddings.New(
		cfg.Embeddings.URL,
		cfg.Embeddings.Model,
		cfg.Embeddings.Key,
		cfg.Embeddings.Timeout,
		cfg.Embeddings.BatchSize,
	)
	qdrantClient := qdrant.New(
		cfg.Qdrant.URL,
		cfg.Qdrant.Collection,
		cfg.Qdrant.APIKey,
		cfg.Qdrant.Timeout,
	)
	var vectors search.VectorStore
	if cfg.Features.Qdrant && cfg.Qdrant.URL != "" {
		vectors = qdrantClient
		ensureCtx, cancel := context.WithTimeout(context.Background(), cfg.Qdrant.Timeout)
		if err := qdrantClient.EnsureCollection(ensureCtx); err != nil {
			logger.Warn("qdrant collection unavailable; search falls back to SQLite", slog.Any("err", err))
		}
		cancel()
	}
	searchService := search.New(db, search.Deps{
		Embedder: embeddingsClient,
		Vectors:  vectors,
	})

	// The conversational agent is the searcher's public interface. It uses the
	// search service and the claim policy server-side; without a DeepSeek key
	// (or with the feature disabled) it runs the deterministic mock.
	deepSeekKey := ""
	if cfg.Features.DeepSeek {
		deepSeekKey = cfg.DeepSeek.Key
	}
	agentService := agentdomain.New(db, agentdomain.Deps{
		Search:          searchService,
		Claim:           claimService,
		Engine:          ownershipEngine,
		DeepSeekKey:     deepSeekKey,
		DeepSeekURL:     cfg.DeepSeek.URL,
		DeepSeekModel:   cfg.DeepSeek.Model,
		DeepSeekTimeout: cfg.DeepSeek.Timeout,
		MaxToolRounds:   cfg.DeepSeek.MaxToolRounds,
		Trace:           tracer,
	})

	// Finder wizard: D3 classifies the photos, DeepSeek writes the photo plan
	// and prefill from the structured result, Mapbox supplies the zone and the
	// walking matrix. The Mapbox client fails closed without a token, so the
	// service falls back to haversine and manual zone entry.
	var wizardPlanner wizarddomain.Planner
	if cfg.Features.DeepSeek && cfg.DeepSeek.Key != "" {
		wizardPlanner = wizarddomain.NewDeepSeekPlanner(
			deepseek.New(cfg.DeepSeek.URL, cfg.DeepSeek.Key, cfg.DeepSeek.Model, cfg.DeepSeek.Timeout).WithTrace(tracer, "wizard"),
		)
	}
	mapboxClient := mapbox.New("", cfg.Mapbox.Token, 5*time.Second)
	wizardService := wizarddomain.New(db, wizarddomain.Deps{
		Items:    itemService,
		Planner:  wizardPlanner,
		Router:   mapboxClient,
		Geocoder: mapboxClient,
	})

	// The institution service owns the PA pickup queue, the transfer into PA
	// custody, official operator intake, claim review and the unclaimed flow.
	institutionService := institutiondomain.New(db, institutiondomain.Deps{
		Items:    itemService,
		Custody:  custodyService,
		Claim:    claimService,
		Pass:     passService,
		Search:   searchService,
		Fee:      feeService,
		Notifier: platformService,
	})

	// Make a channel to listen for an interrupt or terminate signal from the OS.
	// Use a buffered channel because the signal package requires it.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Make a channel to listen for errors coming from the listener. Use a
	// buffered channel so the goroutine can exit if we don't collect this error.
	serverErrors := make(chan error, 1)

	// Create the API router
	corsConfig := cors.Config{
		AllowAllOrigins:  cfg.CORS.AllowAllOrigins,
		AllowOrigins:     cfg.CORS.AllowedOrigins,
		AllowMethods:     cfg.CORS.AllowedMethods,
		AllowHeaders:     cfg.CORS.AllowedHeaders,
		AllowCredentials: cfg.CORS.AllowCredentials,
		MaxAge:           time.Duration(cfg.CORS.MaxAge) * time.Second,
	}
	// gin-contrib/cors rejects AllowedOrigins together with AllowAllOrigins.
	if corsConfig.AllowAllOrigins {
		if corsConfig.AllowCredentials {
			logger.Warn("CORS allows all origins with credentials; browsers may reject this insecure combination")
		}
		corsConfig.AllowOrigins = nil
	}

	apirouter, err := handler.New(
		handler.Config{
			Logger:             logger,
			CORS:               corsConfig,
			NameService:        nameService,
			HealthService:      healthService,
			AuthService:        authService,
			ItemService:        itemService,
			CustodyService:     custodyService,
			InstitutionService: institutionService,
			SearchService:      searchService,
			ClaimService:       claimService,
			PassService:        passService,
			FeeService:         feeService,
			RewardService:      rewardService,
			PlatformService:    platformService,
			AgentService:       agentService,
			WizardService:      wizardService,
			DemoReset:          func(ctx context.Context) error { return seed.Reset(ctx, db, searchService) },
			DemoResetSecret:    cfg.Demo.ResetSecret,
			TrustedProxies:     cfg.Web.TrustedProxies,
		},
	)
	if err != nil {
		logger.Error("error creating the API server instance", slog.Any("err", err))
		return fmt.Errorf("creating the API server instance: %w", err)
	}
	apihandler := apirouter.Handler()

	apihandler, err = registerWebUI(apihandler)
	if err != nil {
		logger.Error("error registering web UI handler", slog.Any("err", err))
		return fmt.Errorf("registering web UI handler: %w", err)
	}

	// Create the API server
	apiserver := http.Server{
		Addr:              cfg.Web.APIHost,
		Handler:           apihandler,
		ReadTimeout:       cfg.Web.ReadTimeout,
		ReadHeaderTimeout: cfg.Web.ReadTimeout,
		WriteTimeout:      cfg.Web.WriteTimeout,
	}

	// Start the service listening for requests in a separate goroutine
	go func() {
		logger.Info("API listening on", slog.String("addr", apiserver.Addr))
		serverErrors <- apiserver.ListenAndServe()
		logger.Info("stopping API server")
	}()

	// Waiting for shutdown signal or POSIX signals
	select {
	case err := <-serverErrors:
		// Non-recoverable server error
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		logger.Info("signal received, start shutdown", slog.String("signal", sig.String()))

		// Stop the custody ticker before the server drains.
		stopTicker()

		// Asking API server to shut down and load shed.
		err := apirouter.Close()
		if err != nil {
			logger.Warn("graceful shutdown of apirouter error", slog.Any("err", err))
		}

		// Give outstanding requests a deadline for completion.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Web.ShutdownTimeout)
		defer cancel()

		// Asking listener to shut down and load shed.
		err = apiserver.Shutdown(ctx)
		if err != nil {
			logger.Warn("error during graceful shutdown of HTTP server", slog.Any("err", err))
			err = apiserver.Close()
		}

		// Log the status of this shutdown.
		switch {
		case sig == syscall.SIGSTOP:
			return errors.New("integrity issue caused shutdown")
		case err != nil:
			return fmt.Errorf("could not stop server gracefully: %w", err)
		}
	}

	return nil
}

// traceFilePath resolves the JSONL trace location: the configured file, or
// traces/model-trace.jsonl under the storage data directory. "off" disables it.
func traceFilePath(cfg WebAPIConfiguration) string {
	configured := strings.TrimSpace(cfg.Trace.File)
	switch strings.ToLower(configured) {
	case "off", "false", "none", "disabled":
		return ""
	case "":
		return filepath.Join(cfg.Storage.DataDir, "traces", "model-trace.jsonl")
	default:
		return configured
	}
}
