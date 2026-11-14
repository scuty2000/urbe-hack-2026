/*
Package name contains the core business entities, interfaces and rules of the application.
It has no dependency on any framework or infrastructure concern (no Gin, no SQL, no HTTP).

Domain errors are defined here so that service implementations and handlers share the same
sentinel values without importing each other.
*/
package nameservice

import (
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// Name represents a Name entry in the system.
// json tags are here for HTTP serialisation only — no db tags, no infra concerns.
type Name struct {
	ID    int64  `json:"id"`
	Value string `json:"name"`
}

// nameFromRow maps a repository.NameRecord to a domain Name.
// All record-to-entity conversions go through here; adding a field to Name means
// updating this function — the compiler will catch any missing assignments.
func nameFromRow(row record.NameRecord) Name {
	return Name{
		ID:    row.ID,
		Value: row.Value,
	}
}
