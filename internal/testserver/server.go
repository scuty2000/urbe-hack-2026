// Package testserver wires the full HTTP application for end-to-end tests.
//
// NewServer starts the same domain services and routes as cmd/webapi/main.go
// against an isolated temporary SQLite database, without any external provider:
// search falls back to SQLite, claims use the deterministic local scorer and the
// conversational agent runs its mock.
//
// The helper lives outside internal/testutil on purpose: several domain
// packages use internal/testutil from internal (white-box) tests, so importing
// the handler from testutil would create an import cycle.
package testserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"github.com/gin-contrib/cors"
	agentdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	feedomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/health"
	institutiondomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/institution"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	nameservice "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/name"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	rewarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/reward"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	wizarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/wizard"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/storage"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

// DemoResetSecret is the shared secret configured on the test server. It is a
// test-only value, never a real credential.
const DemoResetSecret = "test-demo-reset-secret"

// Server is a running test HTTP server with the real router and SQLite stores.
type Server struct {
	*httptest.Server

	// DB is the migrated repository backing the server. Tests use it for the
	// few time-travel steps (backdating a legal term) that no demo route
	// exposes.
	DB repository.Database

	// DataDir is the temporary storage root used for uploaded photos.
	DataDir string
}

// NewServer builds the full application against an isolated SQLite database and
// starts an httptest.Server. The server and the database are closed when the
// test finishes.
func NewServer(t *testing.T) *Server {
	t.Helper()

	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("testserver: seeding demo users: %v", err)
	}

	dataDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	nameService := stubNameService{}
	platformService := platform.New(db)
	authService := auth.New(db, auth.Deps{Auditor: platformService})
	itemService := itemdomain.New(db, itemdomain.Deps{
		Storage: storage.NewLocal(dataDir, 8),
	})
	custodyService := custodydomain.New(db, custodydomain.Deps{Notifier: platformService})
	searchService := search.New(db, search.Deps{})
	claimService := claimdomain.New(db, claimdomain.Deps{Notifier: platformService})
	feeService := feedomain.New(db, feedomain.Deps{Notifier: platformService})
	passService := passdomain.New(db, passdomain.Deps{
		PassSecret: "test-pass-secret",
		Notifier:   platformService,
	})
	institutionService := institutiondomain.New(db, institutiondomain.Deps{
		Items:    itemService,
		Custody:  custodyService,
		Claim:    claimService,
		Pass:     passService,
		Search:   searchService,
		Fee:      feeService,
		Notifier: platformService,
	})
	agentService := agentdomain.New(db, agentdomain.Deps{
		Search: searchService,
		Claim:  claimService,
		// Test infrastructure only: the E2E suite has no DeepSeek, so it
		// drives the deterministic agent explicitly.
		Offline: true,
	})
	// The E2E wizard uses the deterministic fallbacks: D3 nil, planner nil,
	// Mapbox nil (haversine + manual zone).
	wizardService := wizarddomain.New(db, wizarddomain.Deps{Items: itemService})

	router, err := handler.New(handler.Config{
		Logger: logger,
		CORS: cors.Config{
			AllowAllOrigins:  true,
			AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
			AllowHeaders:     []string{"Accept", "Content-Type", "Authorization", "X-Demo-Reset-Secret"},
			AllowCredentials: false,
		},
		NameService:        nameService,
		HealthService:      stubHealthService{},
		AuthService:        authService,
		ItemService:        itemService,
		CustodyService:     custodyService,
		InstitutionService: institutionService,
		SearchService:      searchService,
		ClaimService:       claimService,
		PassService:        passService,
		FeeService:         feeService,
		RewardService:      rewarddomain.New(db),
		PlatformService:    platformService,
		AgentService:       agentService,
		WizardService:      wizardService,
		DemoReset:          func(ctx context.Context) error { return seed.Reset(ctx, db, searchService) },
		DemoResetSecret:    DemoResetSecret,
	})
	if err != nil {
		t.Fatalf("testserver: building the router: %v", err)
	}

	server := httptest.NewServer(router.Handler())
	t.Cleanup(server.Close)

	return &Server{
		Server:  server,
		DB:      db,
		DataDir: dataDir,
	}
}

// NewClient returns an HTTP client with its own cookie jar, one per demo
// persona. The test server uses plain HTTP, so the session cookie needs no
// special handling.
func NewClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("testserver: creating cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

// ResetDemo logs in as the demo admin and calls POST /api/demo/reset with the
// admin session cookie and the reset secret. It mirrors scripts/demo-reset.sh
// and verifies the reset route end to end.
func ResetDemo(t *testing.T, server *Server) {
	t.Helper()

	client := NewClient(t)
	status, _ := postJSON(t, client, server.URL+"/api/auth/login", map[string]string{"userId": "admin-demo"}, nil)
	if status != http.StatusOK {
		t.Fatalf("testserver: admin login status = %d, want 200", status)
	}
	status, body := postJSON(t, client, server.URL+"/api/demo/reset", nil, map[string]string{
		"X-Demo-Reset-Secret": DemoResetSecret,
	})
	if status != http.StatusOK {
		t.Fatalf("testserver: demo reset status = %d (body %s), want 200", status, body)
	}
}

// postJSON is a small helper used only by ResetDemo; the e2e package has its own
// richer request helpers.
func postJSON(t *testing.T, client *http.Client, url string, body, headers map[string]string) (int, string) {
	t.Helper()

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("testserver: encoding request: %v", err)
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(http.MethodPost, url, payload)
	if err != nil {
		t.Fatalf("testserver: building request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("testserver: %s %s: %v", http.MethodPost, url, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("testserver: reading response: %v", err)
	}
	return resp.StatusCode, string(raw)
}

// stubNameService satisfies the legacy name handlers, which the demo does not
// use but handler.New requires.
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

// stubHealthService reports a healthy dependency set without touching external
// providers.
type stubHealthService struct{}

func (stubHealthService) Liveness(context.Context) error { return nil }

func (stubHealthService) Readiness(context.Context) health.Status {
	return health.Status{OK: true, DB: true}
}
