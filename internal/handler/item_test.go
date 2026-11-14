package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/storage"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func newItemRouter(t *testing.T) (http.Handler, string) {
	t.Helper()

	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}

	dataDir := t.TempDir()
	router, err := handler.New(handler.Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORS: cors.Config{
			AllowOrigins:     []string{"http://localhost:5173"},
			AllowMethods:     []string{http.MethodGet, http.MethodPost},
			AllowHeaders:     []string{"Content-Type"},
			AllowCredentials: true,
		},
		NameService:   stubNameService{},
		HealthService: stubHealthService{},
		AuthService:   auth.New(db),
		ItemService: itemdomain.New(db, itemdomain.Deps{
			Storage: storage.NewLocal(dataDir, 1),
		}),
		CustodyService: custodydomain.New(db),
		SearchService:  search.New(db, search.Deps{}),
		ClaimService:   claimdomain.New(db),
		AgentService:   agent.New(db, agent.Deps{DeepSeekKey: ""}),
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	return router.Handler(), dataDir
}

// assertUploadsEmpty verifies that no files were left in <dataDir>/uploads.
func assertUploadsEmpty(t *testing.T, dataDir string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(dataDir, "uploads"))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("reading uploads directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("uploads directory has %d orphan files", len(entries))
	}
}

func loginAs(t *testing.T, api http.Handler, userID string) *http.Cookie {
	t.Helper()

	recorder := doRequest(t, api, http.MethodPost, "/api/auth/login", `{"userId":"`+userID+`"}`, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login %s status = %d (body %s)", userID, recorder.Code, recorder.Body.String())
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("login %s set no cookie", userID)
	}
	return cookies[0]
}

func jpegFixture() []byte {
	return append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x00}, 128)...)
}

type itemPayload struct {
	IntermediaryID string         `json:"intermediaryId,omitempty"`
	Category       string         `json:"category"`
	Brand          string         `json:"brand"`
	Color          string         `json:"color"`
	Condition      string         `json:"condition"`
	PrivateProfile map[string]any `json:"privateProfile"`
	FoundAt        string         `json:"foundAt"`
	FoundLat       float64        `json:"foundLat"`
	FoundLng       float64        `json:"foundLng"`
	FoundZone      string         `json:"foundZone"`
	RightsAssigned bool           `json:"rightsAssigned"`
}

func postItemRequest(t *testing.T, api http.Handler, cookie *http.Cookie, payload itemPayload, withPhoto bool) *httptest.ResponseRecorder {
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
		part, err := writer.CreateFormFile("photos", "found.jpg")
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

	req := httptest.NewRequest(http.MethodPost, "/api/items", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	api.ServeHTTP(recorder, req)
	return recorder
}

func TestItemEnrollmentHttpFlow(t *testing.T) {
	api, _ := newItemRouter(t)

	finder := loginAs(t, api, "finder-demo")
	intermediary := loginAs(t, api, "bar-x")

	created := postItemRequest(t, api, finder, itemPayload{
		IntermediaryID: "bar-x",
		Category:       "smartphone",
		Brand:          "Apple",
		Color:          "black",
		Condition:      "good",
		PrivateProfile: map[string]any{"secret": "unique-detail"},
		FoundAt:        "2026-11-13T18:30:00Z",
		FoundLat:       41.9,
		FoundLng:       12.5,
		FoundZone:      "San Lorenzo",
		RightsAssigned: true,
	}, true)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST item status = %d, want 201 (body %s)", created.Code, created.Body.String())
	}

	var response map[string]any
	if err := json.NewDecoder(created.Body).Decode(&response); err != nil {
		t.Fatalf("decoding item response: %v", err)
	}
	if _, ok := response["privateProfile"]; ok {
		t.Fatal("response leaks privateProfile")
	}
	if _, ok := response["foundLat"]; ok {
		t.Fatal("response leaks exact latitude")
	}
	if _, ok := response["foundLng"]; ok {
		t.Fatal("response leaks exact longitude")
	}
	publicCode, _ := response["publicCode"].(string)
	if !regexp.MustCompile(`^LF-[0-9A-F]{8}$`).MatchString(publicCode) {
		t.Fatalf("publicCode = %q", publicCode)
	}
	if response["status"] != itemdomain.StatusPendingCustody {
		t.Fatalf("status = %v", response["status"])
	}
	photos, _ := response["photos"].([]any)
	if len(photos) != 1 {
		t.Fatalf("photos = %+v, want one", response["photos"])
	}
	photoURL, _ := photos[0].(string)
	if !strings.HasPrefix(photoURL, "/api/uploads/") {
		t.Fatalf("photo url = %q, want /api/uploads/...", photoURL)
	}
	itemPath, _ := response["id"].(string)
	if itemPath == "" {
		t.Fatal("item id missing")
	}
	location := created.Header().Get("Location")
	if location != "/api/items/"+itemPath {
		t.Fatalf("Location = %q", location)
	}

	// Finder only gets the POST receipt, not later reads.
	if got := doRequest(t, api, http.MethodGet, location, "", finder).Code; got != http.StatusForbidden {
		t.Fatalf("finder GET item status = %d, want 403", got)
	}

	// Custodian intermediary can read it.
	read := doRequest(t, api, http.MethodGet, location, "", intermediary)
	if read.Code != http.StatusOK {
		t.Fatalf("intermediary GET item status = %d (body %s)", read.Code, read.Body.String())
	}

	// The authenticated upload route serves the image to the intermediary.
	upload := doRequest(t, api, http.MethodGet, photoURL, "", intermediary)
	if upload.Code != http.StatusOK {
		t.Fatalf("GET upload status = %d (body %s)", upload.Code, upload.Body.String())
	}
	if contentType := upload.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "image/jpeg") {
		t.Fatalf("upload content type = %q, want image/jpeg", contentType)
	}
	if !bytes.Equal(upload.Body.Bytes(), jpegFixture()) {
		t.Fatal("upload bytes differ from the stored image")
	}

	// Searchers never reach items or uploads.
	searcher := loginAs(t, api, "searcher-demo")
	if got := doRequest(t, api, http.MethodGet, photoURL, "", searcher).Code; got != http.StatusForbidden {
		t.Fatalf("searcher GET upload status = %d, want 403", got)
	}
	if got := postItemRequest(t, api, searcher, itemPayload{Category: "wallet", RightsAssigned: true}, false).Code; got != http.StatusForbidden {
		t.Fatalf("searcher POST item status = %d, want 403", got)
	}

	// Intermediary dashboard only lists its own open items.
	list := doRequest(t, api, http.MethodGet, "/api/items?custodian=me", "", intermediary)
	if list.Code != http.StatusOK {
		t.Fatalf("GET items status = %d (body %s)", list.Code, list.Body.String())
	}
	var listed struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatalf("decoding item list: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0]["id"] != itemPath {
		t.Fatalf("item list = %+v", listed.Items)
	}
}

func TestItemEnrollmentRequiresRightsAndAuth(t *testing.T) {
	api, dataDir := newItemRouter(t)
	finder := loginAs(t, api, "finder-demo")

	recorder := postItemRequest(t, api, finder, itemPayload{
		IntermediaryID: "bar-x",
		Category:       "wallet",
		RightsAssigned: false,
	}, true)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 without rights assignment (body %s)", recorder.Code, recorder.Body.String())
	}
	// Validation runs before saving, so the rejected photo must not be on disk.
	assertUploadsEmpty(t, dataDir)

	institution := loginAs(t, api, "institution-demo")
	recorder = postItemRequest(t, api, institution, itemPayload{
		IntermediaryID: "bar-x",
		Category:       "wallet",
		RightsAssigned: true,
	}, false)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("institution POST status = %d, want 403 (body %s)", recorder.Code, recorder.Body.String())
	}

	unauthenticated := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	unauthenticated.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	recorder = httptest.NewRecorder()
	api.ServeHTTP(recorder, unauthenticated)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}
