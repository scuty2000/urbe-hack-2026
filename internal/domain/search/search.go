/*
Package search implements the hybrid item search: Qdrant + vLLM embeddings
when both are available, and a deterministic SQLite fallback otherwise.

The public result is intentionally smaller than the item aggregate. It carries
only the facts the agent is allowed to reveal before verification: the item id,
a score, the category, the found zone, the found time and the custodian type.
Private profile data, document fields and exact coordinates never leave the
registry.
*/
package search

import (
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/qdrant"
)

const (
	// defaultLimit is the number of candidates returned when Query.Limit is 0.
	defaultLimit = 10
	// maxLimit caps the caller-supplied limit.
	maxLimit = 50
	// maxCandidates bounds the SQLite candidate set before in-memory ranking.
	maxCandidates = 200
)

// allowedStatuses are the only item states that may appear in search results.
// pending_custody, custody_rejected and every terminal state are excluded.
var allowedStatuses = []string{
	itemdomain.StatusInCustody,
	"transferred",
}

// Query is a hybrid search request. Empty fields mean "no constraint".
type Query struct {
	Text        string
	Category    string
	Zone        string
	FoundAfter  time.Time
	FoundBefore time.Time
	Limit       int
}

// SearchResult is the role-safe candidate summary.
type SearchResult struct {
	ItemID        string    `json:"itemId"`
	Score         float64   `json:"score"`
	Category      string    `json:"category"`
	FoundZone     string    `json:"foundZone"`
	FoundAt       time.Time `json:"foundAt"`
	CustodianType string    `json:"custodianType"`
}

// Deps configures the optional vector path. A nil Embedder or Vectors (and an
// empty URL) disables it and leaves the SQLite fallback in charge.
type Deps struct {
	Embedder Embedder
	Vectors  VectorStore

	QdrantURL        string
	QdrantCollection string
	QdrantAPIKey     string
	QdrantTimeout    time.Duration

	EmbeddingsURL       string
	EmbeddingsModel     string
	EmbeddingsKey       string
	EmbeddingsTimeout   time.Duration
	EmbeddingsBatchSize int

	// Now is injectable for deterministic circuit-breaker tests.
	Now func() time.Time
}

// vectorFilter returns the Qdrant filter shared by the service.
func vectorFilter(query Query) qdrant.Filter {
	return qdrant.Filter{
		Statuses: allowedStatuses,
		Category: query.Category,
		Zone:     query.Zone,
	}
}
