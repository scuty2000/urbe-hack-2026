package repository

import (
	"context"
	"fmt"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) DeleteNameByID(ctx context.Context, id int64) error {
	res := r.gormdb.WithContext(ctx).Delete(&record.NameRecord{}, id)
	if res.Error != nil {
		return fmt.Errorf("DeleteNameByID id=%d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("DeleteNameByID id=%d: %w", id, errs.ErrNotFound)
	}
	return nil
}
