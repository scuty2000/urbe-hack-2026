package repository

import (
	"context"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// Repository is the full set of persistence operations exposed by this package.
type Repository interface {
	GetNameByID(ctx context.Context, id int64) (record.NameRecord, error)
	UpdateNameByID(ctx context.Context, id int64, name string) error
	CreateName(ctx context.Context, name string) (int64, error)
	DeleteNameByID(ctx context.Context, id int64) error
}

// Transactor provides transaction support.
type Transactor interface {
	RunInTransaction(ctx context.Context, fn func(context.Context, Repository) error) error
}
