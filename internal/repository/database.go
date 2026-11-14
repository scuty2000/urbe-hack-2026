package repository

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
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
