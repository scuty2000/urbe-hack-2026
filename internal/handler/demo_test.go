package handler_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/cors"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func newDemoRouter(t *testing.T) (http.Handler, repository.Database) {
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
		NameService:     stubNameService{},
		HealthService:   stubHealthService{},
		AuthService:     auth.New(db),
		ItemService:     itemdomain.New(db, itemdomain.Deps{}),
		CustodyService:  custodydomain.New(db),
		SearchService:   search.New(db, search.Deps{}),
		ClaimService:    claimdomain.New(db),
		AgentService:    agent.New(db, agent.Deps{DeepSeekKey: ""}),
		DemoReset:       func(ctx context.Context) error { return seed.Reset(ctx, db) },
		DemoResetSecret: "test-reset-secret",
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	return router.Handler(), db
}

func TestDemoResetRequiresAdminAndSecret(t *testing.T) {
	api, db := newDemoRouter(t)

	admin := loginAs(t, api, "admin-demo")
	searcher := loginAs(t, api, "searcher-demo")

	if got := postDemoReset(t, api, searcher, "test-reset-secret", "").Code; got != http.StatusForbidden {
		t.Fatalf("searcher reset status = %d, want 403", got)
	}
	if got := postDemoReset(t, api, admin, "", "").Code; got != http.StatusForbidden {
		t.Fatalf("missing secret status = %d, want 403", got)
	}
	if got := postDemoReset(t, api, admin, "wrong-secret", "").Code; got != http.StatusForbidden {
		t.Fatalf("wrong secret status = %d, want 403", got)
	}

	reset := postDemoReset(t, api, admin, "test-reset-secret", "")
	if reset.Code != http.StatusOK {
		t.Fatalf("admin reset status = %d (body %s)", reset.Code, reset.Body.String())
	}

	items, err := db.SearchItems(context.Background(), repository.ItemFilter{Limit: 50})
	if err != nil {
		t.Fatalf("SearchItems after reset: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("items after reset = %d, want 4", len(items))
	}

	// The secret may also travel in the JSON body.
	if got := postDemoReset(t, api, admin, "", `{"secret":"test-reset-secret"}`).Code; got != http.StatusOK {
		t.Fatalf("JSON secret reset status = %d, want 200", got)
	}
}

func TestDemoResetIsUnavailableWithoutConfiguration(t *testing.T) {
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
		SearchService:  search.New(db, search.Deps{}),
		ClaimService:   claimdomain.New(db),
		AgentService:   agent.New(db, agent.Deps{DeepSeekKey: ""}),
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	api := router.Handler()
	admin := loginAs(t, api, "admin-demo")
	if got := postDemoReset(t, api, admin, "anything", "").Code; got != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured reset status = %d, want 503", got)
	}
}

func postDemoReset(t *testing.T, api http.Handler, cookie *http.Cookie, secret, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/demo/reset", reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if secret != "" {
		req.Header.Set("X-Demo-Reset-Secret", secret)
	}
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, req)
	return recorder
}
