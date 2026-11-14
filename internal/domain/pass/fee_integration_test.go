package pass_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

// TestPassUsesFeeServiceForFreeAndUnpaidPA covers the pass/fee integration:
// direct returns stay free, a PA pass starts as pending_payment and unlocks
// once the mock payment settles the fee.
func TestPassUsesFeeServiceForFreeAndUnpaidPA(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Now().UTC()
	fees := fee.New(db)
	svc := pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute, Fees: fees})
	ctx := context.Background()

	free, err := svc.Issue(ctx, "claim-phone", "searcher-alt", now)
	if err != nil {
		t.Fatalf("Issue(free) error = %v", err)
	}
	if free.Status != pass.StatusIssued {
		t.Fatalf("free pass status = %q, want %q", free.Status, pass.StatusIssued)
	}
	if _, err := db.GetFeeByItemID(ctx, "item-phone"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("direct return created a fee: %v", err)
	}

	seedWalletClaim(t, db, "claim-wallet", "searcher-demo", now)
	unpaid, err := svc.Issue(ctx, "claim-wallet", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue(unpaid PA) error = %v", err)
	}
	if unpaid.Status != pass.StatusPendingPayment {
		t.Fatalf("PA pass status = %q, want %q", unpaid.Status, pass.StatusPendingPayment)
	}

	row, err := db.GetFeeByItemID(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("GetFeeByItemID() error = %v", err)
	}
	if row.AmountCents != 1500 {
		t.Fatalf("fee amount = %d, want 1500", row.AmountCents)
	}

	if _, err := svc.Validate(ctx, unpaid.Token, "institution-demo", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Validate(unpaid) error = %v, want conflict", err)
	} else if pass.ReasonOf(err) != pass.ReasonUnpaid {
		t.Fatalf("Validate(unpaid) reason = %q, want %q", pass.ReasonOf(err), pass.ReasonUnpaid)
	}

	calculated, err := fees.Calculate(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if _, err := fees.Pay(ctx, calculated.ID); err != nil {
		t.Fatalf("Pay() error = %v", err)
	}

	validation, err := svc.Validate(ctx, unpaid.Token, "institution-demo", now)
	if err != nil {
		t.Fatalf("Validate(paid) error = %v", err)
	}
	if !validation.Valid || validation.Status != pass.StatusValidated {
		t.Fatalf("Validate(paid) = %+v, want valid validated", validation)
	}
}

func TestPassIssueWithAlreadyPaidFeeIsIssuedImmediately(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Now().UTC()
	fees := fee.New(db)
	svc := pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute, Fees: fees})
	ctx := context.Background()

	seedWalletClaim(t, db, "claim-wallet-paid", "searcher-demo", now)
	calculated, err := fees.Calculate(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if _, err := fees.Pay(ctx, calculated.ID); err != nil {
		t.Fatalf("Pay() error = %v", err)
	}

	issued, err := svc.Issue(ctx, "claim-wallet-paid", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue(paid PA) error = %v", err)
	}
	if issued.Status != pass.StatusIssued {
		t.Fatalf("PA pass status = %q, want %q", issued.Status, pass.StatusIssued)
	}

	validation, err := svc.Validate(ctx, issued.Token, "institution-demo", now)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !validation.Valid {
		t.Fatalf("Validate() = %+v, want valid", validation)
	}
}

func seedWalletClaim(t *testing.T, db repository.Database, id, searcherID string, now time.Time) {
	t.Helper()
	if err := db.CreateClaim(context.Background(), record.ClaimRecord{
		ID:            id,
		ItemID:        "item-wallet",
		SearcherID:    searcherID,
		Status:        "approved",
		ReturnChannel: "pa",
		Confidence:    0.9,
		Evidence:      "[]",
		ApprovalBy:    "institution-demo",
		ApprovedAt:    &now,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("creating wallet claim: %v", err)
	}
}
