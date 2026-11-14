package reward

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// Deps are the reward-service dependencies. Now is injectable for tests.
type Deps struct {
	Now func() time.Time
}

// Service defines the reward-notice operations.
type Service interface {
	// Request records (or updates) the item's reward notice. Only the item's
	// legal finder may request it; the amount is optional and is never
	// charged by the platform.
	Request(ctx context.Context, itemID, legalFinderID string, amountCents int64) (RewardNotice, error)

	// Get returns the latest notice without marking it as shown.
	Get(ctx context.Context, itemID string) (RewardNotice, error)

	// GetFor returns the notice to a viewer, marking it as shown when the
	// viewer is a searcher.
	GetFor(ctx context.Context, itemID string, viewer Viewer) (RewardNotice, error)
}

type service struct {
	db   repository.Database
	deps Deps
}

// New returns a reward Service backed by db.
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

// Request records the legal finder's reward request. It never creates a fee or
// a payment: the reward is settled outside the system.
func (s *service) Request(
	ctx context.Context,
	itemID, legalFinderID string,
	amountCents int64,
) (RewardNotice, error) {
	itemID = strings.TrimSpace(itemID)
	legalFinderID = strings.TrimSpace(legalFinderID)
	if itemID == "" || legalFinderID == "" {
		return RewardNotice{}, fmt.Errorf("request reward: item id and legal finder id are required: %w", errs.ErrInvalidInput)
	}
	if amountCents < 0 {
		return RewardNotice{}, fmt.Errorf("request reward: amount cannot be negative: %w", errs.ErrInvalidInput)
	}
	now := s.now()

	var notice RewardNotice
	err := s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		item, err := tx.GetItemByID(ctx, itemID)
		if err != nil {
			return err
		}
		if item.LegalFinderID != legalFinderID {
			return fmt.Errorf("request reward %q: actor is not the legal finder: %w", itemID, errs.ErrForbidden)
		}

		eventID, err := newID()
		if err != nil {
			return fmt.Errorf("request reward %q: %w", itemID, err)
		}
		auditID, err := newID()
		if err != nil {
			return fmt.Errorf("request reward %q: %w", itemID, err)
		}

		current, currentErr := tx.GetRewardNoticeByItemID(ctx, itemID)
		switch {
		case currentErr == nil:
			if err := tx.UpdateRewardNoticeFields(ctx, current.ID, map[string]any{
				"legal_finder_id": legalFinderID,
				"requested":       true,
				"amount_cents":    amountCents,
				"payment_status":  PaymentStatusOutsideSystem,
			}); err != nil {
				return fmt.Errorf("request reward %q: %w", itemID, err)
			}
			current.LegalFinderID = legalFinderID
			current.Requested = true
			current.AmountCents = amountCents
			current.PaymentStatus = PaymentStatusOutsideSystem
			notice = fromRecord(current)
		case errors.Is(currentErr, errs.ErrNotFound):
			id, err := newID()
			if err != nil {
				return fmt.Errorf("request reward %q: %w", itemID, err)
			}
			row := record.RewardNoticeRecord{
				ID:            id,
				ItemID:        itemID,
				LegalFinderID: legalFinderID,
				Requested:     true,
				AmountCents:   amountCents,
				PaymentStatus: PaymentStatusOutsideSystem,
			}
			if err := tx.CreateRewardNotice(ctx, row); err != nil {
				return fmt.Errorf("request reward %q: %w", itemID, err)
			}
			notice = fromRecord(row)
		default:
			return currentErr
		}

		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemID,
			Type:      EventRewardRequested,
			ActorID:   legalFinderID,
			Timestamp: now,
			Notes:     "reward requested; it is settled outside the system",
		}); err != nil {
			return fmt.Errorf("request reward %q: %w", itemID, err)
		}
		metadata, err := encodeMetadata(map[string]any{
			"itemId":         itemID,
			"rewardNoticeId": notice.ID,
			"legalFinderId":  legalFinderID,
			"amountCents":    amountCents,
			"paymentStatus":  PaymentStatusOutsideSystem,
		})
		if err != nil {
			return fmt.Errorf("request reward %q: %w", itemID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   legalFinderID,
			Action:    AuditActionRewardRequested,
			Entity:    auditEntityRewardNotice,
			EntityID:  notice.ID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("request reward %q: %w", itemID, err)
		}
		return nil
	})
	if err != nil {
		return RewardNotice{}, err
	}
	return notice, nil
}

// Get returns the latest reward notice for the item.
func (s *service) Get(ctx context.Context, itemID string) (RewardNotice, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return RewardNotice{}, fmt.Errorf("get reward: item id is required: %w", errs.ErrInvalidInput)
	}
	row, err := s.db.GetRewardNoticeByItemID(ctx, itemID)
	if err != nil {
		return RewardNotice{}, fmt.Errorf("get reward %q: %w", itemID, err)
	}
	return fromRecord(row), nil
}

// GetFor returns the notice to a viewer. When a searcher reads it before
// pickup, shown_to_searcher_at is recorded once and then preserved.
func (s *service) GetFor(ctx context.Context, itemID string, viewer Viewer) (RewardNotice, error) {
	notice, err := s.Get(ctx, itemID)
	if err != nil {
		return RewardNotice{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(viewer.Role), RoleSearcher) || notice.ShownToSearcherAt != nil {
		return notice, nil
	}

	now := s.now()
	if err := s.db.UpdateRewardNoticeFields(ctx, notice.ID, map[string]any{
		"shown_to_searcher_at": now,
	}); err != nil {
		return RewardNotice{}, fmt.Errorf("mark reward %q shown: %w", itemID, err)
	}
	notice.ShownToSearcherAt = &now
	return notice, nil
}

func fromRecord(row record.RewardNoticeRecord) RewardNotice {
	return RewardNotice{
		ID:                row.ID,
		ItemID:            row.ItemID,
		LegalFinderID:     row.LegalFinderID,
		Requested:         row.Requested,
		AmountCents:       row.AmountCents,
		ShownToSearcherAt: row.ShownToSearcherAt,
		PaymentStatus:     row.PaymentStatus,
	}
}

func encodeMetadata(fields map[string]any) (string, error) {
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
