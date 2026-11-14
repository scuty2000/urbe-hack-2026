package platform_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	feedomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	institutiondomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/institution"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func TestNotifyAndAuditPersist(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := platform.New(db)
	ctx := context.Background()

	if err := svc.Notify(ctx, "bar-x", platform.NotificationMatch, map[string]any{
		"itemId": "item-phone",
		"status": "in_custody",
	}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if err := svc.Audit(ctx, "bar-x", "login", "user", "bar-x", map[string]any{
		"role": "intermediary",
	}); err != nil {
		t.Fatalf("Audit() error = %v", err)
	}

	notifications, err := svc.ListNotifications(ctx, "bar-x")
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifications))
	}
	if notifications[0].Type != platform.NotificationMatch {
		t.Fatalf("notification type = %q, want %q", notifications[0].Type, platform.NotificationMatch)
	}
	if notifications[0].Payload["itemId"] != "item-phone" {
		t.Fatalf("payload = %+v, want itemId", notifications[0].Payload)
	}

	audits, err := db.ListAuditLogs(ctx, repository.AuditLogFilter{Entity: "user", EntityID: "bar-x"})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(audits) != 1 || audits[0].Action != "login" {
		t.Fatalf("audits = %+v, want one login entry", audits)
	}
}

func TestListNotificationsNewestFirstAndCapped(t *testing.T) {
	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}

	current := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	svc := platform.New(db, platform.Deps{Now: func() time.Time {
		current = current.Add(time.Second)
		return current
	}})

	for i := 0; i < 55; i++ {
		if err := svc.Notify(context.Background(), "bar-x", platform.NotificationMatch, map[string]any{
			"status": fmt.Sprintf("%d", i),
		}); err != nil {
			t.Fatalf("Notify(%d) error = %v", i, err)
		}
	}

	notifications, err := svc.ListNotifications(context.Background(), "bar-x")
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(notifications) != platform.NotificationLimit {
		t.Fatalf("notifications = %d, want %d", len(notifications), platform.NotificationLimit)
	}
	if got := notifications[0].Payload["status"]; got != "54" {
		t.Fatalf("first notification status = %v, want 54", got)
	}
	if got := notifications[len(notifications)-1].Payload["status"]; got != "5" {
		t.Fatalf("last notification status = %v, want 5", got)
	}
	for i := 1; i < len(notifications); i++ {
		if notifications[i].CreatedAt.After(notifications[i-1].CreatedAt) {
			t.Fatalf("notifications are not newest first at %d: %v", i, notifications[i].CreatedAt)
		}
	}
}

func TestMarkReadOwnOnly(t *testing.T) {
	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}
	svc := platform.New(db)
	ctx := context.Background()

	if err := svc.Notify(ctx, "bar-x", platform.NotificationMatch, nil); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	notifications, err := svc.ListNotifications(ctx, "bar-x")
	if err != nil || len(notifications) != 1 {
		t.Fatalf("notifications = %+v, err = %v", notifications, err)
	}

	if err := svc.MarkRead(ctx, notifications[0].ID, "portineria-sapienza"); !errors.Is(err, errs.ErrForbidden) {
		t.Fatalf("MarkRead(other) error = %v, want ErrForbidden", err)
	}
	if err := svc.MarkRead(ctx, "missing", "bar-x"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("MarkRead(missing) error = %v, want ErrNotFound", err)
	}
	if err := svc.MarkRead(ctx, notifications[0].ID, "bar-x"); err != nil {
		t.Fatalf("MarkRead(own) error = %v", err)
	}

	notifications, err = svc.ListNotifications(ctx, "bar-x")
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if notifications[0].ReadAt == nil {
		t.Fatal("notification was not marked read")
	}
}

func TestNotificationPayloadDropsPrivateValues(t *testing.T) {
	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}
	svc := platform.New(db)
	ctx := context.Background()

	secrets := map[string]string{
		"caseColor":      "custodia blu con crepa",
		"documentNumber": "CA00000AA",
		"holder":         "Mario Rossi",
		"token":          "signed-secret-token",
		"expected":       "gatto",
	}
	if err := svc.Notify(ctx, "bar-x", platform.NotificationMatch, map[string]any{
		"itemId":         "item-phone",
		"status":         "in_custody",
		"privateProfile": map[string]any{"case_color": secrets["caseColor"]},
		"documentNumber": secrets["documentNumber"],
		"holder":         secrets["holder"],
		"foundLat":       41.9012,
		"foundLng":       12.5155,
		"token":          secrets["token"],
		"expected":       secrets["expected"],
	}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	notifications, err := svc.ListNotifications(ctx, "bar-x")
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifications))
	}
	for _, forbidden := range []string{
		"privateProfile", "private_profile", "documentNumber", "foundLat", "foundLng", "token", "expected", "holder",
	} {
		if _, ok := notifications[0].Payload[forbidden]; ok {
			t.Fatalf("payload leaks %q: %+v", forbidden, notifications[0].Payload)
		}
	}

	rows, err := db.ListNotificationsByUser(ctx, "bar-x")
	if err != nil || len(rows) != 1 {
		t.Fatalf("stored notifications = %+v, err = %v", rows, err)
	}
	for _, secret := range secrets {
		if strings.Contains(rows[0].Payload, secret) {
			t.Fatalf("stored payload leaks %q: %s", secret, rows[0].Payload)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rows[0].Payload), &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if payload["itemId"] != "item-phone" || payload["status"] != "in_custody" {
		t.Fatalf("payload lost safe fields: %+v", payload)
	}
}

func TestTimelineOrderAndPrivacy(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := platform.New(db)
	ctx := context.Background()

	events, err := svc.Timeline(ctx, "item-phone")
	if err != nil {
		t.Fatalf("Timeline() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if events[0].Type != "rights_assigned" || events[1].Type != "accepted" {
		t.Fatalf("events = %+v, want rights_assigned then accepted", events)
	}
	if events[0].Timestamp.After(events[1].Timestamp) {
		t.Fatalf("events are not chronological: %+v", events)
	}
	if events[0].ActorRole != "finder" {
		t.Fatalf("first actor role = %q, want finder", events[0].ActorRole)
	}
	if events[1].ActorRole != "intermediary" {
		t.Fatalf("second actor role = %q, want intermediary", events[1].ActorRole)
	}
	for _, event := range events {
		for _, private := range []string{"case_color", "CA00000AA", "custodia blu", "41.9"} {
			if strings.Contains(event.Notes, private) || strings.Contains(event.EvidenceRef, private) {
				t.Fatalf("timeline leaks %q: %+v", private, event)
			}
		}
	}

	if _, err := svc.Timeline(ctx, "missing-item"); !errors.Is(err, errs.ErrNotFound) {
		t.Fatalf("Timeline(missing) error = %v, want ErrNotFound", err)
	}
}

func TestTimelineForRoles(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	svc := platform.New(db)
	ctx := context.Background()

	allowed := []platform.Actor{
		{ID: "finder-demo", Role: "finder"},
		{ID: "bar-x", Role: "intermediary"},
		{ID: "institution-demo", Role: "institution"},
		{ID: "admin-demo", Role: "admin"},
	}
	for _, actor := range allowed {
		if _, err := svc.TimelineFor(ctx, "item-phone", actor); err != nil {
			t.Fatalf("TimelineFor(%+v) error = %v", actor, err)
		}
	}

	denied := []platform.Actor{
		{ID: "portineria-sapienza", Role: "intermediary"},
		{ID: "searcher-demo", Role: "searcher"},
	}
	for _, actor := range denied {
		if _, err := svc.TimelineFor(ctx, "item-phone", actor); !errors.Is(err, errs.ErrForbidden) {
			t.Fatalf("TimelineFor(%+v) error = %v, want ErrForbidden", actor, err)
		}
	}
}

func TestClaimCreatedNotifiesCustodianAndSearcher(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	notifier := platform.New(db)
	svc := claimdomain.New(db, claimdomain.Deps{Notifier: notifier})

	created, err := svc.CreateClaim(context.Background(), claimdomain.CreateClaimInput{
		ItemID:     "item-phone",
		SearcherID: "searcher-demo",
	})
	if err != nil {
		t.Fatalf("CreateClaim() error = %v", err)
	}

	assertNotification(t, db, "bar-x", platform.NotificationMatch)
	searcherNotification := assertNotification(t, db, "searcher-demo", platform.NotificationMatch)
	if searcherNotification.Payload["claimId"] != created.ID {
		t.Fatalf("searcher payload = %+v, want claim %s", searcherNotification.Payload, created.ID)
	}

	audits, err := db.ListAuditLogs(context.Background(), repository.AuditLogFilter{
		Entity: "claim", EntityID: created.ID,
	})
	if err != nil || len(audits) != 1 || audits[0].Action != "claim.created" {
		t.Fatalf("claim audits = %+v, err = %v, want one claim.created", audits, err)
	}
}

func TestPassIssueAndHandoverNotify(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	notifier := platform.New(db)
	feeService := feedomain.New(db)
	svc := passdomain.New(db, passdomain.Deps{
		PassSecret: "test-secret",
		Fees:       feeService,
		Notifier:   notifier,
	})
	now := time.Now().UTC()

	issued, err := svc.Issue(context.Background(), "claim-phone", "searcher-alt", now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	assertNotification(t, db, "bar-x", platform.NotificationPassIssued)

	if _, err := svc.Redeem(context.Background(), issued.Token, "bar-x", "bar-x", now.Add(time.Minute)); err != nil {
		t.Fatalf("Redeem() error = %v", err)
	}
	assertNotification(t, db, "bar-x", platform.NotificationHandover)
	assertNotification(t, db, "finder-demo", platform.NotificationHandover)

	audits, err := db.ListAuditLogs(context.Background(), repository.AuditLogFilter{
		Entity: "pass", EntityID: issued.ID,
	})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if countAudit(audits, "pass.issued") != 1 {
		t.Fatalf("pass.issued audits = %+v, want exactly one", audits)
	}
}

func TestFeePaidNotifiesInstitution(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	notifier := platform.New(db)
	svc := feedomain.New(db, feedomain.Deps{Notifier: notifier})

	fee, err := svc.Calculate(context.Background(), "item-wallet")
	if err != nil {
		t.Fatalf("Calculate() error = %v", err)
	}
	if _, err := svc.Pay(context.Background(), fee.ID); err != nil {
		t.Fatalf("Pay() error = %v", err)
	}

	notification := assertNotification(t, db, "institution-demo", platform.NotificationFeePaid)
	if notification.Payload["feeId"] != fee.ID {
		t.Fatalf("payload = %+v, want fee %s", notification.Payload, fee.ID)
	}
	if _, ok := notification.Payload["privateProfile"]; ok {
		t.Fatalf("fee notification leaks private data: %+v", notification.Payload)
	}
}

func TestTransferNotifiesIntermediary(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	notifier := platform.New(db)
	custodyService := custodydomain.New(db, custodydomain.Deps{Notifier: notifier})
	now := time.Now().UTC()

	if _, err := custodyService.Accept(context.Background(), "item-umbrella", "bar-x", now); err != nil {
		t.Fatalf("Accept() error = %v", err)
	}
	assertNotification(t, db, "bar-x", platform.NotificationCustodyAccepted)
	assertNotification(t, db, "finder-demo", platform.NotificationCustodyAccepted)

	if _, err := custodyService.MarkPickupDue(context.Background(), now.Add(8*24*time.Hour)); err != nil {
		t.Fatalf("MarkPickupDue() error = %v", err)
	}

	institutionService := institutiondomain.New(db, institutiondomain.Deps{Notifier: notifier})
	if _, err := institutionService.Transfer(
		context.Background(), "item-umbrella", "institution-demo", now.Add(8*24*time.Hour),
	); err != nil {
		t.Fatalf("Transfer() error = %v", err)
	}

	assertNotification(t, db, "bar-x", platform.NotificationPickupDue)
	assertNotification(t, db, "bar-x", platform.NotificationTransfer)

	audits, err := db.ListAuditLogs(context.Background(), repository.AuditLogFilter{
		Entity: "item", EntityID: "item-umbrella",
	})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if countAudit(audits, "item.transferred") != 1 {
		t.Fatalf("transfer audits = %+v, want exactly one item.transferred", audits)
	}
}

func TestSensitiveIntakeNotifiesInstitution(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	notifier := platform.New(db)
	svc := institutiondomain.New(db, institutiondomain.Deps{Notifier: notifier})

	created, err := svc.CreateOfficialItem(context.Background(), institutiondomain.CreateOfficialItemInput{
		InstitutionID: "institution-demo",
		ActorID:       "institution-demo",
		Category:      "document",
		FoundAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOfficialItem() error = %v", err)
	}
	if !created.Sensitive {
		t.Fatalf("created item is not sensitive: %+v", created)
	}

	notification := assertNotification(t, db, "institution-demo", platform.NotificationSensitive)
	if notification.Payload["itemId"] != created.ID {
		t.Fatalf("payload = %+v, want item %s", notification.Payload, created.ID)
	}
}

func TestLoginIsAudited(t *testing.T) {
	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}
	notifier := platform.New(db)
	svc := auth.New(db, auth.Deps{Auditor: notifier})

	if _, _, err := svc.Login(context.Background(), "bar-x"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	audits, err := db.ListAuditLogs(context.Background(), repository.AuditLogFilter{
		Entity: "user", EntityID: "bar-x",
	})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if countAudit(audits, "login") != 1 {
		t.Fatalf("login audits = %+v, want exactly one", audits)
	}
}

func TestNotifyAccountSkipsUnknownUsers(t *testing.T) {
	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}
	svc := platform.New(db)

	if err := svc.NotifyAccount(context.Background(), "no-such-account", platform.NotificationMatch, nil); err != nil {
		t.Fatalf("NotifyAccount(unknown) error = %v", err)
	}
	notifications, err := svc.ListNotifications(context.Background(), "no-such-account")
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(notifications) != 0 {
		t.Fatalf("notifications = %+v, want none", notifications)
	}
}

func TestNotifyRejectsUnknownType(t *testing.T) {
	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}
	svc := platform.New(db)

	err := svc.Notify(context.Background(), "bar-x", "bogus_type", nil)
	if !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("Notify(unknown type) error = %v, want ErrInvalidInput", err)
	}
}

func TestPostCommitHooksIgnoreCancelledRequestContext(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	hooks := &observingHooks{}
	platform.TryNotifyAccount(cancelled, hooks, "test", "bar-x",
		platform.NotificationMatch, map[string]any{"itemId": "item-phone"})
	platform.TryNotifyRole(cancelled, hooks, "test", platform.RoleInstitution,
		platform.NotificationPickupDue, nil)
	platform.TryAudit(cancelled, hooks, "test", "bar-x", "login", "user", "bar-x", nil)

	if !hooks.accountLive || !hooks.accountHasDeadline {
		t.Fatalf("TryNotifyAccount passed a cancelled/unbounded context: live=%v deadline=%v",
			hooks.accountLive, hooks.accountHasDeadline)
	}
	if !hooks.roleLive || !hooks.roleHasDeadline {
		t.Fatalf("TryNotifyRole passed a cancelled/unbounded context: live=%v deadline=%v",
			hooks.roleLive, hooks.roleHasDeadline)
	}
	if !hooks.auditLive || !hooks.auditHasDeadline {
		t.Fatalf("TryAudit passed a cancelled/unbounded context: live=%v deadline=%v",
			hooks.auditLive, hooks.auditHasDeadline)
	}
}

func TestPostCommitHooksPersistWithCancelledRequestContext(t *testing.T) {
	db := testutil.NewTestDB(t)
	if err := auth.SeedDemoUsers(context.Background(), db); err != nil {
		t.Fatalf("seeding demo users: %v", err)
	}
	svc := platform.New(db)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	platform.TryNotifyAccount(cancelled, svc, "test", "bar-x", platform.NotificationMatch,
		map[string]any{"itemId": "item-phone"})
	platform.TryAudit(cancelled, svc, "test", "bar-x", "login", "user", "bar-x", nil)

	notifications, err := svc.ListNotifications(context.Background(), "bar-x")
	if err != nil {
		t.Fatalf("ListNotifications() error = %v", err)
	}
	if len(notifications) != 1 {
		t.Fatalf("notifications = %+v, want one persisted row", notifications)
	}

	audits, err := db.ListAuditLogs(context.Background(), repository.AuditLogFilter{
		Entity: "user", EntityID: "bar-x",
	})
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if countAudit(audits, "login") != 1 {
		t.Fatalf("login audits = %+v, want one persisted row", audits)
	}
}

// observingHooks records the context each hook receives.
type observingHooks struct {
	accountLive        bool
	accountHasDeadline bool
	roleLive           bool
	roleHasDeadline    bool
	auditLive          bool
	auditHasDeadline   bool
}

func (o *observingHooks) Notify(context.Context, string, string, map[string]any) error {
	return nil
}

func (o *observingHooks) NotifyAccount(ctx context.Context, _, _ string, _ map[string]any) error {
	o.accountLive = ctx.Err() == nil
	_, o.accountHasDeadline = ctx.Deadline()
	return nil
}

func (o *observingHooks) NotifyRole(ctx context.Context, _, _ string, _ map[string]any) error {
	o.roleLive = ctx.Err() == nil
	_, o.roleHasDeadline = ctx.Deadline()
	return nil
}

func (o *observingHooks) Audit(ctx context.Context, _, _, _, _ string, _ map[string]any) error {
	o.auditLive = ctx.Err() == nil
	_, o.auditHasDeadline = ctx.Deadline()
	return nil
}

func assertNotification(t *testing.T, db repository.Database, userID, notificationType string) platform.Notification {
	t.Helper()

	svc := platform.New(db)
	notifications, err := svc.ListNotifications(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListNotifications(%q) error = %v", userID, err)
	}
	for _, notification := range notifications {
		if notification.Type == notificationType {
			return notification
		}
	}
	t.Fatalf("no %q notification for %q (got %+v)", notificationType, userID, notifications)
	return platform.Notification{}
}

func countAudit(entries []record.AuditLogRecord, action string) int {
	count := 0
	for _, entry := range entries {
		if entry.Action == action {
			count++
		}
	}
	return count
}
