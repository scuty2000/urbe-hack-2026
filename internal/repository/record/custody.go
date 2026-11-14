package record

import "time"

// CustodyRecord maps to custody: one row per custodian handover. The current
// custody of an item is the latest row whose closed_at is NULL.
type CustodyRecord struct {
	ID                string     `gorm:"primaryKey;column:id"`
	ItemID            string     `gorm:"column:item_id;not null"`
	CustodianType     string     `gorm:"column:custodian_type;not null"`
	CustodianID       string     `gorm:"column:custodian_id;not null"`
	LegalFinderID     string     `gorm:"column:legal_finder_id;not null"`
	RightsAssignedAt  *time.Time `gorm:"column:rights_assigned_at"`
	ApprovalAuthority string     `gorm:"column:approval_authority;not null"`
	MaxCustodyDays    int        `gorm:"column:max_custody_days"`
	AcceptedAt        *time.Time `gorm:"column:accepted_at"`
	DueAt             *time.Time `gorm:"column:due_at"`
	ClosedAt          *time.Time `gorm:"column:closed_at"`
}

// TableName overrides GORM's pluralised table name.
func (*CustodyRecord) TableName() string { return "custody" }
