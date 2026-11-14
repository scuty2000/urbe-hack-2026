package record

import "time"

// CustodyEventRecord maps to custody_events, the append-only timeline of an
// item's lifecycle (rights assignment, custody, matching, handover, ...).
type CustodyEventRecord struct {
	ID          string    `gorm:"primaryKey;column:id"`
	ItemID      string    `gorm:"column:item_id;not null"`
	Type        string    `gorm:"column:type;not null"`
	ActorID     string    `gorm:"column:actor_id;not null"`
	Timestamp   time.Time `gorm:"column:timestamp"`
	Notes       string    `gorm:"column:notes;not null"`
	EvidenceRef string    `gorm:"column:evidence_ref;not null"`
}

// TableName overrides GORM's pluralised table name.
func (*CustodyEventRecord) TableName() string { return "custody_events" }
