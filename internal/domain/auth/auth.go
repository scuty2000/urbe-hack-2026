/*
Package auth contains the authentication domain: users, roles and server-side
sessions. It has no dependency on any framework or HTTP concern (no Gin, no
cookies): the HTTP layer decides how a session token is transported, while the
service only knows raw tokens and their hashes.
*/
package auth

import (
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// Roles recognised by the application. They mirror the users.role column and
// the authorization rules applied by the handler middleware.
const (
	RoleFinder       = "finder"
	RoleSearcher     = "searcher"
	RoleIntermediary = "intermediary"
	RoleInstitution  = "institution"
	RoleAdmin        = "admin"
)

// User is an authenticated actor.
// json tags are here for HTTP serialisation only — no db tags, no infra concerns.
type User struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

// userFromRecord maps a repository.UserRecord to a domain User.
// All record-to-entity conversions go through here; adding a field to User
// means updating this function — the compiler will catch any missing
// assignments.
func userFromRecord(row record.UserRecord) User {
	return User{
		ID:        row.ID,
		Role:      row.Role,
		Name:      row.Name,
		Email:     row.Email,
		CreatedAt: row.CreatedAt,
	}
}
