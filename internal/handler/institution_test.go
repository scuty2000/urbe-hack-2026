package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	institutiondomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/institution"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/storage"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func newInstitutionRouter(t *testing.T) (http.Handler, repository.Database) {
	t.Helper()

	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	itemService := itemdomain.New(db, itemdomain.Deps{
		Storage: storage.NewLocal(t.TempDir(), 1),
	})
	custodyService := custodydomain.New(db)
	searchService := search.New(db, search.Deps{})
	claimService := claimdomain.New(db)
	passService := passdomain.New(db, passdomain.Deps{PassSecret: "test-secret"})

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
		ItemService:    itemService,
		CustodyService: custodyService,
		InstitutionService: institutiondomain.New(db, institutiondomain.Deps{
			Items:   itemService,
			Custody: custodyService,
			Claim:   claimService,
			Pass:    passService,
			Search:  searchService,
		}),
		SearchService:   searchService,
		ClaimService:    claimService,
		PassService:     passService,
		FeeService:      newFeeService(db),
		RewardService:   newRewardService(db),
		PlatformService: newPlatformService(db),
		AgentService:    agent.New(db, agent.Deps{Offline: true}),
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	return router.Handler(), db
}

func TestInstitutionHttpFlow(t *testing.T) {
	api, db := newInstitutionRouter(t)
	ctx := context.Background()

	finder := loginAs(t, api, "finder-demo")
	searcher := loginAs(t, api, "searcher-demo")
	intermediary := loginAs(t, api, "bar-x")
	institution := loginAs(t, api, "institution-demo")
	admin := loginAs(t, api, "admin-demo")

	// Role gate: institution and admin only.
	if got := doRequest(t, api, http.MethodGet, "/api/institution/pickups", "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated pickups status = %d, want 401", got)
	}
	if got := doRequest(t, api, http.MethodGet, "/api/institution/pickups", "", searcher).Code; got != http.StatusForbidden {
		t.Fatalf("searcher pickups status = %d, want 403", got)
	}
	if got := doRequest(t, api, http.MethodGet, "/api/institution/pickups", "", finder).Code; got != http.StatusForbidden {
		t.Fatalf("finder pickups status = %d, want 403", got)
	}
	if got := doRequest(t, api, http.MethodGet, "/api/institution/pickups", "", institution).Code; got != http.StatusOK {
		t.Fatalf("institution pickups status = %d, want 200", got)
	}
	if got := doRequest(t, api, http.MethodGet, "/api/institution/pickups", "", admin).Code; got != http.StatusOK {
		t.Fatalf("admin pickups status = %d, want 200", got)
	}

	// Make the umbrella (and the seeded phone) due so the queue is not empty.
	now := time.Now().UTC()
	custodyService := custodydomain.New(db)
	if _, err := custodyService.Accept(ctx, "item-umbrella", "bar-x", now); err != nil {
		t.Fatalf("accept umbrella: %v", err)
	}
	if _, err := custodyService.MarkPickupDue(ctx, now.Add(8*24*time.Hour)); err != nil {
		t.Fatalf("mark pickup due: %v", err)
	}

	pickups := doRequest(t, api, http.MethodGet, "/api/institution/pickups", "", institution)
	if pickups.Code != http.StatusOK {
		t.Fatalf("pickups status = %d (body %s)", pickups.Code, pickups.Body.String())
	}
	raw := pickups.Body.String()
	for _, leaked := range []string{"privateProfile", "foundLat", "foundLng", "case_color"} {
		if strings.Contains(raw, leaked) {
			t.Fatalf("pickups response leaks %q: %s", leaked, raw)
		}
	}
	var pickupBody struct {
		Groups []struct {
			Sensitive bool `json:"sensitive"`
			Items     []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"items"`
		} `json:"groups"`
		Total int `json:"total"`
	}
	if err := json.NewDecoder(pickups.Body).Decode(&pickupBody); err != nil {
		t.Fatalf("decoding pickups: %v", err)
	}
	if pickupBody.Total == 0 || len(pickupBody.Groups) == 0 {
		t.Fatalf("pickups = %+v, want at least the umbrella", pickupBody)
	}
	umbrellaDue := false
	for _, group := range pickupBody.Groups {
		for _, item := range group.Items {
			if item.ID == "item-umbrella" {
				umbrellaDue = item.Status == itemdomain.StatusPickupDue
			}
		}
	}
	if !umbrellaDue {
		t.Fatalf("pickups = %+v, want item-umbrella pickup_due", pickupBody)
	}

	// Transfer is institution-only and moves the item into PA custody.
	if got := doRequest(t, api, http.MethodPost, "/api/institution/items/item-umbrella/transfer", "", searcher).Code; got != http.StatusForbidden {
		t.Fatalf("searcher transfer status = %d, want 403", got)
	}
	transferred := doRequest(t, api, http.MethodPost, "/api/institution/items/item-umbrella/transfer", "", institution)
	if transferred.Code != http.StatusOK {
		t.Fatalf("transfer status = %d (body %s)", transferred.Code, transferred.Body.String())
	}
	var transferredBody map[string]any
	if err := json.NewDecoder(transferred.Body).Decode(&transferredBody); err != nil {
		t.Fatalf("decoding transferred item: %v", err)
	}
	if transferredBody["status"] != itemdomain.StatusTransferred {
		t.Fatalf("transfer status = %v, want transferred", transferredBody["status"])
	}
	if _, ok := transferredBody["transferredAt"]; !ok {
		t.Fatalf("transfer response has no transferredAt: %+v", transferredBody)
	}
	if _, ok := transferredBody["privateProfile"]; ok {
		t.Fatal("transfer response leaks privateProfile")
	}
	if got := doRequest(t, api, http.MethodPost, "/api/institution/items/item-umbrella/transfer", "", institution).Code; got != http.StatusConflict {
		t.Fatalf("second transfer status = %d, want 409", got)
	}

	// Official intake uses the same multipart shape as the finder flow.
	if got := postInstitutionItemRequest(t, api, searcher, officialPayload{Category: "umbrella"}, false).Code; got != http.StatusForbidden {
		t.Fatalf("searcher official intake status = %d, want 403", got)
	}
	official := postInstitutionItemRequest(t, api, institution, officialPayload{
		Category:       "wallet",
		Color:          "brown",
		PrivateProfile: map[string]any{"contents": "two cards"},
		FoundZone:      "San Lorenzo",
		FoundNotes:     "handed to the PA desk",
	}, true)
	if official.Code != http.StatusCreated {
		t.Fatalf("official intake status = %d (body %s)", official.Code, official.Body.String())
	}
	var officialBody map[string]any
	if err := json.NewDecoder(official.Body).Decode(&officialBody); err != nil {
		t.Fatalf("decoding official item: %v", err)
	}
	if officialBody["status"] != itemdomain.StatusInCustody {
		t.Fatalf("official status = %v, want in_custody", officialBody["status"])
	}
	if officialBody["custodyMode"] != itemdomain.CustodyModeInstitution {
		t.Fatalf("official custodyMode = %v, want institution", officialBody["custodyMode"])
	}
	if _, ok := officialBody["privateProfile"]; ok {
		t.Fatal("official intake response leaks privateProfile")
	}
	officialID, _ := officialBody["id"].(string)
	officialCustody, err := db.GetCustodyByItemID(ctx, officialID)
	if err != nil {
		t.Fatalf("official custody: %v", err)
	}
	if officialCustody.AcceptedAt == nil || officialCustody.DueAt != nil {
		t.Fatalf("official custody = %+v, want accepted without a deadline", officialCustody)
	}

	// Seed an under-review claim plus its verification report.
	if err := db.CreateClaim(ctx, record.ClaimRecord{
		ID:            "claim-review",
		ItemID:        "item-wallet",
		SearcherID:    "searcher-demo",
		Status:        claimdomain.StatusUnderReview,
		ReturnChannel: claimdomain.ReturnChannelPA,
		Confidence:    0.6,
		Evidence:      `["receipt"]`,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("seed claim: %v", err)
	}
	if err := db.CreateVerificationSession(ctx, record.VerificationSessionRecord{
		ID:            "session-review",
		ClaimID:       "claim-review",
		Questions:     `{"color":{"type":"noul"}}`,
		Answers:       `{"color":"black"}`,
		Probabilities: `{"color":{"type":"noul","noul":0.9}}`,
		Rationale:     "colour matches",
		ModelVersion:  "local",
		Flags:         `[]`,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("seed verification session: %v", err)
	}

	if got := doRequest(t, api, http.MethodGet, "/api/institution/claims/claim-review", "", searcher).Code; got != http.StatusForbidden {
		t.Fatalf("searcher report status = %d, want 403", got)
	}
	report := doRequest(t, api, http.MethodGet, "/api/institution/claims/claim-review", "", institution)
	if report.Code != http.StatusOK {
		t.Fatalf("report status = %d (body %s)", report.Code, report.Body.String())
	}
	reportRaw := report.Body.String()
	for _, leaked := range []string{"privateProfile", "case_color", "foundLat"} {
		if strings.Contains(reportRaw, leaked) {
			t.Fatalf("report leaks %q: %s", leaked, reportRaw)
		}
	}
	var reportBody map[string]any
	if err := json.NewDecoder(report.Body).Decode(&reportBody); err != nil {
		t.Fatalf("decoding report: %v", err)
	}
	verification, ok := reportBody["report"].(map[string]any)
	if !ok {
		t.Fatalf("report body = %+v, want a verification report", reportBody)
	}
	for _, key := range []string{"questions", "answers", "probabilities", "evidence", "flags", "rationale", "modelVersion"} {
		if _, ok := verification[key]; !ok {
			t.Fatalf("verification report misses %q: %+v", key, verification)
		}
	}

	if got := doRequest(t, api, http.MethodPost, "/api/institution/claims/claim-review/review", `{"decision":"approved"}`, intermediary).Code; got != http.StatusForbidden {
		t.Fatalf("intermediary review status = %d, want 403", got)
	}
	reviewed := doRequest(t, api, http.MethodPost, "/api/institution/claims/claim-review/review", `{"decision":"approved"}`, institution)
	if reviewed.Code != http.StatusOK {
		t.Fatalf("institution review status = %d (body %s)", reviewed.Code, reviewed.Body.String())
	}
	var reviewedBody map[string]any
	if err := json.NewDecoder(reviewed.Body).Decode(&reviewedBody); err != nil {
		t.Fatalf("decoding reviewed claim: %v", err)
	}
	if reviewedBody["status"] != claimdomain.StatusApproved {
		t.Fatalf("reviewed status = %v, want approved", reviewedBody["status"])
	}
	if _, ok := reviewedBody["report"].(map[string]any); !ok {
		t.Fatalf("review response has no report: %+v", reviewedBody)
	}

	// Unclaimed flow: early offer and dispose-before-offer are rejected, then
	// the offer/collect and offer/dispose sequences succeed.
	if got := doRequest(t, api, http.MethodPost, "/api/institution/items/item-wallet/offer", "", institution).Code; got != http.StatusBadRequest {
		t.Fatalf("early offer status = %d, want 400", got)
	}
	if got := doRequest(t, api, http.MethodPost, "/api/institution/items/item-umbrella/dispose", "", institution).Code; got != http.StatusConflict {
		t.Fatalf("dispose before offer status = %d, want 409", got)
	}

	backdated := now.AddDate(-1, 0, -1)
	if err := db.UpdateItemFields(ctx, "item-wallet", map[string]any{"legal_term_start": backdated}); err != nil {
		t.Fatalf("backdating wallet: %v", err)
	}
	if err := db.UpdateItemFields(ctx, "item-umbrella", map[string]any{"legal_term_start": backdated}); err != nil {
		t.Fatalf("backdating umbrella: %v", err)
	}

	offeredWallet := doRequest(t, api, http.MethodPost, "/api/institution/items/item-wallet/offer", "", institution)
	if offeredWallet.Code != http.StatusOK {
		t.Fatalf("offer wallet status = %d (body %s)", offeredWallet.Code, offeredWallet.Body.String())
	}
	collected := doRequest(t, api, http.MethodPost, "/api/institution/items/item-wallet/collect", "", institution)
	if collected.Code != http.StatusOK {
		t.Fatalf("collect wallet status = %d (body %s)", collected.Code, collected.Body.String())
	}
	var collectedBody map[string]any
	if err := json.NewDecoder(collected.Body).Decode(&collectedBody); err != nil {
		t.Fatalf("decoding collected item: %v", err)
	}
	if collectedBody["status"] != itemdomain.StatusFinderCollected {
		t.Fatalf("collected status = %v, want finder_collected", collectedBody["status"])
	}

	offeredUmbrella := doRequest(t, api, http.MethodPost, "/api/institution/items/item-umbrella/offer", "", institution)
	if offeredUmbrella.Code != http.StatusOK {
		t.Fatalf("offer umbrella status = %d (body %s)", offeredUmbrella.Code, offeredUmbrella.Body.String())
	}
	disposed := doRequest(t, api, http.MethodPost, "/api/institution/items/item-umbrella/dispose", "", institution)
	if disposed.Code != http.StatusOK {
		t.Fatalf("dispose umbrella status = %d (body %s)", disposed.Code, disposed.Body.String())
	}
	var disposedBody map[string]any
	if err := json.NewDecoder(disposed.Body).Decode(&disposedBody); err != nil {
		t.Fatalf("decoding disposed item: %v", err)
	}
	if disposedBody["status"] != itemdomain.StatusDisposed {
		t.Fatalf("disposed status = %v, want disposed", disposedBody["status"])
	}
	if _, ok := disposedBody["disposedAt"]; !ok {
		t.Fatalf("disposed response has no disposedAt: %+v", disposedBody)
	}
}

type officialPayload struct {
	Category       string         `json:"category"`
	Brand          string         `json:"brand"`
	Model          string         `json:"model"`
	Color          string         `json:"color"`
	Condition      string         `json:"condition"`
	PrivateProfile map[string]any `json:"privateProfile"`
	FoundAt        time.Time      `json:"foundAt"`
	FoundLat       float64        `json:"foundLat"`
	FoundLng       float64        `json:"foundLng"`
	FoundZone      string         `json:"foundZone"`
	FoundNotes     string         `json:"foundNotes"`
}

func postInstitutionItemRequest(t *testing.T, api http.Handler, cookie *http.Cookie, payload officialPayload, withPhoto bool) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encoding payload: %v", err)
	}
	if err := writer.WriteField("payload", string(encoded)); err != nil {
		t.Fatalf("writing payload field: %v", err)
	}
	if withPhoto {
		part, err := writer.CreateFormFile("photos", "official.jpg")
		if err != nil {
			t.Fatalf("creating photo part: %v", err)
		}
		if _, err := part.Write(jpegFixture()); err != nil {
			t.Fatalf("writing photo: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/institution/items", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, req)
	return recorder
}
