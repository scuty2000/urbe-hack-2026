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
	"syscall"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/gin-contrib/cors"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/health"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	nameservice "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/name"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/d3"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/embeddings"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/qdrant"
	platformsqlite "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/sqlite"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/storage"
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
	authService := auth.New(db)
	healthService := health.NewHealthService(dbconn, health.Checks{
		QdrantURL:         cfg.Qdrant.URL,
		QdrantAPIKey:      cfg.Qdrant.APIKey,
		QdrantEnabled:     cfg.Features.Qdrant,
		EmbeddingsURL:     cfg.Embeddings.URL,
		EmbeddingsModel:   cfg.Embeddings.Model,
		EmbeddingsKey:     cfg.Embeddings.Key,
		EmbeddingsEnabled: cfg.Embeddings.URL != "",
	})

	// D3 is optional: without a URL the catalog falls back to the form values.
	var cataloger itemdomain.Cataloger
	if cfg.Features.D3 && cfg.D3.URL != "" {
		cataloger = d3.New(cfg.D3.URL, cfg.D3.Key, cfg.D3.Model, cfg.D3.Timeout)
	}
	itemService := itemdomain.New(db, itemdomain.Deps{
		Storage: storage.NewLocal(cfg.Storage.DataDir, cfg.Storage.MaxUploadMB),
		D3:      cataloger,
	})
	custodyService := custodydomain.New(db, custodydomain.Deps{
		MaxDays:        cfg.Civic.CustodyDays,
		SensitiveHours: cfg.Sensitive.CustodyHours,
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
			Logger:          logger,
			CORS:            corsConfig,
			NameService:     nameService,
			HealthService:   healthService,
			AuthService:     authService,
			ItemService:     itemService,
			CustodyService:  custodyService,
			SearchService:   searchService,
			DemoReset:       func(ctx context.Context) error { return seed.Reset(ctx, db, searchService) },
			DemoResetSecret: cfg.Demo.ResetSecret,
			TrustedProxies:  cfg.Web.TrustedProxies,
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
