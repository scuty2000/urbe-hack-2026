package repository

import (
	"context"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

const (
	defaultAuditLimit = 100
	maxAuditLimit     = 500
)

func (r *repo) CreateAuditLog(ctx context.Context, entry record.AuditLogRecord) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	if entry.Metadata == "" {
		entry.Metadata = "{}"
	}
	entry.Timestamp = utc(entry.Timestamp)
	if err := r.gormdb.WithContext(ctx).Create(&entry).Error; err != nil {
		return wrapErr("CreateAuditLog", err)
	}
	return nil
}

// ListAuditLogs returns matching entries in chronological order, which reads
// naturally as the history of an entity.
func (r *repo) ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]record.AuditLogRecord, error) {
	query := r.gormdb.WithContext(ctx).Model(&record.AuditLogRecord{})
	if filter.Entity != "" {
		query = query.Where("entity = ?", filter.Entity)
	}
	if filter.EntityID != "" {
		query = query.Where("entity_id = ?", filter.EntityID)
	}
	if filter.ActorID != "" {
		query = query.Where("actor_id = ?", filter.ActorID)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultAuditLimit
	}
	if limit > maxAuditLimit {
		limit = maxAuditLimit
	}

	var entries []record.AuditLogRecord
	if err := query.
		Order("timestamp ASC, rowid ASC").
		Limit(limit).
		Find(&entries).Error; err != nil {
		return nil, wrapErr("ListAuditLogs", err)
	}
	return entries, nil
}
