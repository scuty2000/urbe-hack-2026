package qdrant_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/qdrant"
)

func TestEnsureCollectionTreatsConflictAsAlreadyExists(t *testing.T) {
	var (
		putBody  map[string]any
		getCalls int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Errorf("decoding collection body: %v", err)
			}
			w.WriteHeader(http.StatusConflict)
		case http.MethodGet:
			getCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"config":{"params":{"vectors":{"size":4096,"distance":"Cosine"}}}}}`))
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client := qdrant.New(server.URL, "items", "", time.Second)
	if err := client.EnsureCollection(context.Background()); err != nil {
		t.Fatalf("EnsureCollection() error = %v", err)
	}
	if getCalls != 1 {
		t.Fatalf("collection GET calls = %d, want 1", getCalls)
	}
	vectors, ok := putBody["vectors"].(map[string]any)
	if !ok {
		t.Fatalf("vectors = %#v", putBody["vectors"])
	}
	if vectors["size"] != float64(qdrant.VectorSize) || vectors["distance"] != "Cosine" {
		t.Fatalf("vectors = %#v, want size %d cosine", vectors, qdrant.VectorSize)
	}
}

func TestEnsureCollectionRejectsMismatchedExistingCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusConflict)
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"result":{"config":{"params":{"vectors":{"size":1536,"distance":"Dot"}}}}}`))
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	client := qdrant.New(server.URL, "items", "", time.Second)
	err := client.EnsureCollection(context.Background())
	if !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("EnsureCollection() error = %v, want ErrConflict", err)
	}
	if !strings.Contains(err.Error(), "1536") || !strings.Contains(err.Error(), "4096") {
		t.Fatalf("error does not report the dimension mismatch: %v", err)
	}
}

func TestUpsertSendsAllowlistedPayloadAndUUIDv5(t *testing.T) {
	var (
		method string
		path   string
		wait   string
		body   map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, wait = r.Method, r.URL.Path, r.URL.Query().Get("wait")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding upsert body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := qdrant.New(server.URL, "items", "", time.Second)
	point := qdrant.Point{
		ItemID: "item-phone",
		Vector: make([]float32, qdrant.VectorSize),
		Payload: map[string]any{
			"item_id":         "item-phone",
			"category":        "smartphone",
			"zone":            "San Lorenzo",
			"status":          "in_custody",
			"custodian_type":  "intermediary",
			"description":     "Apple iPhone 17 nero",
			"private_profile": "custodia blu con crepa",
			"found_lat":       41.9012,
		},
	}
	if err := client.Upsert(context.Background(), point); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	if method != http.MethodPut {
		t.Fatalf("method = %s, want PUT", method)
	}
	if path != "/collections/items/points" {
		t.Fatalf("path = %s, want /collections/items/points", path)
	}
	if wait != "true" {
		t.Fatalf("wait = %q, want true", wait)
	}

	points, ok := body["points"].([]any)
	if !ok || len(points) != 1 {
		t.Fatalf("points body = %#v", body["points"])
	}
	got := points[0].(map[string]any)

	wantID := qdrant.PointID("item-phone")
	if got["id"] != wantID {
		t.Fatalf("point id = %v, want %s", got["id"], wantID)
	}
	if _, err := uuid.FromString(wantID); err != nil {
		t.Fatalf("point id is not a UUID: %v", err)
	}
	vector, ok := got["vector"].([]any)
	if !ok || len(vector) != qdrant.VectorSize {
		t.Fatalf("vector length = %d, want %d", len(vector), qdrant.VectorSize)
	}

	payload, ok := got["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v", got["payload"])
	}
	wantKeys := map[string]bool{
		"item_id": true, "category": true, "zone": true,
		"status": true, "custodian_type": true, "description": true,
	}
	if len(payload) != len(wantKeys) {
		t.Fatalf("payload keys = %v, want exactly %v", payload, wantKeys)
	}
	for key := range wantKeys {
		if _, ok := payload[key]; !ok {
			t.Fatalf("payload is missing %q: %v", key, payload)
		}
	}
	if _, leaked := payload["private_profile"]; leaked {
		t.Fatal("payload leaked private_profile")
	}
	if _, leaked := payload["found_lat"]; leaked {
		t.Fatal("payload leaked exact coordinates")
	}
}

func TestSearchSendsFilterAndParsesResults(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/items/points/search" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding search body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"id":"7f8c9d8e-0000-5000-8000-000000000001","score":0.87,"payload":{"item_id":"item-phone","category":"smartphone"}}]}`))
	}))
	defer server.Close()

	client := qdrant.New(server.URL, "items", "", time.Second)
	results, err := client.Search(context.Background(), make([]float32, qdrant.VectorSize), qdrant.Filter{
		Statuses: []string{"in_custody", "transferred"},
		Category: "smartphone",
		Zone:     "San Lorenzo",
	}, 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || results[0].Score != 0.87 || results[0].Payload["item_id"] != "item-phone" {
		t.Fatalf("results = %+v", results)
	}

	vector, ok := body["vector"].([]any)
	if !ok || len(vector) != qdrant.VectorSize {
		t.Fatalf("vector length = %d", len(vector))
	}
	if body["limit"] != float64(5) {
		t.Fatalf("limit = %v, want 5", body["limit"])
	}
	if body["with_payload"] != true {
		t.Fatalf("with_payload = %v, want true", body["with_payload"])
	}
	assertFilter(t, body["filter"], "status", []any{"in_custody", "transferred"})
	assertFilter(t, body["filter"], "category", "smartphone")
	assertFilter(t, body["filter"], "zone", "San Lorenzo")
}

func assertFilter(t *testing.T, raw any, key string, want any) {
	t.Helper()

	filter, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("filter = %#v", raw)
	}
	must, ok := filter["must"].([]any)
	if !ok {
		t.Fatalf("filter.must = %#v", filter["must"])
	}
	for _, entry := range must {
		condition, ok := entry.(map[string]any)
		if !ok || condition["key"] != key {
			continue
		}
		match, _ := condition["match"].(map[string]any)
		if key == "status" {
			values, _ := match["any"].([]any)
			if len(values) != len(want.([]any)) {
				t.Fatalf("status filter = %#v, want %#v", match["any"], want)
			}
			return
		}
		if match["value"] != want {
			t.Fatalf("%s filter = %#v, want %#v", key, match["value"], want)
		}
		return
	}
	t.Fatalf("filter has no %q condition: %#v", key, must)
}

func TestDeleteSendsUUIDPoint(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/collections/items/points/delete" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding delete body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := qdrant.New(server.URL, "items", "", time.Second)
	if err := client.Delete(context.Background(), "item-phone"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	points, ok := body["points"].([]any)
	if !ok || len(points) != 1 || points[0] != qdrant.PointID("item-phone") {
		t.Fatalf("delete points = %#v", body["points"])
	}
}

func TestQdrantErrorsAreUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := qdrant.New(server.URL, "items", "", time.Second)
	_, err := client.Search(context.Background(), make([]float32, qdrant.VectorSize), qdrant.Filter{}, 1)
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("Search() error = %v, want ErrUnavailable", err)
	}

	empty := qdrant.New("", "items", "", time.Second)
	if err := empty.EnsureCollection(context.Background()); !errors.Is(err, errs.ErrInvalidInput) && !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("EnsureCollection() error = %v, want ErrInvalidInput or ErrUnavailable", err)
	}
}
