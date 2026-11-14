package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/cors"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/health"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	nameservice "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/name"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

type stubNameService struct{}

func (stubNameService) GetByID(context.Context, int64) (nameservice.Name, error) {
	return nameservice.Name{}, nil
}

func (stubNameService) Create(context.Context, string) (nameservice.Name, error) {
	return nameservice.Name{}, nil
}

func (stubNameService) UpdateByID(context.Context, int64, string) (nameservice.Name, error) {
	return nameservice.Name{}, nil
}

func (stubNameService) DeleteByID(context.Context, int64) error { return nil }

func (stubNameService) Replace(context.Context, int64, string) (nameservice.Name, error) {
	return nameservice.Name{}, nil
}

type stubHealthService struct{}

func (stubHealthService) Liveness(context.Context) error { return nil }

func (stubHealthService) Readiness(context.Context) health.Status {
	return health.Status{OK: true}
}

// newAuthRouter builds the real router with a temporary database and the demo
// users seeded, so the auth routes are exercised through Gin and the middleware.
func newAuthRouter(t *testing.T) http.Handler {
	t.Helper()

	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}

	router, err := handler.New(handler.Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORS: cors.Config{
			AllowOrigins:     []string{"http://localhost:5173"},
			AllowMethods:     []string{http.MethodGet, http.MethodPost},
			AllowHeaders:     []string{"Content-Type"},
			AllowCredentials: true,
		},
		NameService:    stubNameService{},
		HealthService:  stubHealthService{},
		AuthService:    auth.New(db),
		ItemService:    itemdomain.New(db, itemdomain.Deps{}),
		CustodyService: custodydomain.New(db),
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	return router.Handler()
}

func doRequest(t *testing.T, h http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	return recorder
}

func TestAuthLoginMeLogout(t *testing.T) {

	api := newAuthRouter(t)

	login := doRequest(t, api, http.MethodPost, "/api/auth/login", `{"userId":"finder-demo"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d (body %s)", login.Code, http.StatusOK, login.Body.String())
	}

	var user dto.UserResponse
	if err := json.NewDecoder(login.Body).Decode(&user); err != nil {
		t.Fatalf("decoding login response: %v", err)
	}
	if user.ID != "finder-demo" || user.Role != auth.RoleFinder {
		t.Fatalf("login user = %+v, want finder-demo/finder", user)
	}

	cookies := login.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login set %d cookies, want 1", len(cookies))
	}
	session := cookies[0]
	if session.Name != auth.CookieName {
		t.Fatalf("cookie name = %q, want %q", session.Name, auth.CookieName)
	}
	if !session.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite = %v, want Lax", session.SameSite)
	}
	if session.Path != "/" {
		t.Fatalf("cookie path = %q, want /", session.Path)
	}

	me := doRequest(t, api, http.MethodGet, "/api/auth/me", "", session)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d, want %d (body %s)", me.Code, http.StatusOK, me.Body.String())
	}
	var current dto.UserResponse
	if err := json.NewDecoder(me.Body).Decode(&current); err != nil {
		t.Fatalf("decoding me response: %v", err)
	}
	if current.ID != "finder-demo" {
		t.Fatalf("me user id = %q, want finder-demo", current.ID)
	}

	if got := doRequest(t, api, http.MethodGet, "/api/auth/me", "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("me without cookie status = %d, want %d", got, http.StatusUnauthorized)
	}

	logout := doRequest(t, api, http.MethodPost, "/api/auth/logout", "", session)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", logout.Code, http.StatusNoContent)
	}
	cleared := logout.Result().Cookies()
	if len(cleared) != 1 || cleared[0].MaxAge >= 0 {
		t.Fatalf("logout cookies = %+v, want one expiring cookie", cleared)
	}

	if got := doRequest(t, api, http.MethodGet, "/api/auth/me", "", session).Code; got != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestAuthLoginRejectsUnknownUser(t *testing.T) {

	api := newAuthRouter(t)

	if got := doRequest(t, api, http.MethodPost, "/api/auth/login", `{"userId":"ghost"}`, nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func TestAuthLoginRejectsMalformedBody(t *testing.T) {

	api := newAuthRouter(t)

	if got := doRequest(t, api, http.MethodPost, "/api/auth/login", `not-json`, nil).Code; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
	if got := doRequest(t, api, http.MethodPost, "/api/auth/login", `{}`, nil).Code; got != http.StatusBadRequest {
		t.Fatalf("empty userId status = %d, want %d", got, http.StatusBadRequest)
	}
}

func TestAuthCORSPreflightAllowsDevOriginWithCredentials(t *testing.T) {

	api := newAuthRouter(t)

	req := httptest.NewRequest(http.MethodOptions, "/api/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, req)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("allow-origin = %q, want http://localhost:5173", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials = %q, want true", got)
	}
}
