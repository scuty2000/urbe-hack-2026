package record

// NameRecord maps to example_table.
type NameRecord struct {
	ID    int64  `gorm:"primaryKey;autoIncrement"`
	Value string `gorm:"column:name;not null;default:''"`
}

// TableName overrides GORM's default pluralised convention.
func (*NameRecord) TableName() string { return "example_table" }
