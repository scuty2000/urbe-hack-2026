package institution

import (
	"context"
	"time"

	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	feedomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

// Deps are the collaborators of the institution service. Every dependency is
// optional: New fills the missing ones with the default service bound to the
// same database, so institution.New(db) is enough for the demo and the tests.
type Deps struct {
	// Items creates the official-intake item through the normal catalog path
	// (D3 classification, risk tier, public code, first custody row).
	Items itemdomain.Service
	// Custody is the intermediary custody policy; the institution currently
	// only reads its deadlines indirectly through the repository.
	Custody custodydomain.Service
	// Claim applies the deterministic verification policy and records reviews.
	Claim claimdomain.Service
	// Pass is the pickup-pass service. Transfer revokes open passes through
	// the transaction-scoped repository; the dependency is kept on the PA
	// surface for the pass operations of later tasks.
	Pass passdomain.Service
	// Search refreshes the vector index after a transfer.
	Search search.Service
	// Fee is reserved for the PA fee surface that shares this operator role.
	Fee feedomain.Service
	// Now is injectable for deterministic tests.
	Now func() time.Time
}

// Service defines the public-administration operations exposed to the HTTP
// layer.
type Service interface {
	// ListPickups returns the items waiting at the intermediaries, grouped by
	// intermediary and zone with sensitive items first.
	ListPickups(ctx context.Context) ([]PickupGroup, error)

	// Transfer takes an item in pickup_due/sensitive_pickup into institution
	// custody: the intermediary row is closed, a new institution custody row is
	// opened, the item becomes transferred and the legal term starts now.
	Transfer(ctx context.Context, itemID, actorID string, now time.Time) (itemdomain.Item, error)

	// CreateOfficialItem enrolls an item handed directly to the PA. The item
	// starts in in_custody with an accepted institution custody row and no
	// 7-day deadline.
	CreateOfficialItem(ctx context.Context, input CreateOfficialItemInput) (itemdomain.Item, error)

	// ReviewClaim records the institution decision through the claim domain.
	ReviewClaim(ctx context.Context, claimID, decision, actorID string) (claimdomain.Claim, error)

	// OfferUnclaimed offers an item to its legal finder after one year from
	// the start of the legal term.
	OfferUnclaimed(ctx context.Context, itemID string, now time.Time) (itemdomain.Item, error)

	// CollectByLegalFinder records that the legal finder collected an item
	// that had been offered to them.
	CollectByLegalFinder(ctx context.Context, itemID, actorID string) (itemdomain.Item, error)

	// Dispose records the disposal of an offered item that the legal finder
	// did not collect.
	Dispose(ctx context.Context, itemID, actorID string) (itemdomain.Item, error)
}

// New returns an institution Service backed by db. The optional Deps argument
// supplies collaborators; omitted fields fall back to the default service on
// the same database.
func New(db repository.Database, options ...Deps) Service {
	deps := Deps{}
	if len(options) > 0 {
		deps = options[0]
	}
	if deps.Items == nil {
		deps.Items = itemdomain.New(db, itemdomain.Deps{})
	}
	if deps.Custody == nil {
		deps.Custody = custodydomain.New(db)
	}
	if deps.Claim == nil {
		deps.Claim = claimdomain.New(db)
	}
	if deps.Pass == nil {
		deps.Pass = passdomain.New(db, passdomain.Deps{})
	}
	if deps.Search == nil {
		deps.Search = search.New(db, search.Deps{})
	}
	if deps.Fee == nil {
		deps.Fee = feedomain.New(db)
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}
	return &service{db: db, deps: deps}
}
