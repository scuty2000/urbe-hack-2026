package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

// newCustodyRouter builds the real router with a temporary database. It also
// returns the database so tests can seed custody rows with elapsed deadlines.
func newCustodyRouter(t *testing.T, searches ...search.Service) (http.Handler, repository.Database) {
	t.Helper()

	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}

	searchService := search.New(db, search.Deps{})
	if len(searches) > 0 && searches[0] != nil {
		searchService = searches[0]
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
		SearchService:  searchService,
		ClaimService:   claimdomain.New(db),
		PassService:    passdomain.New(db, passdomain.Deps{PassSecret: "test-secret"}),
		AgentService:   agent.New(db, agent.Deps{DeepSeekKey: ""}),
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	return router.Handler(), db
}

func TestCustodyHttpFlow(t *testing.T) {
	api, db := newCustodyRouter(t)

	finder := loginAs(t, api, "finder-demo")
	intermediary := loginAs(t, api, "bar-x")
	other := loginAs(t, api, "portineria-sapienza")
	admin := loginAs(t, api, "admin-demo")
	searcher := loginAs(t, api, "searcher-demo")

	created := postItemRequest(t, api, finder, itemPayload{
		IntermediaryID: "bar-x",
		Category:       "umbrella",
		FoundAt:        "2026-11-13T18:30:00Z",
		FoundZone:      "San Lorenzo",
		RightsAssigned: true,
	}, false)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST item status = %d (body %s)", created.Code, created.Body.String())
	}
	var itemBody map[string]any
	if err := json.NewDecoder(created.Body).Decode(&itemBody); err != nil {
		t.Fatalf("decoding item: %v", err)
	}
	itemID, _ := itemBody["id"].(string)
	acceptPath := "/api/items/" + itemID + "/custody/accept"
	refusePath := "/api/items/" + itemID + "/custody/refuse"

	// Unauthenticated and wrong-role callers never reach the service.
	if got := doRequest(t, api, http.MethodPost, acceptPath, "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated accept status = %d, want 401", got)
	}
	if got := doRequest(t, api, http.MethodPost, acceptPath, "", searcher).Code; got != http.StatusForbidden {
		t.Fatalf("searcher accept status = %d, want 403", got)
	}
	if got := doRequest(t, api, http.MethodPost, acceptPath, "", other).Code; got != http.StatusForbidden {
		t.Fatalf("non-legal-finder accept status = %d, want 403 (body %s)", got, "")
	}

	accepted := doRequest(t, api, http.MethodPost, acceptPath, "", intermediary)
	if accepted.Code != http.StatusOK {
		t.Fatalf("accept status = %d (body %s)", accepted.Code, accepted.Body.String())
	}
	var summary map[string]any
	if err := json.NewDecoder(accepted.Body).Decode(&summary); err != nil {
		t.Fatalf("decoding custody summary: %v", err)
	}
	if summary["status"] != itemdomain.StatusInCustody {
		t.Fatalf("status = %v, want in_custody", summary["status"])
	}
	if summary["maxCustodyDays"] != float64(7) || summary["daysLeft"] != float64(7) {
		t.Fatalf("deadline = %+v, want 7 days", summary)
	}
	if _, ok := summary["dueAt"].(string); !ok {
		t.Fatalf("dueAt missing or not a timestamp: %v", summary["dueAt"])
	}
	if _, ok := summary["privateProfile"]; ok {
		t.Fatal("custody summary leaks privateProfile")
	}

	// A second accept conflicts on the already-transitioned item.
	if got := doRequest(t, api, http.MethodPost, acceptPath, "", intermediary).Code; got != http.StatusConflict {
		t.Fatalf("double accept status = %d, want 409", got)
	}

	// Refusing a pending item returns the item DTO and closes its custody.
	second := postItemRequest(t, api, finder, itemPayload{
		IntermediaryID: "bar-x",
		Category:       "wallet",
		FoundAt:        "2026-11-13T18:30:00Z",
		RightsAssigned: true,
	}, false)
	var secondBody map[string]any
	if err := json.NewDecoder(second.Body).Decode(&secondBody); err != nil {
		t.Fatalf("decoding second item: %v", err)
	}
	secondID, _ := secondBody["id"].(string)
	secondRefuse := "/api/items/" + secondID + "/custody/refuse"

	if got := doRequest(t, api, http.MethodPost, secondRefuse, `{"reason":"  "}`, intermediary).Code; got != http.StatusBadRequest {
		t.Fatalf("blank reason status = %d, want 400", got)
	}
	if got := doRequest(t, api, http.MethodPost, secondRefuse, `not-json`, intermediary).Code; got != http.StatusBadRequest {
		t.Fatalf("malformed body status = %d, want 400", got)
	}

	refused := doRequest(t, api, http.MethodPost, secondRefuse, `{"reason":"storage is full"}`, intermediary)
	if refused.Code != http.StatusOK {
		t.Fatalf("refuse status = %d (body %s)", refused.Code, refused.Body.String())
	}
	var refusedBody map[string]any
	if err := json.NewDecoder(refused.Body).Decode(&refusedBody); err != nil {
		t.Fatalf("decoding refused item: %v", err)
	}
	if refusedBody["status"] != itemdomain.StatusCustodyRejected {
		t.Fatalf("refused status = %v, want custody_rejected", refusedBody["status"])
	}
	if _, ok := refusedBody["privateProfile"]; ok {
		t.Fatal("refused item leaks privateProfile")
	}
	if got := doRequest(t, api, http.MethodPost, refusePath, `{"reason":"late"}`, intermediary).Code; got != http.StatusConflict {
		t.Fatalf("refuse accepted item status = %d, want 409", got)
	}

	// Tick is admin-only and a no-op while deadlines are in the future.
	if got := doRequest(t, api, http.MethodPost, "/api/institution/tick", "", searcher).Code; got != http.StatusForbidden {
		t.Fatalf("searcher tick status = %d, want 403", got)
	}
	emptyTick := doRequest(t, api, http.MethodPost, "/api/institution/tick", "", admin)
	if emptyTick.Code != http.StatusOK {
		t.Fatalf("admin tick status = %d (body %s)", emptyTick.Code, emptyTick.Body.String())
	}
	if body := decodeTick(t, emptyTick); body.Updated != 0 {
		t.Fatalf("tick updated = %d, want 0 before the deadline", body.Updated)
	}

	// Seed two items whose deadline already elapsed: one ordinary, one
	// sensitive, so both pickup statuses are exercised through the route.
	now := time.Now().UTC()
	seedDueItem(t, db, "item-past-due", false, now.Add(-time.Hour))
	seedDueItem(t, db, "item-past-due-sensitive", true, now.Add(-time.Hour))

	tick := doRequest(t, api, http.MethodPost, "/api/institution/tick", "", admin)
	if tick.Code != http.StatusOK {
		t.Fatalf("tick status = %d (body %s)", tick.Code, tick.Body.String())
	}
	body := decodeTick(t, tick)
	if body.Updated != 2 || len(body.Items) != 2 {
		t.Fatalf("tick = %+v, want 2 updated items", body)
	}
	statuses := map[string]string{}
	for _, updated := range body.Items {
		statuses[updated.ID] = updated.Status
	}
	if statuses["item-past-due"] != itemdomain.StatusPickupDue {
		t.Fatalf("ordinary due status = %q, want pickup_due", statuses["item-past-due"])
	}
	if statuses["item-past-due-sensitive"] != itemdomain.StatusSensitivePickup {
		t.Fatalf("sensitive due status = %q, want sensitive_pickup", statuses["item-past-due-sensitive"])
	}

	// The tick is idempotent: a second run finds nothing new.
	if again := decodeTick(t, doRequest(t, api, http.MethodPost, "/api/institution/tick", "", admin)); again.Updated != 0 {
		t.Fatalf("second tick updated = %d, want 0", again.Updated)
	}
}

// failingIndexSearch returns SearchResults normally but fails every index
// write, which lets the accept route test the "indexing is best-effort" rule.
type failingIndexSearch struct{}

func (failingIndexSearch) Search(context.Context, search.Query) ([]search.SearchResult, error) {
	return nil, nil
}

func (failingIndexSearch) IndexItem(context.Context, itemdomain.Item) error {
	return errors.New("index unavailable")
}

func (failingIndexSearch) DeleteItem(context.Context, string) error { return nil }

func TestCustodyAcceptSucceedsWhenIndexingFails(t *testing.T) {
	api, _ := newCustodyRouter(t, failingIndexSearch{})

	finder := loginAs(t, api, "finder-demo")
	intermediary := loginAs(t, api, "bar-x")

	created := postItemRequest(t, api, finder, itemPayload{
		IntermediaryID: "bar-x",
		Category:       "umbrella",
		FoundAt:        "2026-11-13T18:30:00Z",
		FoundZone:      "San Lorenzo",
		RightsAssigned: true,
	}, false)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST item status = %d (body %s)", created.Code, created.Body.String())
	}
	var createdBody map[string]any
	if err := json.NewDecoder(created.Body).Decode(&createdBody); err != nil {
		t.Fatalf("decoding item: %v", err)
	}
	itemID, _ := createdBody["id"].(string)

	accepted := doRequest(t, api, http.MethodPost, "/api/items/"+itemID+"/custody/accept", "", intermediary)
	if accepted.Code != http.StatusOK {
		t.Fatalf("accept status = %d, want 200 even when indexing fails (body %s)", accepted.Code, accepted.Body.String())
	}
}

type tickBody struct {
	Updated int `json:"updated"`
	Items   []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"items"`
}

func decodeTick(t *testing.T, recorder *httptest.ResponseRecorder) tickBody {
	t.Helper()
	var body tickBody
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decoding tick response: %v", err)
	}
	return body
}

// seedDueItem writes an in_custody item whose custody deadline is already in
// the past. Tests use it instead of waiting seven days.
func seedDueItem(t *testing.T, db repository.Database, id string, sensitive bool, due time.Time) {
	t.Helper()

	ctx := context.Background()
	createdAt := due.Add(-8 * 24 * time.Hour)
	acceptedAt := createdAt.Add(time.Hour)

	riskTier := itemdomain.RiskLow
	maxDays := 7
	if sensitive {
		riskTier = itemdomain.RiskSensitive
		maxDays = 1
	}

	itemRow := record.ItemRecord{
		ID:               id,
		PublicCode:       "LF-22222222",
		Source:           itemdomain.SourceFinderSelf,
		CustodyMode:      itemdomain.CustodyModeIntermediary,
		Status:           itemdomain.StatusInCustody,
		LegalFinderID:    "bar-x",
		RightsAssignedAt: &createdAt,
		Category:         "umbrella",
		FoundAt:          createdAt,
		CustodianID:      "bar-x",
		RiskTier:         riskTier,
		Sensitive:        sensitive,
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
	}
	custodyRow := record.CustodyRecord{
		ID:                id + "-custody",
		ItemID:            id,
		CustodianType:     itemdomain.CustodyModeIntermediary,
		CustodianID:       "bar-x",
		LegalFinderID:     "bar-x",
		RightsAssignedAt:  &createdAt,
		ApprovalAuthority: itemdomain.CustodyModeIntermediary,
		MaxCustodyDays:    maxDays,
		AcceptedAt:        &acceptedAt,
		DueAt:             &due,
	}
	event := record.CustodyEventRecord{
		ID:        id + "-rights",
		ItemID:    id,
		Type:      itemdomain.EventRightsAssigned,
		ActorID:   "finder-demo",
		Timestamp: createdAt,
	}
	if err := db.CreateItemWithCustody(ctx, itemRow, custodyRow, event); err != nil {
		t.Fatalf("seeding due item %q: %v", id, err)
	}
}
