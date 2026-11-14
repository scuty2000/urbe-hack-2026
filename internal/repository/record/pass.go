package record

import "time"

// PassRecord maps to pickup_passes: a single-use, short-lived authorisation to
// hand an item to a searcher. Only the token hash is persisted.
type PassRecord struct {
	ID               string    `gorm:"primaryKey;column:id"`
	ClaimID          string    `gorm:"column:claim_id;not null"`
	ItemID           string    `gorm:"column:item_id;not null"`
	SearcherID       string    `gorm:"column:searcher_id;not null"`
	CustodianID      string    `gorm:"column:custodian_id;not null"`
	TokenHash        string    `gorm:"column:token_hash;not null"`
	IssuedAt         time.Time `gorm:"column:issued_at"`
	ExpiresAt        time.Time `gorm:"column:expires_at"`
	Status           string    `gorm:"column:status;not null"`
	IdentityRequired bool      `gorm:"column:identity_required;not null"`
	ReturnChannel    string    `gorm:"column:return_channel;not null"`
	PaymentID        string    `gorm:"column:payment_id;not null"`
	ApprovalID       string    `gorm:"column:approval_id;not null"`
	FeeID            string    `gorm:"column:fee_id;not null"`
	RewardNoticeID   string    `gorm:"column:reward_notice_id;not null"`
}

// TableName overrides GORM's pluralised table name.
func (*PassRecord) TableName() string { return "pickup_passes" }
