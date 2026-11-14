// Package testutil provides helpers shared by integration-style tests.
//
// NewTestDB opens an isolated SQLite database in a temporary directory and
// applies the real goose migrations from db/migrations, so repository and
// domain tests run against the same schema as the application without ever
// touching the developer's database.
package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	platformsqlite "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/sqlite"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewTestDB returns a migrated repository.Database backed by a temporary
// SQLite file. The file and connection are released when the test finishes.
func NewTestDB(t *testing.T) repository.Database {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	gormDB, err := gorm.Open(sqlite.Open(platformsqlite.DSN(dbPath)), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("testutil: opening sqlite database: %v", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("testutil: getting sql.DB: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("testutil: closing sqlite database: %v", err)
		}
	})

	// WAL and busy_timeout mirror the production connection settings and make
	// the temp database safe for the concurrent reads used by some tests.
	if err := gormDB.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		t.Fatalf("testutil: enabling WAL: %v", err)
	}
	if err := gormDB.Exec("PRAGMA busy_timeout=5000;").Error; err != nil {
		t.Fatalf("testutil: setting busy_timeout: %v", err)
	}

	db, err := repository.NewDatabase(gormDB)
	if err != nil {
		t.Fatalf("testutil: creating repository: %v", err)
	}

	migrationsDir := migrationsDir(t)
	if err := db.MigrateUp(os.DirFS(migrationsDir)); err != nil {
		t.Fatalf("testutil: applying migrations from %s: %v", migrationsDir, err)
	}

	return db
}

// migrationsDir locates db/migrations relative to this source file, so tests
// work regardless of the package's working directory.
func migrationsDir(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("testutil: cannot determine source location")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations")
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("testutil: migrations directory %s: %v", dir, err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("testutil: resolving migrations directory: %v", err)
	}
	return abs
}
