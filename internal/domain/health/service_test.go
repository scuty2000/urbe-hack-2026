package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakePinger struct {
	err   error
	delay time.Duration
}

func (p fakePinger) Ping() error {
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	return p.err
}

func TestReadinessAllHealthy(t *testing.T) {
	t.Parallel()

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("qdrant method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/collections" {
			t.Errorf("qdrant path = %s, want /collections", r.URL.Path)
		}
		if got := r.Header.Get("api-key"); got != "qdrant-key" {
			t.Errorf("qdrant api-key = %q, want %q", got, "qdrant-key")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer qdrant.Close()

	embeddings := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("embeddings method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer embeddings-key" {
			t.Errorf("embeddings authorization = %q, want %q", got, "Bearer embeddings-key")
		}

		var payload struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decoding embeddings payload: %v", err)
		}
		if payload.Model != "test-embedding-model" {
			t.Errorf("embeddings model = %q, want %q", payload.Model, "test-embedding-model")
		}
		if len(payload.Input) != 1 {
			t.Errorf("embeddings input length = %d, want 1", len(payload.Input))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer embeddings.Close()

	service := NewHealthService(fakePinger{}, Checks{
		QdrantURL:         qdrant.URL,
		QdrantAPIKey:      "qdrant-key",
		QdrantEnabled:     true,
		EmbeddingsURL:     embeddings.URL,
		EmbeddingsModel:   "test-embedding-model",
		EmbeddingsKey:     "embeddings-key",
		EmbeddingsEnabled: true,
	})

	status := service.Readiness(context.Background())

	if !status.OK || !status.DB || !status.Qdrant || !status.Embeddings {
		t.Fatalf("status = %+v, want every component healthy", status)
	}
}

func TestReadinessReportsFailuresWithoutFailingRequest(t *testing.T) {
	t.Parallel()

	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer qdrant.Close()

	embeddings := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer embeddings.Close()

	service := NewHealthService(fakePinger{err: errors.New("database down")}, Checks{
		QdrantURL:         qdrant.URL,
		QdrantEnabled:     true,
		EmbeddingsURL:     embeddings.URL,
		EmbeddingsEnabled: true,
	})

	status := service.Readiness(context.Background())

	if status.OK {
		t.Fatalf("status = %+v, want OK false", status)
	}
	if status.DB || status.Qdrant || status.Embeddings {
		t.Fatalf("status = %+v, want every component unhealthy", status)
	}
}

func TestReadinessDisabledChecksAreHealthy(t *testing.T) {
	t.Parallel()

	service := NewHealthService(fakePinger{}, Checks{})

	status := service.Readiness(context.Background())

	if !status.OK || !status.DB || !status.Qdrant || !status.Embeddings {
		t.Fatalf("status = %+v, want disabled checks to report true", status)
	}
}

func TestReadinessBoundsDatabaseCheck(t *testing.T) {
	t.Parallel()

	service := NewHealthService(fakePinger{delay: 2 * time.Second}, Checks{})

	start := time.Now()
	status := service.Readiness(context.Background())
	elapsed := time.Since(start)

	if status.DB {
		t.Fatalf("status = %+v, want database unhealthy after timeout", status)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("readiness took %s, want it bounded by the 1s database timeout", elapsed)
	}
}
