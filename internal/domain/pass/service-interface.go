package pass

import (
	"context"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

// FeeChecker reports whether a pickup requires payment and whether it has been
// settled. The repository passed to Check is the transaction-scoped repository,
// so a fake can be used in tests and the Task 10 fee service can be wired in
// without changing the pass domain.
type FeeChecker interface {
	Check(ctx context.Context, repo repository.Repository, itemID, returnChannel string) (FeeStatus, error)
}

// Deps are the pass-service dependencies. An empty PassSecret is a deliberate
// fail-closed configuration: every operation returns errs.ErrUnavailable.
type Deps struct {
	PassSecret string
	PassTTL    time.Duration
	// Now is injectable for RevokePassesByItem; Issue/Validate/Redeem receive
	// the operation time explicitly.
	Now func() time.Time
	// Fees defaults to a repository-backed checker when nil.
	Fees FeeChecker
	// Notifier receives best-effort notifications after issue/redeem commits.
	// Nil disables them.
	Notifier platform.Notifier
}

// Service defines the pickup-pass operations exposed to the HTTP layer and to
// later tasks (transfer revokes open passes).
type Service interface {
	// Issue creates a signed pass for an approved claim. The actor must be the
	// claim's searcher; the raw token is returned only to that caller.
	Issue(ctx context.Context, claimID, actorID string, now time.Time) (Pass, error)

	// Validate verifies a scanned token for a custodian. On failure it returns
	// a Validation with Valid=false, a Reason, and an error carrying a sentinel
	// (ErrForbidden, ErrConflict or ErrInvalidInput).
	Validate(ctx context.Context, token, custodianID string, now time.Time) (Validation, error)

	// Redeem validates a token and atomically marks the pass redeemed, the item
	// returned and the claim handed over, appending the handover event and audit.
	Redeem(ctx context.Context, token, custodianID, actorID string, now time.Time) (itemdomain.Item, error)

	// RevokePassesByItem revokes every still-open pass for an item (used by the
	// institution transfer) and audits the revocation.
	RevokePassesByItem(ctx context.Context, itemID string) (int64, error)
}

// New returns a pass Service backed by db.
func New(db repository.Database, deps Deps) Service {
	if deps.PassTTL <= 0 {
		deps.PassTTL = defaultTTL
	}
	if deps.Fees == nil {
		deps.Fees = repositoryFeeChecker{}
	}
	return &service{db: db, deps: deps}
}
