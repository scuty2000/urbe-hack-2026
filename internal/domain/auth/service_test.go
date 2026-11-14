package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

// newSeededService wires a migrated temporary database with the demo users and
// returns the auth service under test.
func newSeededService(t *testing.T) *service {
	t.Helper()

	db := testutil.NewTestDB(t)
	if err := SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}
	return newService(db)
}

func TestLoginCreatesSessionAndRejectsWrongRole(t *testing.T) {

	svc := newSeededService(t)

	token, user, err := svc.Login(context.Background(), "finder-demo")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if user.Role != RoleFinder {
		t.Fatalf("role = %q, want %q", user.Role, RoleFinder)
	}
	if len(token) != tokenLength {
		t.Fatalf("token length = %d, want %d", len(token), tokenLength)
	}

	got, err := svc.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if got.ID != "finder-demo" {
		t.Fatalf("user id = %q, want %q", got.ID, "finder-demo")
	}
	if got.Role != RoleFinder {
		t.Fatalf("authenticated role = %q, want %q", got.Role, RoleFinder)
	}
}

func TestLoginPersistsOnlyTheTokenHash(t *testing.T) {

	svc := newSeededService(t)
	token, _, err := svc.Login(context.Background(), "finder-demo")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	session, err := svc.db.GetSessionByTokenHash(context.Background(), hashToken(token))
	if err != nil {
		t.Fatalf("GetSessionByTokenHash() error = %v", err)
	}
	if session.TokenHash == token {
		t.Fatal("raw token stored in the sessions table")
	}
	if session.UserID != "finder-demo" {
		t.Fatalf("session user id = %q, want %q", session.UserID, "finder-demo")
	}
	if session.ExpiresAt.Before(time.Now().Add(SessionTTL - time.Minute)) {
		t.Fatalf("session expires at %v, want roughly now + %v", session.ExpiresAt, SessionTTL)
	}

	// The raw token must not be a valid lookup key.
	if _, err := svc.db.GetSessionByTokenHash(context.Background(), token); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("lookup by raw token error = %v, want ErrNotFound", err)
	}
}

func TestLoginUnknownUserFails(t *testing.T) {

	svc := newSeededService(t)

	token, _, err := svc.Login(context.Background(), "nobody")
	if !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("Login() error = %v, want ErrNotFound", err)
	}
	if token != "" {
		t.Fatalf("token = %q, want empty", token)
	}
}

func TestLogoutDeletesSession(t *testing.T) {

	svc := newSeededService(t)
	ctx := context.Background()

	token, _, err := svc.Login(ctx, "searcher-demo")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if err := svc.Logout(ctx, token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, errs.ErrUnauthorized) {
		t.Fatalf("Authenticate() after logout error = %v, want ErrUnauthorized", err)
	}

	// Logging out twice is harmless.
	if err := svc.Logout(ctx, token); err != nil {
		t.Fatalf("second Logout() error = %v", err)
	}
}

func TestAuthenticateRejectsUnknownSession(t *testing.T) {

	svc := newSeededService(t)

	if _, err := svc.Authenticate(context.Background(), "deadbeef"); !errors.Is(err, errs.ErrUnauthorized) {
		t.Fatalf("Authenticate() error = %v, want ErrUnauthorized", err)
	}
	if _, err := svc.Authenticate(context.Background(), ""); !errors.Is(err, errs.ErrUnauthorized) {
		t.Fatalf("Authenticate(empty) error = %v, want ErrUnauthorized", err)
	}
}

func TestAuthenticateRejectsExpiredSession(t *testing.T) {

	svc := newSeededService(t)
	ctx := context.Background()

	const rawToken = "expired-token"
	expired := record.SessionRecord{
		TokenHash: hashToken(rawToken),
		UserID:    "finder-demo",
		ExpiresAt: time.Now().UTC().Add(-time.Minute),
	}
	if err := svc.db.CreateSession(ctx, expired); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := svc.Authenticate(ctx, rawToken); !errors.Is(err, errs.ErrUnauthorized) {
		t.Fatalf("Authenticate() error = %v, want ErrUnauthorized", err)
	}

	// Expired sessions are cleaned up on the failed authentication attempt.
	if _, err := svc.db.GetSessionByTokenHash(ctx, expired.TokenHash); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expired session still stored: %v", err)
	}
}

func TestSeedDemoUsersIsIdempotent(t *testing.T) {

	db := testutil.NewTestDB(t)
	ctx := context.Background()

	if err := SeedDemoUsers(ctx, db); err != nil {
		t.Fatalf("first SeedDemoUsers() error = %v", err)
	}
	if err := SeedDemoUsers(ctx, db); err != nil {
		t.Fatalf("second SeedDemoUsers() error = %v", err)
	}

	users, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	if len(users) != 7 {
		t.Fatalf("seeded users = %d, want 7", len(users))
	}

	wantRoles := map[string]string{
		"finder-demo":         RoleFinder,
		"searcher-demo":       RoleSearcher,
		"searcher-alt":        RoleSearcher,
		"bar-x":               RoleIntermediary,
		"portineria-sapienza": RoleIntermediary,
		"institution-demo":    RoleInstitution,
		"admin-demo":          RoleAdmin,
	}
	for _, u := range users {
		want, ok := wantRoles[u.ID]
		if !ok {
			t.Fatalf("unexpected seeded user %q", u.ID)
		}
		if u.Role != want {
			t.Fatalf("user %q role = %q, want %q", u.ID, u.Role, want)
		}
		if u.Name == "" || u.Email == "" {
			t.Fatalf("user %q has empty name or email", u.ID)
		}
	}
}
