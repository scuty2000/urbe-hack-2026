package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"gorm.io/gorm"
)

func (r *repo) GetNameByID(ctx context.Context, id int64) (record.NameRecord, error) {
	var rec record.NameRecord
	err := r.gormdb.WithContext(ctx).First(&rec, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return rec, fmt.Errorf("GetNameByID id=%d: %w", id, errs.ErrNotFound)
	}
	if err != nil {
		return rec, fmt.Errorf("GetNameByID id=%d: %w", id, err)
	}
	return rec, nil
}
