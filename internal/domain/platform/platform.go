/*
Package platform provides the cross-cutting surfaces of the MVP: in-app
notifications, the append-only audit log and an item's custody timeline.

The package is transport-free. Other domains call the narrow Notifier and
Auditor ports after a successful, already-committed business operation; a
notification failure must never turn a successful handover into an error.
*/
package platform

import (
	"context"
	"strings"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

// Roles recognised by the timeline and role notifications. They mirror the
// users.role column without importing the auth package.
const (
	RoleFinder       = "finder"
	RoleSearcher     = "searcher"
	RoleIntermediary = "intermediary"
	RoleInstitution  = "institution"
	RoleAdmin        = "admin"
)

// Deps are the platform-service dependencies. Now is injectable for
// deterministic tests.
type Deps struct {
	Now func() time.Time
}

// Notifier is the narrow port used by the other domains to raise an in-app
// notification after a successful operation.
type Notifier interface {
	// Notify persists a notification for userID. It does not require the
	// recipient to be an existing account; callers that accept unknown
	// recipients should use NotifyAccount.
	Notify(ctx context.Context, userID, notificationType string, payload map[string]any) error

	// NotifyAccount persists a notification only when userID is a known
	// account. Unknown recipients are skipped silently ("no account, no CIE").
	NotifyAccount(ctx context.Context, userID, notificationType string, payload map[string]any) error

	// NotifyRole notifies every account with the given role. It is used for
	// the institution inbox (pickup due, sensitive intake, fee paid).
	NotifyRole(ctx context.Context, role, notificationType string, payload map[string]any) error
}

// Auditor is the narrow port used by the other domains to append an audit
// entry that is not already written inside their transaction.
type Auditor interface {
	Audit(ctx context.Context, actorID, action, entity, entityID string, metadata map[string]any) error
}

// Actor is the authenticated caller reading a timeline. The role strings mirror
// auth roles without importing the auth package.
type Actor struct {
	ID   string
	Role string
}

// Service defines the platform surface exposed to the HTTP layer and to the
// other domains.
type Service interface {
	Notifier
	Auditor

	// ListNotifications returns the user's newest notifications, capped at
	// NotificationLimit. Only the recipient's own rows are returned.
	ListNotifications(ctx context.Context, userID string) ([]Notification, error)

	// MarkRead marks a notification read for its recipient. Another user's
	// notification is ErrForbidden; an unknown id is ErrNotFound.
	MarkRead(ctx context.Context, notificationID, userID string) error

	// Timeline returns the item's custody events in chronological order with
	// safe fields only.
	Timeline(ctx context.Context, itemID string) ([]CustodyEvent, error)

	// TimelineFor applies the role/ownership rules before returning the
	// timeline. The HTTP layer uses it; Timeline stays a trusted read.
	TimelineFor(ctx context.Context, itemID string, actor Actor) ([]CustodyEvent, error)
}

type service struct {
	db   repository.Database
	deps Deps
}

// New returns a platform Service backed by db. Omitting Deps uses the system
// clock.
func New(db repository.Database, options ...Deps) Service {
	deps := Deps{}
	if len(options) > 0 {
		deps = options[0]
	}
	return &service{db: db, deps: deps}
}

func (s *service) now() time.Time {
	if s.deps.Now != nil {
		return s.deps.Now().UTC()
	}
	return time.Now().UTC()
}

// hookTimeout bounds a post-commit notification or audit insert. The request
// is already answered, so a slow side effect must not run forever; it also
// keeps the hook inside the server write budget.
const hookTimeout = 3 * time.Second

// detachedContext keeps request values (tracing, logging) but deliberately
// ignores request cancellation. Once a business transaction has committed, a
// client disconnect must not silently drop the notification or audit row.
func detachedContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), hookTimeout)
}

func trimmed(value string) string { return strings.TrimSpace(value) }
