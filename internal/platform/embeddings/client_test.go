package embeddings_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/embeddings"
)

// embedResponse builds an OpenAI-compatible embeddings response with one
// 4096-float vector per input.
func embedResponse(t *testing.T, texts []string) []byte {
	t.Helper()

	type datum struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	}
	data := make([]datum, 0, len(texts))
	for i := range texts {
		vector := make([]float32, embeddings.VectorSize)
		vector[0] = float32(i + 1)
		data = append(data, datum{Index: i, Embedding: vector})
	}
	encoded, err := json.Marshal(map[string]any{"data": data})
	if err != nil {
		t.Fatalf("marshalling fake embeddings response: %v", err)
	}
	return encoded
}

func TestEmbedBatchSplitsBatchesAndSendsAuth(t *testing.T) {
	var (
		mu       sync.Mutex
		requests [][]string
		models   []string
		auths    []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path = %s, want /v1/embeddings", r.URL.Path)
		}
		var body struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, body.Input)
		models = append(models, body.Model)
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(embedResponse(t, body.Input))
	}))
	defer server.Close()

	client := embeddings.New(server.URL+"/v1/embeddings", "test-model", "secret-key", time.Second, 2)
	vectors, err := client.EmbedBatch(context.Background(), []string{"one", "two", "three"})
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if len(vectors) != 3 {
		t.Fatalf("got %d vectors, want 3", len(vectors))
	}
	for i, vector := range vectors {
		if len(vector) != embeddings.VectorSize {
			t.Fatalf("vector %d has length %d, want %d", i, len(vector), embeddings.VectorSize)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("got %d requests, want 2 (batch size 2)", len(requests))
	}
	if len(requests[0]) != 2 || len(requests[1]) != 1 {
		t.Fatalf("batch sizes = %d,%d, want 2,1", len(requests[0]), len(requests[1]))
	}
	for _, model := range models {
		if model != "test-model" {
			t.Fatalf("model = %q, want test-model", model)
		}
	}
	for _, auth := range auths {
		if auth != "Bearer secret-key" {
			t.Fatalf("authorization = %q, want Bearer secret-key", auth)
		}
	}
}

func TestEmbedRejectsWrongDimension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2,0.3]}]}`))
	}))
	defer server.Close()

	client := embeddings.New(server.URL, "test-model", "", time.Second, 32)
	_, err := client.Embed(context.Background(), "hello")
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("Embed() error = %v, want ErrUnavailable", err)
	}
}

func TestEmbedWrapsHTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := embeddings.New(server.URL, "test-model", "", time.Second, 32)
	_, err := client.Embed(context.Background(), "hello")
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("Embed() error = %v, want ErrUnavailable", err)
	}
}

func TestEmbedWithoutEndpointIsUnavailable(t *testing.T) {
	client := embeddings.New("", "test-model", "", time.Second, 32)
	_, err := client.Embed(context.Background(), "hello")
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("Embed() error = %v, want ErrUnavailable", err)
	}
}
