/*
Package embeddings is a small OpenAI-compatible client for the vLLM
embeddings endpoint used by the semantic search.

The client is deliberately best-effort: every transport, status or decoding
failure is reported as errs.ErrUnavailable so the search service can fall back
to SQLite without distinguishing outage from misconfiguration.
*/
package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

const (
	// VectorSize is the Nemotron embedding dimension. Qdrant is created with
	// the same size, so a mismatch is a configuration error, not a valid result.
	VectorSize = 4096
	// DefaultModel is the vLLM model served in the demo environment.
	DefaultModel = "nvidia/Nemotron-3.5-Embed-8B-BF16"
	// DefaultBatchSize is the maximum number of inputs per request.
	DefaultBatchSize = 32
	// defaultTimeout is used when the caller passes a non-positive timeout.
	defaultTimeout = 5 * time.Second
	// maxResponseBytes bounds the response body read from the embedding server.
	maxResponseBytes = 64 << 20
)

// Client calls an OpenAI-compatible /embeddings endpoint.
type Client struct {
	url       string
	model     string
	key       string
	batchSize int
	http      *http.Client
}

// New returns an embeddings client. An empty URL makes every call return
// errs.ErrUnavailable, which lets the search service use the SQLite fallback.
func New(url, model, key string, timeout time.Duration, batchSize int) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultModel
	}
	if batchSize <= 0 || batchSize > DefaultBatchSize {
		batchSize = DefaultBatchSize
	}
	return &Client{
		url:       strings.TrimSpace(url),
		model:     model,
		key:       strings.TrimSpace(key),
		batchSize: batchSize,
		http:      &http.Client{Timeout: timeout},
	}
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed returns the vector for a single text.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

// EmbedBatch embeds texts in chunks of the configured batch size. The returned
// slice is in the same order as the input.
func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if c == nil || c.url == "" {
		return nil, fmt.Errorf("embeddings: endpoint is not configured: %w", errs.ErrUnavailable)
	}

	vectors := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += c.batchSize {
		end := start + c.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := c.embedChunk(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batch...)
	}
	return vectors, nil
}

func (c *Client) embedChunk(ctx context.Context, texts []string) ([][]float32, error) {
	payload, err := json.Marshal(embedRequest{Model: c.model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("embeddings: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("embeddings: building request: %w", errs.ErrUnavailable)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings: request failed: %w: %w", errs.ErrUnavailable, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("embeddings: unexpected status %d: %w", resp.StatusCode, errs.ErrUnavailable)
	}

	var decoded embedResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("embeddings: decoding response: %w", errs.ErrUnavailable)
	}
	if len(decoded.Data) != len(texts) {
		return nil, fmt.Errorf("embeddings: got %d vectors for %d inputs: %w", len(decoded.Data), len(texts), errs.ErrUnavailable)
	}

	vectors := make([][]float32, len(texts))
	for i, datum := range decoded.Data {
		if len(datum.Embedding) != VectorSize {
			return nil, fmt.Errorf("embeddings: vector %d has length %d, want %d: %w", i, len(datum.Embedding), VectorSize, errs.ErrUnavailable)
		}
		index := datum.Index
		if index < 0 || index >= len(texts) {
			index = i
		}
		vectors[index] = datum.Embedding
	}
	return vectors, nil
}
