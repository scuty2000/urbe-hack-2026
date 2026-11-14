/*
Command seed opens the SQLite database, applies the embedded migrations and
replaces the registry data with the deterministic demo dataset.

Configuration is read from the same environment variables as the web API:

	API_DB_FILENAME        SQLite file (default db/lostfound.db)
	API_QDRANT_URL         optional Qdrant base URL
	API_EMBEDDINGS_URL     optional vLLM embeddings endpoint

When both vector endpoints are configured the searchable demo items are also
indexed. A vector failure never aborts the seed: the SQLite fallback keeps the
demo working.
*/
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/embeddings"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/qdrant"
	platformsqlite "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/sqlite"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := context.Background()

	dbFilename := envOr("API_DB_FILENAME", "db/lostfound.db")
	gormDB, err := gorm.Open(sqlite.Open(platformsqlite.DSN(dbFilename)), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("opening database %q: %w", dbFilename, err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("getting database connection: %w", err)
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	if err := gormDB.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		return fmt.Errorf("enabling WAL: %w", err)
	}
	if err := gormDB.Exec("PRAGMA busy_timeout=5000;").Error; err != nil {
		return fmt.Errorf("setting busy_timeout: %w", err)
	}

	db, err := repository.NewDatabase(gormDB)
	if err != nil {
		return fmt.Errorf("creating repository: %w", err)
	}
	if err := db.MigrateUp(os.DirFS("db/migrations")); err != nil {
		return fmt.Errorf("migrating database: %w", err)
	}

	var indexer seed.Indexer
	if vectors, embedder, err := vectorClients(ctx, log); err != nil {
		return err
	} else if vectors != nil && embedder != nil {
		indexer = search.New(db, search.Deps{Embedder: embedder, Vectors: vectors})
	}

	if err := seed.Reset(ctx, db, indexer); err != nil {
		return err
	}
	log.Info("demo data seeded", slog.String("database", dbFilename))
	return nil
}

// vectorClients builds the optional Qdrant/embeddings clients. Missing URLs
// disable indexing rather than failing the seed.
func vectorClients(ctx context.Context, log *slog.Logger) (search.VectorStore, search.Embedder, error) {
	qdrantURL := strings.TrimSpace(os.Getenv("API_QDRANT_URL"))
	embeddingsURL := strings.TrimSpace(os.Getenv("API_EMBEDDINGS_URL"))
	if qdrantURL == "" || embeddingsURL == "" {
		return nil, nil, nil
	}

	vectors := qdrant.New(
		qdrantURL,
		envOr("API_QDRANT_COLLECTION", qdrant.DefaultCollection),
		os.Getenv("API_QDRANT_API_KEY"),
		durationOr("API_QDRANT_TIMEOUT", 5*time.Second),
	)
	if err := vectors.EnsureCollection(ctx); err != nil {
		log.Warn("qdrant collection unavailable; seeding without indexing", slog.Any("err", err))
		return nil, nil, nil
	}

	embedder := embeddings.New(
		embeddingsURL,
		envOr("API_EMBEDDINGS_MODEL", embeddings.DefaultModel),
		os.Getenv("API_EMBEDDINGS_KEY"),
		durationOr("API_EMBEDDINGS_TIMEOUT", 5*time.Second),
		intOr("API_EMBEDDINGS_BATCH_SIZE", embeddings.DefaultBatchSize),
	)
	return vectors, embedder, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func durationOr(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func intOr(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
