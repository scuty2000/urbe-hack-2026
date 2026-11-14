package institution_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	institutiondomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/institution"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

// recordingPass counts revocations while delegating to the real pass service,
// so a transfer test can assert both the call and the stored status.
type recordingPass struct {
	passdomain.Service
	mu    sync.Mutex
	calls int
}

func (p *recordingPass) RevokePassesByItem(ctx context.Context, itemID string) (int64, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return p.Service.RevokePassesByItem(ctx, itemID)
}

func (p *recordingPass) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// recordingSearch captures the item snapshots sent to the vector index.
type recordingSearch struct {
	search.Service
	mu      sync.Mutex
	indexed []itemdomain.Item
}

func (s *recordingSearch) IndexItem(ctx context.Context, item itemdomain.Item) error {
	s.mu.Lock()
	s.indexed = append(s.indexed, item)
	s.mu.Unlock()
	return nil
}

func (s *recordingSearch) indexedItems() []itemdomain.Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]itemdomain.Item(nil), s.indexed...)
}

func TestInstitutionTransferAndUnclaimedOrder(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	cs := custody.New(db)
	if _, err := cs.Accept(context.Background(), "item-umbrella", "bar-x", now); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.MarkPickupDue(context.Background(), now.Add(8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	svc := institutiondomain.New(db)
	if _, err := svc.Transfer(context.Background(), "item-umbrella", "institution-demo", now); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OfferUnclaimed(context.Background(), "item-wallet", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("OfferUnclaimed before one year error = %v, want invalid input", err)
	}
}

func TestListPickupsGroupsSensitiveFirst(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Now().UTC()
	cs := custody.New(db)
	if _, err := cs.Accept(context.Background(), "item-umbrella", "bar-x", now); err != nil {
		t.Fatalf("accept umbrella: %v", err)
	}
	if _, err := cs.Accept(context.Background(), "item-document", "bar-x", now); err != nil {
		t.Fatalf("accept document: %v", err)
	}
	if _, err := cs.MarkPickupDue(context.Background(), now.Add(8*24*time.Hour)); err != nil {
		t.Fatalf("mark pickup due: %v", err)
	}

	svc := institutiondomain.New(db)
	groups, err := svc.ListPickups(context.Background())
	if err != nil {
		t.Fatalf("ListPickups() error = %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("ListPickups() returned no groups")
	}
	if !groups[0].Sensitive {
		t.Fatalf("first group = %+v, want a sensitive group first", groups[0])
	}

	var flattened []institutiondomain.PickupItem
	for _, group := range groups {
		flattened = append(flattened, group.Items...)
	}
	documentIndex, umbrellaIndex := -1, -1
	for index, item := range flattened {
		switch item.ID {
		case "item-document":
			documentIndex = index
			if !item.Sensitive || item.Status != itemdomain.StatusSensitivePickup {
				t.Fatalf("document pickup = %+v, want sensitive_pickup", item)
			}
		case "item-umbrella":
			umbrellaIndex = index
		}
	}
	if documentIndex < 0 || umbrellaIndex < 0 {
		t.Fatalf("pickups = %+v, want document and umbrella", flattened)
	}
	if documentIndex > umbrellaIndex {
		t.Fatalf("document index = %d, umbrella index = %d; sensitive items must come first", documentIndex, umbrellaIndex)
	}
}

func TestTransferClosesIntermediaryCustodyAndRevokesPasses(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	cs := custody.New(db)
	if _, err := cs.Accept(context.Background(), "item-umbrella", "bar-x", now); err != nil {
		t.Fatalf("accept umbrella: %v", err)
	}
	if _, err := cs.MarkPickupDue(context.Background(), now.Add(8*24*time.Hour)); err != nil {
		t.Fatalf("mark pickup due: %v", err)
	}

	if err := db.CreatePass(context.Background(), record.PassRecord{
		ID:            "pass-umbrella",
		ClaimID:       "claim-phone",
		ItemID:        "item-umbrella",
		SearcherID:    "searcher-demo",
		CustodianID:   "bar-x",
		TokenHash:     "hash-umbrella",
		IssuedAt:      now,
		ExpiresAt:     now.Add(time.Hour),
		Status:        "issued",
		ReturnChannel: "intermediary_direct",
	}); err != nil {
		t.Fatalf("seed pass: %v", err)
	}

	passes := &recordingPass{Service: passdomain.New(db, passdomain.Deps{PassSecret: "test-secret"})}
	indexer := &recordingSearch{Service: search.New(db, search.Deps{})}
	svc := institutiondomain.New(db, institutiondomain.Deps{
		Pass:   passes,
		Search: indexer,
	})

	transferred, err := svc.Transfer(context.Background(), "item-umbrella", "institution-demo", now)
	if err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}
	if transferred.Status != itemdomain.StatusTransferred {
		t.Fatalf("status = %q, want transferred", transferred.Status)
	}
	if transferred.CustodianID != "institution-demo" || transferred.CustodyMode != itemdomain.CustodyModeInstitution {
		t.Fatalf("transferred item = %+v, want institution custody", transferred)
	}
	if transferred.TransferredAt == nil || !transferred.TransferredAt.Equal(now) {
		t.Fatalf("TransferredAt = %v, want %v", transferred.TransferredAt, now)
	}
	if transferred.LegalTermStart == nil || !transferred.LegalTermStart.Equal(now) {
		t.Fatalf("LegalTermStart = %v, want %v", transferred.LegalTermStart, now)
	}

	oldCustody, err := db.GetCustodyByID(context.Background(), "custody-umbrella")
	if err != nil {
		t.Fatalf("GetCustodyByID() error = %v", err)
	}
	if oldCustody.ClosedAt == nil {
		t.Fatal("intermediary custody was not closed")
	}

	current, err := db.GetCustodyByItemID(context.Background(), "item-umbrella")
	if err != nil {
		t.Fatalf("GetCustodyByItemID() error = %v", err)
	}
	if current.CustodianType != itemdomain.CustodyModeInstitution || current.CustodianID != "institution-demo" {
		t.Fatalf("current custody = %+v, want institution custody", current)
	}
	if current.AcceptedAt == nil || !current.AcceptedAt.Equal(now) {
		t.Fatalf("institution accepted_at = %v, want %v", current.AcceptedAt, now)
	}
	if current.DueAt != nil {
		t.Fatalf("institution due_at = %v, want no 7-day deadline", current.DueAt)
	}

	storedPass, err := db.GetPassByID(context.Background(), "pass-umbrella")
	if err != nil {
		t.Fatalf("GetPassByID() error = %v", err)
	}
	if storedPass.Status != "revoked" {
		t.Fatalf("pass status = %q, want revoked", storedPass.Status)
	}
	if passes.callCount() != 0 {
		t.Fatalf("post-commit pass revoker calls = %d, want 0 (revocation is transactional)", passes.callCount())
	}
	assertAuditAction(t, db, "item-umbrella", "pass.revoked")

	indexed := indexer.indexedItems()
	if len(indexed) != 1 {
		t.Fatalf("indexed items = %d, want 1", len(indexed))
	}
	if indexed[0].ID != "item-umbrella" || indexed[0].Status != itemdomain.StatusTransferred {
		t.Fatalf("indexed item = %+v, want transferred umbrella", indexed[0])
	}

	if _, err := svc.Transfer(context.Background(), "item-umbrella", "institution-demo", now.Add(time.Hour)); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("second Transfer() error = %v, want conflict", err)
	}
}

func TestTransferConcurrentSingleWinner(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	cs := custody.New(db)
	if _, err := cs.Accept(context.Background(), "item-umbrella", "bar-x", now); err != nil {
		t.Fatalf("accept umbrella: %v", err)
	}
	if _, err := cs.MarkPickupDue(context.Background(), now.Add(8*24*time.Hour)); err != nil {
		t.Fatalf("mark pickup due: %v", err)
	}
	svc := institutiondomain.New(db)

	const attempts = 6
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.Transfer(context.Background(), "item-umbrella", "institution-demo", now)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, errs.ErrConflict):
			conflicts++
		default:
			t.Fatalf("concurrent Transfer() error = %v, want nil or conflict", err)
		}
	}
	if successes != 1 || conflicts != attempts-1 {
		t.Fatalf("transfers: successes=%d conflicts=%d, want 1/%d", successes, conflicts, attempts-1)
	}

	open := 0
	for _, id := range []string{"custody-umbrella"} {
		custodyRow, err := db.GetCustodyByID(context.Background(), id)
		if err != nil {
			t.Fatalf("GetCustodyByID(%q) error = %v", id, err)
		}
		if custodyRow.ClosedAt == nil {
			open++
		}
	}
	current, err := db.GetCustodyByItemID(context.Background(), "item-umbrella")
	if err != nil {
		t.Fatalf("GetCustodyByItemID() error = %v", err)
	}
	if current.ClosedAt == nil {
		open++
	}
	if open != 1 {
		t.Fatalf("open custody rows = %d, want exactly the institution row", open)
	}
}

func TestCreateOfficialItemStartsInstitutionCustody(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	indexer := &recordingSearch{Service: search.New(db, search.Deps{})}
	svc := institutiondomain.New(db, institutiondomain.Deps{
		Now:    func() time.Time { return now },
		Search: indexer,
	})

	created, err := svc.CreateOfficialItem(context.Background(), institutiondomain.CreateOfficialItemInput{
		InstitutionID: "institution-demo",
		ActorID:       "institution-demo",
		Category:      "umbrella",
		Color:         "blue",
		FoundZone:     "San Lorenzo",
		FoundNotes:    "handed in at the PA desk",
	})
	if err != nil {
		t.Fatalf("CreateOfficialItem() error = %v", err)
	}
	if created.Status != itemdomain.StatusInCustody {
		t.Fatalf("status = %q, want in_custody", created.Status)
	}
	if created.Source != itemdomain.SourceOperator {
		t.Fatalf("source = %q, want operator", created.Source)
	}
	if created.CustodyMode != itemdomain.CustodyModeInstitution {
		t.Fatalf("custody mode = %q, want institution", created.CustodyMode)
	}
	if created.CustodianID != "institution-demo" || created.LegalFinderID != "institution-demo" {
		t.Fatalf("created item = %+v, want institution as custodian and legal finder", created)
	}
	if created.LegalTermStart == nil || !created.LegalTermStart.Equal(now) {
		t.Fatalf("LegalTermStart = %v, want %v", created.LegalTermStart, now)
	}

	custodyRow, err := db.GetCustodyByItemID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetCustodyByItemID() error = %v", err)
	}
	if custodyRow.CustodianType != itemdomain.CustodyModeInstitution || custodyRow.CustodianID != "institution-demo" {
		t.Fatalf("custody = %+v, want institution", custodyRow)
	}
	if custodyRow.AcceptedAt == nil || !custodyRow.AcceptedAt.Equal(now) {
		t.Fatalf("accepted_at = %v, want %v", custodyRow.AcceptedAt, now)
	}
	if custodyRow.DueAt != nil {
		t.Fatalf("due_at = %v, want no deadline for official intake", custodyRow.DueAt)
	}

	events, err := db.ListCustodyEvents(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("ListCustodyEvents() error = %v", err)
	}
	if !hasEvent(events, institutiondomain.EventAccepted) {
		t.Fatalf("events = %v, want %q", eventTypes(events), institutiondomain.EventAccepted)
	}
	assertAuditAction(t, db, created.ID, institutiondomain.AuditActionOfficialIntake)

	indexed := indexer.indexedItems()
	if len(indexed) != 1 || indexed[0].ID != created.ID || indexed[0].Status != itemdomain.StatusInCustody {
		t.Fatalf("indexed official items = %+v, want the new in_custody item", indexed)
	}
}

func TestCreateOfficialItemRollsBackOnTransactionFailure(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	ctx := context.Background()

	before, err := db.ListItemsByCustodian(ctx, "institution-demo")
	if err != nil {
		t.Fatalf("ListItemsByCustodian(before) error = %v", err)
	}

	svc := institutiondomain.New(db)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := svc.CreateOfficialItem(canceled, institutiondomain.CreateOfficialItemInput{
		InstitutionID: "institution-demo",
		ActorID:       "institution-demo",
		Category:      "umbrella",
	}); err == nil {
		t.Fatal("CreateOfficialItem() with a canceled context returned nil error")
	}

	after, err := db.ListItemsByCustodian(ctx, "institution-demo")
	if err != nil {
		t.Fatalf("ListItemsByCustodian(after) error = %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("items after failed intake = %d, want %d (the transaction must roll back)", len(after), len(before))
	}
}

func TestOfficialItemCanEnterUnclaimedFlow(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	svc := institutiondomain.New(db, institutiondomain.Deps{
		Now: func() time.Time { return now },
	})

	created, err := svc.CreateOfficialItem(ctx, institutiondomain.CreateOfficialItemInput{
		InstitutionID: "institution-demo",
		ActorID:       "institution-demo",
		Category:      "umbrella",
	})
	if err != nil {
		t.Fatalf("CreateOfficialItem() error = %v", err)
	}
	if created.LegalTermStart == nil || !created.LegalTermStart.Equal(now) {
		t.Fatalf("legal term start = %v, want %v", created.LegalTermStart, now)
	}

	if _, err := svc.OfferUnclaimed(ctx, created.ID, now.AddDate(0, 11, 0)); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("early OfferUnclaimed() error = %v, want invalid input", err)
	}
	offered, err := svc.OfferUnclaimed(ctx, created.ID, now.AddDate(1, 0, 0))
	if err != nil {
		t.Fatalf("OfferUnclaimed(official item) error = %v", err)
	}
	if offered.Status != itemdomain.StatusOfferedToFinder || offered.OfferedToFinderAt == nil {
		t.Fatalf("offered official item = %+v, want offered_to_finder", offered)
	}
	disposed, err := svc.Dispose(ctx, created.ID, "institution-demo")
	if err != nil {
		t.Fatalf("Dispose(official item) error = %v", err)
	}
	if disposed.Status != itemdomain.StatusDisposed || disposed.DisposedAt == nil {
		t.Fatalf("disposed official item = %+v, want disposed", disposed)
	}
}

func TestReviewClaimAsInstitution(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := db.CreateClaim(ctx, record.ClaimRecord{
		ID:            "claim-review",
		ItemID:        "item-phone",
		SearcherID:    "searcher-demo",
		Status:        "under_review",
		ReturnChannel: "intermediary_direct",
		Confidence:    0.6,
		Evidence:      "[]",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	svc := institutiondomain.New(db)
	reviewed, err := svc.ReviewClaim(ctx, "claim-review", "approved", "institution-demo")
	if err != nil {
		t.Fatalf("ReviewClaim() error = %v", err)
	}
	if reviewed.Status != "approved" {
		t.Fatalf("status = %q, want approved", reviewed.Status)
	}
	if reviewed.ApprovalBy != "institution-demo" || reviewed.ApprovedAt == nil {
		t.Fatalf("reviewed claim = %+v, want institution approval", reviewed)
	}
	assertAuditAction(t, db, "claim-review", "claim.approved")

	if _, err := svc.ReviewClaim(ctx, "claim-review", "maybe", "institution-demo"); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("invalid decision error = %v, want invalid input", err)
	}
}

func TestUnclaimedOfferCollectDisposeOrder(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	cs := custody.New(db)
	if _, err := cs.Accept(ctx, "item-umbrella", "bar-x", now); err != nil {
		t.Fatalf("accept umbrella: %v", err)
	}
	if _, err := cs.MarkPickupDue(ctx, now.Add(8*24*time.Hour)); err != nil {
		t.Fatalf("mark pickup due: %v", err)
	}
	svc := institutiondomain.New(db)
	if _, err := svc.Transfer(ctx, "item-umbrella", "institution-demo", now); err != nil {
		t.Fatalf("transfer umbrella: %v", err)
	}

	walletRow, err := db.GetItemByID(ctx, "item-wallet")
	if err != nil {
		t.Fatalf("GetItemByID(wallet) error = %v", err)
	}
	walletTerm := *walletRow.LegalTermStart

	if _, err := svc.OfferUnclaimed(ctx, "item-wallet", walletTerm.AddDate(0, 6, 0)); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("early OfferUnclaimed() error = %v, want invalid input", err)
	}
	if _, err := svc.Dispose(ctx, "item-wallet", "institution-demo"); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Dispose() before offer error = %v, want conflict", err)
	}
	if _, err := svc.CollectByLegalFinder(ctx, "item-wallet", "bar-x"); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("Collect() before offer error = %v, want conflict", err)
	}

	offeredWallet, err := svc.OfferUnclaimed(ctx, "item-wallet", walletTerm.AddDate(1, 0, 0))
	if err != nil {
		t.Fatalf("OfferUnclaimed(wallet) error = %v", err)
	}
	if offeredWallet.Status != itemdomain.StatusOfferedToFinder || offeredWallet.OfferedToFinderAt == nil {
		t.Fatalf("offered wallet = %+v, want offered_to_finder", offeredWallet)
	}
	collected, err := svc.CollectByLegalFinder(ctx, "item-wallet", "bar-x")
	if err != nil {
		t.Fatalf("CollectByLegalFinder() error = %v", err)
	}
	if collected.Status != itemdomain.StatusFinderCollected {
		t.Fatalf("collected status = %q, want finder_collected", collected.Status)
	}
	if _, err := svc.CollectByLegalFinder(ctx, "item-wallet", "bar-x"); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("second Collect() error = %v, want conflict", err)
	}

	if _, err := svc.OfferUnclaimed(ctx, "item-umbrella", now.AddDate(1, 0, 0)); err != nil {
		t.Fatalf("OfferUnclaimed(umbrella) error = %v", err)
	}
	disposed, err := svc.Dispose(ctx, "item-umbrella", "institution-demo")
	if err != nil {
		t.Fatalf("Dispose() error = %v", err)
	}
	if disposed.Status != itemdomain.StatusDisposed || disposed.DisposedAt == nil {
		t.Fatalf("disposed item = %+v, want disposed_at", disposed)
	}
	if _, err := svc.Dispose(ctx, "item-umbrella", "institution-demo"); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("second Dispose() error = %v, want conflict", err)
	}
}

func eventTypes(events []record.CustodyEventRecord) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func hasEvent(events []record.CustodyEventRecord, want string) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}

func assertAuditAction(t *testing.T, db repository.Database, entityID, action string) {
	t.Helper()
	logs, err := db.ListAuditLogs(context.Background(), repository.AuditLogFilter{
		EntityID: entityID,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	for _, entry := range logs {
		if entry.Action == action {
			return
		}
	}
	t.Fatalf("audit actions for %q = %v, want %q", entityID, auditActions(logs), action)
}

func auditActions(logs []record.AuditLogRecord) []string {
	actions := make([]string, 0, len(logs))
	for _, entry := range logs {
		actions = append(actions, entry.Action)
	}
	return actions
}
