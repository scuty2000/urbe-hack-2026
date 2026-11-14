package fee_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

// TestDirectReturnIsFreeAndPATariffAppliesAfterTransfer is the brief's core fee
// test: a direct return never owes money, a transferred item owes the PA tariff.
func TestDirectReturnIsFreeAndPATariffAppliesAfterTransfer(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := fee.New(db)

	direct, err := svc.Calculate(context.Background(), "item-phone")
	if err != nil {
		t.Fatalf("Calculate(direct) error = %v", err)
	}
	if direct.Applies != fee.AppliesNoneDirectReturn {
		t.Fatalf("direct applies = %q, want %q", direct.Applies, fee.AppliesNoneDirectReturn)
	}
	if direct.AmountCents != 0 {
		t.Fatalf("direct amount = %d, want 0", direct.AmountCents)
	}
	if !direct.Paid {
		t.Fatal("direct fee Paid = false, want true")
	}

	pa, err := svc.Calculate(context.Background(), "item-wallet")
	if err != nil {
		t.Fatalf("Calculate(pa) error = %v", err)
	}
	if pa.Applies != fee.AppliesPATariff {
		t.Fatalf("pa applies = %q, want %q", pa.Applies, fee.AppliesPATariff)
	}
	// The seeded wallet was transferred 20 hours ago: ceil(20h/24h) = 1 day.
	want := int64(1)*500 + 1000
	if pa.AmountCents != want {
		t.Fatalf("pa amount = %d, want %d", pa.AmountCents, want)
	}
	if pa.CustodyDays != 1 {
		t.Fatalf("pa custody days = %d, want 1", pa.CustodyDays)
	}
	if pa.Currency != "EUR" {
		t.Fatalf("pa currency = %q, want EUR", pa.Currency)
	}
	if pa.Paid {
		t.Fatal("pa fee Paid = true before payment")
	}
	if pa.Status != fee.StatusEstimated {
		t.Fatalf("pa status = %q, want %q", pa.Status, fee.StatusEstimated)
	}
}

func TestCalculateIsIdempotentAndRecalculatesUntilPaid(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Now().UTC()
	svc := fee.New(db, fee.Deps{Now: func() time.Time { return now }})
	ctx := context.Background()

	first, err := svc.Calculate(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("first Calculate() error = %v", err)
	}
	second, err := svc.Calculate(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("second Calculate() error = %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("Calculate() created a new fee: %q != %q", first.ID, second.ID)
	}

	var count int64
	if err := db.GormDB().Model(&record.FeeRecord{}).
		Where("item_id = ?", "item-wallet").Count(&count).Error; err != nil {
		t.Fatalf("counting fees: %v", err)
	}
	if count != 1 {
		t.Fatalf("fee rows = %d, want 1", count)
	}

	// Two more custody days must be reflected by the next recalculation.
	transferred := now.Add(-49 * time.Hour)
	if err := db.UpdateItemFields(ctx, "item-wallet", map[string]any{
		"transferred_at": transferred,
	}); err != nil {
		t.Fatalf("moving transferred_at: %v", err)
	}
	recalculated, err := svc.Calculate(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("recalculating: %v", err)
	}
	if recalculated.ID != first.ID {
		t.Fatalf("recalculation created a new fee: %q != %q", recalculated.ID, first.ID)
	}
	if recalculated.CustodyDays != 3 {
		t.Fatalf("custody days = %d, want 3", recalculated.CustodyDays)
	}
	if want := int64(3)*500 + 1000; recalculated.AmountCents != want {
		t.Fatalf("recalculated amount = %d, want %d", recalculated.AmountCents, want)
	}
}

func TestMockPaymentMarksFeePaidAndRecordsAudit(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := fee.New(db)
	ctx := context.Background()

	pa, err := svc.Calculate(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	payment, err := svc.Pay(ctx, pa.ID)
	if err != nil {
		t.Fatalf("Pay() error = %v", err)
	}
	if payment.Status != "paid" {
		t.Fatalf("payment status = %q, want paid", payment.Status)
	}
	if payment.Provider != fee.ProviderMock {
		t.Fatalf("payment provider = %q, want %q", payment.Provider, fee.ProviderMock)
	}
	if payment.ProviderRef == "" {
		t.Fatal("payment provider ref is empty")
	}
	if want := "/receipts/" + payment.ID; payment.ReceiptURL != want {
		t.Fatalf("receipt URL = %q, want %q", payment.ReceiptURL, want)
	}
	if payment.PaidAt == nil {
		t.Fatal("payment paidAt is nil")
	}

	paidFee, err := svc.Get(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if paidFee.Status != fee.StatusPaid {
		t.Fatalf("fee status = %q, want %q", paidFee.Status, fee.StatusPaid)
	}
	if !paidFee.Paid {
		t.Fatal("fee Paid = false after payment")
	}
	if paidFee.Payment == nil || paidFee.Payment.ID != payment.ID {
		t.Fatalf("fee payment = %+v, want %q", paidFee.Payment, payment.ID)
	}

	if _, err := svc.Pay(ctx, pa.ID); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("second Pay() error = %v, want conflict", err)
	}

	events, err := db.ListCustodyEvents(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("ListCustodyEvents() error = %v", err)
	}
	if !containsEvent(events, fee.EventFeePaid) {
		t.Fatalf("events = %+v, want %q", events, fee.EventFeePaid)
	}
	audit, err := db.ListAuditLogs(ctx, repository.AuditLogFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	foundAudit := false
	for _, entry := range audit {
		if entry.Action == fee.AuditActionFeePaid {
			foundAudit = true
		}
	}
	if !foundAudit {
		t.Fatalf("audit = %+v, want %q", audit, fee.AuditActionFeePaid)
	}
}

func TestIsPaidTransitions(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := fee.New(db)
	ctx := context.Background()

	paid, err := svc.IsPaid(ctx, "item-phone")
	if err != nil {
		t.Fatalf("IsPaid(direct) error = %v", err)
	}
	if !paid {
		t.Fatal("IsPaid(direct) = false, want true")
	}

	paid, err = svc.IsPaid(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("IsPaid(unpaid PA) error = %v", err)
	}
	if paid {
		t.Fatal("IsPaid(unpaid PA) = true, want false")
	}

	pa, err := svc.Calculate(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if _, err := svc.Pay(ctx, pa.ID); err != nil {
		t.Fatalf("Pay() error = %v", err)
	}
	paid, err = svc.IsPaid(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("IsPaid(paid PA) error = %v", err)
	}
	if !paid {
		t.Fatal("IsPaid(paid PA) = false, want true")
	}
}

func TestPayRejectsFreeFeeAndNonOwner(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := fee.New(db)
	ctx := context.Background()

	direct, err := svc.Calculate(ctx, "item-phone")
	if err != nil {
		t.Fatalf("Calculate(direct) error = %v", err)
	}
	if _, err := svc.Pay(ctx, direct.ID); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Pay(direct) error = %v, want conflict", err)
	}

	pa, err := svc.Calculate(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("Calculate(pa) error = %v", err)
	}
	if _, err := svc.PayFor(ctx, pa.ID, "searcher-demo"); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("PayFor(non-owner) error = %v, want forbidden", err)
	}
	if _, err := svc.Pay(ctx, "missing-fee"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("Pay(missing) error = %v, want not found", err)
	}
}

func TestCalculateRejectsUnknownItem(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := fee.New(db)

	if _, err := svc.Calculate(context.Background(), "missing-item"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("Calculate(missing item) error = %v, want not found", err)
	}
}

func containsEvent(events []record.CustodyEventRecord, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
