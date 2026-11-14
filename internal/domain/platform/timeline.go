package platform

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// CustodyEvent is the safe, public view of one item timeline entry. It exposes
// the event type, the actor (role or operational alias), the timestamp, the
// notes and an evidence reference. Private profile values, document contents
// and exact coordinates never enter this type.
type CustodyEvent struct {
	ID          string
	ItemID      string
	Type        string
	Actor       string
	ActorRole   string
	Timestamp   time.Time
	Notes       string
	EvidenceRef string
}

// Timeline returns the item's custody events in chronological order.
func (s *service) Timeline(ctx context.Context, itemID string) ([]CustodyEvent, error) {
	itemID = trimmed(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("timeline: item id is required: %w", errs.ErrInvalidInput)
	}
	if _, err := s.db.GetItemByID(ctx, itemID); err != nil {
		return nil, fmt.Errorf("timeline %q: %w", itemID, err)
	}
	return s.timeline(ctx, itemID)
}

// TimelineFor applies the timeline authorization rules. Finder and
// intermediary callers must be connected to the item; institution and admin
// operators may read any timeline; searchers may not read one.
func (s *service) TimelineFor(ctx context.Context, itemID string, actor Actor) ([]CustodyEvent, error) {
	itemID = trimmed(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("timeline: item id is required: %w", errs.ErrInvalidInput)
	}
	item, err := s.db.GetItemByID(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("timeline %q: %w", itemID, err)
	}
	if err := s.authorizeTimeline(ctx, item, actor); err != nil {
		return nil, fmt.Errorf("timeline %q: %w", itemID, err)
	}
	return s.timeline(ctx, itemID)
}

func (s *service) timeline(ctx context.Context, itemID string) ([]CustodyEvent, error) {
	rows, err := s.db.ListCustodyEvents(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("timeline %q: %w", itemID, err)
	}

	events := make([]CustodyEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, s.eventFromRecord(ctx, row))
	}
	return events, nil
}

// authorizeTimeline enforces the role and ownership rules.
func (s *service) authorizeTimeline(ctx context.Context, item record.ItemRecord, actor Actor) error {
	actorID := trimmed(actor.ID)
	switch actor.Role {
	case RoleInstitution, RoleAdmin:
		return nil
	case RoleIntermediary:
		if actorID != "" && (actorID == item.CustodianID || actorID == item.LegalFinderID) {
			return nil
		}
	case RoleFinder:
		if actorID == "" {
			return errs.ErrForbidden
		}
		if actorID == item.LegalFinderID {
			return nil
		}
		// The finder who enrolled the item is recorded as the actor of the
		// rights-assignment event rather than on the item row.
		rows, err := s.db.ListCustodyEvents(ctx, item.ID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.ActorID == actorID {
				return nil
			}
		}
	}
	return errs.ErrForbidden
}

// eventFromRecord maps one row and resolves the actor to a role when possible.
// User actors are reported by role to avoid exposing an account id; operational
// aliases ("system", a civic-point id that has no user account) stay as-is.
func (s *service) eventFromRecord(ctx context.Context, row record.CustodyEventRecord) CustodyEvent {
	event := CustodyEvent{
		ID:          row.ID,
		ItemID:      row.ItemID,
		Type:        row.Type,
		Actor:       row.ActorID,
		Timestamp:   row.Timestamp.UTC(),
		Notes:       row.Notes,
		EvidenceRef: row.EvidenceRef,
	}
	user, err := s.db.GetUserByID(ctx, row.ActorID)
	if err != nil {
		// A missing account is normal for "system" and civic-point actors.
		if !errors.Is(err, errs.ErrNotFound) {
			return event
		}
		if row.ActorID == "" {
			event.Actor = "system"
		}
		return event
	}
	event.ActorRole = user.Role
	switch user.Role {
	case RoleFinder, RoleSearcher:
		event.Actor = user.Role
	default:
		event.Actor = user.ID
	}
	return event
}
