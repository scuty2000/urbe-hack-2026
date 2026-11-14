package health

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	dbTimeout         = time.Second
	qdrantTimeout     = time.Second
	embeddingsTimeout = 3 * time.Second
)

// Checks configures the optional readiness probes.
type Checks struct {
	QdrantURL         string
	QdrantAPIKey      string
	QdrantEnabled     bool
	EmbeddingsURL     string
	EmbeddingsModel   string
	EmbeddingsKey     string
	EmbeddingsEnabled bool

	// HTTPClient is optional; a default client is used when nil.
	HTTPClient *http.Client
}

type healthService struct {
	pinger Pinger
	checks Checks
}

// NewHealthService returns a HealthService that pings the provided Pinger for
// liveness and the configured external dependencies for readiness.
func NewHealthService(pinger Pinger, checks Checks) HealthService {
	return &healthService{pinger: pinger, checks: checks}
}

func (s *healthService) Liveness(_ context.Context) error {
	return s.pinger.Ping()
}

func (s *healthService) Readiness(ctx context.Context) Status {
	status := Status{
		Qdrant:     true,
		Embeddings: true,
	}

	// Run every probe concurrently so the endpoint is bounded by the slowest
	// check (3s for embeddings) instead of their sum.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		status.DB = s.checkDB(ctx)
	}()

	if s.checks.QdrantEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status.Qdrant = s.checkQdrant(ctx)
		}()
	}
	if s.checks.EmbeddingsEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status.Embeddings = s.checkEmbeddings(ctx)
		}()
	}

	wg.Wait()

	status.OK = status.DB && status.Qdrant && status.Embeddings
	return status
}

func (s *healthService) checkDB(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- s.pinger.Ping()
	}()

	select {
	case err := <-done:
		return err == nil
	case <-ctx.Done():
		return false
	}
}

func (s *healthService) checkQdrant(ctx context.Context) bool {
	baseURL := strings.TrimSpace(s.checks.QdrantURL)
	if baseURL == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, qdrantTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/collections", nil)
	if err != nil {
		return false
	}
	if s.checks.QdrantAPIKey != "" {
		req.Header.Set("api-key", s.checks.QdrantAPIKey)
	}

	return s.do(req)
}

func (s *healthService) checkEmbeddings(ctx context.Context) bool {
	embeddingsURL := strings.TrimSpace(s.checks.EmbeddingsURL)
	if embeddingsURL == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, embeddingsTimeout)
	defer cancel()

	payload, err := json.Marshal(map[string]any{
		"model": s.checks.EmbeddingsModel,
		"input": []string{"health"},
	})
	if err != nil {
		return false
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embeddingsURL, bytes.NewReader(payload))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	if s.checks.EmbeddingsKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.checks.EmbeddingsKey)
	}

	return s.do(req)
}

func (s *healthService) do(req *http.Request) bool {
	client := s.checks.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}
