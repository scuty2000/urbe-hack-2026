package repository

import (
	"context"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateCustody(ctx context.Context, custody record.CustodyRecord) error {
	custody = withCustodyTimes(custody)
	if err := r.gormdb.WithContext(ctx).Create(&custody).Error; err != nil {
		return wrapErr("CreateCustody", err)
	}
	return nil
}

func withCustodyTimes(custody record.CustodyRecord) record.CustodyRecord {
	custody.RightsAssignedAt = utcPtr(custody.RightsAssignedAt)
	custody.AcceptedAt = utcPtr(custody.AcceptedAt)
	custody.DueAt = utcPtr(custody.DueAt)
	custody.ClosedAt = utcPtr(custody.ClosedAt)
	return custody
}

func (r *repo) GetCustodyByID(ctx context.Context, id string) (record.CustodyRecord, error) {
	var custody record.CustodyRecord
	if err := r.gormdb.WithContext(ctx).Where("id = ?", id).First(&custody).Error; err != nil {
		return custody, wrapErr("GetCustodyByID", err)
	}
	return custody, nil
}

// GetCustodyByItemID returns the item's current custody. Open rows win over
// closed ones; ties are broken by insertion order (rowid), so the most recent
// handover is returned.
func (r *repo) GetCustodyByItemID(ctx context.Context, itemID string) (record.CustodyRecord, error) {
	var custody record.CustodyRecord
	err := r.gormdb.WithContext(ctx).
		Where("item_id = ?", itemID).
		Order("closed_at IS NULL DESC, rowid DESC").
		First(&custody).Error
	if err != nil {
		return custody, wrapErr("GetCustodyByItemID", err)
	}
	return custody, nil
}

// ListOpenCustodyByItemIDs returns the open custody row of every requested
// item in a single query. The dashboard needs the deadline for its whole
// in-custody list, so this avoids one custody lookup per item.
func (r *repo) ListOpenCustodyByItemIDs(ctx context.Context, itemIDs []string) (map[string]record.CustodyRecord, error) {
	result := make(map[string]record.CustodyRecord, len(itemIDs))
	if len(itemIDs) == 0 {
		return result, nil
	}

	var rows []record.CustodyRecord
	if err := r.gormdb.WithContext(ctx).
		Where("item_id IN ? AND closed_at IS NULL", itemIDs).
		Order("rowid ASC").
		Find(&rows).Error; err != nil {
		return nil, wrapErr("ListOpenCustodyByItemIDs", err)
	}
	for _, row := range rows {
		// A later row wins if legacy data contains more than one open custody.
		result[row.ItemID] = row
	}
	return result, nil
}

// UpdateCustody performs a partial update; pointer timestamps can be set but
// cannot be cleared with this method.
func (r *repo) UpdateCustody(ctx context.Context, custody record.CustodyRecord) error {
	custody = withCustodyTimes(custody)
	res := r.gormdb.WithContext(ctx).
		Model(&record.CustodyRecord{}).
		Where("id = ?", custody.ID).
		Updates(custody)
	if res.Error != nil {
		return wrapErr("UpdateCustody", res.Error)
	}
	if res.RowsAffected == 0 {
		return notFound("UpdateCustody", custody.ID)
	}
	return nil
}

// UpdateCustodyFields is the map-based escape hatch for values that a struct
// update cannot write: accepted_at=nil, closed_at=nil, custodian_id="".
// Allowed columns are listed in custodyUpdateColumns.
func (r *repo) UpdateCustodyFields(ctx context.Context, id string, fields map[string]any) error {
	return r.updateFields(ctx, "UpdateCustodyFields", "custody", id, fields, custodyUpdateColumns)
}

func (r *repo) CreateCustodyEvent(ctx context.Context, event record.CustodyEventRecord) error {
	event = withEventDefaults(event)
	if err := r.gormdb.WithContext(ctx).Create(&event).Error; err != nil {
		return wrapErr("CreateCustodyEvent", err)
	}
	return nil
}

func (r *repo) ListCustodyEvents(ctx context.Context, itemID string) ([]record.CustodyEventRecord, error) {
	var events []record.CustodyEventRecord
	if err := r.gormdb.WithContext(ctx).
		Where("item_id = ?", itemID).
		Order("timestamp ASC, rowid ASC").
		Find(&events).Error; err != nil {
		return nil, wrapErr("ListCustodyEvents", err)
	}
	return events, nil
}
