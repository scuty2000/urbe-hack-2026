// Package e2e exercises the demo through the real HTTP router, the real domain
// services and a real SQLite database. It never calls DeepSeek, D3, an
// embeddings endpoint, Qdrant or a payment provider: the application falls back
// to its deterministic local implementations exactly as it does on a laptop
// without those services configured.
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testserver"
)

// TestDemoFlow is the scripted demo, end to end. It starts from the same reset
// the presenter runs before going on stage and follows every role through the
// HTTP API.
func TestDemoFlow(t *testing.T) {
	server := testserver.NewServer(t)

	t.Run("reset replaces the dataset", func(t *testing.T) {
		admin := login(t, server, "admin-demo")

		status, body := admin.request(http.MethodPost, "/api/demo/reset", nil, map[string]string{
			"X-Demo-Reset-Secret": "wrong-secret",
		})
		if status != http.StatusForbidden {
			t.Fatalf("reset with a wrong secret = %d (body %s), want 403", status, body)
		}

		testserver.ResetDemo(t, server)

		barX := login(t, server, "bar-x")
		phone := barX.json(http.MethodGet, "/api/items/item-phone", nil, http.StatusOK)
		if phone["status"] != "in_custody" {
			t.Fatalf("seeded phone status = %v, want in_custody", phone["status"])
		}
	})

	t.Run("finder enrollment to returned item", func(t *testing.T) {
		testserver.ResetDemo(t, server)

		finder := login(t, server, "finder-demo")
		barX := login(t, server, "bar-x")
		searcher := login(t, server, "searcher-demo")

		enrolled := finder.enrollItem(t, map[string]any{
			"intermediaryId": "bar-x",
			"category":       "umbrella",
			"color":          "black",
			"condition":      "good",
			"privateProfile": map[string]any{"case_color": "blu"},
			"foundAt":        time.Now().UTC().Format(time.RFC3339),
			"foundLat":       41.9012,
			"foundLng":       12.5155,
			"foundZone":      "San Lorenzo",
			"rightsAssigned": true,
		})
		itemID, _ := enrolled["id"].(string)
		if itemID == "" {
			t.Fatalf("enrollment response has no id: %+v", enrolled)
		}
		if enrolled["status"] != "pending_custody" {
			t.Fatalf("enrolled status = %v, want pending_custody", enrolled["status"])
		}

		// The intermediary accepts the handover and the 7-day clock starts.
		accepted := barX.json(http.MethodPost, "/api/items/"+itemID+"/custody/accept", nil, http.StatusOK)
		if accepted["status"] != "in_custody" {
			t.Fatalf("accepted status = %v, want in_custody", accepted["status"])
		}
		if _, ok := accepted["dueAt"]; !ok {
			t.Fatalf("accepted response has no dueAt: %+v", accepted)
		}

		// The searcher talks to the mock agent. A single turn searches and asks
		// one open question; it never creates a claim or echoes the private hue.
		chat := searcher.json(http.MethodPost, "/api/chat", map[string]any{
			"messages": []map[string]string{
				{"role": "user", "content": "ho perso un ombrello nero in zona San Lorenzo"},
			},
		}, http.StatusOK)
		if message, _ := chat["message"].(string); strings.TrimSpace(message) == "" {
			t.Fatalf("chat reply is empty: %+v", chat)
		}
		if chat["done"] == true || chat["claimId"] != "" {
			t.Fatalf("one chat turn created a claim: %+v", chat)
		}
		if !candidateIncludes(chat["candidates"], itemID) {
			t.Fatalf("chat candidates do not include %s: %+v", itemID, chat["candidates"])
		}
		if strings.Contains(mustJSON(t, chat), "blu") {
			t.Fatalf("chat reply leaks the private case colour: %+v", chat)
		}

		// The explicit claim uses the private detail. The local scorer approves
		// it deterministically and records the policy as the approver.
		claim := searcher.json(http.MethodPost, "/api/claims", map[string]any{
			"itemId": itemID,
			"answers": map[string]string{
				"category":   "umbrella",
				"color":      "black",
				"condition":  "good",
				"case_color": "blu",
			},
			"evidence": []string{"receipt"},
		}, http.StatusCreated)
		claimID, _ := claim["id"].(string)
		if claimID == "" {
			t.Fatalf("claim response has no id: %+v", claim)
		}
		if claim["status"] != "approved" {
			report := barX.json(http.MethodGet, "/api/claims/"+claimID, nil, http.StatusOK)
			t.Fatalf("claim status = %v, want approved (claim %+v report %+v)", claim["status"], claim, report)
		}
		if claim["returnChannel"] != "intermediary_direct" {
			t.Fatalf("claim returnChannel = %v, want intermediary_direct", claim["returnChannel"])
		}

		pass := searcher.json(http.MethodPost, "/api/passes", map[string]any{"claimId": claimID}, http.StatusCreated)
		token, _ := pass["token"].(string)
		if token == "" {
			t.Fatalf("pass has no token: %+v", pass)
		}
		if pass["status"] != "issued" {
			t.Fatalf("pass status = %v, want issued", pass["status"])
		}

		validation := barX.json(http.MethodPost, "/api/passes/validate", map[string]any{"token": token}, http.StatusOK)
		if validation["valid"] != true {
			t.Fatalf("validation = %+v, want valid", validation)
		}

		redeemed := barX.json(http.MethodPost, "/api/passes/redeem", map[string]any{"token": token}, http.StatusOK)
		redeemedItem, _ := redeemed["item"].(map[string]any)
		if redeemedItem["status"] != "returned" {
			t.Fatalf("redeemed status = %v, want returned", redeemedItem["status"])
		}

		// The terminal state is persisted, not just returned in the response.
		after := barX.json(http.MethodGet, "/api/items/"+itemID, nil, http.StatusOK)
		if after["status"] != "returned" {
			t.Fatalf("persisted status = %v, want returned", after["status"])
		}
		if _, ok := after["privateProfile"]; ok {
			t.Fatal("item response leaks privateProfile")
		}
	})

	t.Run("seeded approved claim to returned phone", func(t *testing.T) {
		testserver.ResetDemo(t, server)

		searcher := login(t, server, "searcher-demo")
		barX := login(t, server, "bar-x")

		pass := searcher.json(http.MethodPost, "/api/passes", map[string]any{"claimId": "claim-phone"}, http.StatusCreated)
		token, _ := pass["token"].(string)
		if token == "" {
			t.Fatalf("seeded pass has no token: %+v", pass)
		}

		if got := barX.json(http.MethodPost, "/api/passes/validate", map[string]any{"token": token}, http.StatusOK); got["valid"] != true {
			t.Fatalf("seeded validation = %+v, want valid", got)
		}
		if got := barX.json(http.MethodPost, "/api/passes/redeem", map[string]any{"token": token}, http.StatusOK); got["item"].(map[string]any)["status"] != "returned" {
			t.Fatalf("seeded redemption = %+v, want returned", got)
		}

		phone := barX.json(http.MethodGet, "/api/items/item-phone", nil, http.StatusOK)
		if phone["status"] != "returned" {
			t.Fatalf("seeded phone status = %v, want returned", phone["status"])
		}
	})

	t.Run("seeded phone chat and institution approval", func(t *testing.T) {
		testserver.ResetDemo(t, server)

		searcher := login(t, server, "searcher-demo")
		institution := login(t, server, "institution-demo")
		barX := login(t, server, "bar-x")

		first := searcher.json(http.MethodPost, "/api/chat", map[string]any{
			"messages": []map[string]string{
				{"role": "user", "content": "Ho perso un telefono nero in zona San Lorenzo"},
			},
		}, http.StatusOK)
		if first["claimId"] != "" || first["done"] == true {
			t.Fatalf("first chat turn created a claim: %+v", first)
		}
		if !candidateIncludes(first["candidates"], "item-phone") {
			t.Fatalf("first chat turn does not list item-phone: %+v", first["candidates"])
		}

		// Second turn: the mock extracts the public colour and the private case
		// colour separately, then creates a reviewable claim.
		second := searcher.json(http.MethodPost, "/api/chat", map[string]any{
			"messages": []map[string]string{
				{"role": "user", "content": "Ho perso un telefono nero in zona San Lorenzo"},
				{"role": "user", "content": "È un iPhone 17 nero con custodia blu, una crepa e un adesivo a forma di gatto"},
			},
		}, http.StatusOK)
		claimID, _ := second["claimId"].(string)
		if claimID == "" || second["done"] != true {
			t.Fatalf("second chat turn did not create a claim: %+v", second)
		}

		claim := searcher.json(http.MethodGet, "/api/claims/"+claimID, nil, http.StatusOK)
		if claim["itemId"] != "item-phone" {
			t.Fatalf("chat claim item = %v, want item-phone", claim["itemId"])
		}
		if claim["status"] != "more_info" {
			t.Fatalf("chat claim status = %v, want more_info", claim["status"])
		}
		if claim["confidenceBand"] != "medium" {
			t.Fatalf("chat claim confidenceBand = %v, want medium", claim["confidenceBand"])
		}

		// The institution reviews the same claim and approves it.
		institution.json(http.MethodGet, "/api/institution/claims/"+claimID, nil, http.StatusOK)
		reviewed := institution.json(http.MethodPost, "/api/institution/claims/"+claimID+"/review",
			map[string]any{"decision": "approved"}, http.StatusOK)
		if reviewed["status"] != "approved" {
			t.Fatalf("chat claim review status = %v, want approved", reviewed["status"])
		}

		// The searcher reopens the claim (the UI reload/reopen step) and can now
		// generate the pass.
		reopened := searcher.json(http.MethodGet, "/api/claims/"+claimID, nil, http.StatusOK)
		if reopened["status"] != "approved" {
			t.Fatalf("reopened claim status = %v, want approved", reopened["status"])
		}
		pass := searcher.json(http.MethodPost, "/api/passes", map[string]any{"claimId": claimID}, http.StatusCreated)
		token, _ := pass["token"].(string)
		if pass["status"] != "issued" || token == "" {
			t.Fatalf("chat claim pass = %+v, want issued with token", pass)
		}

		if got := barX.json(http.MethodPost, "/api/passes/redeem", map[string]any{"token": token}, http.StatusOK); got["item"].(map[string]any)["status"] != "returned" {
			t.Fatalf("chat claim redemption = %+v, want returned", got)
		}
	})

	t.Run("institution wallet report fee and PA return", func(t *testing.T) {
		testserver.ResetDemo(t, server)

		searcher := login(t, server, "searcher-demo")
		institution := login(t, server, "institution-demo")

		// A high-risk wallet in PA custody: matching every attribute puts the
		// claim under review instead of auto-approving it.
		claim := searcher.json(http.MethodPost, "/api/claims", map[string]any{
			"itemId": "item-wallet",
			"answers": map[string]string{
				"category":  "wallet",
				"brand":     "Furla",
				"color":     "marrone",
				"condition": "worn",
				"contents":  "bancomat",
				"detail":    "portafoglio Furla marrone con due scomparti",
			},
			"evidence": []string{"receipt"},
		}, http.StatusCreated)
		claimID, _ := claim["id"].(string)
		if claim["status"] != "under_review" {
			t.Fatalf("wallet claim status = %v, want under_review", claim["status"])
		}
		if claim["returnChannel"] != "pa" {
			t.Fatalf("wallet claim returnChannel = %v, want pa", claim["returnChannel"])
		}

		report := institution.json(http.MethodGet, "/api/institution/claims/"+claimID, nil, http.StatusOK)
		verification, ok := report["report"].(map[string]any)
		if !ok {
			t.Fatalf("institution claim response has no report: %+v", report)
		}
		for _, key := range []string{"questions", "answers", "evidence", "rationale"} {
			if _, ok := verification[key]; !ok {
				t.Fatalf("verification report misses %q: %+v", key, verification)
			}
		}

		reviewed := institution.json(http.MethodPost, "/api/institution/claims/"+claimID+"/review",
			map[string]any{"decision": "approved"}, http.StatusOK)
		if reviewed["status"] != "approved" {
			t.Fatalf("reviewed status = %v, want approved", reviewed["status"])
		}

		fee := searcher.json(http.MethodGet, "/api/items/item-wallet/fee", nil, http.StatusOK)
		if fee["applies"] != "pa_tariff" {
			t.Fatalf("wallet fee applies = %v, want pa_tariff", fee["applies"])
		}
		if fee["amountCents"] != float64(1500) || fee["currency"] != "EUR" {
			t.Fatalf("wallet fee = %+v, want 1500 EUR cents", fee)
		}
		if fee["paid"] != false {
			t.Fatalf("wallet fee is already paid: %+v", fee)
		}
		feeID, _ := fee["id"].(string)

		payment := searcher.json(http.MethodPost, "/api/fees/"+feeID+"/pay", nil, http.StatusOK)
		if payment["status"] != "paid" || payment["provider"] != "mock" {
			t.Fatalf("payment = %+v, want a paid mock payment", payment)
		}

		pass := searcher.json(http.MethodPost, "/api/passes", map[string]any{"claimId": claimID}, http.StatusCreated)
		token, _ := pass["token"].(string)
		if pass["status"] != "issued" {
			t.Fatalf("wallet pass status = %v, want issued after payment", pass["status"])
		}

		if got := institution.json(http.MethodPost, "/api/passes/validate", map[string]any{"token": token}, http.StatusOK); got["valid"] != true {
			t.Fatalf("wallet validation = %+v, want valid", got)
		}
		redeemed := institution.json(http.MethodPost, "/api/passes/redeem", map[string]any{"token": token}, http.StatusOK)
		if redeemed["item"].(map[string]any)["status"] != "returned" {
			t.Fatalf("wallet redemption = %+v, want returned", redeemed)
		}

		wallet := institution.json(http.MethodGet, "/api/items/item-wallet", nil, http.StatusOK)
		if wallet["status"] != "returned" {
			t.Fatalf("wallet status = %v, want returned", wallet["status"])
		}
	})

	t.Run("institution pickups transfer offer collect and dispose", func(t *testing.T) {
		testserver.ResetDemo(t, server)

		admin := login(t, server, "admin-demo")
		barX := login(t, server, "bar-x")
		institution := login(t, server, "institution-demo")

		// The seeded wallet is already in PA custody: backdate its legal term
		// (a year and a day) and offer it to the legal finder, who collects it.
		if err := server.DB.UpdateItemFields(t.Context(), "item-wallet", map[string]any{
			"legal_term_start": time.Now().UTC().AddDate(-1, 0, -1),
		}); err != nil {
			t.Fatalf("backdating the wallet legal term: %v", err)
		}
		offered := institution.json(http.MethodPost, "/api/institution/items/item-wallet/offer", nil, http.StatusOK)
		if offered["status"] != "offered_to_finder" {
			t.Fatalf("offered wallet status = %v, want offered_to_finder", offered["status"])
		}
		collected := institution.json(http.MethodPost, "/api/institution/items/item-wallet/collect", nil, http.StatusOK)
		if collected["status"] != "finder_collected" {
			t.Fatalf("collected wallet status = %v, want finder_collected", collected["status"])
		}

		// The umbrella follows the intermediary -> PA -> disposal path. The
		// deadline is backdated so the admin tick marks it pickup due.
		accepted := barX.json(http.MethodPost, "/api/items/item-umbrella/custody/accept", nil, http.StatusOK)
		if accepted["status"] != "in_custody" {
			t.Fatalf("umbrella accept status = %v, want in_custody", accepted["status"])
		}
		if err := server.DB.UpdateCustodyFields(t.Context(), "custody-umbrella", map[string]any{
			"due_at": time.Now().UTC().Add(-time.Hour),
		}); err != nil {
			t.Fatalf("backdating the umbrella deadline: %v", err)
		}

		tick := admin.json(http.MethodPost, "/api/institution/tick", nil, http.StatusOK)
		if tick["updated"] == float64(0) {
			t.Fatalf("tick updated no items: %+v", tick)
		}

		pickups := institution.json(http.MethodGet, "/api/institution/pickups", nil, http.StatusOK)
		if pickups["total"] == float64(0) {
			t.Fatalf("pickup queue is empty after the tick: %+v", pickups)
		}

		transferred := institution.json(http.MethodPost, "/api/institution/items/item-umbrella/transfer", nil, http.StatusOK)
		if transferred["status"] != "transferred" {
			t.Fatalf("transferred status = %v, want transferred", transferred["status"])
		}
		if _, ok := transferred["transferredAt"]; !ok {
			t.Fatalf("transferred response has no transferredAt: %+v", transferred)
		}

		if err := server.DB.UpdateItemFields(t.Context(), "item-umbrella", map[string]any{
			"legal_term_start": time.Now().UTC().AddDate(-1, 0, -1),
		}); err != nil {
			t.Fatalf("backdating the umbrella legal term: %v", err)
		}
		institution.json(http.MethodPost, "/api/institution/items/item-umbrella/offer", nil, http.StatusOK)
		disposed := institution.json(http.MethodPost, "/api/institution/items/item-umbrella/dispose", nil, http.StatusOK)
		if disposed["status"] != "disposed" {
			t.Fatalf("disposed status = %v, want disposed", disposed["status"])
		}
		if _, ok := disposed["disposedAt"]; !ok {
			t.Fatalf("disposed response has no disposedAt: %+v", disposed)
		}
	})
}

// persona is one authenticated demo user with its own cookie jar, so the
// finder, searcher, intermediary, institution and admin never share a session.
type persona struct {
	t      *testing.T
	name   string
	base   string
	client *http.Client
}

func login(t *testing.T, server *testserver.Server, userID string) *persona {
	t.Helper()

	p := &persona{
		t:      t,
		name:   userID,
		base:   server.URL,
		client: testserver.NewClient(t),
	}
	body := p.json(http.MethodPost, "/api/auth/login", map[string]string{"userId": userID}, http.StatusOK)
	if body["id"] != userID {
		t.Fatalf("login %s returned user %v", userID, body["id"])
	}
	return p
}

func (p *persona) json(method, path string, payload any, want int) map[string]any {
	p.t.Helper()

	status, body := p.request(method, path, payload, nil)
	if status != want {
		p.t.Fatalf("%s %s %s = %d (body %s), want %d", p.name, method, path, status, body, want)
	}
	if strings.TrimSpace(string(body)) == "" {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		p.t.Fatalf("%s %s %s: decoding response: %v (body %s)", p.name, method, path, err, body)
	}
	return decoded
}

func (p *persona) request(method, path string, payload any, headers map[string]string) (int, []byte) {
	p.t.Helper()

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			p.t.Fatalf("encoding %s %s: %v", method, path, err)
		}
		body = bytes.NewReader(encoded)
	}
	return p.raw(method, path, body, payload != nil, headers)
}

func (p *persona) raw(method, path string, body io.Reader, contentType bool, headers map[string]string) (int, []byte) {
	p.t.Helper()

	req, err := http.NewRequest(method, p.base+path, body)
	if err != nil {
		p.t.Fatalf("building %s %s: %v", method, path, err)
	}
	if contentType {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		p.t.Fatalf("%s %s %s: %v", p.name, method, path, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		p.t.Fatalf("%s %s %s: reading response: %v", p.name, method, path, err)
	}
	return resp.StatusCode, raw
}

// enrollItem posts the finder's multipart payload plus one JPEG photo. The
// multipart shape is the same the Vue enrollment form sends.
func (p *persona) enrollItem(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encoding item payload: %v", err)
	}
	if err := writer.WriteField("payload", string(encoded)); err != nil {
		t.Fatalf("writing item payload: %v", err)
	}
	part, err := writer.CreateFormFile("photos", "found.jpg")
	if err != nil {
		t.Fatalf("creating photo part: %v", err)
	}
	if _, err := part.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}); err != nil {
		t.Fatalf("writing photo: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing item multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, p.base+"/api/items", &body)
	if err != nil {
		t.Fatalf("building item request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := p.client.Do(req)
	if err != nil {
		t.Fatalf("%s POST /api/items: %v", p.name, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading item response: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("%s POST /api/items = %d (body %s), want 201", p.name, resp.StatusCode, raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decoding item response: %v (body %s)", err, raw)
	}
	return decoded
}

func candidateIncludes(value any, itemID string) bool {
	candidates, ok := value.([]any)
	if !ok {
		return false
	}
	for _, candidate := range candidates {
		entry, ok := candidate.(map[string]any)
		if !ok {
			continue
		}
		if entry["itemId"] == itemID {
			return true
		}
	}
	return false
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding value: %v", err)
	}
	return string(encoded)
}
