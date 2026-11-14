package auth

import "context"

// Service defines the authentication operations available to the HTTP layer.
// Tokens are opaque to the caller: the service creates them, validates them and
// hashes them before touching the database.
type Service interface {
	// Login creates a new session for the user identified by userID and returns
	// the raw token together with the user entity.
	// Returns ErrNotFound when no such user exists.
	Login(ctx context.Context, userID string) (token string, user User, err error)

	// Logout deletes the session identified by token. It is idempotent: an
	// unknown or already-deleted token is not an error.
	Logout(ctx context.Context, token string) error

	// Authenticate resolves a raw session token to its user.
	// Returns ErrUnauthorized when the token is unknown or expired.
	Authenticate(ctx context.Context, token string) (User, error)
}
