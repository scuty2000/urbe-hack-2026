package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
		if r.Method != http.MethodGet {
			t.Errorf("embeddings method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v1/models" {
			t.Errorf("embeddings path = %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer embeddings-key" {
			t.Errorf("embeddings authorization = %q, want %q", got, "Bearer embeddings-key")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer embeddings.Close()

	service := NewHealthService(fakePinger{}, Checks{
		QdrantURL:         qdrant.URL,
		QdrantAPIKey:      "qdrant-key",
		QdrantEnabled:     true,
		EmbeddingsURL:     embeddings.URL + "/v1/embeddings",
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
	if status.D3 {
		t.Fatalf("status.D3 = true, want false when D3 is not configured")
	}
	if status.AgentMode != "mock" {
		t.Fatalf("status.AgentMode = %q, want mock by default", status.AgentMode)
	}
}

func TestReadinessReportsD3AndAgentMode(t *testing.T) {
	t.Parallel()

	service := NewHealthService(fakePinger{}, Checks{
		D3Enabled: true,
		AgentMode: "deepseek",
	})

	status := service.Readiness(context.Background())

	if !status.D3 {
		t.Fatalf("status.D3 = false, want true when configured")
	}
	if status.AgentMode != "deepseek" {
		t.Fatalf("status.AgentMode = %q, want deepseek", status.AgentMode)
	}
}

func TestReadinessCachesEmbeddingsProbe(t *testing.T) {
	t.Parallel()

	var calls int32
	embeddings := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("embeddings probe = %s %s, want GET /v1/models", r.Method, r.URL.Path)
		}
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer embeddings.Close()

	service := NewHealthService(fakePinger{}, Checks{
		EmbeddingsURL:     embeddings.URL + "/v1/embeddings",
		EmbeddingsEnabled: true,
	})

	for i := 0; i < 3; i++ {
		status := service.Readiness(context.Background())
		if !status.OK || !status.Embeddings {
			t.Fatalf("readiness %d = %+v, want healthy", i, status)
		}
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("embeddings probe calls = %d, want 1 within the cache TTL", got)
	}
}

func TestModelsURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "embeddings endpoint",
			input: "http://localhost:8000/v1/embeddings",
			want:  "http://localhost:8000/v1/models",
		},
		{
			name:  "trailing slash",
			input: "http://localhost:8000/v1/embeddings/",
			want:  "http://localhost:8000/v1/models",
		},
		{
			name:  "models endpoint already",
			input: "http://localhost:8000/v1/models",
			want:  "http://localhost:8000/v1/models",
		},
		{
			name:  "no version prefix",
			input: "http://localhost:8000/embeddings",
			want:  "http://localhost:8000/models",
		},
		{
			name:    "missing scheme",
			input:   "localhost:8000/v1/embeddings",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := modelsURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("modelsURL(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("modelsURL(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("modelsURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
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
