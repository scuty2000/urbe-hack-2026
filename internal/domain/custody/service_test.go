package custody_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

// custodyFixture describes the item/custody rows written directly to the
// database. Tests never depend on the demo seed or on the enrollment service.
type custodyFixture struct {
	itemID    string
	legalID   string
	custodian string
	sensitive bool
	rights    bool
	status    string
	maxDays   int
	createdAt time.Time
}

func defaultFixture(now time.Time) custodyFixture {
	return custodyFixture{
		itemID:    "item-umbrella",
		legalID:   "bar-x",
		custodian: "bar-x",
		rights:    true,
		status:    itemdomain.StatusPendingCustody,
		maxDays:   7,
		createdAt: now,
	}
}

func seedCustody(t *testing.T, db repository.Database, f custodyFixture) {
	t.Helper()

	ctx := context.Background()
	var rightsAt *time.Time
	if f.rights {
		rightsAt = &f.createdAt
	}

	itemRow := record.ItemRecord{
		ID:               f.itemID,
		PublicCode:       "LF-11111111",
		Source:           itemdomain.SourceFinderSelf,
		CustodyMode:      itemdomain.CustodyModeIntermediary,
		Status:           f.status,
		LegalFinderID:    f.legalID,
		RightsAssignedAt: rightsAt,
		Category:         "umbrella",
		Color:            "blue",
		Condition:        "good",
		FoundAt:          f.createdAt,
		FoundZone:        "San Lorenzo",
		CustodianID:      f.custodian,
		RiskTier:         itemdomain.RiskLow,
		Sensitive:        f.sensitive,
		CreatedAt:        f.createdAt,
		UpdatedAt:        f.createdAt,
	}
	if f.sensitive {
		itemRow.RiskTier = itemdomain.RiskSensitive
	}

	custodyRow := record.CustodyRecord{
		ID:                f.itemID + "-custody",
		ItemID:            f.itemID,
		CustodianType:     itemdomain.CustodyModeIntermediary,
		CustodianID:       f.custodian,
		LegalFinderID:     f.legalID,
		RightsAssignedAt:  rightsAt,
		ApprovalAuthority: itemdomain.CustodyModeIntermediary,
		MaxCustodyDays:    f.maxDays,
	}
	event := record.CustodyEventRecord{
		ID:        f.itemID + "-rights",
		ItemID:    f.itemID,
		Type:      itemdomain.EventRightsAssigned,
		ActorID:   "finder-demo",
		Timestamp: f.createdAt,
	}

	if err := db.CreateItemWithCustody(ctx, itemRow, custodyRow, event); err != nil {
		t.Fatalf("seeding item and custody: %v", err)
	}
}

func newCustodyService(t *testing.T, deps ...custody.Deps) (custody.Service, repository.Database) {
	t.Helper()
	db := testutil.NewTestDB(t)
	return custody.New(db, deps...), db
}

func TestAcceptSetsSevenDayDeadline(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	accepted, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}

	wantDue := now.Add(7 * 24 * time.Hour)
	if accepted.DueAt == nil || !accepted.DueAt.Equal(wantDue) {
		t.Fatalf("DueAt = %v, want %v", accepted.DueAt, wantDue)
	}
	if accepted.AcceptedAt == nil || !accepted.AcceptedAt.Equal(now) {
		t.Fatalf("AcceptedAt = %v, want %v", accepted.AcceptedAt, now)
	}
	if accepted.MaxCustodyDays != 7 {
		t.Fatalf("MaxCustodyDays = %d, want 7", accepted.MaxCustodyDays)
	}
	if accepted.CustodianID != "bar-x" || accepted.LegalFinderID != "bar-x" {
		t.Fatalf("custody parties = %+v", accepted)
	}

	itemRow, err := db.GetItemByID(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("GetItemByID() error = %v", err)
	}
	if itemRow.Status != itemdomain.StatusInCustody {
		t.Fatalf("item status = %q, want %q", itemRow.Status, itemdomain.StatusInCustody)
	}

	stored, err := db.GetCustodyByItemID(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("GetCustodyByItemID() error = %v", err)
	}
	if stored.DueAt == nil || !stored.DueAt.Equal(wantDue) || stored.MaxCustodyDays != 7 {
		t.Fatalf("stored custody = %+v", stored)
	}

	events, err := db.ListCustodyEvents(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("ListCustodyEvents() error = %v", err)
	}
	if got := eventTypes(events); !contains(got, custody.EventAccepted) {
		t.Fatalf("events = %v, want %q", got, custody.EventAccepted)
	}
	assertAuditAction(t, db, "item-umbrella", "item.custody_accepted")
}

func TestAcceptSensitiveUsesTwentyFourHours(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	fixture := defaultFixture(now)
	fixture.sensitive = true
	fixture.maxDays = 1
	seedCustody(t, db, fixture)

	accepted, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	wantDue := now.Add(24 * time.Hour)
	if accepted.DueAt == nil || !accepted.DueAt.Equal(wantDue) {
		t.Fatalf("DueAt = %v, want %v", accepted.DueAt, wantDue)
	}
	if accepted.MaxCustodyDays != 1 {
		t.Fatalf("MaxCustodyDays = %d, want 1", accepted.MaxCustodyDays)
	}
}

func TestAcceptRejectsMissingRightsAssignment(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	fixture := defaultFixture(now)
	fixture.rights = false
	seedCustody(t, db, fixture)

	_, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
	if !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Accept() error = %v, want conflict", err)
	}

	itemRow, err := db.GetItemByID(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("GetItemByID() error = %v", err)
	}
	if itemRow.Status != itemdomain.StatusPendingCustody {
		t.Fatalf("item status = %q, want untouched pending_custody", itemRow.Status)
	}
}

func TestAcceptRejectsWrongLegalFinder(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	_, err := svc.Accept(ctx, "item-umbrella", "portineria-sapienza", now)
	if !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("Accept() error = %v, want forbidden", err)
	}
}

func TestAcceptTwiceConflicts(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	if _, err := svc.Accept(ctx, "item-umbrella", "bar-x", now); err != nil {
		t.Fatalf("first Accept() error = %v", err)
	}
	if _, err := svc.Accept(ctx, "item-umbrella", "bar-x", now.Add(time.Hour)); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("second Accept() error = %v, want conflict", err)
	}
}

func TestAcceptRequiresAnOpenCustody(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	fixture := defaultFixture(now)
	seedCustody(t, db, fixture)

	closed := now.Add(-time.Hour)
	if err := db.UpdateCustodyFields(ctx, fixture.itemID+"-custody", map[string]any{"closed_at": closed}); err != nil {
		t.Fatalf("closing custody: %v", err)
	}

	_, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
	if !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Accept() error = %v, want conflict for a closed custody", err)
	}
}

func TestCustodyAllowsOnlyOneOpenRowPerItem(t *testing.T) {
	_, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	second := record.CustodyRecord{
		ID:                "item-umbrella-custody-2",
		ItemID:            "item-umbrella",
		CustodianType:     itemdomain.CustodyModeIntermediary,
		CustodianID:       "portineria-sapienza",
		LegalFinderID:     "portineria-sapienza",
		ApprovalAuthority: itemdomain.CustodyModeIntermediary,
	}
	if err := db.CreateCustody(ctx, second); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("CreateCustody() error = %v, want conflict from the unique open index", err)
	}
}

func TestRefuseRejectsCustodyAndClosesIt(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	before := time.Now().UTC()
	refused, err := svc.Refuse(ctx, "item-umbrella", "bar-x", "storage is full")
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Refuse() error = %v", err)
	}
	if refused.Status != itemdomain.StatusCustodyRejected {
		t.Fatalf("status = %q, want %q", refused.Status, itemdomain.StatusCustodyRejected)
	}

	itemRow, err := db.GetItemByID(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("GetItemByID() error = %v", err)
	}
	if itemRow.Status != itemdomain.StatusCustodyRejected {
		t.Fatalf("stored status = %q", itemRow.Status)
	}

	stored, err := db.GetCustodyByItemID(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("GetCustodyByItemID() error = %v", err)
	}
	if stored.ClosedAt == nil {
		t.Fatal("custody was not closed")
	}
	if stored.AcceptedAt != nil || stored.DueAt != nil {
		t.Fatalf("refused custody must not gain deadlines: %+v", stored)
	}
	if stored.ClosedAt.Before(before.Add(-time.Second)) || stored.ClosedAt.After(after.Add(time.Second)) {
		t.Fatalf("ClosedAt = %v, outside [%v, %v]", stored.ClosedAt, before, after)
	}

	events, err := db.ListCustodyEvents(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("ListCustodyEvents() error = %v", err)
	}
	if got := eventTypes(events); !contains(got, custody.EventCustodyRejected) {
		t.Fatalf("events = %v, want %q", got, custody.EventCustodyRejected)
	}
	assertAuditAction(t, db, "item-umbrella", "item.custody_rejected")
}

func TestRefuseRequiresReasonAndPreconditions(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	if _, err := svc.Refuse(ctx, "item-umbrella", "bar-x", "   "); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Refuse() error = %v, want invalid input for a blank reason", err)
	}
	if _, err := svc.Refuse(ctx, "item-umbrella", "portineria-sapienza", "not mine"); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("Refuse() error = %v, want forbidden", err)
	}
}

func TestMarkPickupDueHonoursTheDeadlineBoundary(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	accepted, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	due := *accepted.DueAt

	early, err := svc.MarkPickupDue(ctx, due.Add(-time.Second))
	if err != nil {
		t.Fatalf("MarkPickupDue(early) error = %v", err)
	}
	if len(early) != 0 {
		t.Fatalf("MarkPickupDue(early) = %d items, want 0", len(early))
	}

	boundary, err := svc.MarkPickupDue(ctx, due)
	if err != nil {
		t.Fatalf("MarkPickupDue(boundary) error = %v", err)
	}
	if len(boundary) != 1 {
		t.Fatalf("MarkPickupDue(boundary) = %d items, want 1", len(boundary))
	}
	if boundary[0].Status != itemdomain.StatusPickupDue {
		t.Fatalf("status = %q, want %q", boundary[0].Status, itemdomain.StatusPickupDue)
	}

	// A second tick must not emit a second transition for the same item.
	again, err := svc.MarkPickupDue(ctx, due.Add(time.Hour))
	if err != nil {
		t.Fatalf("MarkPickupDue(again) error = %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("MarkPickupDue(again) = %d items, want 0", len(again))
	}

	pickupEvents := 0
	events, err := db.ListCustodyEvents(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("ListCustodyEvents() error = %v", err)
	}
	for _, event := range events {
		if event.Type == custody.EventPickupScheduled {
			pickupEvents++
		}
	}
	if pickupEvents != 1 {
		t.Fatalf("pickup_scheduled events = %d, want 1", pickupEvents)
	}
	assertAuditAction(t, db, "item-umbrella", "item.pickup_scheduled")
}

func TestMarkPickupDueFlagsSensitiveItems(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	fixture := defaultFixture(now)
	fixture.sensitive = true
	fixture.maxDays = 1
	seedCustody(t, db, fixture)

	accepted, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}

	due, err := svc.MarkPickupDue(ctx, accepted.DueAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("MarkPickupDue() error = %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("MarkPickupDue() = %d items, want 1", len(due))
	}
	if due[0].Status != itemdomain.StatusSensitivePickup {
		t.Fatalf("status = %q, want %q", due[0].Status, itemdomain.StatusSensitivePickup)
	}
}

func TestAcceptUsesConfiguredPolicy(t *testing.T) {
	svc, db := newCustodyService(t, custody.Deps{MaxDays: 3, SensitiveHours: 12})
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

	seedCustody(t, db, defaultFixture(now))
	accepted, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	if accepted.DueAt == nil || !accepted.DueAt.Equal(now.Add(72*time.Hour)) {
		t.Fatalf("DueAt = %v, want %v", accepted.DueAt, now.Add(72*time.Hour))
	}
	if accepted.MaxCustodyDays != 3 {
		t.Fatalf("MaxCustodyDays = %d, want 3", accepted.MaxCustodyDays)
	}

	sensitive := defaultFixture(now.Add(time.Hour))
	sensitive.itemID = "item-passport"
	sensitive.sensitive = true
	seedCustody(t, db, sensitive)
	acceptedSensitive, err := svc.Accept(ctx, "item-passport", "bar-x", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Accept(sensitive) error = %v", err)
	}
	if acceptedSensitive.DueAt == nil || !acceptedSensitive.DueAt.Equal(now.Add(13*time.Hour)) {
		t.Fatalf("sensitive DueAt = %v, want %v", acceptedSensitive.DueAt, now.Add(13*time.Hour))
	}
	if acceptedSensitive.MaxCustodyDays != 1 {
		t.Fatalf("sensitive MaxCustodyDays = %d, want 1", acceptedSensitive.MaxCustodyDays)
	}
}

func TestConcurrentAcceptsHaveSingleWinner(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	const attempts = 8
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes, conflicts := 0, 0
	var unexpected []error
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, errs.ErrConflict):
			conflicts++
		default:
			unexpected = append(unexpected, err)
		}
	}
	if len(unexpected) > 0 {
		t.Fatalf("concurrent Accept() returned non-conflict errors (want a clean 409, not SQLITE_BUSY): %v", unexpected)
	}
	if successes != 1 {
		t.Fatalf("successful accepts = %d, want exactly 1", successes)
	}
	if conflicts != attempts-1 {
		t.Fatalf("conflicting accepts = %d, want %d", conflicts, attempts-1)
	}

	events, err := db.ListCustodyEvents(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("ListCustodyEvents() error = %v", err)
	}
	acceptedEvents := 0
	for _, event := range events {
		if event.Type == custody.EventAccepted {
			acceptedEvents++
		}
	}
	if acceptedEvents != 1 {
		t.Fatalf("accepted events = %d, want exactly 1", acceptedEvents)
	}
}

func TestConcurrentTicksEmitSinglePickupEvent(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	accepted, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	due := *accepted.DueAt

	type outcome struct {
		updated int
		err     error
	}
	const ticks = 8
	start := make(chan struct{})
	results := make(chan outcome, ticks)
	var wg sync.WaitGroup
	for i := 0; i < ticks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			dueItems, err := svc.MarkPickupDue(ctx, due)
			results <- outcome{updated: len(dueItems), err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	total := 0
	var unexpected []error
	for result := range results {
		if result.err != nil {
			unexpected = append(unexpected, result.err)
			continue
		}
		total += result.updated
	}
	if len(unexpected) > 0 {
		t.Fatalf("concurrent MarkPickupDue() errors (want no SQLITE_BUSY): %v", unexpected)
	}
	if total != 1 {
		t.Fatalf("items transitioned across ticks = %d, want exactly 1", total)
	}

	events, err := db.ListCustodyEvents(ctx, "item-umbrella")
	if err != nil {
		t.Fatalf("ListCustodyEvents() error = %v", err)
	}
	pickupEvents := 0
	for _, event := range events {
		if event.Type == custody.EventPickupScheduled {
			pickupEvents++
		}
	}
	if pickupEvents != 1 {
		t.Fatalf("pickup_scheduled events = %d, want exactly 1", pickupEvents)
	}
}

func TestDaysLeftCountsWholeDaysUntilDue(t *testing.T) {
	svc, db := newCustodyService(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	seedCustody(t, db, defaultFixture(now))

	accepted, err := svc.Accept(ctx, "item-umbrella", "bar-x", now)
	if err != nil {
		t.Fatalf("Accept() error = %v", err)
	}

	if got, err := svc.DaysLeft(ctx, "item-umbrella", now); err != nil || got != 7 {
		t.Fatalf("DaysLeft(accepted) = %d, %v; want 7", got, err)
	}
	if got, err := svc.DaysLeft(ctx, "item-umbrella", accepted.DueAt.Add(-time.Hour)); err != nil || got != 1 {
		t.Fatalf("DaysLeft(one hour left) = %d, %v; want 1", got, err)
	}
	if got, err := svc.DaysLeft(ctx, "item-umbrella", *accepted.DueAt); err != nil || got != 0 {
		t.Fatalf("DaysLeft(due) = %d, %v; want 0", got, err)
	}
	if got, err := svc.DaysLeft(ctx, "missing-item", now); err != nil || got != 0 {
		t.Fatalf("DaysLeft(missing) = %d, %v; want 0", got, err)
	}

	if got := custody.DaysLeft(nil, now); got != 0 {
		t.Fatalf("DaysLeft(nil) = %d, want 0", got)
	}
}

func eventTypes(events []record.CustodyEventRecord) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertAuditAction(t *testing.T, db repository.Database, itemID, action string) {
	t.Helper()

	logs, err := db.ListAuditLogs(context.Background(), repository.AuditLogFilter{
		Entity:   "item",
		EntityID: itemID,
	})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	for _, entry := range logs {
		if entry.Action == action {
			return
		}
	}
	t.Fatalf("audit actions = %v, want %q", auditActions(logs), action)
}

func auditActions(logs []record.AuditLogRecord) []string {
	actions := make([]string, 0, len(logs))
	for _, entry := range logs {
		actions = append(actions, entry.Action)
	}
	return actions
}
