package record

import "time"

// AuditLogRecord maps to audit_logs: an append-only record of who did what to
// which entity. Metadata is opaque JSON owned by the domain layer.
type AuditLogRecord struct {
	ID        string    `gorm:"primaryKey;column:id"`
	ActorID   string    `gorm:"column:actor_id;not null"`
	Action    string    `gorm:"column:action;not null"`
	Entity    string    `gorm:"column:entity;not null"`
	EntityID  string    `gorm:"column:entity_id;not null"`
	Metadata  string    `gorm:"column:metadata;not null"`
	Timestamp time.Time `gorm:"column:timestamp"`
}

// TableName overrides GORM's pluralised table name.
func (*AuditLogRecord) TableName() string { return "audit_logs" }
