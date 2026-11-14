package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// fakeAuthService authenticates every non-empty token as the configured user
// or fails with the configured error.
type fakeAuthService struct {
	user auth.User
	err  error
}

func (f fakeAuthService) Login(context.Context, string) (string, auth.User, error) {
	return "", auth.User{}, errors.New("not implemented")
}

func (f fakeAuthService) Logout(context.Context, string) error { return nil }

func (f fakeAuthService) Authenticate(_ context.Context, token string) (auth.User, error) {
	if token == "" || f.err != nil {
		if f.err != nil {
			return auth.User{}, f.err
		}
		return auth.User{}, errs.ErrUnauthorized
	}
	return f.user, nil
}

func newTestRouter(svc auth.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestContextLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	router.GET("/finder", RequireRole(svc, auth.RoleFinder), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.GET("/admin", RequireRole(svc, auth.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.GET("/shared", RequireAnyRole(svc, auth.RoleFinder, auth.RoleIntermediary), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	router.GET("/me", RequireAuth(svc), func(c *gin.Context) {
		rc := reqcontext.GetRequestContext(c)
		if rc.User == nil || rc.User.ID != "finder-demo" {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.JSON(http.StatusOK, rc.User)
	})
	return router
}

func performRequest(router http.Handler, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestRequireRoleAllowsMatchingRole(t *testing.T) {
	t.Parallel()

	svc := fakeAuthService{user: auth.User{ID: "finder-demo", Role: auth.RoleFinder}}
	router := newTestRouter(svc)

	if got := performRequest(router, "/finder", "session-token").Code; got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
}

func TestRequireRoleRejectsWrongRole(t *testing.T) {
	t.Parallel()

	svc := fakeAuthService{user: auth.User{ID: "finder-demo", Role: auth.RoleFinder}}
	router := newTestRouter(svc)

	recorder := performRequest(router, "/admin", "session-token")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	var body map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if body["error"] == "" {
		t.Fatalf("error body = %v, want an error message", body)
	}
}

func TestRequireRoleWithoutSessionReturnsUnauthorized(t *testing.T) {
	t.Parallel()

	svc := fakeAuthService{user: auth.User{ID: "finder-demo", Role: auth.RoleFinder}}
	router := newTestRouter(svc)

	if got := performRequest(router, "/finder", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestRequireAnyRoleAcceptsListedRolesAndRejectsOthers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role string
		want int
	}{
		{auth.RoleFinder, http.StatusOK},
		{auth.RoleIntermediary, http.StatusOK},
		{auth.RoleInstitution, http.StatusForbidden},
		{auth.RoleSearcher, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			t.Parallel()
			svc := fakeAuthService{user: auth.User{ID: "actor", Role: tt.role}}
			router := newTestRouter(svc)
			if got := performRequest(router, "/shared", "session-token").Code; got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}

	svc := fakeAuthService{user: auth.User{ID: "actor", Role: auth.RoleFinder}}
	if got := performRequest(newTestRouter(svc), "/shared", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", got)
	}
}

func TestRequireAuthRejectsInvalidSession(t *testing.T) {
	t.Parallel()

	svc := fakeAuthService{err: errs.ErrUnauthorized}
	router := newTestRouter(svc)

	if got := performRequest(router, "/me", "stale-token").Code; got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestRequireAuthReportsInfrastructureFailure(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	svc := fakeAuthService{err: errors.New("database is down")}
	router := gin.New()
	router.Use(RequestContextLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))

	reachedHandler := false
	router.GET("/me", RequireAuth(svc), func(c *gin.Context) {
		reachedHandler = true
		c.Status(http.StatusOK)
	})

	recorder := performRequest(router, "/me", "session-token")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if reachedHandler {
		t.Fatal("downstream handler ran after an authentication infrastructure error")
	}

	var body map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if body["error"] == "" {
		t.Fatalf("error body = %v, want an error message", body)
	}
}

func TestRequireAuthExposesUserOnRequestContext(t *testing.T) {
	t.Parallel()

	svc := fakeAuthService{user: auth.User{ID: "finder-demo", Role: auth.RoleFinder}}
	router := newTestRouter(svc)

	recorder := performRequest(router, "/me", "session-token")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var got auth.User
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decoding user: %v", err)
	}
	if got.ID != "finder-demo" || got.Role != auth.RoleFinder {
		t.Fatalf("user = %+v, want finder-demo/finder", got)
	}
}

func TestRequireAuthThenRequireRoleReusesAuthenticatedUser(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	svc := fakeAuthService{user: auth.User{ID: "finder-demo", Role: auth.RoleFinder}}
	router := gin.New()
	router.Use(RequestContextLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	router.GET("/me", RequireAuth(svc), RequireRole(svc, auth.RoleFinder), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	if got := performRequest(router, "/me", "session-token").Code; got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
}
