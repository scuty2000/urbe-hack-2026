package pass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// claimStatusApproved mirrors claim.StatusApproved without importing the claim
// package (the dependency direction stays claim -> pass, never the reverse).
const claimStatusApproved = "approved"

// systemActor is used when a revocation has no authenticated actor (the
// institution transfer may run from an automated tick).
const systemActor = "system"

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

// Issue creates a signed, single-use pass for an approved claim. The item is
// re-read inside the transaction: the return channel and the custodian always
// come from the current item state, never from the caller or the token.
func (s *service) Issue(ctx context.Context, claimID, actorID string, now time.Time) (Pass, error) {
	if strings.TrimSpace(s.deps.PassSecret) == "" {
		return Pass{}, fmt.Errorf("issue pass: signing secret is not configured: %w", errs.ErrUnavailable)
	}
	claimID = strings.TrimSpace(claimID)
	actorID = strings.TrimSpace(actorID)
	if claimID == "" || actorID == "" {
		return Pass{}, fmt.Errorf("issue pass: claim id and actor id are required: %w", errs.ErrInvalidInput)
	}
	now = now.UTC()

	passID, err := newID()
	if err != nil {
		return Pass{}, fmt.Errorf("issue pass %q: %w", claimID, err)
	}
	eventID, err := newID()
	if err != nil {
		return Pass{}, fmt.Errorf("issue pass %q: %w", claimID, err)
	}
	auditID, err := newID()
	if err != nil {
		return Pass{}, fmt.Errorf("issue pass %q: %w", claimID, err)
	}

	var issued Pass
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		claim, err := tx.GetClaimByID(ctx, claimID)
		if err != nil {
			return fmt.Errorf("issue pass %q: %w", claimID, err)
		}
		// Ownership is checked before reading the item so a non-owner cannot
		// probe whether the claim's item exists.
		if claim.SearcherID != actorID {
			return fmt.Errorf("issue pass %q: actor is not the claim owner: %w", claimID, errs.ErrForbidden)
		}
		if claim.Status != claimStatusApproved {
			return fmt.Errorf("issue pass %q: claim is %q: %w", claimID, claim.Status, errs.ErrConflict)
		}
		itemRow, err := tx.GetItemByID(ctx, claim.ItemID)
		if err != nil {
			return fmt.Errorf("issue pass %q: %w", claimID, err)
		}
		summary := itemSummary(itemRow)

		channel, ok := returnChannelFor(itemRow.Status)
		if !ok {
			return fmt.Errorf("issue pass %q: item is %q: %w", claimID, itemRow.Status, errs.ErrConflict)
		}
		if claim.ReturnChannel != channel {
			return fmt.Errorf(
				"issue pass %q: claim channel %q conflicts with item status %q: %w",
				claimID, claim.ReturnChannel, itemRow.Status, errs.ErrConflict,
			)
		}
		custodianID := strings.TrimSpace(itemRow.CustodianID)
		if custodianID == "" {
			return fmt.Errorf("issue pass %q: item has no custodian: %w", claimID, errs.ErrConflict)
		}

		fee, err := s.deps.Fees.Check(ctx, tx, itemRow.ID, channel)
		if err != nil {
			return fmt.Errorf("issue pass %q: checking fee: %w", claimID, err)
		}
		status := StatusIssued
		if fee.Required && !fee.Paid {
			status = StatusPendingPayment
		}

		expiresAt := now.Add(s.deps.PassTTL)
		payload := tokenPayload{
			PassID:        passID,
			ClaimID:       claim.ID,
			ItemID:        itemRow.ID,
			CustodianID:   custodianID,
			ReturnChannel: channel,
			Exp:           expiresAt.Unix(),
		}
		token, err := signToken(s.deps.PassSecret, payload)
		if err != nil {
			return fmt.Errorf("issue pass %q: %w", claimID, err)
		}

		row := record.PassRecord{
			ID:               passID,
			ClaimID:          claim.ID,
			ItemID:           itemRow.ID,
			SearcherID:       claim.SearcherID,
			CustodianID:      custodianID,
			TokenHash:        hashToken(token),
			IssuedAt:         now,
			ExpiresAt:        expiresAt,
			Status:           status,
			IdentityRequired: false,
			ReturnChannel:    channel,
			FeeID:            fee.FeeID,
			PaymentID:        fee.PaymentID,
		}
		if err := tx.IssuePass(ctx, row); err != nil {
			return fmt.Errorf("issue pass %q: %w", claimID, err)
		}
		// Only one live token may exist per item: re-issuing invalidates older
		// open passes (pending, issued or validated) without touching the new
		// one. This prevents a shared old QR from staying usable after a
		// re-issue.
		revokedPrevious, err := tx.RevokePassesByItem(ctx, itemRow.ID, passID)
		if err != nil {
			return fmt.Errorf("issue pass %q: revoking previous passes: %w", claimID, err)
		}

		metadata, err := encodeMetadata(map[string]any{
			"claimId":         claim.ID,
			"itemId":          itemRow.ID,
			"status":          status,
			"returnChannel":   channel,
			"expiresAt":       expiresAt.Format(time.RFC3339),
			"revokedPrevious": revokedPrevious,
		})
		if err != nil {
			return fmt.Errorf("issue pass %q: %w", claimID, err)
		}
		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    itemRow.ID,
			Type:      EventPassIssued,
			ActorID:   actorID,
			Timestamp: now,
			Notes:     "pass " + passID + " issued for claim " + claim.ID,
		}); err != nil {
			return fmt.Errorf("issue pass %q: %w", claimID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   actorID,
			Action:    auditActionIssued,
			Entity:    "pass",
			EntityID:  passID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("issue pass %q: %w", claimID, err)
		}

		issued = Pass{
			ID:               passID,
			ClaimID:          claim.ID,
			ItemID:           itemRow.ID,
			SearcherID:       claim.SearcherID,
			CustodianID:      custodianID,
			Token:            token,
			Status:           status,
			ReturnChannel:    channel,
			IssuedAt:         now,
			ExpiresAt:        expiresAt,
			IdentityRequired: false,
			Item:             &summary,
		}
		issued.Instructions = instructions(issued)
		return nil
	})
	if err != nil {
		return Pass{}, err
	}
	return issued, nil
}

// Validate verifies the signature, the stored hash, the lifecycle state, the
// claim approval, the item custodian/state and the fee, in that order.
func (s *service) Validate(ctx context.Context, token, custodianID string, now time.Time) (Validation, error) {
	return s.validate(ctx, token, custodianID, now.UTC(), true)
}

func (s *service) validate(
	ctx context.Context,
	token, custodianID string,
	now time.Time,
	markValidated bool,
) (Validation, error) {
	if strings.TrimSpace(s.deps.PassSecret) == "" {
		return Validation{}, fmt.Errorf("validate pass: signing secret is not configured: %w", errs.ErrUnavailable)
	}
	token = strings.TrimSpace(token)
	custodianID = strings.TrimSpace(custodianID)
	if token == "" {
		return invalidValidation(ReasonInvalidToken, errs.ErrInvalidInput)
	}
	if custodianID == "" {
		return Validation{}, fmt.Errorf("validate pass: custodian id is required: %w", errs.ErrInvalidInput)
	}

	payload, err := verifyToken(s.deps.PassSecret, token)
	if err != nil {
		return invalidValidation(ReasonInvalidToken, errs.ErrInvalidInput)
	}
	row, err := s.db.GetPassByTokenHash(ctx, hashToken(token))
	if errors.Is(err, errs.ErrNotFound) {
		return invalidValidation(ReasonInvalidToken, errs.ErrInvalidInput)
	}
	if err != nil {
		return Validation{}, fmt.Errorf("validate pass: %w", err)
	}
	if payload.PassID != row.ID || payload.ClaimID != row.ClaimID ||
		payload.ItemID != row.ItemID || payload.CustodianID != row.CustodianID ||
		payload.ReturnChannel != row.ReturnChannel ||
		row.ExpiresAt.IsZero() || payload.Exp != row.ExpiresAt.Unix() {
		return invalidValidation(ReasonInvalidToken, errs.ErrInvalidInput)
	}

	switch row.Status {
	case StatusIssued, StatusValidated:
	case StatusRedeemed:
		return invalidValidation(ReasonAlreadyRedeemed, errs.ErrConflict)
	case StatusRevoked:
		return invalidValidation(ReasonRevoked, errs.ErrConflict)
	case StatusPendingPayment:
		return invalidValidation(ReasonUnpaid, errs.ErrConflict)
	case StatusExpired:
		return invalidValidation(ReasonExpired, errs.ErrInvalidInput)
	default:
		return invalidValidation(ReasonInvalidToken, errs.ErrInvalidInput)
	}
	if row.ExpiresAt.IsZero() || !now.Before(row.ExpiresAt) {
		return invalidValidation(ReasonExpired, errs.ErrInvalidInput)
	}

	claim, err := s.db.GetClaimByID(ctx, row.ClaimID)
	if errors.Is(err, errs.ErrNotFound) {
		return invalidValidation(ReasonClaimNotApproved, errs.ErrConflict)
	}
	if err != nil {
		return Validation{}, fmt.Errorf("validate pass: %w", err)
	}
	if claim.Status != claimStatusApproved {
		return invalidValidation(ReasonClaimNotApproved, errs.ErrConflict)
	}

	itemRow, err := s.db.GetItemByID(ctx, row.ItemID)
	if errors.Is(err, errs.ErrNotFound) {
		return invalidValidation(ReasonItemNotInCustody, errs.ErrConflict)
	}
	if err != nil {
		return Validation{}, fmt.Errorf("validate pass: %w", err)
	}
	summary := itemSummary(itemRow)

	if row.CustodianID != custodianID || itemRow.CustodianID != custodianID {
		return invalidValidation(ReasonWrongCustodian, errs.ErrForbidden)
	}
	channel, ok := returnChannelFor(itemRow.Status)
	if !ok || row.ReturnChannel != channel {
		return invalidValidation(ReasonItemNotInCustody, errs.ErrConflict)
	}

	fee, err := s.deps.Fees.Check(ctx, s.db.Repository(), row.ItemID, row.ReturnChannel)
	if err != nil {
		return Validation{}, fmt.Errorf("validate pass: checking fee: %w", err)
	}
	if fee.Required && !fee.Paid {
		return invalidValidation(ReasonUnpaid, errs.ErrConflict)
	}

	if markValidated && row.Status == StatusIssued {
		if err := s.db.UpdatePassFields(ctx, row.ID, map[string]any{"status": StatusValidated}); err != nil {
			return Validation{}, fmt.Errorf("validate pass: marking validated: %w", err)
		}
		row.Status = StatusValidated
	}

	return Validation{
		Valid:         true,
		PassID:        row.ID,
		ClaimID:       row.ClaimID,
		ItemID:        row.ItemID,
		CustodianID:   row.CustodianID,
		ReturnChannel: row.ReturnChannel,
		Status:        row.Status,
		ExpiresAt:     row.ExpiresAt,
		Item:          &summary,
	}, nil
}

// Redeem validates a token and performs the handover in one transaction. The
// pre-validation gives precise reasons; the transaction re-checks the state so
// an expiry, revocation or double scan racing with the scan cannot slip in.
func (s *service) Redeem(
	ctx context.Context,
	token, custodianID, actorID string,
	now time.Time,
) (itemdomain.Item, error) {
	if strings.TrimSpace(s.deps.PassSecret) == "" {
		return itemdomain.Item{}, fmt.Errorf("redeem pass: signing secret is not configured: %w", errs.ErrUnavailable)
	}
	token = strings.TrimSpace(token)
	custodianID = strings.TrimSpace(custodianID)
	actorID = strings.TrimSpace(actorID)
	if custodianID == "" || actorID == "" {
		return itemdomain.Item{}, fmt.Errorf("redeem pass: custodian id and actor id are required: %w", errs.ErrInvalidInput)
	}
	if actorID != custodianID {
		return itemdomain.Item{}, fmt.Errorf("redeem pass: actor is not the custodian: %w", errs.ErrForbidden)
	}
	now = now.UTC()

	if _, err := s.validate(ctx, token, custodianID, now, false); err != nil {
		return itemdomain.Item{}, err
	}

	eventID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("redeem pass: %w", err)
	}
	auditID, err := newID()
	if err != nil {
		return itemdomain.Item{}, fmt.Errorf("redeem pass: %w", err)
	}

	var updated itemdomain.Item
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := tx.GetPassByTokenHash(ctx, hashToken(token))
		if errors.Is(err, errs.ErrNotFound) {
			return reasonError(ReasonInvalidToken, errs.ErrInvalidInput)
		}
		if err != nil {
			return fmt.Errorf("redeem pass: %w", err)
		}

		switch row.Status {
		case StatusIssued, StatusValidated:
		case StatusRedeemed:
			return reasonError(ReasonAlreadyRedeemed, errs.ErrConflict)
		case StatusRevoked:
			return reasonError(ReasonRevoked, errs.ErrConflict)
		case StatusPendingPayment:
			return reasonError(ReasonUnpaid, errs.ErrConflict)
		case StatusExpired:
			return reasonError(ReasonExpired, errs.ErrInvalidInput)
		default:
			return reasonError(ReasonInvalidToken, errs.ErrInvalidInput)
		}
		if row.ExpiresAt.IsZero() || !now.Before(row.ExpiresAt) {
			return reasonError(ReasonExpired, errs.ErrInvalidInput)
		}
		if row.CustodianID != custodianID {
			return reasonError(ReasonWrongCustodian, errs.ErrForbidden)
		}

		claim, err := tx.GetClaimByID(ctx, row.ClaimID)
		if errors.Is(err, errs.ErrNotFound) {
			return reasonError(ReasonClaimNotApproved, errs.ErrConflict)
		}
		if err != nil {
			return fmt.Errorf("redeem pass: %w", err)
		}
		if claim.Status != claimStatusApproved {
			return reasonError(ReasonClaimNotApproved, errs.ErrConflict)
		}

		itemRow, err := tx.GetItemByID(ctx, row.ItemID)
		if errors.Is(err, errs.ErrNotFound) {
			return reasonError(ReasonItemNotInCustody, errs.ErrConflict)
		}
		if err != nil {
			return fmt.Errorf("redeem pass: %w", err)
		}
		if itemRow.CustodianID != custodianID {
			return reasonError(ReasonWrongCustodian, errs.ErrForbidden)
		}
		channel, ok := returnChannelFor(itemRow.Status)
		if !ok || row.ReturnChannel != channel {
			return reasonError(ReasonItemNotInCustody, errs.ErrConflict)
		}

		fee, err := s.deps.Fees.Check(ctx, tx, row.ItemID, row.ReturnChannel)
		if err != nil {
			return fmt.Errorf("redeem pass: checking fee: %w", err)
		}
		if fee.Required && !fee.Paid {
			return reasonError(ReasonUnpaid, errs.ErrConflict)
		}

		// RedeemPass performs the guarded, single-use state transition for the
		// pass, the item and the claim. Events and audit are appended in this
		// same outer transaction.
		if err := tx.RedeemPass(ctx, row.ID, row.ItemID, row.ClaimID, now); err != nil {
			return fmt.Errorf("redeem pass: %w", err)
		}

		metadata, err := encodeMetadata(map[string]any{
			"claimId":       row.ClaimID,
			"itemId":        row.ItemID,
			"passId":        row.ID,
			"custodianId":   custodianID,
			"returnChannel": row.ReturnChannel,
		})
		if err != nil {
			return fmt.Errorf("redeem pass: %w", err)
		}
		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    row.ItemID,
			Type:      EventHandover,
			ActorID:   actorID,
			Timestamp: now,
			Notes:     "item handed over with pass " + row.ID,
		}); err != nil {
			return fmt.Errorf("redeem pass: %w", err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   actorID,
			Action:    auditActionRedeemed,
			Entity:    "pass",
			EntityID:  row.ID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("redeem pass: %w", err)
		}

		itemRow.Status = itemdomain.StatusReturned
		itemRow.UpdatedAt = now
		updated, err = itemdomain.FromRecord(itemRow)
		if err != nil {
			return fmt.Errorf("redeem pass: %w", err)
		}
		return nil
	})
	if err != nil {
		return itemdomain.Item{}, err
	}
	return updated, nil
}

// RevokePassesByItem revokes every open pass for an item, e.g. when the
// institution takes over custody. The revocation and its audit entry are
// atomic; a second call is a no-op. The transfer is an automated/system action,
// so the audit records the system actor.
func (s *service) RevokePassesByItem(ctx context.Context, itemID string) (int64, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return 0, fmt.Errorf("revoke passes: item id is required: %w", errs.ErrInvalidInput)
	}
	actorID := systemActor
	now := s.now()

	auditID, err := newID()
	if err != nil {
		return 0, fmt.Errorf("revoke passes %q: %w", itemID, err)
	}

	var revoked int64
	err = s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		count, err := tx.RevokePassesByItem(ctx, itemID, "")
		if err != nil {
			return fmt.Errorf("revoke passes %q: %w", itemID, err)
		}
		revoked = count
		if count == 0 {
			return nil
		}
		metadata, err := encodeMetadata(map[string]any{
			"itemId":  itemID,
			"revoked": count,
		})
		if err != nil {
			return fmt.Errorf("revoke passes %q: %w", itemID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   actorID,
			Action:    auditActionRevoked,
			Entity:    "item",
			EntityID:  itemID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("revoke passes %q: %w", itemID, err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return revoked, nil
}

// itemSummary projects an item row onto the safe pass view. It deliberately
// copies only public listing fields: the private profile, notes, photos and
// coordinates never enter the pass domain.
func itemSummary(row record.ItemRecord) ItemSummary {
	return ItemSummary{
		ID:          row.ID,
		PublicCode:  row.PublicCode,
		Category:    row.Category,
		Brand:       row.Brand,
		Model:       row.Model,
		Color:       row.Color,
		Status:      row.Status,
		CustodianID: row.CustodianID,
	}
}

// returnChannelFor derives the legitimate pickup channel from the item state.
func returnChannelFor(itemStatus string) (string, bool) {
	switch itemStatus {
	case itemdomain.StatusInCustody:
		return ReturnChannelIntermediaryDirect, true
	case itemStatusTransferred:
		return ReturnChannelPA, true
	default:
		return "", false
	}
}

func invalidValidation(reason string, sentinel error) (Validation, error) {
	return Validation{Valid: false, Reason: reason}, reasonError(reason, sentinel)
}

func reasonError(reason string, sentinel error) error {
	return &ReasonError{Reason: reason, Err: sentinel}
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

// repositoryFeeChecker is the Task 9 default fee lookup. It is (deliberately)
// conservative: only a PA return can owe money, and a missing fee row means
// free. Task 10 replaces it with the fee service.
type repositoryFeeChecker struct{}

func (repositoryFeeChecker) Check(
	ctx context.Context,
	repo repository.Repository,
	itemID, returnChannel string,
) (FeeStatus, error) {
	if returnChannel != ReturnChannelPA {
		return FeeStatus{}, nil
	}
	fee, err := repo.GetFeeByItemID(ctx, itemID)
	if errors.Is(err, errs.ErrNotFound) {
		return FeeStatus{}, nil
	}
	if err != nil {
		return FeeStatus{}, err
	}

	status := FeeStatus{FeeID: fee.ID, AmountCents: fee.AmountCents}
	if fee.AmountCents <= 0 {
		return status, nil
	}
	status.Required = true
	switch strings.ToLower(strings.TrimSpace(fee.Status)) {
	case "paid", "waived":
		status.Paid = true
		return status, nil
	}

	payment, err := repo.GetPaymentByFeeID(ctx, fee.ID)
	if errors.Is(err, errs.ErrNotFound) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	if strings.EqualFold(payment.Status, "paid") || payment.PaidAt != nil {
		status.Paid = true
		status.PaymentID = payment.ID
	}
	return status, nil
}
