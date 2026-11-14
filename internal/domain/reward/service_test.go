package reward_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/reward"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func TestLegalFinderRequestsRewardOutsideTheSystem(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Now().UTC()
	svc := reward.New(db, reward.Deps{Now: func() time.Time { return now }})
	ctx := context.Background()

	notice, err := svc.Request(ctx, "item-phone", "bar-x", 2500)
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if !notice.Requested {
		t.Fatal("Request() requested = false, want true")
	}
	if notice.AmountCents != 2500 {
		t.Fatalf("amount = %d, want 2500", notice.AmountCents)
	}
	if notice.PaymentStatus != reward.PaymentStatusOutsideSystem {
		t.Fatalf("payment status = %q, want %q", notice.PaymentStatus, reward.PaymentStatusOutsideSystem)
	}
	if notice.ShownToSearcherAt != nil {
		t.Fatalf("shown at = %v, want nil before the searcher reads it", notice.ShownToSearcherAt)
	}

	// The system records the reward but never charges or pays it.
	var payments int64
	if err := db.GormDB().Model(&record.PaymentRecord{}).Count(&payments).Error; err != nil {
		t.Fatalf("counting payments: %v", err)
	}
	if payments != 0 {
		t.Fatalf("payments = %d, want 0 (rewards settle outside the system)", payments)
	}

	events, err := db.ListCustodyEvents(ctx, "item-phone")
	if err != nil {
		t.Fatalf("ListCustodyEvents() error = %v", err)
	}
	if !hasEvent(events, reward.EventRewardRequested) {
		t.Fatalf("events = %+v, want %q", events, reward.EventRewardRequested)
	}
	audit, err := db.ListAuditLogs(ctx, repository.AuditLogFilter{Limit: 50})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	found := false
	for _, entry := range audit {
		if entry.Action == reward.AuditActionRewardRequested {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit = %+v, want %q", audit, reward.AuditActionRewardRequested)
	}
}

func TestOnlyTheLegalFinderCanRequestAReward(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := reward.New(db)
	ctx := context.Background()

	if _, err := svc.Request(ctx, "item-phone", "searcher-demo", 0); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("Request(non legal finder) error = %v, want forbidden", err)
	}
	if _, err := svc.Request(ctx, "missing-item", "bar-x", 0); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("Request(missing item) error = %v, want not found", err)
	}
	if _, err := svc.Request(ctx, "item-phone", "bar-x", -1); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Request(negative amount) error = %v, want invalid input", err)
	}
}

func TestSearcherReadMarksRewardNoticeShown(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Now().UTC()
	svc := reward.New(db, reward.Deps{Now: func() time.Time { return now }})
	ctx := context.Background()

	if _, err := svc.Request(ctx, "item-phone", "bar-x", 1000); err != nil {
		t.Fatalf("Request() error = %v", err)
	}

	// The intermediary's read does not mark the notice as shown.
	if _, err := svc.GetFor(ctx, "item-phone", reward.Viewer{ID: "bar-x", Role: "intermediary"}); err != nil {
		t.Fatalf("GetFor(intermediary) error = %v", err)
	}
	before, err := svc.Get(ctx, "item-phone")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if before.ShownToSearcherAt != nil {
		t.Fatal("intermediary read marked the notice as shown")
	}

	seen, err := svc.GetFor(ctx, "item-phone", reward.Viewer{ID: "searcher-demo", Role: "searcher"})
	if err != nil {
		t.Fatalf("GetFor(searcher) error = %v", err)
	}
	if seen.ShownToSearcherAt == nil {
		t.Fatal("searcher read did not mark shown_to_searcher_at")
	}
	if !seen.ShownToSearcherAt.Equal(now) {
		t.Fatalf("shown at = %v, want %v", seen.ShownToSearcherAt, now)
	}

	// A second searcher read keeps the original timestamp.
	later := now.Add(time.Hour)
	svc = reward.New(db, reward.Deps{Now: func() time.Time { return later }})
	again, err := svc.GetFor(ctx, "item-phone", reward.Viewer{ID: "searcher-demo", Role: "searcher"})
	if err != nil {
		t.Fatalf("second GetFor(searcher) error = %v", err)
	}
	if again.ShownToSearcherAt == nil || !again.ShownToSearcherAt.Equal(now) {
		t.Fatalf("shown at = %v, want the original %v", again.ShownToSearcherAt, now)
	}
}

func TestRequestWithoutAmountKeepsNoticeRequested(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := reward.New(db)
	ctx := context.Background()

	notice, err := svc.Request(ctx, "item-phone", "bar-x", 0)
	if err != nil {
		t.Fatalf("Request(no amount) error = %v", err)
	}
	if !notice.Requested || notice.AmountCents != 0 {
		t.Fatalf("notice = %+v, want requested with no amount", notice)
	}
	if notice.PaymentStatus != reward.PaymentStatusOutsideSystem {
		t.Fatalf("payment status = %q, want outside_system", notice.PaymentStatus)
	}
}

func hasEvent(events []record.CustodyEventRecord, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}
