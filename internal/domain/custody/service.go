package custody

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

const (
	// defaultMaxDays is the legal custody window before the item moves to the
	// pickup-due flow.
	defaultMaxDays = 7
	// defaultSensitiveHours is the shorter window for documents, keys, SIMs...
	defaultSensitiveHours = 24
	// hoursPerDay converts the configured day window into hours.
	hoursPerDay = 24
)

// service implements Service.
type service struct {
	db   repository.Database
	deps Deps
}

// New returns a custody Service backed by db. The optional Deps argument
// carries the policy configuration; omitting it (or leaving its fields zero)
// uses the legal defaults of 7 days / 24 hours.
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

func (s *service) maxDays() int {
	if s.deps.MaxDays > 0 {
		return s.deps.MaxDays
	}
	return defaultMaxDays
}

func (s *service) sensitiveHours() int {
	if s.deps.SensitiveHours > 0 {
		return s.deps.SensitiveHours
	}
	return defaultSensitiveHours
}

// Accept acknowledges the handover for the legal finder. The item status, the
// custody deadline, the event and the audit entry are committed together.
func (s *service) Accept(ctx context.Context, itemID, actorID string, now time.Time) (Custody, error) {
	itemID = strings.TrimSpace(itemID)
	actorID = strings.TrimSpace(actorID)
	if itemID == "" || actorID == "" {
		return Custody{}, fmt.Errorf("accept custody: item id and actor id are required: %w", errs.ErrInvalidInput)
	}
	now = now.UTC()

	eventID, err := newID()
	if err != nil {
		return Custody{}, fmt.Errorf("accept custody %q: %w", itemID, err)
	}
	auditID, err := newID()
	if err != nil {
		return Custody{}, fmt.Errorf("accept custody %q: %w", itemID, err)
	}

	var accepted Custody
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := tx.GetItemByID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("accept custody %q: %w", itemID, err)
		}
		if err := checkPending(&row, actorID); err != nil {
			return fmt.Errorf("accept custody %q: %w", itemID, err)
		}

		custodyRow, err := tx.GetCustodyByItemID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("accept custody %q: %w", itemID, err)
		}
		if custodyRow.ClosedAt != nil {
			return fmt.Errorf("accept custody %q: no open custody: %w", itemID, errs.ErrConflict)
		}

		maxDays := s.maxDays()
		dueAt := now.Add(time.Duration(maxDays) * hoursPerDay * time.Hour)
		if row.Sensitive {
			hours := s.sensitiveHours()
			maxDays = (hours + hoursPerDay - 1) / hoursPerDay
			dueAt = now.Add(time.Duration(hours) * time.Hour)
		}

		// The guarded update is the race-proof gate: only one concurrent
		// accept can move the item out of pending_custody. The pre-read above
		// only exists for a clearer error message.
		transitioned, err := tx.TransitionItemStatus(
			ctx, itemID, itemdomain.StatusPendingCustody, itemdomain.StatusInCustody,
		)
		if err != nil {
			return fmt.Errorf("accept custody %q: %w", itemID, err)
		}
		if !transitioned {
			return fmt.Errorf("accept custody %q: status changed concurrently: %w", itemID, errs.ErrConflict)
		}

		if err := tx.UpdateCustodyFields(ctx, custodyRow.ID, map[string]any{
			"accepted_at":      now,
			"due_at":           dueAt,
			"max_custody_days": maxDays,
		}); err != nil {
			return fmt.Errorf("accept custody %q: %w", itemID, err)
		}

		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemID,
			Type:      EventAccepted,
			ActorID:   actorID,
			Timestamp: now,
			Notes:     "custody accepted; due at " + dueAt.Format(time.RFC3339),
		}); err != nil {
			return fmt.Errorf("accept custody %q: %w", itemID, err)
		}

		metadata, err := auditMetadata(map[string]any{
			"status":         itemdomain.StatusInCustody,
			"dueAt":          dueAt.Format(time.RFC3339),
			"maxCustodyDays": maxDays,
			"sensitive":      row.Sensitive,
		})
		if err != nil {
			return fmt.Errorf("accept custody %q: %w", itemID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   actorID,
			Action:    auditActionAccepted,
			Entity:    "item",
			EntityID:  itemID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("accept custody %q: %w", itemID, err)
		}

		custodyRow.AcceptedAt = &now
		custodyRow.DueAt = &dueAt
		custodyRow.MaxCustodyDays = maxDays
		accepted = custodyFromRecord(custodyRow)
		return nil
	})
	if err != nil {
		return Custody{}, err
	}

	// Best-effort notifications after the custody transition is committed.
	s.notifyCustodyAccepted(ctx, accepted.ItemID, accepted.DueAt)

	return accepted, nil
}

// notifyCustodyAccepted tells the intermediary and the finder that the
// handover was acknowledged. The finder is resolved from the rights-assignment
// event; a finder without an account is skipped by the platform notifier.
func (s *service) notifyCustodyAccepted(ctx context.Context, itemID string, dueAt *time.Time) {
	if s.deps.Notifier == nil {
		return
	}
	row, err := s.db.GetItemByID(ctx, itemID)
	if err != nil {
		slog.Default().Warn("custody accepted notification skipped",
			slog.String("item", itemID), slog.Any("err", err))
		return
	}

	payload := map[string]any{
		"itemId": itemID,
		"status": itemdomain.StatusInCustody,
	}
	if dueAt != nil {
		payload["dueAt"] = dueAt.UTC().Format(time.RFC3339)
	}

	seen := map[string]bool{}
	for _, recipient := range []string{row.CustodianID, row.LegalFinderID} {
		if recipient == "" || seen[recipient] {
			continue
		}
		seen[recipient] = true
		platform.TryNotifyAccount(ctx, s.deps.Notifier, "custody.accepted", recipient,
			platform.NotificationCustodyAccepted, payload)
	}

	events, err := s.db.ListCustodyEvents(ctx, itemID)
	if err != nil {
		return
	}
	for _, event := range events {
		if event.Type != itemdomain.EventRightsAssigned || seen[event.ActorID] || event.ActorID == "" {
			continue
		}
		seen[event.ActorID] = true
		platform.TryNotifyAccount(ctx, s.deps.Notifier, "custody.accepted", event.ActorID,
			platform.NotificationCustodyAccepted, payload)
		break
	}
}

// notifyPickupDue tells the holding intermediary and the institution inbox that
// an item reached its pickup deadline.
func (s *service) notifyPickupDue(ctx context.Context, row record.ItemRecord) {
	if s.deps.Notifier == nil {
		return
	}
	payload := map[string]any{
		"itemId":    row.ID,
		"status":    row.Status,
		"sensitive": row.Sensitive,
	}
	seen := map[string]bool{}
	for _, recipient := range []string{row.CustodianID, row.LegalFinderID} {
		if recipient == "" || seen[recipient] {
			continue
		}
		seen[recipient] = true
		platform.TryNotifyAccount(ctx, s.deps.Notifier, "custody.pickup_due", recipient,
			platform.NotificationPickupDue, payload)
	}
	platform.TryNotifyRole(ctx, s.deps.Notifier, "custody.pickup_due", platform.RoleInstitution,
		platform.NotificationPickupDue, payload)
}

// Refuse rejects the handover for the legal finder and closes the custody row.
func (s *service) Refuse(ctx context.Context, itemID, actorID, reason string) (itemdomain.Item, error) {
	itemID = strings.TrimSpace(itemID)
	actorID = strings.TrimSpace(actorID)
	reason = strings.TrimSpace(reason)
	if itemID == "" || actorID == "" {
		return itemdomain.Item{}, fmt.Errorf("refuse custody: item id and actor id are required: %w", errs.ErrInvalidInput)
	}
	if reason == "" {
		return itemdomain.Item{}, fmt.Errorf("refuse custody: a reason is required: %w", errs.ErrInvalidInput)
	}

	now := s.now()
	eventID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("refuse custody %q: %w", itemID, err)
	}
	auditID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("refuse custody %q: %w", itemID, err)
	}
	metadata, err := auditMetadata(map[string]any{
		"status": itemdomain.StatusCustodyRejected,
		"reason": reason,
	})
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("refuse custody %q: %w", itemID, err)
	}

	var refused itemdomain.Item
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := tx.GetItemByID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("refuse custody %q: %w", itemID, err)
		}
		if err := checkPending(&row, actorID); err != nil {
			return fmt.Errorf("refuse custody %q: %w", itemID, err)
		}

		custodyRow, err := tx.GetCustodyByItemID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("refuse custody %q: %w", itemID, err)
		}
		if custodyRow.ClosedAt != nil {
			return fmt.Errorf("refuse custody %q: no open custody: %w", itemID, errs.ErrConflict)
		}

		// Guarded transition: a concurrent accept/refuse that already moved the
		// item leaves this UPDATE without a row and yields a clean conflict.
		transitioned, err := tx.TransitionItemStatus(
			ctx, itemID, itemdomain.StatusPendingCustody, itemdomain.StatusCustodyRejected,
		)
		if err != nil {
			return fmt.Errorf("refuse custody %q: %w", itemID, err)
		}
		if !transitioned {
			return fmt.Errorf("refuse custody %q: status changed concurrently: %w", itemID, errs.ErrConflict)
		}
		if err := tx.UpdateCustodyFields(ctx, custodyRow.ID, map[string]any{
			"closed_at": now,
		}); err != nil {
			return fmt.Errorf("refuse custody %q: %w", itemID, err)
		}

		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemID,
			Type:      EventCustodyRejected,
			ActorID:   actorID,
			Timestamp: now,
			Notes:     reason,
		}); err != nil {
			return fmt.Errorf("refuse custody %q: %w", itemID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   actorID,
			Action:    auditActionRefused,
			Entity:    "item",
			EntityID:  itemID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("refuse custody %q: %w", itemID, err)
		}

		row.Status = itemdomain.StatusCustodyRejected
		row.UpdatedAt = now
		refused, err = itemdomain.FromRecord(row)
		if err != nil {
			return fmt.Errorf("refuse custody %q: %w", itemID, err)
		}
		return nil
	})
	if err != nil {
		return itemdomain.Item{}, err
	}
	return refused, nil
}

// MarkPickupDue transitions every in_custody item whose deadline has elapsed.
// Each item is updated in its own transaction so one failure does not leave a
// half-written item behind.
func (s *service) MarkPickupDue(ctx context.Context, now time.Time) ([]itemdomain.Item, error) {
	now = now.UTC()

	rows, err := s.db.ListPickupDue(ctx, now)
	if err != nil {
		return nil, fmt.Errorf("mark pickup due: %w", err)
	}

	due := make([]itemdomain.Item, 0, len(rows))
	for _, row := range rows {
		// ListPickupDue already restricts the query, but re-checking here keeps
		// the transition idempotent: an item that is already pickup_due must
		// not collect a second event on the next tick.
		if row.Status != itemdomain.StatusInCustody {
			continue
		}

		status := itemdomain.StatusPickupDue
		notes := "custody deadline reached; pickup due"
		if row.Sensitive {
			status = itemdomain.StatusSensitivePickup
			notes = "sensitive custody deadline reached; pickup due"
		}

		transitioned, err := s.markOneDue(ctx, row, status, notes, now)
		if err != nil {
			return nil, fmt.Errorf("mark pickup due %q: %w", row.ID, err)
		}
		// Another concurrent tick already transitioned this item: it must not
		// collect a second event or appear twice in the response.
		if !transitioned {
			continue
		}

		row.Status = status
		row.UpdatedAt = now
		updated, err := itemdomain.FromRecord(row)
		if err != nil {
			return nil, fmt.Errorf("mark pickup due %q: %w", row.ID, err)
		}
		due = append(due, updated)

		// Notifications are best-effort and run after the guarded transition.
		s.notifyPickupDue(ctx, row)
	}
	return due, nil
}

// markOneDue writes one pickup-due transition atomically. It reports false when
// a concurrent tick already moved the item away from in_custody, in which case
// no event or audit entry is written.
func (s *service) markOneDue(
	ctx context.Context,
	row record.ItemRecord,
	status, notes string,
	now time.Time,
) (bool, error) {
	eventID, err := newID()
	if err != nil {
		return false, err
	}
	auditID, err := newID()
	if err != nil {
		return false, err
	}
	metadata, err := auditMetadata(map[string]any{
		"status":    status,
		"sensitive": row.Sensitive,
	})
	if err != nil {
		return false, err
	}

	transitioned := false
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		ok, err := tx.TransitionItemStatus(
			ctx, row.ID, itemdomain.StatusInCustody, status,
		)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    row.ID,
			Type:      EventPickupScheduled,
			ActorID:   systemActor,
			Timestamp: now,
			Notes:     notes,
		}); err != nil {
			return err
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   systemActor,
			Action:    auditActionPickupScheduled,
			Entity:    "item",
			EntityID:  row.ID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return err
		}
		transitioned = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return transitioned, nil
}

// DaysLeft returns the whole days remaining before the item's open custody
// deadline. Unknown items have no deadline and yield 0.
func (s *service) DaysLeft(ctx context.Context, itemID string, now time.Time) (int, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return 0, fmt.Errorf("days left: item id is required: %w", errs.ErrInvalidInput)
	}

	custodyRow, err := s.db.GetCustodyByItemID(ctx, itemID)
	if errors.Is(err, errs.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("days left for %q: %w", itemID, err)
	}
	if custodyRow.ClosedAt != nil {
		return 0, nil
	}
	return DaysLeft(custodyRow.DueAt, now), nil
}

// checkPending enforces the preconditions shared by Accept and Refuse: the item
// must still be waiting for custody, rights must be assigned and the actor must
// be the legal finder.
func checkPending(row *record.ItemRecord, actorID string) error {
	if row.Status != itemdomain.StatusPendingCustody {
		return fmt.Errorf("item is %q, not pending custody: %w", row.Status, errs.ErrConflict)
	}
	if row.RightsAssignedAt == nil {
		return fmt.Errorf("rights are not assigned: %w", errs.ErrConflict)
	}
	if row.LegalFinderID != actorID {
		return fmt.Errorf("actor %q is not the legal finder: %w", actorID, errs.ErrForbidden)
	}
	return nil
}

// auditMetadata serialises the structured fields of an audit entry. The values
// are internal bookkeeping (statuses, deadlines), never private item data.
func auditMetadata(fields map[string]any) (string, error) {
	encoded, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("encoding audit metadata: %w", err)
	}
	return string(encoded), nil
}

func newID() (string, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
