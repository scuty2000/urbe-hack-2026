package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

// Map-based field updates.
//
// The struct-based Update* methods use GORM `Updates`, which deliberately skips
// zero values. That is right for status transitions, but some flows must write
// a zero value back: clearing custodian_id, flipping `sensitive` to false,
// reverting an approval (approved_at = nil, approval_by = ""). The
// Update*Fields methods below are the supported escape hatch, so callers never
// need to reach for Database.GormDB() and bypass the repository layer.
//
// Every Update*Fields method:
//   - rejects columns outside its documented allowlist with
//     errs.ErrInvalidInput (typos and accidental PII columns fail loudly);
//   - rejects an empty field map with errs.ErrInvalidInput;
//   - normalises time.Time and *time.Time values to UTC;
//   - returns errs.ErrNotFound when the row does not exist.

// itemUpdateColumns is the allowlist for UpdateItemFields. Identity
// (id, created_at) is immutable and intentionally excluded.
var itemUpdateColumns = allowlist(
	"public_code", "source", "custody_mode", "status", "legal_finder_id",
	"rights_assigned_at", "category", "brand", "model", "color", "condition",
	"private_profile", "photos", "found_at", "found_lat", "found_lng",
	"found_zone", "found_notes", "custodian_id", "risk_tier", "sensitive",
	"transferred_at", "legal_term_start", "offered_to_finder_at", "disposed_at",
	"updated_at",
)

// claimUpdateColumns is the allowlist for UpdateClaimFields. item_id and
// searcher_id are immutable after creation.
var claimUpdateColumns = allowlist(
	"status", "return_channel", "confidence", "evidence", "approval_by",
	"approved_at", "pickup_pass_id", "reward_notice_id",
)

// custodyUpdateColumns is the allowlist for UpdateCustodyFields. item_id is
// immutable: a handover inserts a new custody row instead.
var custodyUpdateColumns = allowlist(
	"custodian_type", "custodian_id", "legal_finder_id", "rights_assigned_at",
	"approval_authority", "max_custody_days", "accepted_at", "due_at", "closed_at",
)

// passUpdateColumns is the allowlist for UpdatePassFields. claim_id and item_id
// are immutable; RedeemPass derives them from the stored pass.
var passUpdateColumns = allowlist(
	"searcher_id", "custodian_id", "token_hash", "issued_at", "expires_at",
	"status", "identity_required", "return_channel", "payment_id",
	"approval_id", "fee_id", "reward_notice_id",
)

// feeUpdateColumns is the allowlist for UpdateFeeFields.
var feeUpdateColumns = allowlist(
	"applies", "pa_tariff_id", "custody_days", "amount_cents", "currency", "status",
)

// rewardNoticeUpdateColumns is the allowlist for UpdateRewardNoticeFields.
var rewardNoticeUpdateColumns = allowlist(
	"legal_finder_id", "requested", "amount_cents", "shown_to_searcher_at",
	"payment_status",
)

func allowlist(columns ...string) map[string]bool {
	set := make(map[string]bool, len(columns))
	for _, column := range columns {
		set[column] = true
	}
	return set
}

// updateFields applies a map-based partial update after validating every
// column against allowed and normalising timestamps to UTC.
func (r *repo) updateFields(
	ctx context.Context,
	op, table, id string,
	fields map[string]any,
	allowed map[string]bool,
) error {
	if len(fields) == 0 {
		return fmt.Errorf("%s: at least one field is required: %w", op, errs.ErrInvalidInput)
	}

	normalized := make(map[string]any, len(fields))
	for column, value := range fields {
		if !allowed[column] {
			return fmt.Errorf("%s: column %q is not updatable: %w", op, column, errs.ErrInvalidInput)
		}
		normalized[column] = normalizeTimeValue(value)
	}

	res := r.gormdb.WithContext(ctx).Table(table).Where("id = ?", id).Updates(normalized)
	if res.Error != nil {
		return wrapErr(op, res.Error)
	}
	if res.RowsAffected == 0 {
		return notFound(op, id)
	}
	return nil
}

// normalizeTimeValue converts time values to UTC before persistence; other
// values pass through untouched so SQLite receives the native bool/int/string.
func normalizeTimeValue(value any) any {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC()
	case *time.Time:
		return utcPtr(typed)
	default:
		return value
	}
}

// rejectNilColumns rejects NULL for record fields that are non-pointer
// time.Time, because go-sqlite3 cannot scan NULL into them.
func rejectNilColumns(fields map[string]any, columns ...string) error {
	for _, column := range columns {
		if value, ok := fields[column]; ok && value == nil {
			return fmt.Errorf("column %q cannot be null: %w", column, errs.ErrInvalidInput)
		}
	}
	return nil
}
