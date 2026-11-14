/*
Package d3 is a small HTTP client for the D3 decision engine.

D3 answers structured questions (choice / noul / score) over text and images and
returns probabilities. The client deliberately has no retry logic: enrollment
must never block on an external dependency, so the item service falls back to
the form values as soon as the client reports errs.ErrUnavailable.
*/
package d3

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

const (
	// askPath is the D3 system-one endpoint.
	askPath = "/v1/systemone"
	// maxResponseBytes bounds the response body read from D3.
	maxResponseBytes = 1 << 20
	// defaultTimeout is used when the caller passes a non-positive timeout.
	defaultTimeout = 10 * time.Second
)

// Client calls a D3 HTTP endpoint.
type Client struct {
	baseURL string
	key     string
	model   string
	http    *http.Client
}

// New returns a D3 client. An empty baseURL makes every call return
// errs.ErrUnavailable, which lets the service use the form fallback without a
// feature flag of its own.
func New(baseURL, key, model string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		key:     strings.TrimSpace(key),
		model:   model,
		http:    &http.Client{Timeout: timeout},
	}
}

type askRequest struct {
	Model     string                   `json:"model"`
	State     string                   `json:"state"`
	Images    []string                 `json:"images,omitempty"`
	Questions map[string]item.Question `json:"questions,omitempty"`
}

type askResponse struct {
	Answers map[string]item.Answer `json:"answers"`
}

// Ask sends one structured question batch and returns D3's answers. Any
// transport, status or decoding failure is reported as errs.ErrUnavailable so
// the caller can fall back deterministically.
func (c *Client) Ask(ctx context.Context, state string, images []string, questions map[string]item.Question) (map[string]item.Answer, error) {
	if c == nil || c.baseURL == "" {
		return nil, fmt.Errorf("d3 ask: endpoint is not configured: %w", errs.ErrUnavailable)
	}

	payload, err := json.Marshal(askRequest{
		Model:     c.model,
		State:     state,
		Images:    images,
		Questions: questions,
	})
	if err != nil {
		return nil, fmt.Errorf("d3 ask: encoding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+askPath, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("d3 ask: building request: %w", errs.ErrUnavailable)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("d3 ask: request failed: %w: %w", errs.ErrUnavailable, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("d3 ask: unexpected status %d: %w", resp.StatusCode, errs.ErrUnavailable)
	}

	var decoded askResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("d3 ask: decoding response: %w", errs.ErrUnavailable)
	}
	if decoded.Answers == nil {
		decoded.Answers = map[string]item.Answer{}
	}
	return decoded.Answers, nil
}
