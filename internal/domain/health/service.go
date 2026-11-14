package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	dbTimeout         = time.Second
	qdrantTimeout     = time.Second
	embeddingsTimeout = 3 * time.Second

	// embeddingsCacheTTL bounds how often the embeddings endpoint is probed.
	// `/api/health` is public and polled by the UI, so a short cache avoids
	// turning health checks into a request amplifier.
	embeddingsCacheTTL = 5 * time.Second
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

	// D3Enabled reports that the decision engine is enabled and configured.
	D3Enabled bool
	// AgentMode is "deepseek" when the conversational provider is enabled and
	// keyed, "mock" otherwise. It is reported as-is to the UI.
	AgentMode string

	// HTTPClient is optional; a default client is used when nil.
	HTTPClient *http.Client
}

type healthService struct {
	pinger Pinger
	checks Checks

	embeddingsMu       sync.Mutex
	embeddingsCached   bool
	embeddingsOK       bool
	embeddingsCachedAt time.Time
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
	agentMode := strings.TrimSpace(s.checks.AgentMode)
	if agentMode == "" {
		agentMode = "mock"
	}
	status := Status{
		Qdrant:     true,
		Embeddings: true,
		D3:         s.checks.D3Enabled,
		AgentMode:  agentMode,
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
	s.embeddingsMu.Lock()
	defer s.embeddingsMu.Unlock()

	if s.embeddingsCached && time.Since(s.embeddingsCachedAt) < embeddingsCacheTTL {
		return s.embeddingsOK
	}

	s.embeddingsOK = s.probeEmbeddings(ctx)
	s.embeddingsCachedAt = time.Now()
	s.embeddingsCached = true

	return s.embeddingsOK
}

// probeEmbeddings performs a cheap readiness probe by reading the model list
// instead of requesting an embedding. Requesting an embedding would run the
// full 8B model on every unauthenticated health check.
func (s *healthService) probeEmbeddings(ctx context.Context) bool {
	endpoint, err := modelsURL(strings.TrimSpace(s.checks.EmbeddingsURL))
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, embeddingsTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false
	}
	if s.checks.EmbeddingsKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.checks.EmbeddingsKey)
	}

	return s.do(req)
}

// modelsURL derives the OpenAI-compatible /models endpoint from the configured
// embeddings endpoint (e.g. http://host/v1/embeddings -> http://host/v1/models).
func modelsURL(embeddingsURL string) (string, error) {
	u, err := url.Parse(embeddingsURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid embeddings URL %q", embeddingsURL)
	}

	path := strings.TrimRight(u.Path, "/")
	switch {
	case strings.HasSuffix(path, "/embeddings"):
		path = strings.TrimSuffix(path, "/embeddings") + "/models"
	case strings.HasSuffix(path, "/models"):
		// Already a models endpoint.
	default:
		path += "/models"
	}
	u.Path = path

	return u.String(), nil
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
