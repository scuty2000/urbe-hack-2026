package handler_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	feedomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	rewarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/reward"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func newFeeRouter(t *testing.T) (http.Handler, repository.Database) {
	t.Helper()

	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	fees := feedomain.New(db)
	rewards := rewarddomain.New(db)

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
		PassService:    passdomain.New(db, passdomain.Deps{PassSecret: "test-secret", Fees: fees}),
		FeeService:     fees,
		RewardService:  rewards,
		AgentService:   agent.New(db, agent.Deps{DeepSeekKey: ""}),
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	return router.Handler(), db
}

func TestFeeHttpRolesAndDirectReturnIsFree(t *testing.T) {
	api, _ := newFeeRouter(t)

	searcher := loginAs(t, api, "searcher-demo")
	intermediary := loginAs(t, api, "bar-x")
	institution := loginAs(t, api, "institution-demo")
	finder := loginAs(t, api, "finder-demo")

	if got := doRequest(t, api, http.MethodGet, "/api/items/item-phone/fee", "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated fee status = %d, want 401", got)
	}
	for name, cookie := range map[string]*http.Cookie{"finder": finder, "intermediary": intermediary, "institution": institution, "searcher": searcher} {
		want := http.StatusOK
		if name == "finder" {
			want = http.StatusForbidden
		}
		if got := doRequest(t, api, http.MethodGet, "/api/items/item-phone/fee", "", cookie).Code; got != want {
			t.Fatalf("%s fee status = %d, want %d", name, got, want)
		}
	}

	fee := doRequest(t, api, http.MethodGet, "/api/items/item-phone/fee", "", searcher)
	var body map[string]any
	if err := json.NewDecoder(fee.Body).Decode(&body); err != nil {
		t.Fatalf("decoding fee: %v", err)
	}
	if body["applies"] != feedomain.AppliesNoneDirectReturn {
		t.Fatalf("applies = %v, want %s", body["applies"], feedomain.AppliesNoneDirectReturn)
	}
	if body["amountCents"] != float64(0) || body["paid"] != true {
		t.Fatalf("direct fee = %+v, want amount 0 and paid", body)
	}
	if _, ok := body["payment"]; ok {
		t.Fatalf("free fee returned a payment: %+v", body)
	}
	for _, leaked := range []string{"privateProfile", "foundLat", "foundLng", "foundNotes", "legalFinderId"} {
		if _, ok := body[leaked]; ok {
			t.Fatalf("fee response leaks %q: %+v", leaked, body)
		}
	}

	if got := doRequest(t, api, http.MethodGet, "/api/items/missing-item/fee", "", searcher).Code; got != http.StatusNotFound {
		t.Fatalf("missing item fee status = %d, want 404", got)
	}
}

func TestFeeHttpPATariffAndMockPayment(t *testing.T) {
	api, db := newFeeRouter(t)
	searcher := loginAs(t, api, "searcher-demo")
	intermediary := loginAs(t, api, "bar-x")
	institution := loginAs(t, api, "institution-demo")

	now := time.Now().UTC()
	if err := db.CreateClaim(t.Context(), record.ClaimRecord{
		ID:            "claim-wallet",
		ItemID:        "item-wallet",
		SearcherID:    "searcher-demo",
		Status:        "approved",
		ReturnChannel: "pa",
		Confidence:    0.9,
		Evidence:      "[]",
		ApprovalBy:    "institution-demo",
		ApprovedAt:    &now,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("creating wallet claim: %v", err)
	}

	feeResponse := doRequest(t, api, http.MethodGet, "/api/items/item-wallet/fee", "", searcher)
	if feeResponse.Code != http.StatusOK {
		t.Fatalf("wallet fee status = %d (body %s)", feeResponse.Code, feeResponse.Body.String())
	}
	var feeBody map[string]any
	if err := json.NewDecoder(feeResponse.Body).Decode(&feeBody); err != nil {
		t.Fatalf("decoding fee: %v", err)
	}
	if feeBody["applies"] != feedomain.AppliesPATariff {
		t.Fatalf("applies = %v, want %s", feeBody["applies"], feedomain.AppliesPATariff)
	}
	if feeBody["amountCents"] != float64(1500) || feeBody["custodyDays"] != float64(1) {
		t.Fatalf("PA fee = %+v, want 1500 cents over 1 day", feeBody)
	}
	if feeBody["currency"] != "EUR" || feeBody["paid"] != false {
		t.Fatalf("PA fee = %+v, want EUR and unpaid", feeBody)
	}
	feeID, _ := feeBody["id"].(string)
	if feeID == "" {
		t.Fatalf("fee id missing: %+v", feeBody)
	}

	for name, cookie := range map[string]*http.Cookie{"intermediary": intermediary, "institution": institution} {
		if got := doRequest(t, api, http.MethodPost, "/api/fees/"+feeID+"/pay", "", cookie).Code; got != http.StatusForbidden {
			t.Fatalf("%s pay status = %d, want 403", name, got)
		}
	}

	paid := doRequest(t, api, http.MethodPost, "/api/fees/"+feeID+"/pay", "", searcher)
	if paid.Code != http.StatusOK {
		t.Fatalf("pay status = %d (body %s)", paid.Code, paid.Body.String())
	}
	if got := paid.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var payment map[string]any
	if err := json.NewDecoder(paid.Body).Decode(&payment); err != nil {
		t.Fatalf("decoding payment: %v", err)
	}
	paymentID, _ := payment["id"].(string)
	if payment["status"] != "paid" || payment["provider"] != feedomain.ProviderMock {
		t.Fatalf("payment = %+v, want paid via mock", payment)
	}
	if want := "/receipts/" + paymentID; payment["receiptUrl"] != want {
		t.Fatalf("receiptUrl = %v, want %q", payment["receiptUrl"], want)
	}
	if payment["feeId"] != feeID {
		t.Fatalf("payment feeId = %v, want %q", payment["feeId"], feeID)
	}
	if _, ok := payment["paidAt"]; !ok {
		t.Fatalf("payment has no paidAt: %+v", payment)
	}

	after := doRequest(t, api, http.MethodGet, "/api/items/item-wallet/fee", "", institution)
	var afterBody map[string]any
	if err := json.NewDecoder(after.Body).Decode(&afterBody); err != nil {
		t.Fatalf("decoding paid fee: %v", err)
	}
	if afterBody["paid"] != true || afterBody["status"] != feedomain.StatusPaid {
		t.Fatalf("paid fee = %+v, want paid", afterBody)
	}
	if got, _ := afterBody["payment"].(map[string]any); got["id"] != paymentID {
		t.Fatalf("fee payment = %+v, want %q", afterBody["payment"], paymentID)
	}

	if got := doRequest(t, api, http.MethodPost, "/api/fees/"+feeID+"/pay", "", searcher).Code; got != http.StatusConflict {
		t.Fatalf("second pay status = %d, want 409", got)
	}
	if got := doRequest(t, api, http.MethodPost, "/api/fees/missing-fee/pay", "", searcher).Code; got != http.StatusNotFound {
		t.Fatalf("missing fee pay status = %d, want 404", got)
	}
}

func TestFeeHttpPayFreeReturnConflicts(t *testing.T) {
	api, _ := newFeeRouter(t)
	searcher := loginAs(t, api, "searcher-demo")

	fee := doRequest(t, api, http.MethodGet, "/api/items/item-phone/fee", "", searcher)
	var body map[string]any
	if err := json.NewDecoder(fee.Body).Decode(&body); err != nil {
		t.Fatalf("decoding fee: %v", err)
	}
	feeID, _ := body["id"].(string)
	if got := doRequest(t, api, http.MethodPost, "/api/fees/"+feeID+"/pay", "", searcher).Code; got != http.StatusConflict {
		t.Fatalf("free fee pay status = %d, want 409", got)
	}
}

func TestRewardNoticeHttpFlow(t *testing.T) {
	api, _ := newFeeRouter(t)
	searcher := loginAs(t, api, "searcher-demo")
	barX := loginAs(t, api, "bar-x")
	otherIntermediary := loginAs(t, api, "portineria-sapienza")
	institution := loginAs(t, api, "institution-demo")
	finder := loginAs(t, api, "finder-demo")

	if got := doRequest(t, api, http.MethodPost, "/api/items/item-phone/reward-notice", `{"amountCents":2500}`, nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated reward status = %d, want 401", got)
	}
	for name, cookie := range map[string]*http.Cookie{"searcher": searcher, "institution": institution, "finder": finder, "other intermediary": otherIntermediary} {
		if got := doRequest(t, api, http.MethodPost, "/api/items/item-phone/reward-notice", `{"amountCents":2500}`, cookie).Code; got != http.StatusForbidden {
			t.Fatalf("%s reward status = %d, want 403", name, got)
		}
	}

	created := doRequest(t, api, http.MethodPost, "/api/items/item-phone/reward-notice", `{"amountCents":2500}`, barX)
	if created.Code != http.StatusCreated {
		t.Fatalf("reward create status = %d (body %s)", created.Code, created.Body.String())
	}
	var notice map[string]any
	if err := json.NewDecoder(created.Body).Decode(&notice); err != nil {
		t.Fatalf("decoding reward: %v", err)
	}
	if notice["requested"] != true || notice["amountCents"] != float64(2500) {
		t.Fatalf("reward = %+v, want requested 2500", notice)
	}
	if notice["paymentStatus"] != rewarddomain.PaymentStatusOutsideSystem {
		t.Fatalf("paymentStatus = %v, want outside_system", notice["paymentStatus"])
	}
	if notice["legalFinderId"] != "bar-x" || notice["itemId"] != "item-phone" {
		t.Fatalf("reward = %+v, want bar-x/item-phone", notice)
	}
	if _, ok := notice["shownToSearcherAt"]; ok {
		t.Fatalf("reward is shown before the searcher read: %+v", notice)
	}
	for _, leaked := range []string{"privateProfile", "foundLat", "foundLng", "foundNotes"} {
		if _, ok := notice[leaked]; ok {
			t.Fatalf("reward response leaks %q: %+v", leaked, notice)
		}
	}
	if got := created.Header().Get("Location"); got != "/api/items/item-phone/reward-notice" {
		t.Fatalf("Location = %q", got)
	}

	if got := doRequest(t, api, http.MethodPost, "/api/items/item-phone/reward-notice", `{"amountCents":-1}`, barX).Code; got != http.StatusBadRequest {
		t.Fatalf("negative reward status = %d, want 400", got)
	}
	if got := doRequest(t, api, http.MethodPost, "/api/items/missing-item/reward-notice", `{"amountCents":100}`, barX).Code; got != http.StatusNotFound {
		t.Fatalf("missing item reward status = %d, want 404", got)
	}

	if got := doRequest(t, api, http.MethodGet, "/api/items/item-phone/reward-notice", "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated reward read status = %d, want 401", got)
	}
	if got := doRequest(t, api, http.MethodGet, "/api/items/item-phone/reward-notice", "", finder).Code; got != http.StatusForbidden {
		t.Fatalf("finder reward read status = %d, want 403", got)
	}

	// Neither the intermediary nor the institution marks the notice as shown.
	for name, cookie := range map[string]*http.Cookie{"intermediary": barX, "institution": institution} {
		read := doRequest(t, api, http.MethodGet, "/api/items/item-phone/reward-notice", "", cookie)
		if read.Code != http.StatusOK {
			t.Fatalf("%s reward read status = %d", name, read.Code)
		}
		var seen map[string]any
		if err := json.NewDecoder(read.Body).Decode(&seen); err != nil {
			t.Fatalf("decoding reward read: %v", err)
		}
		if _, ok := seen["shownToSearcherAt"]; ok {
			t.Fatalf("%s read marked the notice shown: %+v", name, seen)
		}
	}

	shown := doRequest(t, api, http.MethodGet, "/api/items/item-phone/reward-notice", "", searcher)
	if shown.Code != http.StatusOK {
		t.Fatalf("searcher reward read status = %d", shown.Code)
	}
	var shownBody map[string]any
	if err := json.NewDecoder(shown.Body).Decode(&shownBody); err != nil {
		t.Fatalf("decoding searcher reward read: %v", err)
	}
	if _, ok := shownBody["shownToSearcherAt"]; !ok {
		t.Fatalf("searcher read did not set shownToSearcherAt: %+v", shownBody)
	}

	if got := doRequest(t, api, http.MethodGet, "/api/items/missing-item/reward-notice", "", searcher).Code; got != http.StatusNotFound {
		t.Fatalf("missing item reward read status = %d, want 404", got)
	}
}

func TestRewardNoticeHttpAmountIsOptional(t *testing.T) {
	api, _ := newFeeRouter(t)
	barX := loginAs(t, api, "bar-x")

	created := doRequest(t, api, http.MethodPost, "/api/items/item-phone/reward-notice", "", barX)
	if created.Code != http.StatusCreated {
		t.Fatalf("reward without body status = %d (body %s)", created.Code, created.Body.String())
	}
	var notice map[string]any
	if err := json.NewDecoder(created.Body).Decode(&notice); err != nil {
		t.Fatalf("decoding reward: %v", err)
	}
	if notice["requested"] != true || notice["amountCents"] != float64(0) {
		t.Fatalf("reward = %+v, want requested with no amount", notice)
	}
}
