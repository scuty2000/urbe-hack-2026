package search

import (
	"context"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/qdrant"
)

// Embedder is the vLLM embeddings port.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// VectorStore is the Qdrant port. Item ids are mapped to UUIDv5 point ids by
// the adapter; the payload is restricted to the documented allowlist.
type VectorStore interface {
	Upsert(ctx context.Context, point qdrant.Point) error
	Search(ctx context.Context, vector []float32, filter qdrant.Filter, limit int) ([]qdrant.ScoredPoint, error)
	Delete(ctx context.Context, itemID string) error
}

// Service is the search API exposed to the HTTP and domain layers.
type Service interface {
	// Search returns role-safe candidates for a query. It uses the vector
	// path when configured and falls back to SQLite on any failure.
	Search(ctx context.Context, query Query) ([]SearchResult, error)

	// IndexItem embeds and upserts an item. Items that are not searchable are
	// removed from the index instead, so a status change cannot leak them.
	IndexItem(ctx context.Context, item itemdomain.Item) error

	// DeleteItem removes an item from the vector index. It is a no-op when
	// the vector path is not configured.
	DeleteItem(ctx context.Context, itemID string) error
}
