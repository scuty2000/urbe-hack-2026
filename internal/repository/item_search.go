package repository

import (
	"context"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// SearchItems is the SQLite side of the hybrid search. The repository applies
// the structural filters (status, category, zone, time window) and a coarse
// token LIKE filter; the search domain re-ranks the candidates by overlap.
//
// Private profile data is intentionally not part of the LIKE columns: a
// search must never confirm a detail that only the owner should know.
func (r *repo) SearchItems(ctx context.Context, filter ItemFilter) ([]record.ItemRecord, error) {
	query := r.gormdb.WithContext(ctx).Model(&record.ItemRecord{})
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	} else if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.Zone != "" {
		query = query.Where("found_zone = ?", filter.Zone)
	}
	if filter.CustodianID != "" {
		query = query.Where("custodian_id = ?", filter.CustodianID)
	}
	if filter.LegalFinderID != "" {
		query = query.Where("legal_finder_id = ?", filter.LegalFinderID)
	}
	if filter.FoundAfter != nil {
		query = query.Where("found_at >= ?", filter.FoundAfter.UTC())
	}
	if filter.FoundBefore != nil {
		query = query.Where("found_at <= ?", filter.FoundBefore.UTC())
	}
	// Every free-text token must match at least one searchable column. This is
	// a cheap pre-filter; token overlap scoring happens in the search domain.
	for _, token := range strings.Fields(filter.Query) {
		like := "%" + token + "%"
		query = query.Where(
			"(category LIKE ? OR brand LIKE ? OR model LIKE ? OR color LIKE ? OR condition LIKE ? OR found_zone LIKE ? OR found_notes LIKE ?)",
			like, like, like, like, like, like, like,
		)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultItemLimit
	}
	if limit > maxItemLimit {
		limit = maxItemLimit
	}

	var items []record.ItemRecord
	if err := query.Order("found_at DESC, created_at DESC, rowid DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, wrapErr("SearchItems", err)
	}
	return items, nil
}
