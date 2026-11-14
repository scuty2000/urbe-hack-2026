package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testserver"
)

// TestFinderWizardFallback exercises the wizard without D3, DeepSeek or Mapbox:
// the analyze endpoint returns the manual analysis, the drop points use the
// haversine estimate, and the final item enrollment still succeeds.
func TestFinderWizardFallback(t *testing.T) {
	server := testserver.NewServer(t)
	testserver.ResetDemo(t, server)

	finder := login(t, server, "finder-demo")
	searcher := login(t, server, "searcher-demo")

	if status, body := searcher.request(http.MethodPost, "/api/drop-points", map[string]any{"lat": 41.9, "lng": 12.5}, nil); status != http.StatusForbidden {
		t.Fatalf("searcher drop points = %d (body %s), want 403", status, body)
	}

	reverse := finder.json(http.MethodPost, "/api/geo/reverse", map[string]any{"lat": 41.9012, "lng": 12.5155}, http.StatusOK)
	if reverse["source"] != "unavailable" {
		t.Fatalf("reverse = %+v, want unavailable fallback", reverse)
	}

	drops := finder.json(http.MethodPost, "/api/drop-points", map[string]any{"lat": 41.9012, "lng": 12.5155, "limit": 5}, http.StatusOK)
	if drops["source"] != "haversine" || drops["approximate"] != true {
		t.Fatalf("drop points = %+v, want the haversine estimate", drops)
	}
	points, _ := drops["points"].([]any)
	if len(points) != 5 {
		t.Fatalf("drop points = %d, want 5", len(points))
	}
	firstPoint, _ := points[0].(map[string]any)
	intermediaryID, _ := firstPoint["id"].(string)
	if intermediaryID == "" || firstPoint["walkingMinutes"] == nil {
		t.Fatalf("first drop point = %+v", firstPoint)
	}

	analysis := finder.wizardAnalyze(t, map[string]any{"slot": "hero"})
	if analysis["source"] != "manual" {
		t.Fatalf("analysis = %+v, want the manual fallback", analysis)
	}
	if plan, _ := analysis["photoPlan"].([]any); len(plan) == 0 {
		t.Fatalf("analysis has no fallback photo plan: %+v", analysis)
	}
	if _, leaked := analysis["privateProfile"]; leaked {
		t.Fatal("analysis leaks privateProfile")
	}

	created := finder.enrollItem(t, map[string]any{
		"intermediaryId": intermediaryID,
		"category":       "smartphone",
		"color":          "nero",
		"condition":      "good",
		"foundAt":        "2026-11-14T10:00:00Z",
		"foundLat":       41.9012,
		"foundLng":       12.5155,
		"foundZone":      "San Lorenzo",
		"rightsAssigned": true,
	})
	if created["status"] != "pending_custody" {
		t.Fatalf("created status = %v, want pending_custody", created["status"])
	}
	if created["custodianId"] != intermediaryID {
		t.Fatalf("created custodian = %v, want %s", created["custodianId"], intermediaryID)
	}
}

func (p *persona) wizardAnalyze(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encoding wizard payload: %v", err)
	}
	if err := writer.WriteField("payload", string(encoded)); err != nil {
		t.Fatalf("writing wizard payload: %v", err)
	}
	part, err := writer.CreateFormFile("photos", "wizard.jpg")
	if err != nil {
		t.Fatalf("creating wizard photo part: %v", err)
	}
	if _, err := part.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}); err != nil {
		t.Fatalf("writing wizard photo: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing wizard multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, p.base+"/api/finder/wizard/analyze", &body)
	if err != nil {
		t.Fatalf("building wizard request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := p.client.Do(req)
	if err != nil {
		t.Fatalf("%s POST /api/finder/wizard/analyze: %v", p.name, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading wizard response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s wizard analyze = %d (body %s), want 200", p.name, resp.StatusCode, raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decoding wizard response: %v (body %s)", err, raw)
	}
	return decoded
}
