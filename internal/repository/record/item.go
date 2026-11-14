package record

import "time"

// ItemRecord maps to items, the aggregate root of the lost-and-found registry.
//
// JSON-shaped values (private_profile, photos) are stored as opaque TEXT: the
// domain layer owns their serialisation and the repository never inspects them.
// Nullable lifecycle timestamps are pointers so "not happened yet" is
// distinguishable from the zero time.
type ItemRecord struct {
	ID                string     `gorm:"primaryKey;column:id"`
	PublicCode        string     `gorm:"column:public_code;not null"`
	Source            string     `gorm:"column:source;not null"`
	CustodyMode       string     `gorm:"column:custody_mode;not null"`
	Status            string     `gorm:"column:status;not null"`
	LegalFinderID     string     `gorm:"column:legal_finder_id;not null"`
	RightsAssignedAt  *time.Time `gorm:"column:rights_assigned_at"`
	Category          string     `gorm:"column:category;not null"`
	Brand             string     `gorm:"column:brand;not null"`
	Model             string     `gorm:"column:model;not null"`
	Color             string     `gorm:"column:color;not null"`
	Condition         string     `gorm:"column:condition;not null"`
	PrivateProfile    string     `gorm:"column:private_profile;not null"`
	Photos            string     `gorm:"column:photos;not null"`
	FoundAt           time.Time  `gorm:"column:found_at"`
	FoundLat          float64    `gorm:"column:found_lat"`
	FoundLng          float64    `gorm:"column:found_lng"`
	FoundZone         string     `gorm:"column:found_zone;not null"`
	FoundNotes        string     `gorm:"column:found_notes;not null"`
	CustodianID       string     `gorm:"column:custodian_id;not null"`
	RiskTier          string     `gorm:"column:risk_tier;not null"`
	Sensitive         bool       `gorm:"column:sensitive;not null"`
	TransferredAt     *time.Time `gorm:"column:transferred_at"`
	LegalTermStart    *time.Time `gorm:"column:legal_term_start"`
	OfferedToFinderAt *time.Time `gorm:"column:offered_to_finder_at"`
	DisposedAt        *time.Time `gorm:"column:disposed_at"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;autoUpdateTime:false"`
}

// TableName overrides GORM's pluralised table name.
func (*ItemRecord) TableName() string { return "items" }
