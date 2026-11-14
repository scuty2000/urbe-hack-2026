package repository

import (
	"context"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// NameRepository is the persistence contract of the sample Name aggregate.
type NameRepository interface {
	GetNameByID(ctx context.Context, id int64) (record.NameRecord, error)
	UpdateNameByID(ctx context.Context, id int64, name string) error
	CreateName(ctx context.Context, name string) (int64, error)
	DeleteNameByID(ctx context.Context, id int64) error
}

// UserRepository persists actors and their server-side sessions.
type UserRepository interface {
	CreateUser(ctx context.Context, user record.UserRecord) error
	GetUserByID(ctx context.Context, id string) (record.UserRecord, error)
	ListUsers(ctx context.Context) ([]record.UserRecord, error)

	CreateSession(ctx context.Context, session record.SessionRecord) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (record.SessionRecord, error)
	// DeleteSession is idempotent: deleting an unknown session is not an error.
	DeleteSession(ctx context.Context, tokenHash string) error
	// DeleteExpiredSessions removes sessions whose expires_at is at or before
	// now and returns how many rows were deleted.
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
}

// IntermediaryRepository persists civic points that accept found items.
type IntermediaryRepository interface {
	CreateIntermediary(ctx context.Context, intermediary record.IntermediaryRecord) error
	GetIntermediaryByID(ctx context.Context, id string) (record.IntermediaryRecord, error)
	ListIntermediaries(ctx context.Context) ([]record.IntermediaryRecord, error)
}

// ItemFilter narrows SearchItems. Empty fields mean "no constraint"; Limit
// defaults to 50 and is capped at 200.
type ItemFilter struct {
	// Status is a single-status convenience filter kept for existing callers.
	// Statuses takes precedence when both are set.
	Status string
	// Statuses restricts the result to any of the listed statuses. Search uses
	// it to keep pending_custody and other non-public states out of results.
	Statuses      []string
	Category      string
	Zone          string
	CustodianID   string
	LegalFinderID string
	// FoundAfter and FoundBefore are inclusive bounds on found_at.
	FoundAfter  *time.Time
	FoundBefore *time.Time
	// Query matches free text against category, brand, model, color,
	// condition, found_zone and found_notes. Private profile data is
	// deliberately excluded so a search can never confirm private details.
	Query string
	Limit int
}

// ItemRepository persists the Item aggregate.
type ItemRepository interface {
	CreateItem(ctx context.Context, item record.ItemRecord) error
	GetItemByID(ctx context.Context, id string) (record.ItemRecord, error)
	// UpdateItem applies a partial update: zero-valued fields are left as-is.
	UpdateItem(ctx context.Context, item record.ItemRecord) error
	// UpdateItemFields is the map-based escape hatch for writing zero values
	// (sensitive=false, custodian_id="", transferred_at=nil). Column allowlist:
	// public_code, source, custody_mode, status, legal_finder_id,
	// rights_assigned_at, category, brand, model, color, condition,
	// private_profile, photos, found_at, found_lat, found_lng, found_zone,
	// found_notes, custodian_id, risk_tier, sensitive, transferred_at,
	// legal_term_start, offered_to_finder_at, disposed_at, updated_at.
	// Unknown columns or an empty map return errs.ErrInvalidInput; updated_at
	// defaults to now. Timestamps are stored in UTC.
	UpdateItemFields(ctx context.Context, id string, fields map[string]any) error
	// TransitionItemStatus applies a guarded UPDATE items SET status=? WHERE
	// id=? AND status=? and reports whether a row changed. A false result means
	// the item was already in another status (lost race or repeated request);
	// callers map it to errs.ErrConflict or skip the item.
	TransitionItemStatus(ctx context.Context, id, fromStatus, toStatus string) (bool, error)
	ListItemsByCustodian(ctx context.Context, custodianID string) ([]record.ItemRecord, error)
	SearchItems(ctx context.Context, filter ItemFilter) ([]record.ItemRecord, error)
	// ListPickupDue returns items whose current custody is open and due at or
	// before now, ordered by due date (sensitive items included).
	ListPickupDue(ctx context.Context, now time.Time) ([]record.ItemRecord, error)
	// CreateItemWithCustody atomically inserts the item, its custody row and
	// the first custody event.
	CreateItemWithCustody(ctx context.Context, item record.ItemRecord, custody record.CustodyRecord, event record.CustodyEventRecord) error
}

// CustodyRepository persists custody rows and their append-only event trail.
type CustodyRepository interface {
	CreateCustody(ctx context.Context, custody record.CustodyRecord) error
	GetCustodyByID(ctx context.Context, id string) (record.CustodyRecord, error)
	// GetCustodyByItemID returns the current custody: the latest open row, or
	// the most recently inserted row when every custody is closed.
	GetCustodyByItemID(ctx context.Context, itemID string) (record.CustodyRecord, error)
	UpdateCustody(ctx context.Context, custody record.CustodyRecord) error
	// UpdateCustodyFields writes zero values (accepted_at=nil, closed_at=nil,
	// custodian_id=""). Allowlist: custodian_type, custodian_id,
	// legal_finder_id, rights_assigned_at, approval_authority,
	// max_custody_days, accepted_at, due_at, closed_at. Unknown columns or an
	// empty map return errs.ErrInvalidInput; timestamps are stored in UTC.
	UpdateCustodyFields(ctx context.Context, id string, fields map[string]any) error
	CreateCustodyEvent(ctx context.Context, event record.CustodyEventRecord) error
	// ListCustodyEvents returns the item timeline in chronological order.
	ListCustodyEvents(ctx context.Context, itemID string) ([]record.CustodyEventRecord, error)
}

// ClaimRepository persists claims and their verification sessions.
type ClaimRepository interface {
	CreateClaim(ctx context.Context, claim record.ClaimRecord) error
	GetClaimByID(ctx context.Context, id string) (record.ClaimRecord, error)
	UpdateClaim(ctx context.Context, claim record.ClaimRecord) error
	// UpdateClaimFields writes zero values (approval_by="", approved_at=nil,
	// confidence=0). Allowlist: status, return_channel, confidence, evidence,
	// approval_by, approved_at, pickup_pass_id, reward_notice_id. Unknown
	// columns or an empty map return errs.ErrInvalidInput.
	UpdateClaimFields(ctx context.Context, id string, fields map[string]any) error
	CreateVerificationSession(ctx context.Context, session record.VerificationSessionRecord) error
	// GetVerificationSessionByClaimID returns the most recent session.
	GetVerificationSessionByClaimID(ctx context.Context, claimID string) (record.VerificationSessionRecord, error)
	ListClaimsByItem(ctx context.Context, itemID string) ([]record.ClaimRecord, error)
	ListClaimsBySearcher(ctx context.Context, searcherID string) ([]record.ClaimRecord, error)
}

// PassRepository persists single-use pickup passes.
type PassRepository interface {
	CreatePass(ctx context.Context, pass record.PassRecord) error
	GetPassByID(ctx context.Context, id string) (record.PassRecord, error)
	GetPassByTokenHash(ctx context.Context, tokenHash string) (record.PassRecord, error)
	UpdatePass(ctx context.Context, pass record.PassRecord) error
	// UpdatePassFields writes zero values (identity_required=false,
	// payment_id=""). Allowlist: searcher_id, custodian_id, token_hash,
	// issued_at, expires_at, status, identity_required, return_channel,
	// payment_id, approval_id, fee_id, reward_notice_id. claim_id and item_id
	// are immutable. Unknown columns or an empty map return
	// errs.ErrInvalidInput; issued_at/expires_at cannot be null.
	UpdatePassFields(ctx context.Context, id string, fields map[string]any) error
	// IssuePass atomically inserts the pass and links it onto its claim.
	IssuePass(ctx context.Context, pass record.PassRecord) error
	// RevokePassesByItem revokes every still-open pass for the item except
	// exceptPassID (pass "" to revoke all) and returns the number revoked.
	RevokePassesByItem(ctx context.Context, itemID, exceptPassID string) (int64, error)
	// RedeemPass atomically marks the pass redeemed, the item returned and the
	// claim handed over. itemID and claimID are derived from the stored pass;
	// the arguments are optional assertions (pass "" to skip) and a mismatch is
	// errs.ErrInvalidInput. A non-open pass is errs.ErrConflict.
	RedeemPass(ctx context.Context, passID, itemID, claimID string, at time.Time) error
}

// FeeRepository persists PA fees, their payments and reward notices.
type FeeRepository interface {
	CreateFee(ctx context.Context, fee record.FeeRecord) error
	GetFeeByID(ctx context.Context, id string) (record.FeeRecord, error)
	// GetFeeByItemID returns the most recently created fee for the item.
	GetFeeByItemID(ctx context.Context, itemID string) (record.FeeRecord, error)
	UpdateFee(ctx context.Context, fee record.FeeRecord) error
	// UpdateFeeFields writes zero values (amount_cents=0, pa_tariff_id="").
	// Allowlist: applies, pa_tariff_id, custody_days, amount_cents, currency,
	// status. Unknown columns or an empty map return errs.ErrInvalidInput.
	UpdateFeeFields(ctx context.Context, id string, fields map[string]any) error

	CreatePayment(ctx context.Context, payment record.PaymentRecord) error
	GetPaymentByID(ctx context.Context, id string) (record.PaymentRecord, error)
	// GetPaymentByFeeID returns the most recent payment for the fee.
	GetPaymentByFeeID(ctx context.Context, feeID string) (record.PaymentRecord, error)

	CreateRewardNotice(ctx context.Context, notice record.RewardNoticeRecord) error
	// GetRewardNoticeByItemID returns the most recently created notice.
	GetRewardNoticeByItemID(ctx context.Context, itemID string) (record.RewardNoticeRecord, error)
	UpdateRewardNotice(ctx context.Context, notice record.RewardNoticeRecord) error
	// UpdateRewardNoticeFields writes zero values (requested=false,
	// amount_cents=0, shown_to_searcher_at=nil). Allowlist: legal_finder_id,
	// requested, amount_cents, shown_to_searcher_at, payment_status. Unknown
	// columns or an empty map return errs.ErrInvalidInput.
	UpdateRewardNoticeFields(ctx context.Context, id string, fields map[string]any) error
}

// NotificationRepository persists in-app notifications.
type NotificationRepository interface {
	CreateNotification(ctx context.Context, notification record.NotificationRecord) error
	// ListNotificationsByUser returns newest first.
	ListNotificationsByUser(ctx context.Context, userID string) ([]record.NotificationRecord, error)
	MarkNotificationRead(ctx context.Context, id string, readAt time.Time) error
}

// AuditLogFilter narrows ListAuditLogs. Empty fields mean "no constraint";
// Limit defaults to 100 and is capped at 500.
type AuditLogFilter struct {
	Entity   string
	EntityID string
	ActorID  string
	Limit    int
}

// AuditRepository persists the append-only audit trail.
type AuditRepository interface {
	CreateAuditLog(ctx context.Context, entry record.AuditLogRecord) error
	// ListAuditLogs returns entries in chronological order.
	ListAuditLogs(ctx context.Context, filter AuditLogFilter) ([]record.AuditLogRecord, error)
}

// TariffRepository persists the configurable PA tariffs.
type TariffRepository interface {
	CreateTariff(ctx context.Context, tariff record.TariffRecord) error
	// GetActiveTariff returns the most recently created active tariff.
	GetActiveTariff(ctx context.Context) (record.TariffRecord, error)
	ListTariffs(ctx context.Context) ([]record.TariffRecord, error)
}

// LostReportRepository persists proactive lost-item reports.
type LostReportRepository interface {
	CreateLostReport(ctx context.Context, report record.LostReportRecord) error
	// ListLostReports returns newest first; an empty status means "all".
	ListLostReports(ctx context.Context, status string) ([]record.LostReportRecord, error)
}

// Repository is the full set of persistence operations exposed by this package.
type Repository interface {
	NameRepository
	UserRepository
	IntermediaryRepository
	ItemRepository
	CustodyRepository
	ClaimRepository
	PassRepository
	FeeRepository
	NotificationRepository
	AuditRepository
	TariffRepository
	LostReportRepository
}

// Transactor provides transaction support.
type Transactor interface {
	RunInTransaction(ctx context.Context, fn func(context.Context, Repository) error) error
}
