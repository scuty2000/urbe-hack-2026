package record

import "time"

// ClaimRecord maps to claims: a searcher's ownership claim on an item, with
// the verification outcome and the pickup approval.
type ClaimRecord struct {
	ID             string     `gorm:"primaryKey;column:id"`
	ItemID         string     `gorm:"column:item_id;not null"`
	SearcherID     string     `gorm:"column:searcher_id;not null"`
	Status         string     `gorm:"column:status;not null"`
	ReturnChannel  string     `gorm:"column:return_channel;not null"`
	Confidence     float64    `gorm:"column:confidence"`
	Evidence       string     `gorm:"column:evidence;not null"`
	ApprovalBy     string     `gorm:"column:approval_by;not null"`
	ApprovedAt     *time.Time `gorm:"column:approved_at"`
	PickupPassID   string     `gorm:"column:pickup_pass_id;not null"`
	RewardNoticeID string     `gorm:"column:reward_notice_id;not null"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
}

// TableName overrides GORM's pluralised table name.
func (*ClaimRecord) TableName() string { return "claims" }
