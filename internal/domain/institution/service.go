package institution

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

const (
	// legalTermYears is the art. 929 window before an unclaimed item is
	// offered to the legal finder.
	legalTermYears = 1
	// institutionCustodyDays is the reference duration of the PA legal term;
	// no deadline is set on the institution custody row.
	institutionCustodyDays = 365
	// hoursPerDay converts the overdue duration to whole days.
	hoursPerDay = 24
	// publicCodeAttempts bounds retries on a public-code collision.
	publicCodeAttempts = 3
	// indexTimeout bounds the best-effort index refresh after an official
	// intake. It is detached from request cancellation so a client disconnect
	// does not interrupt a useful index write.
	indexTimeout = 2 * time.Second
	// auditActionPassesRevoked mirrors the pass domain's audit action.
	auditActionPassesRevoked = "pass.revoked"
)

// service implements Service.
type service struct {
	db   repository.Database
	deps Deps
}

func (s *service) now() time.Time {
	if s.deps.Now != nil {
		return s.deps.Now().UTC()
	}
	return time.Now().UTC()
}

// ListPickups groups the pickup-due items by holding intermediary and zone.
// Sensitive items and groups come first; an item with an elapsed deadline also
// reports how many whole days it is overdue.
func (s *service) ListPickups(ctx context.Context) ([]PickupGroup, error) {
	rows, err := s.db.ListInstitutionPickups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pickups: %w", err)
	}
	now := s.now()

	type groupKey struct {
		intermediaryID string
		zone           string
	}
	groups := make(map[groupKey]*PickupGroup)
	for _, row := range rows {
		dueAt, err := s.pickupDueAt(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("list pickups %q: %w", row.ID, err)
		}

		key := groupKey{intermediaryID: row.CustodianID, zone: row.FoundZone}
		group, ok := groups[key]
		if !ok {
			group = &PickupGroup{IntermediaryID: row.CustodianID, Zone: row.FoundZone}
			groups[key] = group
		}
		if row.Sensitive {
			group.Sensitive = true
		}
		group.Items = append(group.Items, PickupItem{
			ID:             row.ID,
			PublicCode:     row.PublicCode,
			Category:       row.Category,
			Brand:          row.Brand,
			Model:          row.Model,
			Color:          row.Color,
			Status:         row.Status,
			FoundZone:      row.FoundZone,
			IntermediaryID: row.CustodianID,
			Sensitive:      row.Sensitive,
			DueAt:          dueAt,
			DaysOverdue:    daysOverdue(dueAt, now),
		})
	}

	result := make([]PickupGroup, 0, len(groups))
	for _, group := range groups {
		sortPickupItems(group.Items)
		result = append(result, *group)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Sensitive != result[j].Sensitive {
			return result[i].Sensitive
		}
		if result[i].IntermediaryID != result[j].IntermediaryID {
			return result[i].IntermediaryID < result[j].IntermediaryID
		}
		return result[i].Zone < result[j].Zone
	})
	return result, nil
}

// pickupDueAt returns the deadline of the item's open custody. The pickup query
// only returns items whose deadline elapsed, so a missing row is unexpected but
// is not allowed to break the whole list.
func (s *service) pickupDueAt(ctx context.Context, itemID string) (*time.Time, error) {
	custodyRow, err := s.db.GetCustodyByItemID(ctx, itemID)
	if errors.Is(err, errs.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if custodyRow.ClosedAt != nil {
		return nil, nil
	}
	return custodyRow.DueAt, nil
}

// Transfer moves an item from the intermediary to the institution.
//
// The transaction closes the intermediary custody, inserts the institution
// custody, advances the item to transferred and revokes every open pass. The
// storage-level partial unique index guarantees that the old row is closed
// before the new one opens, and the pass revocation cannot be lost after the
// item has already moved.
func (s *service) Transfer(ctx context.Context, itemID, actorID string, now time.Time) (itemdomain.Item, error) {
	itemID = strings.TrimSpace(itemID)
	actorID = strings.TrimSpace(actorID)
	if itemID == "" || actorID == "" {
		return itemdomain.Item{}, fmt.Errorf("transfer item: item id and actor id are required: %w", errs.ErrInvalidInput)
	}
	now = now.UTC()

	eventID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("transfer item %q: %w", itemID, err)
	}
	custodyID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("transfer item %q: %w", itemID, err)
	}
	auditID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("transfer item %q: %w", itemID, err)
	}
	passAuditID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("transfer item %q: %w", itemID, err)
	}

	var transferred itemdomain.Item
	var previousCustodianID string
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := tx.GetItemByID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}
		fromStatus := row.Status
		if fromStatus != itemdomain.StatusPickupDue && fromStatus != itemdomain.StatusSensitivePickup {
			return fmt.Errorf("transfer item %q: item is %q, not pickup due: %w", itemID, fromStatus, errs.ErrConflict)
		}

		custodyRow, err := tx.GetCustodyByItemID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}
		if custodyRow.ClosedAt != nil {
			return fmt.Errorf("transfer item %q: no open custody: %w", itemID, errs.ErrConflict)
		}
		if custodyRow.CustodianType != itemdomain.CustodyModeIntermediary {
			return fmt.Errorf("transfer item %q: current custody is %q: %w", itemID, custodyRow.CustodianType, errs.ErrConflict)
		}
		previousCustodianID = custodyRow.CustodianID

		// The guarded transition is the race-proof gate: exactly one concurrent
		// transfer can move the item out of the pickup status.
		transitioned, err := tx.TransitionItemStatus(ctx, itemID, fromStatus, itemdomain.StatusTransferred)
		if err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}
		if !transitioned {
			return fmt.Errorf("transfer item %q: status changed concurrently: %w", itemID, errs.ErrConflict)
		}

		if err := tx.UpdateCustodyFields(ctx, custodyRow.ID, map[string]any{
			"closed_at": now,
		}); err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}

		if err := tx.CreateCustody(ctx, record.CustodyRecord{
			ID:                custodyID,
			ItemID:            itemID,
			CustodianType:     itemdomain.CustodyModeInstitution,
			CustodianID:       actorID,
			LegalFinderID:     custodyRow.LegalFinderID,
			RightsAssignedAt:  custodyRow.RightsAssignedAt,
			ApprovalAuthority: itemdomain.CustodyModeInstitution,
			MaxCustodyDays:    institutionCustodyDays,
			AcceptedAt:        &now,
		}); err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}

		if err := tx.UpdateItemFields(ctx, itemID, map[string]any{
			"custody_mode":     itemdomain.CustodyModeInstitution,
			"custodian_id":     actorID,
			"transferred_at":   now,
			"legal_term_start": now,
		}); err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}

		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemID,
			Type:      EventTransferred,
			ActorID:   actorID,
			Timestamp: now,
			Notes:     "transferred from the intermediary to the public administration",
		}); err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}

		metadata, err := auditMetadata(map[string]any{
			"from":        fromStatus,
			"to":          itemdomain.StatusTransferred,
			"institution": actorID,
		})
		if err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   actorID,
			Action:    AuditActionTransferred,
			Entity:    "item",
			EntityID:  itemID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}

		revoked, err := tx.RevokePassesByItem(ctx, itemID, "")
		if err != nil {
			return fmt.Errorf("transfer item %q: revoking passes: %w", itemID, err)
		}
		if revoked > 0 {
			passMetadata, err := auditMetadata(map[string]any{
				"itemId":  itemID,
				"revoked": revoked,
			})
			if err != nil {
				return fmt.Errorf("transfer item %q: %w", itemID, err)
			}
			if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
				ID:        passAuditID,
				ActorID:   actorID,
				Action:    auditActionPassesRevoked,
				Entity:    "item",
				EntityID:  itemID,
				Metadata:  passMetadata,
				Timestamp: now,
			}); err != nil {
				return fmt.Errorf("transfer item %q: %w", itemID, err)
			}
		}

		row.Status = itemdomain.StatusTransferred
		row.CustodyMode = itemdomain.CustodyModeInstitution
		row.CustodianID = actorID
		row.TransferredAt = &now
		row.LegalTermStart = &now
		row.UpdatedAt = now
		transferred, err = itemdomain.FromRecord(row)
		if err != nil {
			return fmt.Errorf("transfer item %q: %w", itemID, err)
		}
		return nil
	})
	if err != nil {
		return itemdomain.Item{}, err
	}

	// The transfer is committed; the intermediary notification is
	// best-effort.
	s.notifyTransfer(ctx, transferred, previousCustodianID)

	// Indexing is best-effort: the transfer is already committed and a Qdrant
	// outage must not turn a successful handover into an HTTP error.
	if s.deps.Search != nil {
		_ = s.deps.Search.IndexItem(ctx, transferred)
	}
	return transferred, nil
}

// notifyTransfer tells the intermediary that its item moved to the PA.
func (s *service) notifyTransfer(ctx context.Context, item itemdomain.Item, previousCustodianID string) {
	if s.deps.Notifier == nil {
		return
	}
	payload := map[string]any{
		"itemId":        item.ID,
		"status":        item.Status,
		"institutionId": item.CustodianID,
	}
	platform.TryNotifyAccount(ctx, s.deps.Notifier, "item.transferred", previousCustodianID,
		platform.NotificationTransfer, payload)
}

// CreateOfficialItem enrolls an item that was handed directly to the PA. It
// reuses the item domain's catalog/risk path (and therefore D3 when configured)
// and writes the item, its accepted institution custody, the event and the
// audit entry in one transaction. The item starts in in_custody with no 7-day
// deadline and with the legal term already running, so it can later follow the
// unclaimed offer/collect/dispose flow.
func (s *service) CreateOfficialItem(ctx context.Context, input CreateOfficialItemInput) (itemdomain.Item, error) {
	input = normalizeOfficialInput(input)
	if input.InstitutionID == "" {
		return itemdomain.Item{}, fmt.Errorf("create official item: institution id is required: %w", errs.ErrInvalidInput)
	}
	if input.ActorID == "" {
		input.ActorID = input.InstitutionID
	}

	acceptedAt := s.now()
	foundAt := input.FoundAt
	if foundAt.IsZero() {
		foundAt = acceptedAt
	}
	foundAt = foundAt.UTC()

	// Reuse the item service catalog so D3, the deterministic risk policy and
	// the document detection behave exactly like the finder/intermediary path.
	catalogResult, err := s.deps.Items.Catalog(ctx, catalogImages(input.Photos), itemdomain.CatalogForm{
		Category:  input.Category,
		Color:     input.Color,
		Condition: input.Condition,
	})
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("create official item catalog: %w", err)
	}
	if input.Category == "" {
		input.Category = catalogResult.Category
	}
	if input.Color == "" {
		input.Color = catalogResult.Color
	}
	if input.Condition == "" {
		input.Condition = catalogResult.Condition
	}
	if input.PrivateProfile == nil {
		input.PrivateProfile = map[string]any{}
	}
	if catalogResult.DocumentType != "" {
		input.PrivateProfile["document_type"] = catalogResult.DocumentType
	}
	if catalogResult.Source != "" {
		input.PrivateProfile["catalog_source"] = catalogResult.Source
	}
	if input.Category == "" {
		input.Category = "other"
	}

	riskTier, sensitive := itemdomain.RiskForCategory(input.Category)
	if catalogResult.IsDocument || catalogResult.DocumentType != "" {
		riskTier = itemdomain.RiskSensitive
		sensitive = true
	}

	privateProfile, err := marshalOfficialProfile(input.PrivateProfile)
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("create official item: %w", err)
	}
	photos, err := marshalOfficialPhotos(input.Photos)
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("create official item: %w", err)
	}

	var accepted itemdomain.Item
	var lastErr error
	for attempt := 0; attempt < publicCodeAttempts; attempt++ {
		publicCode, err := newPublicCode()
		if err != nil {
			return itemdomain.Item{}, fmt.Errorf("create official item: %w", err)
		}
		itemID, err := newID()
		if err != nil {
			return itemdomain.Item{}, fmt.Errorf("create official item: %w", err)
		}
		custodyID, err := newID()
		if err != nil {
			return itemdomain.Item{}, fmt.Errorf("create official item: %w", err)
		}
		eventID, err := newID()
		if err != nil {
			return itemdomain.Item{}, fmt.Errorf("create official item: %w", err)
		}
		auditID, err := newID()
		if err != nil {
			return itemdomain.Item{}, fmt.Errorf("create official item: %w", err)
		}

		row := record.ItemRecord{
			ID:               itemID,
			PublicCode:       publicCode,
			Source:           itemdomain.SourceOperator,
			CustodyMode:      itemdomain.CustodyModeInstitution,
			Status:           itemdomain.StatusInCustody,
			LegalFinderID:    input.InstitutionID,
			RightsAssignedAt: &acceptedAt,
			Category:         input.Category,
			Brand:            input.Brand,
			Model:            input.Model,
			Color:            input.Color,
			Condition:        input.Condition,
			PrivateProfile:   privateProfile,
			Photos:           photos,
			FoundAt:          foundAt,
			FoundLat:         input.FoundLat,
			FoundLng:         input.FoundLng,
			FoundZone:        input.FoundZone,
			FoundNotes:       input.FoundNotes,
			CustodianID:      input.InstitutionID,
			RiskTier:         riskTier,
			Sensitive:        sensitive,
			LegalTermStart:   &acceptedAt,
			CreatedAt:        acceptedAt,
			UpdatedAt:        acceptedAt,
		}
		custodyRow := record.CustodyRecord{
			ID:                custodyID,
			ItemID:            itemID,
			CustodianType:     itemdomain.CustodyModeInstitution,
			CustodianID:       input.InstitutionID,
			LegalFinderID:     input.InstitutionID,
			RightsAssignedAt:  &acceptedAt,
			ApprovalAuthority: itemdomain.CustodyModeInstitution,
			MaxCustodyDays:    institutionCustodyDays,
			AcceptedAt:        &acceptedAt,
		}
		event := record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemID,
			Type:      EventAccepted,
			ActorID:   input.ActorID,
			Timestamp: acceptedAt,
			Notes:     "official intake; institution custody accepted",
		}
		metadata, err := auditMetadata(map[string]any{
			"source":           itemdomain.SourceOperator,
			"custodyMode":      itemdomain.CustodyModeInstitution,
			"institution":      input.InstitutionID,
			"legalTermStartAt": acceptedAt.Format(time.RFC3339),
		})
		if err != nil {
			return itemdomain.Item{}, fmt.Errorf("create official item: %w", err)
		}
		audit := record.AuditLogRecord{
			ID:        auditID,
			ActorID:   input.ActorID,
			Action:    AuditActionOfficialIntake,
			Entity:    "item",
			EntityID:  itemID,
			Metadata:  metadata,
			Timestamp: acceptedAt,
		}

		err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
			if err := tx.CreateItem(ctx, row); err != nil {
				return fmt.Errorf("create official item %q: %w", itemID, err)
			}
			if err := tx.CreateCustody(ctx, custodyRow); err != nil {
				return fmt.Errorf("create official item %q: %w", itemID, err)
			}
			if err := tx.CreateCustodyEvent(ctx, event); err != nil {
				return fmt.Errorf("create official item %q: %w", itemID, err)
			}
			return tx.CreateAuditLog(ctx, audit)
		})
		if err != nil {
			lastErr = err
			if !errors.Is(err, errs.ErrConflict) {
				return itemdomain.Item{}, err
			}
			continue
		}

		accepted, err = itemdomain.FromRecord(row)
		if err != nil {
			return itemdomain.Item{}, fmt.Errorf("create official item %q: %w", itemID, err)
		}
		break
	}
	if accepted.ID == "" {
		return itemdomain.Item{}, fmt.Errorf("create official item after %d attempts: %w", publicCodeAttempts, lastErr)
	}

	// The sensitive item is committed; the institution inbox notification is
	// best-effort.
	s.notifySensitiveIntake(ctx, accepted)

	// The item is committed; indexing is best effort and bounded so a vector
	// outage can never fail the intake.
	if s.deps.Search != nil {
		indexCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), indexTimeout)
		defer cancel()
		_ = s.deps.Search.IndexItem(indexCtx, accepted)
	}
	return accepted, nil
}

// notifySensitiveIntake raises the urgent inbox entry for documents, keys and
// other sensitive items received by the PA.
func (s *service) notifySensitiveIntake(ctx context.Context, item itemdomain.Item) {
	if s.deps.Notifier == nil || !item.Sensitive {
		return
	}
	payload := map[string]any{
		"itemId":    item.ID,
		"status":    item.Status,
		"sensitive": true,
	}
	platform.TryNotifyRole(ctx, s.deps.Notifier, "item.sensitive_intake", platform.RoleInstitution,
		platform.NotificationSensitive, payload)
}

// ReviewClaim delegates the policy and the persistence to the claim domain;
// the handler middleware already restricts the route to institution/admin.
func (s *service) ReviewClaim(ctx context.Context, claimID, decision, actorID string) (claimdomain.Claim, error) {
	reviewed, err := s.deps.Claim.ReviewClaim(ctx, claimID, decision, actorID)
	if err != nil {
		return claimdomain.Claim{}, fmt.Errorf("review claim %q: %w", claimID, err)
	}
	return reviewed, nil
}

// OfferUnclaimed offers an item to its legal finder once one year has elapsed
// since legal_term_start. An earlier request is an invalid input rather than a
// silent no-op, so the operator gets a clear answer.
func (s *service) OfferUnclaimed(ctx context.Context, itemID string, now time.Time) (itemdomain.Item, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return itemdomain.Item{}, fmt.Errorf("offer unclaimed: item id is required: %w", errs.ErrInvalidInput)
	}
	now = now.UTC()

	eventID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("offer unclaimed %q: %w", itemID, err)
	}
	auditID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("offer unclaimed %q: %w", itemID, err)
	}

	var offered itemdomain.Item
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := tx.GetItemByID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("offer unclaimed %q: %w", itemID, err)
		}
		switch row.Status {
		case itemdomain.StatusTransferred, itemdomain.StatusUnclaimed:
		case itemdomain.StatusInCustody:
			// An officially intaken item starts in institution custody with the
			// legal term already running; it follows the same one-year rule.
			if row.CustodyMode != itemdomain.CustodyModeInstitution {
				return fmt.Errorf("offer unclaimed %q: item is %q: %w", itemID, row.Status, errs.ErrConflict)
			}
		default:
			return fmt.Errorf("offer unclaimed %q: item is %q: %w", itemID, row.Status, errs.ErrConflict)
		}
		if row.LegalTermStart == nil {
			return fmt.Errorf("offer unclaimed %q: legal term has not started: %w", itemID, errs.ErrConflict)
		}
		offerAt := row.LegalTermStart.AddDate(legalTermYears, 0, 0)
		if now.Before(offerAt) {
			return fmt.Errorf("offer unclaimed %q: legal term ends at %s: %w", itemID, offerAt.Format(time.RFC3339), errs.ErrInvalidInput)
		}

		transitioned, err := tx.TransitionItemStatus(ctx, itemID, row.Status, itemdomain.StatusOfferedToFinder)
		if err != nil {
			return fmt.Errorf("offer unclaimed %q: %w", itemID, err)
		}
		if !transitioned {
			return fmt.Errorf("offer unclaimed %q: status changed concurrently: %w", itemID, errs.ErrConflict)
		}
		if err := tx.UpdateItemFields(ctx, itemID, map[string]any{
			"offered_to_finder_at": now,
		}); err != nil {
			return fmt.Errorf("offer unclaimed %q: %w", itemID, err)
		}

		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemID,
			Type:      EventOfferedToFinder,
			ActorID:   "system",
			Timestamp: now,
			Notes:     "legal term elapsed; item offered to the legal finder",
		}); err != nil {
			return fmt.Errorf("offer unclaimed %q: %w", itemID, err)
		}
		metadata, err := auditMetadata(map[string]any{
			"from": row.Status,
			"to":   itemdomain.StatusOfferedToFinder,
		})
		if err != nil {
			return fmt.Errorf("offer unclaimed %q: %w", itemID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   "system",
			Action:    AuditActionOffered,
			Entity:    "item",
			EntityID:  itemID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("offer unclaimed %q: %w", itemID, err)
		}

		row.Status = itemdomain.StatusOfferedToFinder
		row.OfferedToFinderAt = &now
		row.UpdatedAt = now
		offered, err = itemdomain.FromRecord(row)
		if err != nil {
			return fmt.Errorf("offer unclaimed %q: %w", itemID, err)
		}
		return nil
	})
	if err != nil {
		return itemdomain.Item{}, err
	}
	return offered, nil
}

// CollectByLegalFinder records that the legal finder collected the item after
// it had been offered to them.
func (s *service) CollectByLegalFinder(ctx context.Context, itemID, actorID string) (itemdomain.Item, error) {
	itemID = strings.TrimSpace(itemID)
	actorID = strings.TrimSpace(actorID)
	if itemID == "" || actorID == "" {
		return itemdomain.Item{}, fmt.Errorf("collect unclaimed: item id and actor id are required: %w", errs.ErrInvalidInput)
	}
	now := s.now()

	eventID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("collect unclaimed %q: %w", itemID, err)
	}
	auditID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("collect unclaimed %q: %w", itemID, err)
	}

	var collected itemdomain.Item
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := tx.GetItemByID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("collect unclaimed %q: %w", itemID, err)
		}
		if row.Status != itemdomain.StatusOfferedToFinder {
			return fmt.Errorf("collect unclaimed %q: item is %q: %w", itemID, row.Status, errs.ErrConflict)
		}

		transitioned, err := tx.TransitionItemStatus(ctx, itemID, itemdomain.StatusOfferedToFinder, itemdomain.StatusFinderCollected)
		if err != nil {
			return fmt.Errorf("collect unclaimed %q: %w", itemID, err)
		}
		if !transitioned {
			return fmt.Errorf("collect unclaimed %q: status changed concurrently: %w", itemID, errs.ErrConflict)
		}

		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemID,
			Type:      EventFinderCollected,
			ActorID:   actorID,
			Timestamp: now,
			Notes:     "unclaimed item collected by the legal finder",
		}); err != nil {
			return fmt.Errorf("collect unclaimed %q: %w", itemID, err)
		}
		metadata, err := auditMetadata(map[string]any{
			"from": itemdomain.StatusOfferedToFinder,
			"to":   itemdomain.StatusFinderCollected,
		})
		if err != nil {
			return fmt.Errorf("collect unclaimed %q: %w", itemID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   actorID,
			Action:    AuditActionCollected,
			Entity:    "item",
			EntityID:  itemID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("collect unclaimed %q: %w", itemID, err)
		}

		row.Status = itemdomain.StatusFinderCollected
		row.UpdatedAt = now
		collected, err = itemdomain.FromRecord(row)
		if err != nil {
			return fmt.Errorf("collect unclaimed %q: %w", itemID, err)
		}
		return nil
	})
	if err != nil {
		return itemdomain.Item{}, err
	}
	return collected, nil
}

// Dispose records the disposal of an offered item. Disposal before the offer
// is rejected: the legal finder has the first right to the item.
func (s *service) Dispose(ctx context.Context, itemID, actorID string) (itemdomain.Item, error) {
	itemID = strings.TrimSpace(itemID)
	actorID = strings.TrimSpace(actorID)
	if itemID == "" || actorID == "" {
		return itemdomain.Item{}, fmt.Errorf("dispose item: item id and actor id are required: %w", errs.ErrInvalidInput)
	}
	now := s.now()

	eventID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("dispose item %q: %w", itemID, err)
	}
	auditID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("dispose item %q: %w", itemID, err)
	}

	var disposed itemdomain.Item
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := tx.GetItemByID(ctx, itemID)
		if err != nil {
			return fmt.Errorf("dispose item %q: %w", itemID, err)
		}
		if row.Status != itemdomain.StatusOfferedToFinder {
			return fmt.Errorf("dispose item %q: item is %q, not offered: %w", itemID, row.Status, errs.ErrConflict)
		}

		transitioned, err := tx.TransitionItemStatus(ctx, itemID, itemdomain.StatusOfferedToFinder, itemdomain.StatusDisposed)
		if err != nil {
			return fmt.Errorf("dispose item %q: %w", itemID, err)
		}
		if !transitioned {
			return fmt.Errorf("dispose item %q: status changed concurrently: %w", itemID, errs.ErrConflict)
		}
		if err := tx.UpdateItemFields(ctx, itemID, map[string]any{
			"disposed_at": now,
		}); err != nil {
			return fmt.Errorf("dispose item %q: %w", itemID, err)
		}

		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemID,
			Type:      EventDisposed,
			ActorID:   actorID,
			Timestamp: now,
			Notes:     "offered item not collected; disposed",
		}); err != nil {
			return fmt.Errorf("dispose item %q: %w", itemID, err)
		}
		metadata, err := auditMetadata(map[string]any{
			"from": itemdomain.StatusOfferedToFinder,
			"to":   itemdomain.StatusDisposed,
		})
		if err != nil {
			return fmt.Errorf("dispose item %q: %w", itemID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   actorID,
			Action:    AuditActionDisposed,
			Entity:    "item",
			EntityID:  itemID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("dispose item %q: %w", itemID, err)
		}

		row.Status = itemdomain.StatusDisposed
		row.DisposedAt = &now
		row.UpdatedAt = now
		disposed, err = itemdomain.FromRecord(row)
		if err != nil {
			return fmt.Errorf("dispose item %q: %w", itemID, err)
		}
		return nil
	})
	if err != nil {
		return itemdomain.Item{}, err
	}
	return disposed, nil
}

// sortPickupItems orders sensitive items first, then the earliest deadline.
func sortPickupItems(items []PickupItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Sensitive != items[j].Sensitive {
			return items[i].Sensitive
		}
		switch {
		case items[i].DueAt == nil && items[j].DueAt != nil:
			return false
		case items[i].DueAt != nil && items[j].DueAt == nil:
			return true
		case items[i].DueAt != nil && items[j].DueAt != nil && !items[i].DueAt.Equal(*items[j].DueAt):
			return items[i].DueAt.Before(*items[j].DueAt)
		}
		return items[i].ID < items[j].ID
	})
}

// daysOverdue is the number of whole days elapsed after dueAt.
func daysOverdue(dueAt *time.Time, now time.Time) int {
	if dueAt == nil || !dueAt.Before(now) {
		return 0
	}
	return int(now.Sub(*dueAt).Hours() / hoursPerDay)
}

// normalizeOfficialInput trims user-supplied strings and drops empty photo
// entries before the catalog and the transaction.
func normalizeOfficialInput(input CreateOfficialItemInput) CreateOfficialItemInput {
	input.InstitutionID = strings.TrimSpace(input.InstitutionID)
	input.ActorID = strings.TrimSpace(input.ActorID)
	input.Category = strings.TrimSpace(input.Category)
	input.Brand = strings.TrimSpace(input.Brand)
	input.Model = strings.TrimSpace(input.Model)
	input.Color = strings.TrimSpace(input.Color)
	input.Condition = strings.TrimSpace(input.Condition)
	input.FoundZone = strings.TrimSpace(input.FoundZone)
	input.FoundNotes = strings.TrimSpace(input.FoundNotes)

	photos := make([]string, 0, len(input.Photos))
	for _, photo := range input.Photos {
		if trimmed := strings.TrimSpace(photo); trimmed != "" {
			photos = append(photos, trimmed)
		}
	}
	input.Photos = photos
	if input.PrivateProfile == nil {
		input.PrivateProfile = map[string]any{}
	}
	return input
}

// catalogImages turns stored relative paths into catalog inputs.
func catalogImages(paths []string) []itemdomain.ImageInput {
	images := make([]itemdomain.ImageInput, 0, len(paths))
	for _, path := range paths {
		images = append(images, itemdomain.ImageInput{Path: path})
	}
	return images
}

// marshalOfficialProfile serialises the private profile, defaulting to an empty
// JSON object so the NOT NULL column is always valid JSON.
func marshalOfficialProfile(profile map[string]any) (string, error) {
	if len(profile) == 0 {
		return "{}", nil
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		return "", fmt.Errorf("encoding private profile: %w", err)
	}
	return string(encoded), nil
}

// marshalOfficialPhotos serialises the upload paths, defaulting to an empty JSON
// array.
func marshalOfficialPhotos(photos []string) (string, error) {
	if len(photos) == 0 {
		return "[]", nil
	}
	encoded, err := json.Marshal(photos)
	if err != nil {
		return "", fmt.Errorf("encoding photos: %w", err)
	}
	return string(encoded), nil
}

// newPublicCode returns "LF-" plus 8 uppercase hex characters, matching the item
// domain's receipt handle.
func newPublicCode() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("LF-%X", buf), nil
}

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
