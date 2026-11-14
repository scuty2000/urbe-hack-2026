package repository_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func timePtr(t time.Time) *time.Time { return &t }

// TestMigrationsCreateLostFoundTables guards the physical schema: every table
// the lost-and-found flow relies on must exist after the goose migrations run.
func TestMigrationsCreateLostFoundTables(t *testing.T) {
	db := testutil.NewTestDB(t)

	want := []string{
		"users",
		"sessions",
		"intermediaries",
		"items",
		"custody",
		"custody_events",
		"claims",
		"verification_sessions",
		"pickup_passes",
		"fees",
		"payments",
		"reward_notices",
		"notifications",
		"audit_logs",
		"lost_reports",
		"tariffs",
	}
	for _, table := range want {
		var name string
		err := db.GormDB().Raw(
			"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&name).Error
		if err != nil {
			t.Fatalf("querying sqlite_master for %q: %v", table, err)
		}
		if name != table {
			t.Errorf("table %q missing after migrations", table)
		}
	}

	// The query plans the MVP depends on must have their indexes.
	wantIndexes := []string{
		"idx_items_status_category_zone",
		"idx_custody_due_at",
		"idx_claims_item_id",
		"idx_pickup_passes_token_hash",
		"idx_sessions_expires_at",
	}
	for _, index := range wantIndexes {
		var name string
		err := db.GormDB().Raw(
			"SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?", index,
		).Scan(&name).Error
		if err != nil {
			t.Fatalf("querying sqlite_master for index %q: %v", index, err)
		}
		if name != index {
			t.Errorf("index %q missing after migrations", index)
		}
	}
}

func TestMigrationsDownRemovesLostFoundTables(t *testing.T) {
	db := testutil.NewTestDB(t)
	sqlDB, err := db.GormDB().DB()
	if err != nil {
		t.Fatal(err)
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate migration directory")
	}
	migrations := filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations")
	goose.SetBaseFS(os.DirFS(migrations))
	if err := goose.SetDialect("sqlite"); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownTo(sqlDB, ".", 0); err != nil {
		t.Fatalf("goose down: %v", err)
	}

	for _, table := range []string{"items", "custody", "users", "example_table"} {
		var count int
		if err := sqlDB.QueryRow(
			"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("table %q still present after goose down", table)
		}
	}
}

func TestCreateAndGetItem(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	rec := record.ItemRecord{
		ID:        "item-1",
		Status:    "pending_custody",
		Category:  "smartphone",
		RiskTier:  "high",
		CreatedAt: time.Now().UTC(),
	}
	if err := db.CreateItem(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetItemByID(ctx, "item-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "smartphone" {
		t.Fatalf("got %q", got.Category)
	}
	if got.Status != "pending_custody" || got.RiskTier != "high" {
		t.Fatalf("unexpected item: %+v", got)
	}
}

func TestItemRoundTripAndUpdate(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	foundAt := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)
	rightsAt := foundAt.Add(time.Minute)

	rec := record.ItemRecord{
		ID:               "item-phone",
		PublicCode:       "LF-2026-0001",
		Source:           "finder_self",
		CustodyMode:      "intermediary",
		Status:           "pending_custody",
		LegalFinderID:    "bar-x",
		RightsAssignedAt: &rightsAt,
		Category:         "smartphone",
		Brand:            "Apple",
		Model:            "iPhone 17",
		Color:            "nero",
		Condition:        "good",
		PrivateProfile:   `{"unique":"graffio in basso a sinistra"}`,
		Photos:           `["data/uploads/a.jpg"]`,
		FoundAt:          foundAt,
		FoundLat:         41.9,
		FoundLng:         12.5,
		FoundZone:        "San Lorenzo",
		FoundNotes:       "trovato sul marciapiede",
		CustodianID:      "bar-x",
		RiskTier:         "high",
		Sensitive:        true,
		CreatedAt:        foundAt,
		UpdatedAt:        foundAt,
	}
	if err := db.CreateItem(ctx, rec); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetItemByID(ctx, "item-phone")
	if err != nil {
		t.Fatal(err)
	}
	if got.Brand != "Apple" || got.Model != "iPhone 17" {
		t.Fatalf("brand/model not persisted: %+v", got)
	}
	if got.PrivateProfile != rec.PrivateProfile {
		t.Fatalf("private profile: got %q", got.PrivateProfile)
	}
	if !got.Sensitive {
		t.Fatal("sensitive flag not persisted")
	}
	if got.FoundAt.UTC() != foundAt {
		t.Fatalf("found_at: got %v want %v", got.FoundAt, foundAt)
	}
	if got.RightsAssignedAt == nil || !got.RightsAssignedAt.Equal(rightsAt) {
		t.Fatalf("rights_assigned_at: got %v want %v", got.RightsAssignedAt, rightsAt)
	}

	updatedAt := foundAt.Add(time.Hour)
	if err := db.UpdateItem(ctx, record.ItemRecord{
		ID:          "item-phone",
		Status:      "in_custody",
		CustodianID: "bar-x",
		UpdatedAt:   updatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetItemByID(ctx, "item-phone")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "in_custody" {
		t.Fatalf("status after update: %q", got.Status)
	}
	if !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated_at: got %v want %v", got.UpdatedAt, updatedAt)
	}
	// Fields not supplied to UpdateItem must be left untouched.
	if got.Brand != "Apple" {
		t.Fatalf("partial update wiped brand: %q", got.Brand)
	}
}

func TestSearchItemsAndListPickupDue(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

	seedItem := func(id, status, zone string) {
		t.Helper()
		if err := db.CreateItem(ctx, record.ItemRecord{
			ID: id, Status: status, Category: "smartphone", FoundZone: zone,
			FoundAt: now, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	seedItem("item-due", "in_custody", "San Lorenzo")
	seedItem("item-not-due", "in_custody", "San Lorenzo")
	seedItem("item-closed", "in_custody", "San Lorenzo")
	seedItem("item-transferred", "transferred", "San Lorenzo")

	seedCustody := func(id, itemID string, dueAt *time.Time, closedAt *time.Time) {
		t.Helper()
		if err := db.CreateCustody(ctx, record.CustodyRecord{
			ID: id, ItemID: itemID, CustodianType: "intermediary", CustodianID: "bar-x",
			DueAt: dueAt, ClosedAt: closedAt,
		}); err != nil {
			t.Fatal(err)
		}
	}
	seedCustody("custody-due", "item-due", timePtr(now.Add(-time.Hour)), nil)
	seedCustody("custody-not-due", "item-not-due", timePtr(now.Add(48*time.Hour)), nil)
	seedCustody("custody-closed", "item-closed", timePtr(now.Add(-time.Hour)), timePtr(now))
	seedCustody("custody-transferred", "item-transferred", timePtr(now.Add(-time.Hour)), nil)

	due, err := db.ListPickupDue(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != "item-due" {
		t.Fatalf("ListPickupDue: got %+v", due)
	}

	byZone, err := db.SearchItems(ctx, repository.ItemFilter{Zone: "San Lorenzo", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byZone) != 4 {
		t.Fatalf("SearchItems by zone: got %d items", len(byZone))
	}

	byStatus, err := db.SearchItems(ctx, repository.ItemFilter{Status: "in_custody", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byStatus) != 3 {
		t.Fatalf("SearchItems by status: got %d items", len(byStatus))
	}

	byText, err := db.SearchItems(ctx, repository.ItemFilter{Query: "smartphone", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byText) != 4 {
		t.Fatalf("SearchItems text search: got %d items", len(byText))
	}

	limited, err := db.SearchItems(ctx, repository.ItemFilter{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 {
		t.Fatalf("SearchItems limit: got %d items", len(limited))
	}

	byCustodian, err := db.ListItemsByCustodian(ctx, "bar-x")
	if err != nil {
		t.Fatal(err)
	}
	if len(byCustodian) != 0 {
		t.Fatalf("ListItemsByCustodian: got %d items", len(byCustodian))
	}
}

func TestCreateItemWithCustodyIsAtomic(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	dueAt := now.Add(7 * 24 * time.Hour)

	item := record.ItemRecord{
		ID: "item-umbrella", Status: "pending_custody", Category: "umbrella",
		LegalFinderID: "bar-x", CustodianID: "bar-x", FoundAt: now, CreatedAt: now, UpdatedAt: now,
	}
	custody := record.CustodyRecord{
		ID: "custody-umbrella", ItemID: "item-umbrella", CustodianType: "intermediary",
		CustodianID: "bar-x", LegalFinderID: "bar-x", MaxCustodyDays: 7,
		RightsAssignedAt: &now, DueAt: &dueAt,
	}
	event := record.CustodyEventRecord{
		ID: "event-rights", ItemID: "item-umbrella", Type: "rights_assigned", ActorID: "finder-demo",
	}

	if err := db.CreateItemWithCustody(ctx, item, custody, event); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetCustodyByItemID(ctx, "item-umbrella"); err != nil {
		t.Fatalf("custody missing after atomic create: %v", err)
	}
	events, err := db.ListCustodyEvents(ctx, "item-umbrella")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "rights_assigned" {
		t.Fatalf("events after atomic create: %+v", events)
	}

	// A failing custody insert must roll back the item and the event.
	bad := record.CustodyRecord{ID: "custody-umbrella", ItemID: "item-dup", CustodianType: "intermediary"}
	err = db.CreateItemWithCustody(ctx, record.ItemRecord{
		ID: "item-dup", Status: "pending_custody", CreatedAt: now, UpdatedAt: now,
	}, bad, record.CustodyEventRecord{ID: "event-dup", ItemID: "item-dup", Type: "enrolled"})
	if !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	if _, err := db.GetItemByID(ctx, "item-dup"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("item-dup should have been rolled back, got %v", err)
	}
}

func TestUsersAndSessions(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	users := []record.UserRecord{
		{ID: "finder-demo", Role: "finder", Name: "Finder", Email: "finder@example.com", CreatedAt: now},
		{ID: "bar-x", Role: "intermediary", Name: "Bar X", Email: "bar@example.com", CreatedAt: now},
	}
	for _, u := range users {
		if err := db.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.GetUserByID(ctx, "finder-demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Role != "finder" {
		t.Fatalf("role: %q", got.Role)
	}
	all, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListUsers: got %d", len(all))
	}

	expired := now.Add(-time.Hour)
	if err := db.CreateSession(ctx, record.SessionRecord{
		TokenHash: "hash-expired", UserID: "finder-demo", ExpiresAt: expired, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(ctx, record.SessionRecord{
		TokenHash: "hash-live", UserID: "finder-demo", ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	sess, err := db.GetSessionByTokenHash(ctx, "hash-live")
	if err != nil {
		t.Fatal(err)
	}
	if sess.UserID != "finder-demo" {
		t.Fatalf("session user: %q", sess.UserID)
	}

	// Duplicate token hashes are a conflict, not a silent overwrite.
	err = db.CreateSession(ctx, record.SessionRecord{
		TokenHash: "hash-live", UserID: "bar-x", ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	})
	if !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("want ErrConflict on duplicate session, got %v", err)
	}

	deleted, err := db.DeleteExpiredSessions(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("DeleteExpiredSessions: got %d want 1", deleted)
	}
	if _, err := db.GetSessionByTokenHash(ctx, "hash-expired"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("expired session still present: %v", err)
	}

	if err := db.DeleteSession(ctx, "hash-live"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSessionByTokenHash(ctx, "hash-live"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("deleted session still present: %v", err)
	}
	// Deleting an unknown session is idempotent (logout twice).
	if err := db.DeleteSession(ctx, "hash-live"); err != nil {
		t.Fatalf("second delete should be idempotent: %v", err)
	}
}

func TestIntermediaries(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	rec := record.IntermediaryRecord{
		ID: "bar-x", Name: "Bar X", Address: "Via dei Volsci 1", GeoLat: 41.9, GeoLng: 12.5,
		Hours: "8-20", Capacity: 20, Status: "active", AgreementRef: "conv-2026", Contact: "info@barx.it",
	}
	if err := db.CreateIntermediary(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateIntermediary(ctx, record.IntermediaryRecord{ID: "portineria-sapienza", Name: "Portineria"}); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetIntermediaryByID(ctx, "bar-x")
	if err != nil {
		t.Fatal(err)
	}
	if got.Capacity != 20 || got.AgreementRef != "conv-2026" {
		t.Fatalf("intermediary: %+v", got)
	}
	all, err := db.ListIntermediaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListIntermediaries: got %d", len(all))
	}
	if _, err := db.GetIntermediaryByID(ctx, "missing"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestCustodyLifecycleAndEvents(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

	if err := db.CreateItem(ctx, record.ItemRecord{
		ID: "item-wallet", Status: "in_custody", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	first := record.CustodyRecord{
		ID: "custody-1", ItemID: "item-wallet", CustodianType: "intermediary", CustodianID: "bar-x",
		MaxCustodyDays: 7, AcceptedAt: &now, DueAt: timePtr(now.Add(7 * 24 * time.Hour)),
	}
	if err := db.CreateCustody(ctx, first); err != nil {
		t.Fatal(err)
	}
	closed := now.Add(time.Hour)
	second := record.CustodyRecord{
		ID: "custody-2", ItemID: "item-wallet", CustodianType: "institution", CustodianID: "institution-demo",
		AcceptedAt: &closed,
	}
	if err := db.CreateCustody(ctx, second); err != nil {
		t.Fatal(err)
	}

	// GetCustodyByItemID returns the current (latest open) custody row.
	got, err := db.GetCustodyByItemID(ctx, "item-wallet")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "custody-2" {
		t.Fatalf("current custody: got %q", got.ID)
	}

	// Closing the newest row falls back to the previous open row.
	if err := db.UpdateCustody(ctx, record.CustodyRecord{ID: "custody-2", ClosedAt: &closed}); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetCustodyByItemID(ctx, "item-wallet")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "custody-1" {
		t.Fatalf("open custody: got %q", got.ID)
	}

	events := []record.CustodyEventRecord{
		{ID: "e-late", ItemID: "item-wallet", Type: "accepted", ActorID: "bar-x", Timestamp: now.Add(time.Minute)},
		{ID: "e-early", ItemID: "item-wallet", Type: "enrolled", ActorID: "finder-demo", Timestamp: now},
		{ID: "e-other", ItemID: "item-phone", Type: "enrolled", Timestamp: now},
	}
	for _, e := range events {
		if err := db.CreateCustodyEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	timeline, err := db.ListCustodyEvents(ctx, "item-wallet")
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 2 || timeline[0].Type != "enrolled" || timeline[1].Type != "accepted" {
		t.Fatalf("timeline order: %+v", timeline)
	}
}

func TestClaimsAndVerificationSessions(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

	if err := db.CreateItem(ctx, record.ItemRecord{ID: "item-phone", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	claim := record.ClaimRecord{
		ID: "claim-phone", ItemID: "item-phone", SearcherID: "searcher-demo",
		Status: "under_review", ReturnChannel: "intermediary_direct", Confidence: 0.82,
		Evidence: `["receipt"]`, CreatedAt: now,
	}
	if err := db.CreateClaim(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateClaim(ctx, record.ClaimRecord{
		ID: "claim-2", ItemID: "item-phone", SearcherID: "other", Status: "rejected", CreatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := db.GetClaimByID(ctx, "claim-phone")
	if err != nil {
		t.Fatal(err)
	}
	if got.Confidence != 0.82 || got.Evidence != `["receipt"]` {
		t.Fatalf("claim: %+v", got)
	}

	approvedAt := now.Add(2 * time.Hour)
	if err := db.UpdateClaim(ctx, record.ClaimRecord{
		ID: "claim-phone", Status: "approved", ApprovalBy: "bar-x", ApprovedAt: &approvedAt,
	}); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetClaimByID(ctx, "claim-phone")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "approved" || got.ApprovalBy != "bar-x" || got.ApprovedAt == nil {
		t.Fatalf("approved claim: %+v", got)
	}

	byItem, err := db.ListClaimsByItem(ctx, "item-phone")
	if err != nil {
		t.Fatal(err)
	}
	if len(byItem) != 2 || byItem[0].ID != "claim-2" {
		t.Fatalf("ListClaimsByItem: %+v", byItem)
	}
	bySearcher, err := db.ListClaimsBySearcher(ctx, "searcher-demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(bySearcher) != 1 || bySearcher[0].ID != "claim-phone" {
		t.Fatalf("ListClaimsBySearcher: %+v", bySearcher)
	}

	first := record.VerificationSessionRecord{
		ID: "vs-1", ClaimID: "claim-phone", Questions: `{"color":{}}`, Answers: `{}`,
		Probabilities: `{"color":0.9}`, Rationale: "partial match", ModelVersion: "d3-26b",
		Flags: `["manual_review"]`, CreatedAt: now,
	}
	second := record.VerificationSessionRecord{
		ID: "vs-2", ClaimID: "claim-phone", Questions: `{"color":{},"case":{}}`, Answers: `{}`,
		Probabilities: `{"color":0.95,"case":0.8}`, ModelVersion: "d3-26b",
		CreatedAt: now.Add(time.Minute),
	}
	for _, s := range []record.VerificationSessionRecord{first, second} {
		if err := db.CreateVerificationSession(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := db.GetVerificationSessionByClaimID(ctx, "claim-phone")
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != "vs-2" {
		t.Fatalf("latest verification session: %q", latest.ID)
	}
}

func TestPassIssueRevokeAndRedeem(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

	if err := db.CreateItem(ctx, record.ItemRecord{
		ID: "item-phone", Status: "in_custody", CustodianID: "bar-x", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateClaim(ctx, record.ClaimRecord{
		ID: "claim-phone", ItemID: "item-phone", SearcherID: "searcher-demo",
		Status: "approved", ReturnChannel: "intermediary_direct", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	pass := record.PassRecord{
		ID: "pass-1", ClaimID: "claim-phone", ItemID: "item-phone", SearcherID: "searcher-demo",
		CustodianID: "bar-x", TokenHash: "hash-1", IssuedAt: now, ExpiresAt: now.Add(30 * time.Minute),
		Status: "issued", IdentityRequired: true, ReturnChannel: "intermediary_direct",
	}
	if err := db.IssuePass(ctx, pass); err != nil {
		t.Fatal(err)
	}

	// Issuing a pass links it back to the claim atomically.
	claim, err := db.GetClaimByID(ctx, "claim-phone")
	if err != nil {
		t.Fatal(err)
	}
	if claim.PickupPassID != "pass-1" {
		t.Fatalf("claim pickup_pass_id: %q", claim.PickupPassID)
	}

	got, err := db.GetPassByTokenHash(ctx, "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IdentityRequired || got.CustodianID != "bar-x" {
		t.Fatalf("pass: %+v", got)
	}

	// A second pass on the same item is revoked when the custodian is replaced.
	second := pass
	second.ID = "pass-2"
	second.TokenHash = "hash-2"
	second.CustodianID = "institution-demo"
	if err := db.CreatePass(ctx, second); err != nil {
		t.Fatal(err)
	}
	revoked, err := db.RevokePassesByItem(ctx, "item-phone", "pass-2")
	if err != nil {
		t.Fatal(err)
	}
	if revoked != 1 {
		t.Fatalf("RevokePassesByItem: got %d want 1", revoked)
	}
	got, err = db.GetPassByTokenHash(ctx, "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "revoked" {
		t.Fatalf("revoked pass status: %q", got.Status)
	}

	// Redeeming updates pass, item and claim in one transaction.
	if err := db.RedeemPass(ctx, "pass-2", "item-phone", "claim-phone", now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetPassByTokenHash(ctx, "hash-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "redeemed" {
		t.Fatalf("redeemed pass status: %q", got.Status)
	}
	item, err := db.GetItemByID(ctx, "item-phone")
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != "returned" {
		t.Fatalf("item status after redeem: %q", item.Status)
	}
	claim, err = db.GetClaimByID(ctx, "claim-phone")
	if err != nil {
		t.Fatal(err)
	}
	if claim.Status != "handed_over" {
		t.Fatalf("claim status after redeem: %q", claim.Status)
	}

	// A pass is single-use at the storage layer too.
	if err := db.RedeemPass(ctx, "pass-2", "item-phone", "claim-phone", now.Add(6*time.Minute)); !errors.Is(err, errs.ErrConflict) {
		t.Fatalf("second redeem: want ErrConflict, got %v", err)
	}
}

func TestTimestampsAreNormalisedToUTC(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	cest := time.FixedZone("CEST", 2*60*60)
	foundAt := time.Date(2026, 11, 13, 12, 0, 0, 0, cest)

	if err := db.CreateItem(ctx, record.ItemRecord{
		ID: "item-utc", Status: "enrolled", FoundAt: foundAt, CreatedAt: foundAt, UpdatedAt: foundAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCustody(ctx, record.CustodyRecord{
		ID: "custody-utc", ItemID: "item-utc", DueAt: &foundAt,
	}); err != nil {
		t.Fatal(err)
	}

	var itemFoundAt time.Time
	if err := db.GormDB().Raw("SELECT found_at FROM items WHERE id = ?", "item-utc").Scan(&itemFoundAt).Error; err != nil {
		t.Fatal(err)
	}
	if itemFoundAt.Location() != time.UTC {
		t.Errorf("items.found_at not stored in UTC: %v (%v)", itemFoundAt, itemFoundAt.Location())
	}
	custody, err := db.GetCustodyByItemID(ctx, "item-utc")
	if err != nil {
		t.Fatal(err)
	}
	if custody.DueAt == nil || custody.DueAt.Location() != time.UTC {
		t.Errorf("custody.due_at not stored in UTC: %v", custody.DueAt)
	}

	// The round-tripped value still represents the same instant.
	got, err := db.GetItemByID(ctx, "item-utc")
	if err != nil {
		t.Fatal(err)
	}
	if !got.FoundAt.Equal(foundAt) {
		t.Fatalf("found_at round trip: got %v want %v", got.FoundAt, foundAt)
	}
}

func TestFeesPaymentsAndRewardNotices(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	fee := record.FeeRecord{
		ID: "fee-1", ItemID: "item-wallet", Applies: "pa_tariff", PATariffID: "tariff-1",
		CustodyDays: 9, AmountCents: 1500, Currency: "EUR", Status: "final",
	}
	if err := db.CreateFee(ctx, record.FeeRecord{
		ID: "fee-0", ItemID: "item-wallet", Applies: "none_direct_return", Status: "estimated",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateFee(ctx, fee); err != nil {
		t.Fatal(err)
	}
	gotFee, err := db.GetFeeByItemID(ctx, "item-wallet")
	if err != nil {
		t.Fatal(err)
	}
	if gotFee.ID != "fee-1" || gotFee.AmountCents != 1500 {
		t.Fatalf("current fee: %+v", gotFee)
	}
	if err := db.UpdateFee(ctx, record.FeeRecord{ID: "fee-1", Status: "paid"}); err != nil {
		t.Fatal(err)
	}
	gotFee, err = db.GetFeeByID(ctx, "fee-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotFee.Status != "paid" {
		t.Fatalf("fee status: %q", gotFee.Status)
	}

	paidAt := now
	payment := record.PaymentRecord{
		ID: "pay-1", FeeID: "fee-1", Provider: "mock", ProviderRef: "ref-1",
		Status: "paid", PaidAt: &paidAt, ReceiptURL: "/receipts/pay-1",
	}
	if err := db.CreatePayment(ctx, payment); err != nil {
		t.Fatal(err)
	}
	gotPay, err := db.GetPaymentByID(ctx, "pay-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotPay.Provider != "mock" || gotPay.PaidAt == nil {
		t.Fatalf("payment: %+v", gotPay)
	}
	byFee, err := db.GetPaymentByFeeID(ctx, "fee-1")
	if err != nil {
		t.Fatal(err)
	}
	if byFee.ID != "pay-1" {
		t.Fatalf("payment by fee: %+v", byFee)
	}

	reward := record.RewardNoticeRecord{
		ID: "reward-1", ItemID: "item-phone", LegalFinderID: "bar-x", Requested: true,
		AmountCents: 500, PaymentStatus: "outside_system",
	}
	if err := db.CreateRewardNotice(ctx, reward); err != nil {
		t.Fatal(err)
	}
	gotReward, err := db.GetRewardNoticeByItemID(ctx, "item-phone")
	if err != nil {
		t.Fatal(err)
	}
	if !gotReward.Requested || gotReward.AmountCents != 500 {
		t.Fatalf("reward notice: %+v", gotReward)
	}
	shownAt := now.Add(time.Minute)
	if err := db.UpdateRewardNotice(ctx, record.RewardNoticeRecord{
		ID: "reward-1", ShownToSearcherAt: &shownAt, PaymentStatus: "paid_directly",
	}); err != nil {
		t.Fatal(err)
	}
	gotReward, err = db.GetRewardNoticeByItemID(ctx, "item-phone")
	if err != nil {
		t.Fatal(err)
	}
	if gotReward.ShownToSearcherAt == nil || gotReward.PaymentStatus != "paid_directly" {
		t.Fatalf("updated reward notice: %+v", gotReward)
	}
}

func TestNotificationsAndAuditLogs(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

	first := record.NotificationRecord{
		ID: "n-1", RecipientID: "bar-x", Type: "match", Payload: `{"itemId":"item-phone"}`, CreatedAt: now,
	}
	second := record.NotificationRecord{
		ID: "n-2", RecipientID: "bar-x", Type: "pickup_due", Payload: `{}`, CreatedAt: now.Add(time.Minute),
	}
	for _, n := range []record.NotificationRecord{first, second} {
		if err := db.CreateNotification(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	list, err := db.ListNotificationsByUser(ctx, "bar-x")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "n-2" {
		t.Fatalf("notifications: %+v", list)
	}
	readAt := now.Add(2 * time.Minute)
	if err := db.MarkNotificationRead(ctx, "n-1", readAt); err != nil {
		t.Fatal(err)
	}
	list, err = db.ListNotificationsByUser(ctx, "bar-x")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range list {
		if n.ID == "n-1" && (n.ReadAt == nil || !n.ReadAt.Equal(readAt)) {
			t.Fatalf("read_at: %+v", n)
		}
	}

	audit := []record.AuditLogRecord{
		{ID: "a-1", ActorID: "bar-x", Action: "custody_accepted", Entity: "item", EntityID: "item-phone", Metadata: `{}`, Timestamp: now},
		{ID: "a-2", ActorID: "bar-x", Action: "pass_issued", Entity: "pass", EntityID: "pass-1", Metadata: `{}`, Timestamp: now.Add(time.Minute)},
		{ID: "a-3", ActorID: "institution-demo", Action: "transferred", Entity: "item", EntityID: "item-phone", Metadata: `{}`, Timestamp: now.Add(2 * time.Minute)},
	}
	for _, a := range audit {
		if err := db.CreateAuditLog(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	byEntity, err := db.ListAuditLogs(ctx, repository.AuditLogFilter{Entity: "item", EntityID: "item-phone"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byEntity) != 2 || byEntity[0].ID != "a-1" {
		t.Fatalf("audit by entity: %+v", byEntity)
	}
	byActor, err := db.ListAuditLogs(ctx, repository.AuditLogFilter{ActorID: "bar-x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byActor) != 2 {
		t.Fatalf("audit by actor: got %d", len(byActor))
	}
}

func TestTariffsAndLostReports(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	if err := db.CreateTariff(ctx, record.TariffRecord{
		ID: "tariff-old", DailyRateCents: 100, FixedFeeCents: 500, Currency: "EUR", Active: false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateTariff(ctx, record.TariffRecord{
		ID: "tariff-1", DailyRateCents: 200, FixedFeeCents: 700, Currency: "EUR", Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	active, err := db.GetActiveTariff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != "tariff-1" || active.DailyRateCents != 200 {
		t.Fatalf("active tariff: %+v", active)
	}

	reports := []record.LostReportRecord{
		{ID: "lr-1", SearcherID: "searcher-demo", Description: "iphone nero", Category: "smartphone", Location: "San Lorenzo", TimeRange: "ieri sera", Embedding: `[0.1,0.2]`, Status: "open"},
		{ID: "lr-2", SearcherID: "searcher-demo", Description: "ombrello", Category: "umbrella", Status: "closed"},
	}
	for _, r := range reports {
		if err := db.CreateLostReport(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	all, err := db.ListLostReports(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("ListLostReports all: got %d", len(all))
	}
	open, err := db.ListLostReports(ctx, "open")
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != "lr-1" {
		t.Fatalf("ListLostReports open: %+v", open)
	}
}

func TestNotFoundAndTransactionRollback(t *testing.T) {
	db := testutil.NewTestDB(t)
	ctx := context.Background()

	if _, err := db.GetItemByID(ctx, "missing"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("GetItemByID: want ErrNotFound, got %v", err)
	}
	if _, err := db.GetUserByID(ctx, "missing"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("GetUserByID: want ErrNotFound, got %v", err)
	}
	if err := db.UpdateItem(ctx, record.ItemRecord{ID: "missing", Status: "x"}); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("UpdateItem: want ErrNotFound, got %v", err)
	}
	if err := db.MarkNotificationRead(ctx, "missing", time.Now()); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("MarkNotificationRead: want ErrNotFound, got %v", err)
	}

	boom := errors.New("boom")
	err := db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		if err := tx.CreateItem(ctx, record.ItemRecord{ID: "item-tx", Status: "enrolled"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("transaction error: got %v", err)
	}
	if _, err := db.GetItemByID(ctx, "item-tx"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("transaction was not rolled back: %v", err)
	}
}
