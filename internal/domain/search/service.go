package search

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/embeddings"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/qdrant"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

// service implements Service.
type service struct {
	db   repository.Database
	deps Deps
}

// New returns a search service. When Deps carries URLs instead of clients, the
// platform clients are created here; empty URLs leave the SQLite fallback in
// charge, which is how tests and offline demos run.
func New(db repository.Database, deps Deps) Service {
	if deps.Embedder == nil && strings.TrimSpace(deps.EmbeddingsURL) != "" {
		deps.Embedder = embeddings.New(
			deps.EmbeddingsURL,
			deps.EmbeddingsModel,
			deps.EmbeddingsKey,
			deps.EmbeddingsTimeout,
			deps.EmbeddingsBatchSize,
		)
	}
	if deps.Vectors == nil && strings.TrimSpace(deps.QdrantURL) != "" {
		deps.Vectors = qdrant.New(
			deps.QdrantURL,
			deps.QdrantCollection,
			deps.QdrantAPIKey,
			deps.QdrantTimeout,
		)
	}
	return &service{db: db, deps: deps}
}

// Search tries the vector path when a query text and both clients are
// available; every failure or empty vector result falls through to SQLite so
// the demo never depends on Qdrant being up.
func (s *service) Search(ctx context.Context, query Query) ([]SearchResult, error) {
	query = normalizeQuery(query)

	if query.Text != "" && s.deps.Embedder != nil && s.deps.Vectors != nil {
		if results, err := s.vectorSearch(ctx, query); err == nil && len(results) > 0 {
			return results, nil
		}
	}
	return s.fallbackSearch(ctx, query)
}

func (s *service) vectorSearch(ctx context.Context, query Query) ([]SearchResult, error) {
	vector, err := s.deps.Embedder.Embed(ctx, query.Text)
	if err != nil {
		return nil, fmt.Errorf("search: embedding query: %w", err)
	}
	points, err := s.deps.Vectors.Search(ctx, vector, vectorFilter(query), query.Limit)
	if err != nil {
		return nil, fmt.Errorf("search: vector search: %w", err)
	}

	results := make([]SearchResult, 0, len(points))
	seen := make(map[string]bool, len(points))
	for _, point := range points {
		itemID, _ := point.Payload["item_id"].(string)
		if itemID == "" || seen[itemID] {
			continue
		}
		row, err := s.db.GetItemByID(ctx, itemID)
		if errors.Is(err, errs.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("search: loading item %q: %w", itemID, err)
		}
		item, err := itemdomain.FromRecord(row)
		if err != nil {
			return nil, fmt.Errorf("search: mapping item %q: %w", itemID, err)
		}
		if !matchesQuery(item, query) {
			continue
		}
		seen[itemID] = true
		results = append(results, toResult(item, point.Score))
		if len(results) == query.Limit {
			break
		}
	}
	return results, nil
}

// fallbackSearch applies the structural filters in SQLite and then ranks the
// candidates by token overlap in Go.
func (s *service) fallbackSearch(ctx context.Context, query Query) ([]SearchResult, error) {
	filter := repository.ItemFilter{
		Statuses:    allowedStatuses,
		Category:    query.Category,
		Zone:        query.Zone,
		Limit:       maxCandidates,
		FoundAfter:  timePtr(query.FoundAfter),
		FoundBefore: timePtr(query.FoundBefore),
	}
	rows, err := s.db.SearchItems(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("search: sqlite fallback: %w", err)
	}

	queryTokens := tokenSet(query.Text)
	scored := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		item, err := itemdomain.FromRecord(row)
		if err != nil {
			return nil, fmt.Errorf("search: mapping item %q: %w", row.ID, err)
		}
		if !matchesQuery(item, query) {
			continue
		}

		score := 1.0
		if len(queryTokens) > 0 {
			score = overlapScore(queryTokens, tokenSet(publicText(item)))
			if score == 0 {
				continue
			}
		}
		scored = append(scored, toResult(item, score))
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].FoundAt.After(scored[j].FoundAt)
	})
	if len(scored) > query.Limit {
		scored = scored[:query.Limit]
	}
	return scored, nil
}

// IndexItem embeds and upserts an item. A non-searchable item is removed from
// the index instead, so a pending or rejected item can never be returned by a
// stale vector.
func (s *service) IndexItem(ctx context.Context, item itemdomain.Item) error {
	if s.deps.Vectors == nil {
		return nil
	}
	if !searchableStatus(item.Status) {
		return s.DeleteItem(ctx, item.ID)
	}
	if s.deps.Embedder == nil {
		return nil
	}

	text := indexText(item)
	if text == "" {
		return nil
	}
	vector, err := s.deps.Embedder.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("index item %q: embedding: %w", item.ID, err)
	}

	payload := map[string]any{
		"item_id":        item.ID,
		"category":       item.Category,
		"zone":           item.FoundZone,
		"status":         item.Status,
		"custodian_type": item.CustodyMode,
		"description":    text,
	}
	if err := s.deps.Vectors.Upsert(ctx, qdrant.Point{
		ItemID:  item.ID,
		Vector:  vector,
		Payload: payload,
	}); err != nil {
		return fmt.Errorf("index item %q: upsert: %w", item.ID, err)
	}
	return nil
}

// DeleteItem removes an item from the vector index.
func (s *service) DeleteItem(ctx context.Context, itemID string) error {
	if s.deps.Vectors == nil || strings.TrimSpace(itemID) == "" {
		return nil
	}
	if err := s.deps.Vectors.Delete(ctx, itemID); err != nil {
		return fmt.Errorf("delete item %q from index: %w", itemID, err)
	}
	return nil
}

func normalizeQuery(query Query) Query {
	query.Text = strings.TrimSpace(query.Text)
	query.Category = strings.TrimSpace(query.Category)
	query.Zone = strings.TrimSpace(query.Zone)
	query.FoundAfter = query.FoundAfter.UTC()
	query.FoundBefore = query.FoundBefore.UTC()
	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}
	if query.Limit > maxLimit {
		query.Limit = maxLimit
	}
	return query
}

func matchesQuery(item itemdomain.Item, query Query) bool {
	if !searchableStatus(item.Status) {
		return false
	}
	if query.Category != "" && !strings.EqualFold(item.Category, query.Category) {
		return false
	}
	if query.Zone != "" && !strings.EqualFold(item.FoundZone, query.Zone) {
		return false
	}
	if !query.FoundAfter.IsZero() && item.FoundAt.Before(query.FoundAfter) {
		return false
	}
	if !query.FoundBefore.IsZero() && item.FoundAt.After(query.FoundBefore) {
		return false
	}
	return true
}

func searchableStatus(status string) bool {
	for _, allowed := range allowedStatuses {
		if status == allowed {
			return true
		}
	}
	return false
}

func toResult(item itemdomain.Item, score float64) SearchResult {
	return SearchResult{
		ItemID:        item.ID,
		Score:         score,
		Category:      item.Category,
		FoundZone:     item.FoundZone,
		FoundAt:       item.FoundAt,
		CustodianType: item.CustodyMode,
	}
}

// indexText is the only textual payload sent to the embedding model. It keeps
// public descriptive fields and deliberately omits the private profile,
// document fields, photos, free-text notes and exact coordinates.
func indexText(item itemdomain.Item) string {
	parts := []string{
		item.Category,
		item.Brand,
		item.Model,
		item.Color,
		item.Condition,
		item.FoundZone,
	}
	return strings.Join(nonEmpty(parts), " ")
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}
