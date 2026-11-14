package record

// TariffRecord maps to tariffs: the configurable PA tariff used to compute a
// fee after an item has been transferred to the institution.
type TariffRecord struct {
	ID             string `gorm:"primaryKey;column:id"`
	DailyRateCents int64  `gorm:"column:daily_rate_cents"`
	FixedFeeCents  int64  `gorm:"column:fixed_fee_cents"`
	Currency       string `gorm:"column:currency;not null"`
	Active         bool   `gorm:"column:active;not null"`
}

// TableName overrides GORM's pluralised table name.
func (*TariffRecord) TableName() string { return "tariffs" }
