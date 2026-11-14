package record

// LostReportRecord maps to lost_reports: a searcher's proactive report used to
// notify them when a matching item is found. The embedding is stored as JSON
// text because SQLite has no vector type.
type LostReportRecord struct {
	ID          string `gorm:"primaryKey;column:id"`
	SearcherID  string `gorm:"column:searcher_id;not null"`
	Description string `gorm:"column:description;not null"`
	Category    string `gorm:"column:category;not null"`
	Location    string `gorm:"column:location;not null"`
	TimeRange   string `gorm:"column:time_range;not null"`
	Embedding   string `gorm:"column:embedding;not null"`
	Status      string `gorm:"column:status;not null"`
}

// TableName overrides GORM's pluralised table name.
func (*LostReportRecord) TableName() string { return "lost_reports" }
