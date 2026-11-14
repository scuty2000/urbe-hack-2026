package repository

import (
	"context"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

func (r *repo) CreateClaim(ctx context.Context, claim record.ClaimRecord) error {
	if claim.CreatedAt.IsZero() {
		claim.CreatedAt = time.Now().UTC()
	}
	claim.CreatedAt = utc(claim.CreatedAt)
	claim.ApprovedAt = utcPtr(claim.ApprovedAt)
	if err := r.gormdb.WithContext(ctx).Create(&claim).Error; err != nil {
		return wrapErr("CreateClaim", err)
	}
	return nil
}

func (r *repo) GetClaimByID(ctx context.Context, id string) (record.ClaimRecord, error) {
	var claim record.ClaimRecord
	if err := r.gormdb.WithContext(ctx).Where("id = ?", id).First(&claim).Error; err != nil {
		return claim, wrapErr("GetClaimByID", err)
	}
	return claim, nil
}

// UpdateClaim performs a partial update: only non-zero fields are written, and
// approved_at can be set but not cleared.
func (r *repo) UpdateClaim(ctx context.Context, claim record.ClaimRecord) error {
	claim.ApprovedAt = utcPtr(claim.ApprovedAt)
	res := r.gormdb.WithContext(ctx).
		Model(&record.ClaimRecord{}).
		Where("id = ?", claim.ID).
		Updates(claim)
	if res.Error != nil {
		return wrapErr("UpdateClaim", res.Error)
	}
	if res.RowsAffected == 0 {
		return notFound("UpdateClaim", claim.ID)
	}
	return nil
}

func (r *repo) CreateVerificationSession(ctx context.Context, session record.VerificationSessionRecord) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	session.CreatedAt = utc(session.CreatedAt)
	if err := r.gormdb.WithContext(ctx).Create(&session).Error; err != nil {
		return wrapErr("CreateVerificationSession", err)
	}
	return nil
}

func (r *repo) GetVerificationSessionByClaimID(ctx context.Context, claimID string) (record.VerificationSessionRecord, error) {
	var session record.VerificationSessionRecord
	err := r.gormdb.WithContext(ctx).
		Where("claim_id = ?", claimID).
		Order("created_at DESC, rowid DESC").
		First(&session).Error
	if err != nil {
		return session, wrapErr("GetVerificationSessionByClaimID", err)
	}
	return session, nil
}

func (r *repo) ListClaimsByItem(ctx context.Context, itemID string) ([]record.ClaimRecord, error) {
	var claims []record.ClaimRecord
	if err := r.gormdb.WithContext(ctx).
		Where("item_id = ?", itemID).
		Order("created_at DESC, rowid DESC").
		Find(&claims).Error; err != nil {
		return nil, wrapErr("ListClaimsByItem", err)
	}
	return claims, nil
}

func (r *repo) ListClaimsBySearcher(ctx context.Context, searcherID string) ([]record.ClaimRecord, error) {
	var claims []record.ClaimRecord
	if err := r.gormdb.WithContext(ctx).
		Where("searcher_id = ?", searcherID).
		Order("created_at DESC, rowid DESC").
		Find(&claims).Error; err != nil {
		return nil, wrapErr("ListClaimsBySearcher", err)
	}
	return claims, nil
}
