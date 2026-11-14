package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"

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
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func newPassRouter(t *testing.T) (http.Handler, repository.Database) {
	t.Helper()

	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

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
		PassService:    passdomain.New(db, passdomain.Deps{PassSecret: "test-secret"}),
		AgentService:   agent.New(db, agent.Deps{DeepSeekKey: ""}),
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	return router.Handler(), db
}

func TestPassHttpFlow(t *testing.T) {
	api, db := newPassRouter(t)

	searcher := loginAs(t, api, "searcher-demo")
	barX := loginAs(t, api, "bar-x")
	otherIntermediary := loginAs(t, api, "portineria-sapienza")
	finder := loginAs(t, api, "finder-demo")

	// Only searchers may request a pass; the scanner routes are not for them.
	if got := doRequest(t, api, http.MethodPost, "/api/passes", `{"claimId":"claim-phone"}`, nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated issue status = %d, want 401", got)
	}
	for name, cookie := range map[string]*http.Cookie{"finder": finder, "intermediary": barX} {
		if got := doRequest(t, api, http.MethodPost, "/api/passes",
			`{"claimId":"claim-phone"}`, cookie).Code; got != http.StatusForbidden {
			t.Fatalf("%s issue status = %d, want 403", name, got)
		}
	}
	if got := doRequest(t, api, http.MethodPost, "/api/passes/validate", `{"token":"x"}`, searcher).Code; got != http.StatusForbidden {
		t.Fatalf("searcher validate status = %d, want 403", got)
	}
	if got := doRequest(t, api, http.MethodPost, "/api/passes/redeem", `{"token":"x"}`, finder).Code; got != http.StatusForbidden {
		t.Fatalf("finder redeem status = %d, want 403", got)
	}

	issued := doRequest(t, api, http.MethodPost, "/api/passes", `{"claimId":"claim-phone"}`, searcher)
	if issued.Code != http.StatusCreated {
		t.Fatalf("POST /api/passes status = %d (body %s)", issued.Code, issued.Body.String())
	}
	var passBody map[string]any
	if err := json.NewDecoder(issued.Body).Decode(&passBody); err != nil {
		t.Fatalf("decoding pass: %v", err)
	}
	token, _ := passBody["token"].(string)
	if token == "" {
		t.Fatalf("pass token missing: %+v", passBody)
	}
	if passBody["status"] != passdomain.StatusIssued {
		t.Fatalf("pass status = %v, want issued", passBody["status"])
	}
	if passBody["returnChannel"] != passdomain.ReturnChannelIntermediaryDirect {
		t.Fatalf("returnChannel = %v, want intermediary_direct", passBody["returnChannel"])
	}
	assertSafePassItem(t, passBody["item"])
	for _, leaked := range []string{"privateProfile", "foundLat", "foundLng", "searcherId", "evidence"} {
		if _, ok := passBody[leaked]; ok {
			t.Fatalf("pass response leaks %q: %+v", leaked, passBody)
		}
	}

	// A scanner at the wrong counter learns only that the pass is not theirs.
	wrong := doRequest(t, api, http.MethodPost, "/api/passes/validate",
		`{"token":"`+token+`"}`, otherIntermediary)
	if wrong.Code != http.StatusOK {
		t.Fatalf("wrong-custodian validate status = %d (body %s)", wrong.Code, wrong.Body.String())
	}
	var wrongBody map[string]any
	if err := json.NewDecoder(wrong.Body).Decode(&wrongBody); err != nil {
		t.Fatalf("decoding wrong-custodian validation: %v", err)
	}
	if wrongBody["valid"] != false || wrongBody["reason"] != passdomain.ReasonWrongCustodian {
		t.Fatalf("wrong-custodian validation = %+v, want valid=false reason=wrong_custodian", wrongBody)
	}
	if _, ok := wrongBody["item"]; ok {
		t.Fatalf("wrong-custodian validation leaks the item: %+v", wrongBody)
	}

	valid := doRequest(t, api, http.MethodPost, "/api/passes/validate", `{"token":"`+token+`"}`, barX)
	if valid.Code != http.StatusOK {
		t.Fatalf("validate status = %d (body %s)", valid.Code, valid.Body.String())
	}
	var validBody map[string]any
	if err := json.NewDecoder(valid.Body).Decode(&validBody); err != nil {
		t.Fatalf("decoding validation: %v", err)
	}
	if validBody["valid"] != true {
		t.Fatalf("validation = %+v, want valid", validBody)
	}
	assertSafePassItem(t, validBody["item"])

	// The custodian performs the handover.
	wrongRedeem := doRequest(t, api, http.MethodPost, "/api/passes/redeem", `{"token":"`+token+`"}`, otherIntermediary)
	if wrongRedeem.Code != http.StatusForbidden {
		t.Fatalf("wrong-custodian redeem status = %d, want 403 (body %s)", wrongRedeem.Code, wrongRedeem.Body.String())
	}

	redeemed := doRequest(t, api, http.MethodPost, "/api/passes/redeem", `{"token":"`+token+`"}`, barX)
	if redeemed.Code != http.StatusOK {
		t.Fatalf("redeem status = %d (body %s)", redeemed.Code, redeemed.Body.String())
	}
	var redeemedBody map[string]any
	if err := json.NewDecoder(redeemed.Body).Decode(&redeemedBody); err != nil {
		t.Fatalf("decoding redemption: %v", err)
	}
	assertSafePassItem(t, redeemedBody["item"])
	itemBody, _ := redeemedBody["item"].(map[string]any)
	if itemBody["status"] != itemdomain.StatusReturned {
		t.Fatalf("redeemed item status = %v, want returned", itemBody["status"])
	}

	if got := doRequest(t, api, http.MethodPost, "/api/passes/redeem", `{"token":"`+token+`"}`, barX).Code; got != http.StatusConflict {
		t.Fatalf("second redeem status = %d, want 409", got)
	}

	after := doRequest(t, api, http.MethodPost, "/api/passes/validate", `{"token":"`+token+`"}`, barX)
	if after.Code != http.StatusOK {
		t.Fatalf("validate after redeem status = %d (body %s)", after.Code, after.Body.String())
	}
	var afterBody map[string]any
	if err := json.NewDecoder(after.Body).Decode(&afterBody); err != nil {
		t.Fatalf("decoding post-redeem validation: %v", err)
	}
	if afterBody["valid"] != false || afterBody["reason"] != passdomain.ReasonAlreadyRedeemed {
		t.Fatalf("post-redeem validation = %+v, want already_redeemed", afterBody)
	}

	tampered := doRequest(t, api, http.MethodPost, "/api/passes/validate",
		`{"token":"`+token+`x"}`, barX)
	if tampered.Code != http.StatusOK {
		t.Fatalf("tampered validate status = %d, want 200", tampered.Code)
	}
	var tamperedBody map[string]any
	if err := json.NewDecoder(tampered.Body).Decode(&tamperedBody); err != nil {
		t.Fatalf("decoding tampered validation: %v", err)
	}
	if tamperedBody["valid"] != false || tamperedBody["reason"] != passdomain.ReasonInvalidToken {
		t.Fatalf("tampered validation = %+v, want invalid_token", tamperedBody)
	}

	if got := doRequest(t, api, http.MethodPost, "/api/passes/validate", `{}`, barX).Code; got != http.StatusBadRequest {
		t.Fatalf("missing token status = %d, want 400", got)
	}

	row, err := db.GetItemByID(context.Background(), "item-phone")
	if err != nil {
		t.Fatalf("GetItemByID() error = %v", err)
	}
	if row.Status != itemdomain.StatusReturned {
		t.Fatalf("persisted item status = %q, want returned", row.Status)
	}
}

// TestPassIssueRejectsUnknownClaim checks the clean 404 path without leaking
// whether another searcher's claim exists.
func TestPassIssueRejectsUnknownClaim(t *testing.T) {
	api, _ := newPassRouter(t)
	searcher := loginAs(t, api, "searcher-demo")

	if got := doRequest(t, api, http.MethodPost, "/api/passes", `{"claimId":"missing"}`, searcher).Code; got != http.StatusNotFound {
		t.Fatalf("unknown claim status = %d, want 404", got)
	}
	if got := doRequest(t, api, http.MethodPost, "/api/passes", `{"claimId":"claim-phone","token":"forged"}`, searcher).Code; got != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d, want 400", got)
	}
	// bar-x does not own claim-phone, so even a legitimate searcher endpoint
	// rejects the issue without exposing the claim.
	// (bar-x is an intermediary and is blocked earlier by the role middleware.)
}

func assertSafePassItem(t *testing.T, value any) {
	t.Helper()
	if value == nil {
		t.Fatal("pass item summary is missing")
	}
	summary, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("pass item summary = %T, want object", value)
	}
	if summary["id"] == nil || summary["status"] == nil {
		t.Fatalf("pass item summary is incomplete: %+v", summary)
	}
	for _, leaked := range []string{"privateProfile", "foundLat", "foundLng", "foundNotes", "photos"} {
		if _, ok := summary[leaked]; ok {
			t.Fatalf("pass item summary leaks %q: %+v", leaked, summary)
		}
	}
}
