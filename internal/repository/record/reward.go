package record

import "time"

// RewardNoticeRecord maps to reward_notices: the art. 930 reward requested by
// the legal finder and shown to the searcher. The system records it but never
// charges it; payment_status says how it was settled outside the system.
type RewardNoticeRecord struct {
	ID                string     `gorm:"primaryKey;column:id"`
	ItemID            string     `gorm:"column:item_id;not null"`
	LegalFinderID     string     `gorm:"column:legal_finder_id;not null"`
	Requested         bool       `gorm:"column:requested;not null"`
	AmountCents       int64      `gorm:"column:amount_cents"`
	ShownToSearcherAt *time.Time `gorm:"column:shown_to_searcher_at"`
	PaymentStatus     string     `gorm:"column:payment_status;not null"`
}

// TableName overrides GORM's pluralised table name.
func (*RewardNoticeRecord) TableName() string { return "reward_notices" }
