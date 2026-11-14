package repository

import (
	"context"
	"fmt"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateName(ctx context.Context, name string) (int64, error) {
	rec := record.NameRecord{Value: name}
	if err := r.gormdb.WithContext(ctx).Create(&rec).Error; err != nil {
		return 0, fmt.Errorf("CreateName: %w", err)
	}
	return rec.ID, nil
}
