package record

// FeeRecord maps to fees: the PA tariff estimate/final amount for an item.
// Direct returns before transfer are free (amount_cents = 0).
type FeeRecord struct {
	ID          string `gorm:"primaryKey;column:id"`
	ItemID      string `gorm:"column:item_id;not null"`
	Applies     string `gorm:"column:applies;not null"`
	PATariffID  string `gorm:"column:pa_tariff_id;not null"`
	CustodyDays int    `gorm:"column:custody_days"`
	AmountCents int64  `gorm:"column:amount_cents"`
	Currency    string `gorm:"column:currency;not null"`
	Status      string `gorm:"column:status;not null"`
}

// TableName overrides GORM's pluralised table name.
func (*FeeRecord) TableName() string { return "fees" }
