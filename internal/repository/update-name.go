package repository

import (
	"context"
	"fmt"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) UpdateNameByID(ctx context.Context, id int64, name string) error {
	res := r.gormdb.WithContext(ctx).Model(&record.NameRecord{}).Where("id = ?", id).Update("name", name)
	if res.Error != nil {
		return fmt.Errorf("UpdateNameByID id=%d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("UpdateNameByID id=%d: %w", id, errs.ErrNotFound)
	}
	return nil
}
