package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-contrib/cors"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

func newNotificationRouter(t *testing.T) (http.Handler, repository.Database) {
	t.Helper()

	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	router, err := handler.New(handler.Config{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORS: cors.Config{
			AllowOrigins:     []string{"http://localhost:5173"},
			AllowMethods:     []string{http.MethodGet, http.MethodPost},
			AllowHeaders:     []string{"Content-Type"},
			AllowCredentials: true,
		},
		NameService:        stubNameService{},
		HealthService:      stubHealthService{},
		AuthService:        auth.New(db),
		ItemService:        itemdomain.New(db, itemdomain.Deps{}),
		CustodyService:     custodydomain.New(db),
		InstitutionService: newInstitutionService(db),
		SearchService:      search.New(db, search.Deps{}),
		ClaimService:       claimdomain.New(db),
		PassService:        passdomain.New(db, passdomain.Deps{PassSecret: "test-secret"}),
		FeeService:         newFeeService(db),
		RewardService:      newRewardService(db),
		PlatformService:    newPlatformService(db),
		AgentService:       agent.New(db, agent.Deps{Offline: true}),
	})
	if err != nil {
		t.Fatalf("handler.New() error = %v", err)
	}
	return router.Handler(), db
}

func TestNotificationsHTTPFlow(t *testing.T) {
	api, db := newNotificationRouter(t)

	if got := doRequest(t, api, http.MethodGet, "/api/notifications", "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated notifications status = %d, want 401", got)
	}
	if got := doRequest(t, api, http.MethodPost, "/api/notifications/x/read", "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated mark-read status = %d, want 401", got)
	}

	barX := loginAs(t, api, "bar-x")
	other := loginAs(t, api, "portineria-sapienza")

	empty := doRequest(t, api, http.MethodGet, "/api/notifications", "", barX)
	if empty.Code != http.StatusOK {
		t.Fatalf("empty notifications status = %d (body %s)", empty.Code, empty.Body.String())
	}
	var emptyList dto.NotificationList
	if err := json.NewDecoder(empty.Body).Decode(&emptyList); err != nil {
		t.Fatalf("decoding empty notifications: %v", err)
	}
	if emptyList.Notifications == nil || len(emptyList.Notifications) != 0 {
		t.Fatalf("empty notifications = %+v, want an empty array", emptyList)
	}

	notifier := platform.New(db)
	if err := notifier.Notify(context.Background(), "bar-x", platform.NotificationMatch, map[string]any{
		"itemId":         "item-phone",
		"privateProfile": map[string]any{"case_color": "blu"},
		"foundLat":       41.9,
	}); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	listed := doRequest(t, api, http.MethodGet, "/api/notifications", "", barX)
	if listed.Code != http.StatusOK {
		t.Fatalf("notifications status = %d (body %s)", listed.Code, listed.Body.String())
	}
	var list dto.NotificationList
	if err := json.NewDecoder(listed.Body).Decode(&list); err != nil {
		t.Fatalf("decoding notifications: %v", err)
	}
	if len(list.Notifications) != 1 {
		t.Fatalf("notifications = %+v, want 1", list)
	}
	notification := list.Notifications[0]
	if notification.ID == "" || notification.Type != platform.NotificationMatch {
		t.Fatalf("notification = %+v", notification)
	}
	if notification.Payload["itemId"] != "item-phone" {
		t.Fatalf("payload = %+v, want itemId", notification.Payload)
	}
	for _, forbidden := range []string{"privateProfile", "foundLat"} {
		if _, ok := notification.Payload[forbidden]; ok {
			t.Fatalf("payload leaks %q: %+v", forbidden, notification.Payload)
		}
	}

	otherList := doRequest(t, api, http.MethodGet, "/api/notifications", "", other)
	var otherBody dto.NotificationList
	if err := json.NewDecoder(otherList.Body).Decode(&otherBody); err != nil {
		t.Fatalf("decoding other notifications: %v", err)
	}
	if len(otherBody.Notifications) != 0 {
		t.Fatalf("other user sees %+v, want none", otherBody.Notifications)
	}

	if got := doRequest(t, api, http.MethodPost, "/api/notifications/"+notification.ID+"/read", "", other).Code; got != http.StatusForbidden {
		t.Fatalf("mark another user's notification = %d, want 403", got)
	}
	if got := doRequest(t, api, http.MethodPost, "/api/notifications/missing/read", "", barX).Code; got != http.StatusNotFound {
		t.Fatalf("mark missing notification = %d, want 404", got)
	}

	read := doRequest(t, api, http.MethodPost, "/api/notifications/"+notification.ID+"/read", "", barX)
	if read.Code != http.StatusOK {
		t.Fatalf("mark own notification = %d (body %s)", read.Code, read.Body.String())
	}
	var readBody dto.NotificationRead
	if err := json.NewDecoder(read.Body).Decode(&readBody); err != nil {
		t.Fatalf("decoding mark-read: %v", err)
	}
	if !readBody.Read || readBody.ID != notification.ID {
		t.Fatalf("mark-read response = %+v", readBody)
	}

	afterRead := doRequest(t, api, http.MethodGet, "/api/notifications", "", barX)
	var afterList dto.NotificationList
	if err := json.NewDecoder(afterRead.Body).Decode(&afterList); err != nil {
		t.Fatalf("decoding notifications after read: %v", err)
	}
	if len(afterList.Notifications) != 1 || !afterList.Notifications[0].Read || afterList.Notifications[0].ReadAt == nil {
		t.Fatalf("notifications after read = %+v, want one read notification", afterList.Notifications)
	}
}

func TestItemTimelineHTTPFlow(t *testing.T) {
	api, _ := newNotificationRouter(t)

	if got := doRequest(t, api, http.MethodGet, "/api/items/item-phone/timeline", "", nil).Code; got != http.StatusUnauthorized {
		t.Fatalf("unauthenticated timeline status = %d, want 401", got)
	}

	barX := loginAs(t, api, "bar-x")
	institution := loginAs(t, api, "institution-demo")
	finder := loginAs(t, api, "finder-demo")
	otherIntermediary := loginAs(t, api, "portineria-sapienza")
	searcher := loginAs(t, api, "searcher-demo")

	for name, cookie := range map[string]*http.Cookie{
		"intermediary": barX,
		"institution":  institution,
		"finder":       finder,
	} {
		recorder := doRequest(t, api, http.MethodGet, "/api/items/item-phone/timeline", "", cookie)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s timeline status = %d (body %s)", name, recorder.Code, recorder.Body.String())
		}
		body := recorder.Body.String()
		for _, forbidden := range []string{"privateProfile", "foundLat", "foundLng", "case_color", "CA00000AA"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s timeline leaks %q: %s", name, forbidden, body)
			}
		}

		var timeline dto.ItemTimeline
		if err := json.NewDecoder(strings.NewReader(body)).Decode(&timeline); err != nil {
			t.Fatalf("decoding %s timeline: %v", name, err)
		}
		if timeline.ItemID != "item-phone" || len(timeline.Events) != 2 {
			t.Fatalf("%s timeline = %+v, want item-phone with 2 events", name, timeline)
		}
		if timeline.Events[0].Type != "rights_assigned" || timeline.Events[1].Type != "accepted" {
			t.Fatalf("%s events = %+v, want rights_assigned then accepted", name, timeline.Events)
		}
		if timeline.Events[0].Timestamp.After(timeline.Events[1].Timestamp) {
			t.Fatalf("%s events are not chronological: %+v", name, timeline.Events)
		}
		if timeline.Events[0].ActorRole != "finder" {
			t.Fatalf("%s first actor role = %q, want finder", name, timeline.Events[0].ActorRole)
		}
	}

	for name, cookie := range map[string]*http.Cookie{
		"other intermediary": otherIntermediary,
		"searcher":           searcher,
	} {
		if got := doRequest(t, api, http.MethodGet, "/api/items/item-phone/timeline", "", cookie).Code; got != http.StatusForbidden {
			t.Fatalf("%s timeline status = %d, want 403", name, got)
		}
	}

	if got := doRequest(t, api, http.MethodGet, "/api/items/missing/timeline", "", institution).Code; got != http.StatusNotFound {
		t.Fatalf("missing item timeline status = %d, want 404", got)
	}
}
