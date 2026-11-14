package record

import "time"

// NotificationRecord maps to notifications: an in-app message for a user.
type NotificationRecord struct {
	ID          string     `gorm:"primaryKey;column:id"`
	RecipientID string     `gorm:"column:recipient_id;not null"`
	Type        string     `gorm:"column:type;not null"`
	Payload     string     `gorm:"column:payload;not null"`
	ReadAt      *time.Time `gorm:"column:read_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
}

// TableName overrides GORM's pluralised table name.
func (*NotificationRecord) TableName() string { return "notifications" }
