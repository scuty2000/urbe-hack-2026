package fee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

const day = 24 * time.Hour

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

// Calculate creates or recalculates the latest fee for the item. A paid or
// waived fee is final and is returned untouched.
func (s *service) Calculate(ctx context.Context, itemID string) (Fee, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return Fee{}, fmt.Errorf("calculate fee: item id is required: %w", errs.ErrInvalidInput)
	}

	var result Fee
	err := s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := s.ensureFee(ctx, tx, itemID, s.now())
		if err != nil {
			return err
		}
		result, err = s.feeView(ctx, tx, row)
		return err
	})
	if err != nil {
		return Fee{}, fmt.Errorf("calculate fee %q: %w", itemID, err)
	}
	return result, nil
}

// Get returns the latest fee without recalculating it.
func (s *service) Get(ctx context.Context, itemID string) (Fee, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return Fee{}, fmt.Errorf("get fee: item id is required: %w", errs.ErrInvalidInput)
	}
	row, err := s.db.GetFeeByItemID(ctx, itemID)
	if err != nil {
		return Fee{}, fmt.Errorf("get fee %q: %w", itemID, err)
	}
	view, err := s.feeView(ctx, s.db.Repository(), row)
	if err != nil {
		return Fee{}, fmt.Errorf("get fee %q: %w", itemID, err)
	}
	return view, nil
}

// Pay settles a fee through the mock provider. Callers that act on behalf of a
// searcher must use PayFor.
func (s *service) Pay(ctx context.Context, feeID string) (Payment, error) {
	return s.pay(ctx, feeID, "")
}

// PayFor settles a fee only when searcherID owns a claim on the item.
func (s *service) PayFor(ctx context.Context, feeID, searcherID string) (Payment, error) {
	searcherID = strings.TrimSpace(searcherID)
	if searcherID == "" {
		return Payment{}, fmt.Errorf("pay fee: searcher id is required: %w", errs.ErrInvalidInput)
	}
	return s.pay(ctx, feeID, searcherID)
}

func (s *service) pay(ctx context.Context, feeID, searcherID string) (Payment, error) {
	feeID = strings.TrimSpace(feeID)
	if feeID == "" {
		return Payment{}, fmt.Errorf("pay fee: fee id is required: %w", errs.ErrInvalidInput)
	}
	now := s.now()

	var paid Payment
	err := s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		row, err := tx.GetFeeByID(ctx, feeID)
		if err != nil {
			return err
		}
		if searcherID != "" {
			owned, err := ownsClaim(ctx, tx, row.ItemID, searcherID)
			if err != nil {
				return fmt.Errorf("pay fee %q: %w", feeID, err)
			}
			if !owned {
				return fmt.Errorf("pay fee %q: actor is not the claim owner: %w", feeID, errs.ErrForbidden)
			}
		}
		if row.Applies == AppliesNoneDirectReturn || row.AmountCents <= 0 {
			return fmt.Errorf("pay fee %q: %s: %w", feeID, reasonFreeFee, errs.ErrConflict)
		}
		switch strings.ToLower(strings.TrimSpace(row.Status)) {
		case StatusPaid, StatusWaived:
			return fmt.Errorf("pay fee %q: %s: %w", feeID, reasonAlreadyPaid, errs.ErrConflict)
		}

		paymentID, err := newID()
		if err != nil {
			return fmt.Errorf("pay fee %q: %w", feeID, err)
		}
		eventID, err := newID()
		if err != nil {
			return fmt.Errorf("pay fee %q: %w", feeID, err)
		}
		auditID, err := newID()
		if err != nil {
			return fmt.Errorf("pay fee %q: %w", feeID, err)
		}

		paymentRow := record.PaymentRecord{
			ID:          paymentID,
			FeeID:       row.ID,
			Provider:    ProviderMock,
			ProviderRef: "mock-" + paymentID,
			Status:      StatusPaid,
			PaidAt:      &now,
			ReceiptURL:  "/receipts/" + paymentID,
		}
		if err := tx.CreatePayment(ctx, paymentRow); err != nil {
			return fmt.Errorf("pay fee %q: %w", feeID, err)
		}
		if err := tx.UpdateFeeFields(ctx, row.ID, map[string]any{"status": StatusPaid}); err != nil {
			return fmt.Errorf("pay fee %q: %w", feeID, err)
		}
		row.Status = StatusPaid

		if err := tx.CreateCustodyEvent(ctx, record.CustodyEventRecord{
			ID:        eventID,
			ItemID:    row.ItemID,
			Type:      EventFeePaid,
			ActorID:   searcherID,
			Timestamp: now,
			Notes:     "PA fee " + row.ID + " paid via the mock provider",
		}); err != nil {
			return fmt.Errorf("pay fee %q: %w", feeID, err)
		}
		metadata, err := encodeMetadata(map[string]any{
			"itemId":      row.ItemID,
			"feeId":       row.ID,
			"paymentId":   paymentID,
			"amountCents": row.AmountCents,
			"currency":    row.Currency,
			"provider":    ProviderMock,
		})
		if err != nil {
			return fmt.Errorf("pay fee %q: %w", feeID, err)
		}
		if err := tx.CreateAuditLog(ctx, record.AuditLogRecord{
			ID:        auditID,
			ActorID:   searcherID,
			Action:    AuditActionFeePaid,
			Entity:    auditEntityFee,
			EntityID:  row.ID,
			Metadata:  metadata,
			Timestamp: now,
		}); err != nil {
			return fmt.Errorf("pay fee %q: %w", feeID, err)
		}

		paid = paymentFromRecord(paymentRow)
		return nil
	})
	if err != nil {
		return Payment{}, err
	}
	return paid, nil
}

// IsPaid reports whether the item's pickup is free or settled. A transferred
// item without an estimate is not free; only a direct return (or an amount of
// zero) is. A PA fee is settled by a paid payment or a final status.
func (s *service) IsPaid(ctx context.Context, itemID string) (bool, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return false, fmt.Errorf("check fee: item id is required: %w", errs.ErrInvalidInput)
	}
	item, err := s.db.GetItemByID(ctx, itemID)
	if err != nil {
		return false, fmt.Errorf("check fee %q: %w", itemID, err)
	}

	row, err := s.db.GetFeeByItemID(ctx, itemID)
	if errors.Is(err, errs.ErrNotFound) {
		// A transferred item is never free just because its fee row has not
		// been estimated yet.
		return item.Status != itemStatusTransferred, nil
	}
	if err != nil {
		return false, fmt.Errorf("check fee %q: %w", itemID, err)
	}
	paid, _, err := feeSettled(ctx, s.db.Repository(), row)
	if err != nil {
		return false, fmt.Errorf("check fee %q: %w", itemID, err)
	}
	return paid, nil
}

// Check implements pass.FeeChecker. A direct return is always free. A PA return
// ensures the fee exists (so a transferred item can never be collected for
// free) and reports whether it is still outstanding.
func (s *service) Check(
	ctx context.Context,
	repo repository.Repository,
	itemID, returnChannel string,
) (pass.FeeStatus, error) {
	if returnChannel != pass.ReturnChannelPA {
		return pass.FeeStatus{}, nil
	}
	row, err := s.ensureFee(ctx, repo, itemID, s.now())
	if errors.Is(err, errs.ErrNotFound) {
		// Let the pass service report the item's own state.
		return pass.FeeStatus{}, nil
	}
	if err != nil {
		return pass.FeeStatus{}, err
	}

	status := pass.FeeStatus{FeeID: row.ID, AmountCents: row.AmountCents}
	if row.Applies == AppliesNoneDirectReturn || row.AmountCents <= 0 {
		return status, nil
	}
	status.Required = true
	paid, paymentID, err := feeSettled(ctx, repo, row)
	if err != nil {
		return pass.FeeStatus{}, err
	}
	status.Paid = paid
	status.PaymentID = paymentID
	return status, nil
}

// ensureFee creates or recalculates the latest fee row using repo. The repo may
// be a transaction-scoped repository, so no nested transaction is opened.
func (s *service) ensureFee(
	ctx context.Context,
	repo repository.Repository,
	itemID string,
	now time.Time,
) (record.FeeRecord, error) {
	item, err := repo.GetItemByID(ctx, itemID)
	if err != nil {
		return record.FeeRecord{}, err
	}

	current, currentErr := repo.GetFeeByItemID(ctx, itemID)
	if currentErr != nil && !errors.Is(currentErr, errs.ErrNotFound) {
		return record.FeeRecord{}, currentErr
	}
	if currentErr == nil && isFinal(current.Status) {
		return current, nil
	}

	fields, err := s.feeFieldsFor(ctx, repo, item, now)
	if err != nil {
		return record.FeeRecord{}, err
	}
	if currentErr == nil {
		if err := repo.UpdateFeeFields(ctx, current.ID, fields); err != nil {
			return record.FeeRecord{}, err
		}
		applyFields(&current, fields)
		return current, nil
	}

	id, err := newID()
	if err != nil {
		return record.FeeRecord{}, err
	}
	row := record.FeeRecord{ID: id, ItemID: itemID}
	applyFields(&row, fields)
	if err := repo.CreateFee(ctx, row); err != nil {
		return record.FeeRecord{}, err
	}
	return row, nil
}

// feeFieldsFor computes the current estimate for an item. Direct returns are
// free; a transferred item uses the active tariff (or the configured fallback).
func (s *service) feeFieldsFor(
	ctx context.Context,
	repo repository.Repository,
	item record.ItemRecord,
	now time.Time,
) (map[string]any, error) {
	if item.Status != itemStatusTransferred {
		currency := s.deps.Tariff.Currency
		if currency == "" {
			currency = defaultCurrencyCode
		}
		return map[string]any{
			"applies":      AppliesNoneDirectReturn,
			"pa_tariff_id": "",
			"custody_days": 0,
			"amount_cents": int64(0),
			"currency":     currency,
			"status":       StatusFinal,
		}, nil
	}

	tariff, err := repo.GetActiveTariff(ctx)
	if errors.Is(err, errs.ErrNotFound) {
		if s.deps.Tariff.DailyRateCents == 0 && s.deps.Tariff.FixedFeeCents == 0 {
			return nil, fmt.Errorf("no active PA tariff is configured: %w", errs.ErrConflict)
		}
		tariff = record.TariffRecord{
			DailyRateCents: s.deps.Tariff.DailyRateCents,
			FixedFeeCents:  s.deps.Tariff.FixedFeeCents,
			Currency:       s.deps.Tariff.Currency,
		}
	} else if err != nil {
		return nil, err
	}

	days := 1
	if item.TransferredAt != nil {
		days = custodyDays(now, *item.TransferredAt)
	}
	amount := int64(days)*tariff.DailyRateCents + tariff.FixedFeeCents
	currency := tariff.Currency
	if currency == "" {
		currency = defaultCurrencyCode
	}
	return map[string]any{
		"applies":      AppliesPATariff,
		"pa_tariff_id": tariff.ID,
		"custody_days": days,
		"amount_cents": amount,
		"currency":     currency,
		"status":       StatusEstimated,
	}, nil
}

// feeView maps a row and its payment onto the read model.
func (s *service) feeView(
	ctx context.Context,
	repo repository.Repository,
	row record.FeeRecord,
) (Fee, error) {
	view := Fee{
		ID:          row.ID,
		ItemID:      row.ItemID,
		Applies:     row.Applies,
		PATariffID:  row.PATariffID,
		CustodyDays: row.CustodyDays,
		AmountCents: row.AmountCents,
		Currency:    row.Currency,
		Status:      row.Status,
		Paid:        row.Applies == AppliesNoneDirectReturn || row.AmountCents <= 0 || isFinal(row.Status),
	}

	payment, err := repo.GetPaymentByFeeID(ctx, row.ID)
	if errors.Is(err, errs.ErrNotFound) {
		return view, nil
	}
	if err != nil {
		return Fee{}, err
	}
	mapped := paymentFromRecord(payment)
	view.Payment = &mapped
	if paymentSettled(payment) {
		view.Paid = true
	}
	return view, nil
}

func paymentFromRecord(row record.PaymentRecord) Payment {
	return Payment{
		ID:          row.ID,
		FeeID:       row.FeeID,
		Provider:    row.Provider,
		ProviderRef: row.ProviderRef,
		Status:      row.Status,
		PaidAt:      row.PaidAt,
		ReceiptURL:  row.ReceiptURL,
	}
}

// applyFields copies the map into a record. Fields are intentionally written
// together: the fee row is a snapshot of one calculation.
func applyFields(row *record.FeeRecord, fields map[string]any) {
	if value, ok := fields["applies"].(string); ok {
		row.Applies = value
	}
	if value, ok := fields["pa_tariff_id"].(string); ok {
		row.PATariffID = value
	}
	if value, ok := fields["custody_days"].(int); ok {
		row.CustodyDays = value
	}
	if value, ok := fields["amount_cents"].(int64); ok {
		row.AmountCents = value
	}
	if value, ok := fields["currency"].(string); ok {
		row.Currency = value
	}
	if value, ok := fields["status"].(string); ok {
		row.Status = value
	}
}

// custodyDays rounds the elapsed custody time up to whole days, with a floor of
// one day. The transfer itself starts the first day.
func custodyDays(now, transferredAt time.Time) int {
	if !now.After(transferredAt) {
		return 1
	}
	days := int((now.Sub(transferredAt) + day - 1) / day)
	if days < 1 {
		return 1
	}
	return days
}

func isFinal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusPaid, StatusWaived:
		return true
	default:
		return false
	}
}

func paymentSettled(payment record.PaymentRecord) bool {
	return strings.EqualFold(payment.Status, StatusPaid) || payment.PaidAt != nil
}

// feeSettled reports whether the fee is free or settled and returns the
// settling payment id when there is one. It is shared by IsPaid and the pass
// FeeChecker so both answer the same question.
func feeSettled(
	ctx context.Context,
	repo repository.Repository,
	row record.FeeRecord,
) (bool, string, error) {
	payment, err := repo.GetPaymentByFeeID(ctx, row.ID)
	if err != nil && !errors.Is(err, errs.ErrNotFound) {
		return false, "", err
	}
	if err == nil && paymentSettled(payment) {
		return true, payment.ID, nil
	}
	if row.Applies == AppliesNoneDirectReturn || row.AmountCents <= 0 || isFinal(row.Status) {
		return true, "", nil
	}
	return false, "", nil
}

func ownsClaim(ctx context.Context, repo repository.Repository, itemID, searcherID string) (bool, error) {
	claims, err := repo.ListClaimsByItem(ctx, itemID)
	if err != nil {
		return false, err
	}
	for _, claim := range claims {
		if claim.SearcherID == searcherID {
			return true, nil
		}
	}
	return false, nil
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
