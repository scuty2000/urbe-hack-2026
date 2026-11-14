/*
Package qdrant is a small REST client for the Qdrant vector database.

The client keeps the payload surface deliberately tiny: only the fields in
allowedPayloadKeys ever leave the process. Private profile data, document
fields, photos and exact coordinates must never reach the vector store.
*/
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

const (
	// DefaultCollection is the collection used by the lost-and-found registry.
	DefaultCollection = "items"
	// VectorSize is the Nemotron embedding dimension.
	VectorSize = 4096
	// defaultTimeout is used when the caller passes a non-positive timeout.
	defaultTimeout = 5 * time.Second
	// maxResponseBytes bounds the response body read from Qdrant.
	maxResponseBytes = 8 << 20
)

// pointNamespace is the stable namespace for item-id -> point-id mapping.
var pointNamespace = uuid.Must(uuid.FromString("a3c1f0f4-6c5e-5f5a-9a1e-6a0d7f2c4b10"))

// allowedPayloadKeys is the complete payload allowlist. Everything else is
// dropped by Upsert, so a caller mistake cannot leak private fields.
var allowedPayloadKeys = map[string]bool{
	"item_id":        true,
	"category":       true,
	"zone":           true,
	"status":         true,
	"custodian_type": true,
	"description":    true,
}

// Point is one vector to store. The point id is derived from ItemID so the
// same item always maps to the same Qdrant point.
type Point struct {
	ItemID  string
	Vector  []float32
	Payload map[string]any
}

// ScoredPoint is one search result.
type ScoredPoint struct {
	ID      string
	Score   float64
	Payload map[string]any
}

// Filter narrows a vector search. Empty fields mean "no constraint".
type Filter struct {
	Statuses []string
	Category string
	Zone     string
}

// Client calls a Qdrant HTTP API.
type Client struct {
	baseURL    string
	collection string
	apiKey     string
	http       *http.Client
}

// New returns a Qdrant client. An empty baseURL makes every call return
// errs.ErrUnavailable so the search service can fall back to SQLite.
func New(baseURL, collection, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if strings.TrimSpace(collection) == "" {
		collection = DefaultCollection
	}
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		collection: collection,
		apiKey:     strings.TrimSpace(apiKey),
		http:       &http.Client{Timeout: timeout},
	}
}

// PointID returns the deterministic UUIDv5 for an item id.
func PointID(itemID string) string {
	return uuid.NewV5(pointNamespace, itemID).String()
}

// EnsureCollection creates the collection with the expected vector size and
// cosine distance. Qdrant answers 409 when the collection already exists; that
// is success for startup wiring.
func (c *Client) EnsureCollection(ctx context.Context) error {
	body := map[string]any{
		"vectors": map[string]any{
			"size":     VectorSize,
			"distance": "Cosine",
		},
	}
	status, err := c.do(ctx, http.MethodPut, "/collections/"+c.collection, body)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		return nil
	}
	if err := statusError("EnsureCollection", status); err != nil {
		return err
	}
	return nil
}

// Upsert writes one point. The payload is reduced to the allowlist and the
// item id is always present in the payload.
func (c *Client) Upsert(ctx context.Context, point Point) error {
	if strings.TrimSpace(point.ItemID) == "" {
		return fmt.Errorf("qdrant upsert: item id is required: %w", errs.ErrInvalidInput)
	}
	if len(point.Vector) != VectorSize {
		return fmt.Errorf("qdrant upsert: vector length %d, want %d: %w", len(point.Vector), VectorSize, errs.ErrInvalidInput)
	}

	payload := sanitizedPayload(point.ItemID, point.Payload)
	body := map[string]any{
		"points": []map[string]any{{
			"id":      PointID(point.ItemID),
			"vector":  point.Vector,
			"payload": payload,
		}},
	}
	status, err := c.do(ctx, http.MethodPut, "/collections/"+c.collection+"/points?wait=true", body)
	if err != nil {
		return err
	}
	return statusError("Upsert", status)
}

// Search returns the nearest points, optionally filtered by status, category
// and zone. The payload is always requested so callers can map hits back to
// item ids.
func (c *Client) Search(ctx context.Context, vector []float32, filter Filter, limit int) ([]ScoredPoint, error) {
	if len(vector) != VectorSize {
		return nil, fmt.Errorf("qdrant search: vector length %d, want %d: %w", len(vector), VectorSize, errs.ErrInvalidInput)
	}
	if limit <= 0 {
		limit = 10
	}

	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	if must := filterBody(filter); len(must) > 0 {
		body["filter"] = map[string]any{"must": must}
	}

	status, response, err := c.doDecode(ctx, http.MethodPost, "/collections/"+c.collection+"/points/search", body)
	if err != nil {
		return nil, err
	}
	if err := statusError("Search", status); err != nil {
		return nil, err
	}

	var decoded struct {
		Result []struct {
			ID      string         `json:"id"`
			Score   float64        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response, &decoded); err != nil {
		return nil, fmt.Errorf("qdrant search: decoding response: %w: %w", errs.ErrUnavailable, err)
	}

	points := make([]ScoredPoint, 0, len(decoded.Result))
	for _, hit := range decoded.Result {
		points = append(points, ScoredPoint{ID: hit.ID, Score: hit.Score, Payload: hit.Payload})
	}
	return points, nil
}

// Delete removes the point for an item id.
func (c *Client) Delete(ctx context.Context, itemID string) error {
	if strings.TrimSpace(itemID) == "" {
		return fmt.Errorf("qdrant delete: item id is required: %w", errs.ErrInvalidInput)
	}
	body := map[string]any{"points": []string{PointID(itemID)}}
	status, err := c.do(ctx, http.MethodPost, "/collections/"+c.collection+"/points/delete", body)
	if err != nil {
		return err
	}
	return statusError("Delete", status)
}

// do sends a request and drains the body.
func (c *Client) do(ctx context.Context, method, path string, body any) (int, error) {
	status, _, err := c.doDecode(ctx, method, path, body)
	return status, err
}

// doDecode sends a request and returns the status and raw response body.
func (c *Client) doDecode(ctx context.Context, method, path string, body any) (int, []byte, error) {
	if c == nil || c.baseURL == "" {
		return 0, nil, fmt.Errorf("qdrant: endpoint is not configured: %w", errs.ErrUnavailable)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("qdrant: encoding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, fmt.Errorf("qdrant: building request: %w", errs.ErrUnavailable)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("qdrant: request failed: %w: %w", errs.ErrUnavailable, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("qdrant: reading response: %w: %w", errs.ErrUnavailable, err)
	}
	return resp.StatusCode, data, nil
}

func statusError(operation string, status int) error {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}
	return fmt.Errorf("qdrant %s: unexpected status %d: %w", operation, status, errs.ErrUnavailable)
}

func sanitizedPayload(itemID string, payload map[string]any) map[string]any {
	sanitized := make(map[string]any, len(allowedPayloadKeys))
	for key, value := range payload {
		if allowedPayloadKeys[key] {
			sanitized[key] = value
		}
	}
	sanitized["item_id"] = itemID
	return sanitized
}

func filterBody(filter Filter) []map[string]any {
	must := make([]map[string]any, 0, 3)
	if len(filter.Statuses) > 0 {
		must = append(must, map[string]any{
			"key":   "status",
			"match": map[string]any{"any": filter.Statuses},
		})
	}
	if strings.TrimSpace(filter.Category) != "" {
		must = append(must, map[string]any{
			"key":   "category",
			"match": map[string]any{"value": filter.Category},
		})
	}
	if strings.TrimSpace(filter.Zone) != "" {
		must = append(must, map[string]any{
			"key":   "zone",
			"match": map[string]any{"value": filter.Zone},
		})
	}
	return must
}
