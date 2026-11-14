package fee

import (
	"context"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

// Deps are the fee-service dependencies. Now is injectable for deterministic
// tests. Tariff is only a fallback for an environment without an active
// `tariffs` row; the database tariff always wins.
type Deps struct {
	Now    func() time.Time
	Tariff Tariff
	// Notifier receives the best-effort fee-paid notification. Nil disables it.
	Notifier platform.Notifier
}

// Service defines the fee and mock-payment operations exposed to the HTTP
// layer and to the pickup-pass domain.
type Service interface {
	// Calculate returns the fee for the item, creating or recalculating the
	// latest fee row when it is still unpaid. It is idempotent.
	Calculate(ctx context.Context, itemID string) (Fee, error)

	// Get returns the latest fee for the item without recalculating it.
	Get(ctx context.Context, itemID string) (Fee, error)

	// Pay settles a PA fee through the mock provider and returns the paid
	// payment. It is used by the domain and by tests; HTTP callers must use
	// PayFor so ownership is enforced.
	Pay(ctx context.Context, feeID string) (Payment, error)

	// PayFor settles a PA fee only when searcherID owns a claim on the item.
	PayFor(ctx context.Context, feeID, searcherID string) (Payment, error)

	// IsPaid reports whether the item's pickup is free or settled. It is the
	// pass service's fee check.
	IsPaid(ctx context.Context, itemID string) (bool, error)

	// Check implements pass.FeeChecker so the pass domain can ask about the
	// fee inside its own transaction.
	Check(ctx context.Context, repo repository.Repository, itemID, returnChannel string) (pass.FeeStatus, error)
}

// New returns a fee Service backed by db. Omitting Deps uses the system clock
// and the seeded/database tariff.
func New(db repository.Database, options ...Deps) Service {
	deps := Deps{}
	if len(options) > 0 {
		deps = options[0]
	}
	if deps.Tariff.Currency == "" {
		deps.Tariff.Currency = defaultCurrencyCode
	}
	return &service{db: db, deps: deps}
}
