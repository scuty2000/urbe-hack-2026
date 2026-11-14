package record

// IntermediaryRecord maps to intermediaries: a civic point that accepts found
// items and becomes their legal finder by assignment from the finder.
type IntermediaryRecord struct {
	ID           string  `gorm:"primaryKey;column:id"`
	Name         string  `gorm:"column:name;not null"`
	Address      string  `gorm:"column:address;not null"`
	GeoLat       float64 `gorm:"column:geo_lat"`
	GeoLng       float64 `gorm:"column:geo_lng"`
	Hours        string  `gorm:"column:hours;not null"`
	Capacity     int     `gorm:"column:capacity"`
	Status       string  `gorm:"column:status;not null"`
	AgreementRef string  `gorm:"column:agreement_ref;not null"`
	Contact      string  `gorm:"column:contact;not null"`
}

// TableName overrides GORM's pluralised table name.
func (*IntermediaryRecord) TableName() string { return "intermediaries" }
