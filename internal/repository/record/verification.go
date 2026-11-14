package record

import "time"

// VerificationSessionRecord maps to verification_sessions: the questions asked
// by the agent, the searcher's answers, and the D3 probabilities with their
// rationale and model version for audit.
type VerificationSessionRecord struct {
	ID            string    `gorm:"primaryKey;column:id"`
	ClaimID       string    `gorm:"column:claim_id;not null"`
	Questions     string    `gorm:"column:questions;not null"`
	Answers       string    `gorm:"column:answers;not null"`
	Probabilities string    `gorm:"column:probabilities;not null"`
	Rationale     string    `gorm:"column:rationale;not null"`
	ModelVersion  string    `gorm:"column:model_version;not null"`
	Flags         string    `gorm:"column:flags;not null"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

// TableName overrides GORM's pluralised table name.
func (*VerificationSessionRecord) TableName() string { return "verification_sessions" }
