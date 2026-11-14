package pass_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

const testSecret = "test-secret"

func newService(t *testing.T) pass.Service {
	t.Helper()
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	return pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute})
}

// TestPassRedeemsOnceAndRejectsWrongCustodian is the brief's end-to-end domain
// test: the seeded claim-phone pass works once at Bar X and only at Bar X.
func TestPassRedeemsOnceAndRejectsWrongCustodian(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute})
	now := time.Now().UTC()

	p, err := svc.Issue(context.Background(), "claim-phone", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if p.Token == "" {
		t.Fatal("Issue() returned an empty token")
	}
	if p.CustodianID != "bar-x" {
		t.Fatalf("CustodianID = %q, want bar-x", p.CustodianID)
	}
	if p.Status != pass.StatusIssued {
		t.Fatalf("Status = %q, want %q", p.Status, pass.StatusIssued)
	}

	if _, err := svc.Validate(context.Background(), p.Token, "other-shop", now); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("Validate(other custodian) error = %v, want forbidden", err)
	} else if pass.ReasonOf(err) != pass.ReasonWrongCustodian {
		t.Fatalf("Validate(other custodian) reason = %q, want %q", pass.ReasonOf(err), pass.ReasonWrongCustodian)
	}

	item, err := svc.Redeem(context.Background(), p.Token, "bar-x", "bar-x", now)
	if err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	if item.Status != itemdomain.StatusReturned {
		t.Fatalf("Redeem() item status = %q, want returned", item.Status)
	}

	if _, err := svc.Redeem(context.Background(), p.Token, "bar-x", "bar-x", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("second Redeem() error = %v, want conflict", err)
	}
	if _, err := svc.Validate(context.Background(), p.Token, "bar-x", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Validate(after redeem) error = %v, want conflict", err)
	} else if pass.ReasonOf(err) != pass.ReasonAlreadyRedeemed {
		t.Fatalf("Validate(after redeem) reason = %q, want %q", pass.ReasonOf(err), pass.ReasonAlreadyRedeemed)
	}

	row, err := db.GetItemByID(context.Background(), "item-phone")
	if err != nil {
		t.Fatalf("GetItemByID() error = %v", err)
	}
	if row.Status != itemdomain.StatusReturned {
		t.Fatalf("persisted item status = %q, want returned", row.Status)
	}
}

func TestPassValidateRejectsExpiredAndTamperedTokens(t *testing.T) {
	svc := newService(t)
	now := time.Now().UTC()

	p, err := svc.Issue(context.Background(), "claim-phone", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	expired, err := svc.Validate(context.Background(), p.Token, "bar-x", now.Add(31*time.Minute))
	if !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Validate(expired) error = %v, want invalid input", err)
	}
	if expired.Valid {
		t.Fatal("Validate(expired) valid = true, want false")
	}
	if pass.ReasonOf(err) != pass.ReasonExpired {
		t.Fatalf("expired reason = %q, want %q", pass.ReasonOf(err), pass.ReasonExpired)
	}

	for name, token := range map[string]string{
		"appended garbage": p.Token + "x",
		"flipped payload":  "A" + p.Token[1:],
		"empty":            "",
	} {
		validation, err := svc.Validate(context.Background(), token, "bar-x", now)
		if !errors.Is(err, errs.ErrInvalidInput) {
			t.Fatalf("%s: Validate() error = %v, want invalid input", name, err)
		}
		if validation.Valid {
			t.Fatalf("%s: validation valid = true, want false", name)
		}
		if pass.ReasonOf(err) != pass.ReasonInvalidToken {
			t.Fatalf("%s: reason = %q, want %q", name, pass.ReasonOf(err), pass.ReasonInvalidToken)
		}
	}
}

func TestPassFailsClosedWithoutSecret(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := pass.New(db, pass.Deps{})
	now := time.Now().UTC()

	if _, err := svc.Issue(context.Background(), "claim-phone", "searcher-demo", now); !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("Issue() without secret error = %v, want unavailable", err)
	}
	if _, err := svc.Validate(context.Background(), "any-token", "bar-x", now); !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("Validate() without secret error = %v, want unavailable", err)
	}
	if _, err := svc.Redeem(context.Background(), "any-token", "bar-x", "bar-x", now); !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("Redeem() without secret error = %v, want unavailable", err)
	}
}

func TestPassIssueRequiresApprovedClaimAndItemState(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute})
	now := time.Now().UTC()
	ctx := context.Background()

	if _, err := svc.Issue(ctx, "claim-phone", "someone-else", now); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("Issue(other searcher) error = %v, want forbidden", err)
	}
	if _, err := svc.Issue(ctx, "missing-claim", "searcher-demo", now); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("Issue(missing claim) error = %v, want not found", err)
	}

	// An item that is still pending custody has no legitimate return channel.
	if err := db.CreateClaim(ctx, record.ClaimRecord{
		ID: "claim-umbrella", ItemID: "item-umbrella", SearcherID: "searcher-demo",
		Status: "approved", ReturnChannel: "intermediary_direct", Evidence: "[]",
		ApprovalBy: "bar-x", CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateClaim(umbrella) error = %v", err)
	}
	if _, err := svc.Issue(ctx, "claim-umbrella", "searcher-demo", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Issue(pending item) error = %v, want conflict", err)
	}

	// A claim whose channel disagrees with the item state must not be issued.
	if err := db.CreateClaim(ctx, record.ClaimRecord{
		ID: "claim-phone-pa", ItemID: "item-phone", SearcherID: "searcher-demo",
		Status: "approved", ReturnChannel: "pa", Evidence: "[]",
		ApprovalBy: "bar-x", CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateClaim(phone pa) error = %v", err)
	}
	if _, err := svc.Issue(ctx, "claim-phone-pa", "searcher-demo", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Issue(channel mismatch) error = %v, want conflict", err)
	}

	// A rejected claim is not redeemable.
	if err := db.CreateClaim(ctx, record.ClaimRecord{
		ID: "claim-rejected", ItemID: "item-phone", SearcherID: "searcher-demo",
		Status: "rejected", ReturnChannel: "intermediary_direct", Evidence: "[]",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateClaim(rejected) error = %v", err)
	}
	if _, err := svc.Issue(ctx, "claim-rejected", "searcher-demo", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Issue(rejected claim) error = %v, want conflict", err)
	}
}

func TestPassPAFeeMustBePaidBeforeRedemption(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute})
	now := time.Now().UTC()
	ctx := context.Background()

	if err := db.CreateClaim(ctx, record.ClaimRecord{
		ID: "claim-wallet", ItemID: "item-wallet", SearcherID: "searcher-demo",
		Status: "approved", ReturnChannel: "pa", Evidence: "[]",
		ApprovalBy: "institution-demo", CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateClaim(wallet) error = %v", err)
	}
	if err := db.CreateFee(ctx, record.FeeRecord{
		ID: "fee-wallet", ItemID: "item-wallet", Applies: "pa_return",
		AmountCents: 1500, Currency: "EUR", Status: "unpaid",
	}); err != nil {
		t.Fatalf("CreateFee(wallet) error = %v", err)
	}

	pending, err := svc.Issue(ctx, "claim-wallet", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue(unpaid PA) error = %v", err)
	}
	if pending.Status != pass.StatusPendingPayment {
		t.Fatalf("unpaid pass status = %q, want %q", pending.Status, pass.StatusPendingPayment)
	}
	if _, err := svc.Validate(ctx, pending.Token, "institution-demo", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Validate(unpaid) error = %v, want conflict", err)
	} else if pass.ReasonOf(err) != pass.ReasonUnpaid {
		t.Fatalf("Validate(unpaid) reason = %q, want %q", pass.ReasonOf(err), pass.ReasonUnpaid)
	}
	if _, err := svc.Redeem(ctx, pending.Token, "institution-demo", "institution-demo", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Redeem(unpaid) error = %v, want conflict", err)
	}

	// Marking the fee paid allows a fresh pass to be issued and redeemed.
	if err := db.UpdateFeeFields(ctx, "fee-wallet", map[string]any{"status": "paid"}); err != nil {
		t.Fatalf("UpdateFeeFields() error = %v", err)
	}
	p, err := svc.Issue(ctx, "claim-wallet", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue(paid PA) error = %v", err)
	}
	if p.Status != pass.StatusIssued {
		t.Fatalf("paid pass status = %q, want %q", p.Status, pass.StatusIssued)
	}
	if _, err := svc.Validate(ctx, p.Token, "institution-demo", now); err != nil {
		t.Fatalf("Validate(paid PA) error = %v", err)
	}
	item, err := svc.Redeem(ctx, p.Token, "institution-demo", "institution-demo", now)
	if err != nil {
		t.Fatalf("Redeem(paid PA) error = %v", err)
	}
	if item.ID != "item-wallet" || item.Status != itemdomain.StatusReturned {
		t.Fatalf("Redeem(paid PA) item = %+v, want returned wallet", item)
	}
}

func TestPassDirectReturnIgnoresFeesAndZeroFeeIsFree(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute})
	now := time.Now().UTC()
	ctx := context.Background()

	// A direct handover is free even if a stray fee row exists: the channel is
	// derived from the item state, not from the fee.
	if err := db.CreateFee(ctx, record.FeeRecord{
		ID: "fee-phone", ItemID: "item-phone", Applies: "pa_return",
		AmountCents: 1000, Currency: "EUR", Status: "unpaid",
	}); err != nil {
		t.Fatalf("CreateFee(phone) error = %v", err)
	}
	p, err := svc.Issue(ctx, "claim-phone", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue(direct) error = %v", err)
	}
	if p.Status != pass.StatusIssued {
		t.Fatalf("direct pass status = %q, want %q", p.Status, pass.StatusIssued)
	}

	// A zero-amount PA fee needs no payment.
	if err := db.CreateClaim(ctx, record.ClaimRecord{
		ID: "claim-wallet-zero", ItemID: "item-wallet", SearcherID: "searcher-demo",
		Status: "approved", ReturnChannel: "pa", Evidence: "[]",
		ApprovalBy: "institution-demo", CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateClaim(wallet zero) error = %v", err)
	}
	if err := db.CreateFee(ctx, record.FeeRecord{
		ID: "fee-wallet-zero", ItemID: "item-wallet", Applies: "pa_return",
		AmountCents: 0, Currency: "EUR", Status: "waived",
	}); err != nil {
		t.Fatalf("CreateFee(wallet zero) error = %v", err)
	}
	zero, err := svc.Issue(ctx, "claim-wallet-zero", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue(zero fee) error = %v", err)
	}
	if zero.Status != pass.StatusIssued {
		t.Fatalf("zero fee pass status = %q, want %q", zero.Status, pass.StatusIssued)
	}
}

func TestPassRevokeByItem(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute})
	now := time.Now().UTC()
	ctx := context.Background()

	p, err := svc.Issue(ctx, "claim-phone", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	revoked, err := svc.RevokePassesByItem(ctx, "item-phone")
	if err != nil {
		t.Fatalf("RevokePassesByItem() error = %v", err)
	}
	if revoked != 1 {
		t.Fatalf("RevokePassesByItem() = %d, want 1", revoked)
	}
	if _, err := svc.Validate(ctx, p.Token, "bar-x", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Validate(revoked) error = %v, want conflict", err)
	} else if pass.ReasonOf(err) != pass.ReasonRevoked {
		t.Fatalf("Validate(revoked) reason = %q, want %q", pass.ReasonOf(err), pass.ReasonRevoked)
	}
	// Revoking twice is idempotent.
	if again, err := svc.RevokePassesByItem(ctx, "item-phone"); err != nil || again != 0 {
		t.Fatalf("second RevokePassesByItem() = %d, %v; want 0, nil", again, err)
	}
}

func TestPassReissueRevokesPreviousToken(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute})
	now := time.Now().UTC()
	ctx := context.Background()

	first, err := svc.Issue(ctx, "claim-phone", "searcher-demo", now)
	if err != nil {
		t.Fatalf("first Issue() error = %v", err)
	}
	second, err := svc.Issue(ctx, "claim-phone", "searcher-demo", now)
	if err != nil {
		t.Fatalf("second Issue() error = %v", err)
	}

	if _, err := svc.Validate(ctx, first.Token, "bar-x", now); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Validate(old token) error = %v, want conflict", err)
	} else if pass.ReasonOf(err) != pass.ReasonRevoked {
		t.Fatalf("Validate(old token) reason = %q, want %q", pass.ReasonOf(err), pass.ReasonRevoked)
	}
	if _, err := svc.Validate(ctx, second.Token, "bar-x", now); err != nil {
		t.Fatalf("Validate(new token) error = %v", err)
	}
}

func TestPassStoresOnlyTheTokenHash(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := pass.New(db, pass.Deps{PassSecret: testSecret, PassTTL: 30 * time.Minute})
	now := time.Now().UTC()
	ctx := context.Background()

	p, err := svc.Issue(ctx, "claim-phone", "searcher-demo", now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(p.Token)))
	row, err := db.GetPassByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPassByID() error = %v", err)
	}
	if row.TokenHash != want {
		t.Fatalf("stored token hash = %q, want %q", row.TokenHash, want)
	}
	if row.TokenHash == p.Token {
		t.Fatal("raw token was stored in token_hash")
	}
	if _, err := db.GetPassByTokenHash(ctx, p.Token); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("GetPassByTokenHash(raw token) error = %v, want not found", err)
	}
}
