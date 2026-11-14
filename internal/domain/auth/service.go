package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

const (
	// SessionTTL is the lifetime of a server-side session and of the cookie
	// that carries its token.
	SessionTTL = 12 * time.Hour

	// CookieName is the name of the session cookie used by the HTTP layer.
	CookieName = "lf_session"

	// tokenBytes is the amount of entropy in a session token.
	tokenBytes = 32

	// tokenLength is the length of the hex-encoded token.
	tokenLength = tokenBytes * 2
)

// demoUser is the seed data for the demo accounts. Login is by user id only in
// the MVP, so these rows are what the demo UI offers.
type demoUser struct {
	id    string
	role  string
	name  string
	email string
}

var demoUsers = []demoUser{
	{id: "finder-demo", role: RoleFinder, name: "Giulia Ferrari", email: "finder-demo@example.test"},
	{id: "searcher-demo", role: RoleSearcher, name: "Marco Bianchi", email: "searcher-demo@example.test"},
	{id: "bar-x", role: RoleIntermediary, name: "Bar X", email: "bar-x@example.test"},
	{id: "portineria-sapienza", role: RoleIntermediary, name: "Portineria Sapienza", email: "portineria-sapienza@example.test"},
	{id: "institution-demo", role: RoleInstitution, name: "Comune di Roma", email: "institution-demo@example.test"},
	{id: "admin-demo", role: RoleAdmin, name: "Admin Demo", email: "admin-demo@example.test"},
}

// service is the concrete implementation of Service.
type service struct {
	db   repository.Database
	deps Deps
}

// New returns a Service backed by the provided repository.
func New(db repository.Database, options ...Deps) Service {
	deps := Deps{}
	if len(options) > 0 {
		deps = options[0]
	}
	return newServiceWithDeps(db, deps)
}

func newService(db repository.Database) *service {
	return newServiceWithDeps(db, Deps{})
}

func newServiceWithDeps(db repository.Database, deps Deps) *service {
	return &service{db: db, deps: deps}
}

// Login creates a random session token for the user, stores only its SHA-256
// hash and returns the raw token to the caller.
func (s *service) Login(ctx context.Context, userID string) (string, User, error) {
	row, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		return "", User{}, fmt.Errorf("login user %q: %w", userID, err)
	}

	token, err := newToken()
	if err != nil {
		return "", User{}, fmt.Errorf("login user %q: %w", userID, err)
	}

	now := time.Now().UTC()
	session := record.SessionRecord{
		TokenHash: hashToken(token),
		UserID:    userID,
		ExpiresAt: now.Add(SessionTTL),
		CreatedAt: now,
	}
	if err := s.db.CreateSession(ctx, session); err != nil {
		return "", User{}, fmt.Errorf("login user %q: %w", userID, err)
	}

	// Login is the one audited action that no domain transaction owns. The
	// hook is best-effort: a failed audit must not block the session.
	platform.TryAudit(ctx, s.deps.Auditor, "auth.login", userID, "login", "user", userID,
		map[string]any{"role": row.Role})

	return token, userFromRecord(row), nil
}

// Logout deletes the session associated with token. Unknown tokens are ignored
// so that logging out twice (or with a cleared cookie) is harmless.
func (s *service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.db.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// Authenticate hashes the raw token, loads the session, validates its expiry
// and returns the owning user. Expired sessions are deleted on the way out.
func (s *service) Authenticate(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, fmt.Errorf("authenticate: empty token: %w", errs.ErrUnauthorized)
	}

	session, err := s.db.GetSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return User{}, fmt.Errorf("authenticate: unknown session: %w", errs.ErrUnauthorized)
		}
		return User{}, fmt.Errorf("authenticate: %w", err)
	}

	if !time.Now().UTC().Before(session.ExpiresAt) {
		// Best effort: a failed cleanup must not mask the authentication error.
		_ = s.db.DeleteSession(ctx, session.TokenHash)
		return User{}, fmt.Errorf("authenticate: session expired: %w", errs.ErrUnauthorized)
	}

	row, err := s.db.GetUserByID(ctx, session.UserID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return User{}, fmt.Errorf("authenticate: session user %q missing: %w", session.UserID, errs.ErrUnauthorized)
		}
		return User{}, fmt.Errorf("authenticate: %w", err)
	}

	return userFromRecord(row), nil
}

// SeedDemoUsers inserts the demo accounts when they are missing. It is safe to
// call on every startup: existing rows are left untouched and a concurrent
// duplicate insert is treated as already seeded.
func SeedDemoUsers(ctx context.Context, db repository.Database) error {
	for _, demo := range demoUsers {
		_, err := db.GetUserByID(ctx, demo.id)
		switch {
		case err == nil:
			continue
		case !errors.Is(err, errs.ErrNotFound):
			return fmt.Errorf("seed demo user %q: %w", demo.id, err)
		}

		row := record.UserRecord{
			ID:        demo.id,
			Role:      demo.role,
			Name:      demo.name,
			Email:     demo.email,
			CreatedAt: time.Now().UTC(),
		}
		if err := db.CreateUser(ctx, row); err != nil {
			if errors.Is(err, errs.ErrConflict) {
				continue
			}
			return fmt.Errorf("seed demo user %q: %w", demo.id, err)
		}
	}
	return nil
}

// newToken returns a cryptographically random hex token.
func newToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating session token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// hashToken returns the SHA-256 hash stored in the sessions table. Only the
// hash is persisted, so a database leak does not expose usable tokens.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
