package repository

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"gorm.io/gorm"
)

type repo struct {
	gormdb *gorm.DB
}

func newRepository(gormdb *gorm.DB) repo {
	return repo{gormdb}
}

func NewRepository(gormdb *gorm.DB) Repository {
	return new(newRepository(gormdb))
}

// Database wraps the Repository adding transactions.
type Database struct {
	repo
}

// New creates a new Database backed by the provided GORM instance.
func NewDatabase(gormdb *gorm.DB) (Database, error) {
	if gormdb == nil {
		return Database{}, errors.New("can't init database gorm.DB is required")
	}
	return Database{newRepository(gormdb)}, nil
}

// Repository returns a Repository.
func (db *Database) Repository() Repository {
	return db
}

// GormDB exposes the underlying GORM handle. It is intended for migrations,
// demo seeding/reset and reporting queries that span several aggregates; all
// regular persistence should go through the typed repository methods.
func (db *Database) GormDB() *gorm.DB {
	return db.gormdb
}

// RunInTransaction executes fn within a single database transaction.
// If fn returns an error, the transaction is rolled back; otherwise committed.
func (db *Database) RunInTransaction(ctx context.Context, fn func(context.Context, Repository) error) error {
	return db.gormdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, NewRepository(tx))
	})
}

func (db *Database) MigrateUp(migrationDir fs.FS) error {
	dbconn, err := db.gormdb.DB()
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	goose.SetBaseFS(migrationDir)
	if err := goose.SetDialect(db.gormdb.Dialector.Name()); err != nil {
		return err
	}
	if err := goose.Up(dbconn, "."); err != nil {
		return err
	}
	return nil
}

// wrapErr translates driver-level outcomes into the application's sentinel
// errors, keeping the original error wrapped for logging.
//
//   - gorm.ErrRecordNotFound -> errs.ErrNotFound
//   - UNIQUE/PRIMARY KEY violations -> errs.ErrConflict
//   - context deadline exceeded -> errs.ErrTimeout
func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return fmt.Errorf("%s: %w", op, errs.ErrNotFound)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s: %w", op, errs.ErrTimeout)
	case isConstraintViolation(err):
		return fmt.Errorf("%s: %w: %w", op, errs.ErrConflict, err)
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

func isConstraintViolation(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "constraint failed") && strings.Contains(msg, "primary key")
}

func notFound(op, id string) error {
	return fmt.Errorf("%s id=%s: %w", op, id, errs.ErrNotFound)
}

// utc normalises a timestamp to UTC before it is persisted. SQLite compares
// DATETIME values lexicographically, so mixing offsets would corrupt due-date
// and expiry queries; the repository is the single place that guarantees UTC.
func utc(t time.Time) time.Time { return t.UTC() }

// utcPtr normalises an optional timestamp without mutating the caller's value.
func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	normalized := t.UTC()
	return &normalized
}
