package record

import "time"

// UserRecord maps to users: an actor with a role used for authentication and
// authorization.
type UserRecord struct {
	ID        string    `gorm:"primaryKey;column:id"`
	Role      string    `gorm:"column:role;not null"`
	Name      string    `gorm:"column:name;not null"`
	Email     string    `gorm:"column:email;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName overrides GORM's pluralised table name.
func (*UserRecord) TableName() string { return "users" }
