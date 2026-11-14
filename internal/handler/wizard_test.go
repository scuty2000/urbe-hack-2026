package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
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
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/wizard"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/mapbox"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

type fakeWizard struct {
	analysis     wizard.Analysis
	points       []wizard.DropPoint
	place        mapbox.Place
	err          error
	analyzeCalls int
	lastPhotos   []itemdomain.ImageInput
}

func (f *fakeWizard) Analyze(_ context.Context, input wizard.AnalyzeInput) (wizard.Analysis, error) {
	f.analyzeCalls++
	f.lastPhotos = input.Photos
	return f.analysis, f.err
}

func (f *fakeWizard) Nearest(context.Context, *mapbox.Point, int) ([]wizard.DropPoint, error) {
	return f.points, f.err
}

func (f *fakeWizard) Reverse(context.Context, mapbox.Point) (mapbox.Place, error) {
	return f.place, f.err
}

func newWizardRouter(t *testing.T, fake *fakeWizard) http.Handler {
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
		NameService:        stubNameService{},
		HealthService:      stubHealthService{},
		AuthService:        auth.New(db),
		ItemService:        itemdomain.New(db, itemdomain.Deps{}),
		CustodyService:     custodydomain.New(db),
		InstitutionService: newInstitutionService(db),
		SearchService:      search.New(db, search.Deps{}),
		ClaimService:       claimdomain.New(db),
		PassService:        passdomain.New(db, passdomain.Deps{PassSecret: "test-secret"}),
		FeeService:         newFeeService(db),
		RewardService:      newRewardService(db),
		PlatformService:    newPlatformService(db),
		AgentService:       agent.New(db, agent.Deps{Offline: true}),
		WizardService:      fake,
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	return router.Handler()
}

func wizardMultipart(t *testing.T, payload string, photos ...[]byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if payload != "" {
		if err := writer.WriteField("payload", payload); err != nil {
			t.Fatalf("payload field: %v", err)
		}
	}
	for i, photo := range photos {
		part, err := writer.CreateFormFile("photos", fmt.Sprintf("photo-%d.jpg", i+1))
		if err != nil {
			t.Fatalf("photo part: %v", err)
		}
		if _, err := part.Write(photo); err != nil {
			t.Fatalf("photo write: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing multipart: %v", err)
	}
	return body, writer.FormDataContentType()
}

func doMultipart(t *testing.T, api http.Handler, path string, body *bytes.Buffer, contentType string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", contentType)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, req)
	return recorder
}

func TestWizardAnalyzeRolesAndPrivacy(t *testing.T) {
	fake := &fakeWizard{analysis: wizard.Analysis{
		ObjectType: "smartphone",
		PhotoPlan:  []wizard.PhotoSlot{{Key: "back", Label: "Retro", Instruction: "Fotografa il retro", Required: true}},
		Prefill:    wizard.Prefill{Category: "smartphone", Brand: "Apple", Model: "iPhone 17", Color: "nero", Condition: "good"},
		Warnings:   []string{"Controlla i campi"},
		Source:     "d3+deepseek",
	}}
	api := newWizardRouter(t, fake)

	body, contentType := wizardMultipart(t, `{}`, jpegFixture())
	if got := doMultipart(t, api, "/api/finder/wizard/analyze", body, contentType, nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", got)
	}

	searcher := loginAs(t, api, "searcher-demo")
	body, contentType = wizardMultipart(t, `{}`, jpegFixture())
	if got := doMultipart(t, api, "/api/finder/wizard/analyze", body, contentType, searcher).Code; got != http.StatusForbidden {
		t.Fatalf("searcher status = %d, want 403", got)
	}

	finder := loginAs(t, api, "finder-demo")
	body, contentType = wizardMultipart(t, `{"slot":"hero"}`, jpegFixture())
	recorder := doMultipart(t, api, "/api/finder/wizard/analyze", body, contentType, finder)
	if recorder.Code != http.StatusOK {
		t.Fatalf("finder status = %d (body %s)", recorder.Code, recorder.Body.String())
	}
	var response map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if response["source"] != "d3+deepseek" || response["objectType"] != "smartphone" {
		t.Fatalf("response = %+v", response)
	}
	prefill, _ := response["prefill"].(map[string]any)
	if prefill["brand"] != "Apple" || prefill["model"] != "iPhone 17" {
		t.Fatalf("prefill = %+v, want the vision prefill", prefill)
	}
	plan, _ := response["photoPlan"].([]any)
	if len(plan) != 1 {
		t.Fatalf("photoPlan = %+v", response["photoPlan"])
	}
	raw := recorder.Body.String()
	for _, forbidden := range []string{"token", "base64", "\"lat\"", "\"lng\""} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("response leaks %q: %s", forbidden, raw)
		}
	}
	if fake.analyzeCalls != 1 || len(fake.lastPhotos) != 1 {
		t.Fatalf("analyzer calls=%d photos=%d", fake.analyzeCalls, len(fake.lastPhotos))
	}
}

func TestWizardAnalyzeRejectsInvalidInputs(t *testing.T) {
	api := newWizardRouter(t, &fakeWizard{analysis: wizard.Analysis{Source: "manual"}})
	finder := loginAs(t, api, "finder-demo")

	body, contentType := wizardMultipart(t, `{}`)
	if got := doMultipart(t, api, "/api/finder/wizard/analyze", body, contentType, finder).Code; got != http.StatusBadRequest {
		t.Fatalf("no photo status = %d, want 400", got)
	}

	body, contentType = wizardMultipart(t, `{}`, []byte("not an image"))
	if got := doMultipart(t, api, "/api/finder/wizard/analyze", body, contentType, finder).Code; got != http.StatusBadRequest {
		t.Fatalf("bad MIME status = %d, want 400", got)
	}

	body, contentType = wizardMultipart(t, `{}`, jpegFixture(), jpegFixture(), jpegFixture(), jpegFixture(), jpegFixture())
	if got := doMultipart(t, api, "/api/finder/wizard/analyze", body, contentType, finder).Code; got != http.StatusBadRequest {
		t.Fatalf("five photos status = %d, want 400", got)
	}
}

func TestWizardAnalyzeRateLimit(t *testing.T) {
	api := newWizardRouter(t, &fakeWizard{analysis: wizard.Analysis{Source: "manual"}})
	finder := loginAs(t, api, "finder-demo")
	for i := 0; i < 20; i++ {
		body, contentType := wizardMultipart(t, `{}`, jpegFixture())
		if got := doMultipart(t, api, "/api/finder/wizard/analyze", body, contentType, finder).Code; got != http.StatusOK {
			t.Fatalf("request %d status = %d", i+1, got)
		}
	}
	body, contentType := wizardMultipart(t, `{}`, jpegFixture())
	if got := doMultipart(t, api, "/api/finder/wizard/analyze", body, contentType, finder).Code; got != http.StatusTooManyRequests {
		t.Fatalf("request 21 status = %d, want 429", got)
	}
}

func TestWizardReverseAndDropPoints(t *testing.T) {
	fake := &fakeWizard{
		place: mapbox.Place{Zone: "San Lorenzo", PlaceName: "San Lorenzo, Roma", Source: "mapbox"},
		points: []wizard.DropPoint{
			{ID: "bar-x", Name: "Bar X", Address: "Via dei Volsci 12", Hours: "07:00-21:00", DistanceMeters: 120, DurationSeconds: 90, WalkingMinutes: 2, Source: "mapbox"},
			{ID: "portineria-sapienza", Name: "Portineria Sapienza", Address: "Piazzale Aldo Moro 5", Hours: "08:00-20:00", DistanceMeters: 300, DurationSeconds: 240, WalkingMinutes: 4, Approximate: true, Source: "haversine"},
		},
	}
	api := newWizardRouter(t, fake)
	finder := loginAs(t, api, "finder-demo")

	reverse := doRequest(t, api, http.MethodPost, "/api/geo/reverse", `{"lat":41.9,"lng":12.5}`, finder)
	if reverse.Code != http.StatusOK {
		t.Fatalf("reverse status = %d (body %s)", reverse.Code, reverse.Body.String())
	}
	var place map[string]any
	if err := json.NewDecoder(reverse.Body).Decode(&place); err != nil || place["zone"] != "San Lorenzo" {
		t.Fatalf("reverse = %+v, %v", place, err)
	}

	if got := doRequest(t, api, http.MethodPost, "/api/geo/reverse", `{"lat":999,"lng":12.5}`, finder).Code; got != http.StatusBadRequest {
		t.Fatalf("invalid coordinate status = %d, want 400", got)
	}

	drops := doRequest(t, api, http.MethodPost, "/api/drop-points", `{"lat":41.9,"lng":12.5,"limit":5}`, finder)
	if drops.Code != http.StatusOK {
		t.Fatalf("drop points status = %d (body %s)", drops.Code, drops.Body.String())
	}
	var response map[string]any
	if err := json.NewDecoder(drops.Body).Decode(&response); err != nil {
		t.Fatalf("decoding drop points: %v", err)
	}
	if response["source"] != "mapbox" || response["approximate"] != false {
		t.Fatalf("drop points = %+v", response)
	}
	points, _ := response["points"].([]any)
	if len(points) != 2 {
		t.Fatalf("points = %+v", response["points"])
	}

	withoutLocation := doRequest(t, api, http.MethodPost, "/api/drop-points", `{"limit":5}`, finder)
	if withoutLocation.Code != http.StatusOK {
		t.Fatalf("drop points without location status = %d, want 200", withoutLocation.Code)
	}

	searcher := loginAs(t, api, "searcher-demo")
	if got := doRequest(t, api, http.MethodPost, "/api/drop-points", `{"lat":41.9,"lng":12.5}`, searcher).Code; got != http.StatusForbidden {
		t.Fatalf("searcher drop points status = %d, want 403", got)
	}
}
