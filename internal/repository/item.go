package repository

import (
	"context"
	"strings"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"gorm.io/gorm"
)

const (
	defaultItemLimit = 50
	maxItemLimit     = 200
)

func (r *repo) CreateItem(ctx context.Context, item record.ItemRecord) error {
	item = withItemDefaults(item)
	if err := r.gormdb.WithContext(ctx).Create(&item).Error; err != nil {
		return wrapErr("CreateItem", err)
	}
	return nil
}

func (r *repo) GetItemByID(ctx context.Context, id string) (record.ItemRecord, error) {
	var item record.ItemRecord
	if err := r.gormdb.WithContext(ctx).Where("id = ?", id).First(&item).Error; err != nil {
		return item, wrapErr("GetItemByID", err)
	}
	return item, nil
}

// UpdateItem performs a partial update: only non-zero fields of item are
// written, so callers can change a status without re-sending the whole row.
func (r *repo) UpdateItem(ctx context.Context, item record.ItemRecord) error {
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = time.Now().UTC()
	}
	item.UpdatedAt = utc(item.UpdatedAt)
	item.FoundAt = utc(item.FoundAt)
	item.RightsAssignedAt = utcPtr(item.RightsAssignedAt)
	item.TransferredAt = utcPtr(item.TransferredAt)
	item.LegalTermStart = utcPtr(item.LegalTermStart)
	item.OfferedToFinderAt = utcPtr(item.OfferedToFinderAt)
	item.DisposedAt = utcPtr(item.DisposedAt)
	res := r.gormdb.WithContext(ctx).
		Model(&record.ItemRecord{}).
		Where("id = ?", item.ID).
		Updates(item)
	if res.Error != nil {
		return wrapErr("UpdateItem", res.Error)
	}
	if res.RowsAffected == 0 {
		return notFound("UpdateItem", item.ID)
	}
	return nil
}

func (r *repo) ListItemsByCustodian(ctx context.Context, custodianID string) ([]record.ItemRecord, error) {
	var items []record.ItemRecord
	if err := r.gormdb.WithContext(ctx).
		Where("custodian_id = ?", custodianID).
		Order("created_at DESC, rowid DESC").
		Find(&items).Error; err != nil {
		return nil, wrapErr("ListItemsByCustodian", err)
	}
	return items, nil
}

func (r *repo) SearchItems(ctx context.Context, filter ItemFilter) ([]record.ItemRecord, error) {
	query := r.gormdb.WithContext(ctx).Model(&record.ItemRecord{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.Zone != "" {
		query = query.Where("found_zone = ?", filter.Zone)
	}
	if filter.CustodianID != "" {
		query = query.Where("custodian_id = ?", filter.CustodianID)
	}
	if filter.LegalFinderID != "" {
		query = query.Where("legal_finder_id = ?", filter.LegalFinderID)
	}
	// Every free-text token must match at least one searchable column, which
	// keeps the SQLite fallback useful without a vector index.
	for _, token := range strings.Fields(filter.Query) {
		like := "%" + token + "%"
		query = query.Where(
			"(category LIKE ? OR brand LIKE ? OR model LIKE ? OR color LIKE ? OR condition LIKE ? OR found_zone LIKE ? OR private_profile LIKE ?)",
			like, like, like, like, like, like, like,
		)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultItemLimit
	}
	if limit > maxItemLimit {
		limit = maxItemLimit
	}

	var items []record.ItemRecord
	if err := query.Order("created_at DESC, rowid DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, wrapErr("SearchItems", err)
	}
	return items, nil
}

func (r *repo) ListPickupDue(ctx context.Context, now time.Time) ([]record.ItemRecord, error) {
	var items []record.ItemRecord
	err := r.gormdb.WithContext(ctx).
		Table("items").
		Select("items.*").
		Joins("JOIN custody ON custody.item_id = items.id AND custody.closed_at IS NULL").
		Where("custody.due_at IS NOT NULL AND custody.due_at <= ?", now.UTC()).
		Where("items.status IN ?", []string{"in_custody", "pickup_due", "sensitive_pickup"}).
		Order("custody.due_at ASC, items.id ASC").
		Find(&items).Error
	if err != nil {
		return nil, wrapErr("ListPickupDue", err)
	}
	return items, nil
}

// CreateItemWithCustody inserts the item, its custody row and the first
// custody event in a single transaction.
func (r *repo) CreateItemWithCustody(
	ctx context.Context,
	item record.ItemRecord,
	custody record.CustodyRecord,
	event record.CustodyEventRecord,
) error {
	item = withItemDefaults(item)
	custody = withCustodyTimes(custody)
	event = withEventDefaults(event)
	return r.gormdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&item).Error; err != nil {
			return wrapErr("CreateItemWithCustody item", err)
		}
		if err := tx.Create(&custody).Error; err != nil {
			return wrapErr("CreateItemWithCustody custody", err)
		}
		if err := tx.Create(&event).Error; err != nil {
			return wrapErr("CreateItemWithCustody event", err)
		}
		return nil
	})
}

func withItemDefaults(item record.ItemRecord) record.ItemRecord {
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}
	if item.PrivateProfile == "" {
		item.PrivateProfile = "{}"
	}
	if item.Photos == "" {
		item.Photos = "[]"
	}
	item.CreatedAt = utc(item.CreatedAt)
	item.UpdatedAt = utc(item.UpdatedAt)
	item.FoundAt = utc(item.FoundAt)
	item.RightsAssignedAt = utcPtr(item.RightsAssignedAt)
	item.TransferredAt = utcPtr(item.TransferredAt)
	item.LegalTermStart = utcPtr(item.LegalTermStart)
	item.OfferedToFinderAt = utcPtr(item.OfferedToFinderAt)
	item.DisposedAt = utcPtr(item.DisposedAt)
	return item
}

func withEventDefaults(event record.CustodyEventRecord) record.CustodyEventRecord {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	event.Timestamp = utc(event.Timestamp)
	return event
}
