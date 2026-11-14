package record

import "time"

// PaymentRecord maps to payments: a provider-agnostic record of a fee payment.
type PaymentRecord struct {
	ID          string     `gorm:"primaryKey;column:id"`
	FeeID       string     `gorm:"column:fee_id;not null"`
	Provider    string     `gorm:"column:provider;not null"`
	ProviderRef string     `gorm:"column:provider_ref;not null"`
	Status      string     `gorm:"column:status;not null"`
	PaidAt      *time.Time `gorm:"column:paid_at"`
	ReceiptURL  string     `gorm:"column:receipt_url;not null"`
}

// TableName overrides GORM's pluralised table name.
func (*PaymentRecord) TableName() string { return "payments" }
