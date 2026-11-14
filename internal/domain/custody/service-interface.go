package custody

import (
	"context"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
)

// Deps are the policy knobs of the custody service. Zero values fall back to
// the legal defaults: 7 days for ordinary items and 24 hours for sensitive ones.
type Deps struct {
	// MaxDays is the maximum custody window for ordinary items.
	MaxDays int
	// SensitiveHours is the custody window for documents, keys and SIMs.
	SensitiveHours int
	// Now is injectable for deterministic tests of Refuse; the other
	// transitions take the current time as an argument.
	Now func() time.Time
	// Notifier receives best-effort notifications after a transition commits.
	// Nil disables them.
	Notifier platform.Notifier
}

// Service defines the custody transitions exposed to the HTTP layer.
type Service interface {
	// Accept acknowledges the handover for the legal finder, moves the item to
	// in_custody and stores the deadline. It returns ErrConflict when the item
	// is not pending custody or has no open custody, and ErrForbidden when the
	// actor is not the legal finder.
	Accept(ctx context.Context, itemID, actorID string, now time.Time) (Custody, error)

	// Refuse rejects the handover for the legal finder: the item becomes
	// custody_rejected and its custody row is closed. A blank reason is
	// ErrInvalidInput.
	Refuse(ctx context.Context, itemID, actorID, reason string) (itemdomain.Item, error)

	// MarkPickupDue moves every in_custody item whose deadline is at or before
	// now to pickup_due, or to sensitive_pickup for sensitive items. It
	// returns the items that changed status.
	MarkPickupDue(ctx context.Context, now time.Time) ([]itemdomain.Item, error)

	// DaysLeft returns the whole days remaining before the item's open custody
	// deadline; 0 when the item has no open custody or no deadline.
	DaysLeft(ctx context.Context, itemID string, now time.Time) (int, error)
}
