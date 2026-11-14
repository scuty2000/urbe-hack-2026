package repository

import (
	"context"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// pickupStatuses are the two states in which an intermediary must hand an item
// over to the public administration.
var pickupStatuses = []string{"pickup_due", "sensitive_pickup"}

// ListInstitutionPickups returns the items waiting for the PA, sensitive items
// first and then ordered by zone and holding intermediary. The caller still has
// to load the open custody row for the deadline: this query deliberately stays
// a simple item read so the domain owns the pickup grouping.
func (r *repo) ListInstitutionPickups(ctx context.Context) ([]record.ItemRecord, error) {
	var items []record.ItemRecord
	err := r.gormdb.WithContext(ctx).
		Where("status IN ?", pickupStatuses).
		Order("CASE status WHEN 'sensitive_pickup' THEN 0 ELSE 1 END, found_zone ASC, custodian_id ASC, id ASC").
		Find(&items).Error
	if err != nil {
		return nil, wrapErr("ListInstitutionPickups", err)
	}
	return items, nil
}
