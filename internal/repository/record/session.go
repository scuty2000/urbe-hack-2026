package record

import "time"

// SessionRecord maps to sessions. Only the hash of the session token is
// stored, so a database leak does not expose usable tokens.
type SessionRecord struct {
	TokenHash string    `gorm:"primaryKey;column:token_hash"`
	UserID    string    `gorm:"column:user_id;not null"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName overrides GORM's pluralised table name.
func (*SessionRecord) TableName() string { return "sessions" }
