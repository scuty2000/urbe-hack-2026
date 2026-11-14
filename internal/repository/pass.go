package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"gorm.io/gorm"
)

// openPassStatuses are the states in which a pass can still be scanned.
var openPassStatuses = []string{"pending_payment", "issued", "validated"}

func (r *repo) CreatePass(ctx context.Context, pass record.PassRecord) error {
	pass = withPassTimes(pass)
	if err := r.gormdb.WithContext(ctx).Create(&pass).Error; err != nil {
		return wrapErr("CreatePass", err)
	}
	return nil
}

func withPassTimes(pass record.PassRecord) record.PassRecord {
	if pass.IssuedAt.IsZero() {
		pass.IssuedAt = time.Now().UTC()
	}
	pass.IssuedAt = utc(pass.IssuedAt)
	pass.ExpiresAt = utc(pass.ExpiresAt)
	return pass
}

func (r *repo) GetPassByID(ctx context.Context, id string) (record.PassRecord, error) {
	var pass record.PassRecord
	if err := r.gormdb.WithContext(ctx).Where("id = ?", id).First(&pass).Error; err != nil {
		return pass, wrapErr("GetPassByID", err)
	}
	return pass, nil
}

func (r *repo) GetPassByTokenHash(ctx context.Context, tokenHash string) (record.PassRecord, error) {
	var pass record.PassRecord
	if err := r.gormdb.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&pass).Error; err != nil {
		return pass, wrapErr("GetPassByTokenHash", err)
	}
	return pass, nil
}

// UpdatePass performs a partial update: only non-zero fields are written.
func (r *repo) UpdatePass(ctx context.Context, pass record.PassRecord) error {
	pass.IssuedAt = utc(pass.IssuedAt)
	pass.ExpiresAt = utc(pass.ExpiresAt)
	res := r.gormdb.WithContext(ctx).
		Model(&record.PassRecord{}).
		Where("id = ?", pass.ID).
		Updates(pass)
	if res.Error != nil {
		return wrapErr("UpdatePass", res.Error)
	}
	if res.RowsAffected == 0 {
		return notFound("UpdatePass", pass.ID)
	}
	return nil
}

// IssuePass inserts the pass and links it onto its claim atomically, so a pass
// can never exist without the claim pointing at it.
func (r *repo) IssuePass(ctx context.Context, pass record.PassRecord) error {
	pass = withPassTimes(pass)
	return r.gormdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&pass).Error; err != nil {
			return wrapErr("IssuePass create", err)
		}
		res := tx.Model(&record.ClaimRecord{}).
			Where("id = ?", pass.ClaimID).
			Update("pickup_pass_id", pass.ID)
		if res.Error != nil {
			return wrapErr("IssuePass link claim", res.Error)
		}
		if res.RowsAffected == 0 {
			return notFound("IssuePass claim", pass.ClaimID)
		}
		return nil
	})
}

// RevokePassesByItem revokes open passes for an item, except the pass that
// must survive a re-issue (pass "" to revoke all).
func (r *repo) RevokePassesByItem(ctx context.Context, itemID, exceptPassID string) (int64, error) {
	query := r.gormdb.WithContext(ctx).
		Model(&record.PassRecord{}).
		Where("item_id = ?", itemID).
		Where("status IN ?", openPassStatuses)
	if exceptPassID != "" {
		query = query.Where("id <> ?", exceptPassID)
	}
	res := query.Update("status", "revoked")
	if res.Error != nil {
		return 0, wrapErr("RevokePassesByItem", res.Error)
	}
	return res.RowsAffected, nil
}

// RedeemPass performs the handover state change: pass redeemed, item returned,
// claim handed over. All three writes happen in one transaction, and an
// already redeemed/revoked/expired pass is a conflict, which enforces the
// single-use rule at the storage layer.
func (r *repo) RedeemPass(ctx context.Context, passID, itemID, claimID string, at time.Time) error {
	at = at.UTC()
	return r.gormdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		passRes := tx.Model(&record.PassRecord{}).
			Where("id = ? AND status IN ?", passID, openPassStatuses).
			Update("status", "redeemed")
		if passRes.Error != nil {
			return wrapErr("RedeemPass pass", passRes.Error)
		}
		if passRes.RowsAffected == 0 {
			return fmt.Errorf("RedeemPass pass id=%s: %w", passID, errs.ErrConflict)
		}

		itemRes := tx.Model(&record.ItemRecord{}).
			Where("id = ?", itemID).
			Updates(map[string]any{"status": "returned", "updated_at": at})
		if itemRes.Error != nil {
			return wrapErr("RedeemPass item", itemRes.Error)
		}
		if itemRes.RowsAffected == 0 {
			return notFound("RedeemPass item", itemID)
		}

		claimRes := tx.Model(&record.ClaimRecord{}).
			Where("id = ?", claimID).
			Update("status", "handed_over")
		if claimRes.Error != nil {
			return wrapErr("RedeemPass claim", claimRes.Error)
		}
		if claimRes.RowsAffected == 0 {
			return notFound("RedeemPass claim", claimID)
		}
		return nil
	})
}
