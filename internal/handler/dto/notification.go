package dto

import (
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
)

// Notification is the role-safe representation of one in-app message. The
// payload is already sanitised by the platform service: it contains only IDs,
// statuses and amounts.
type Notification struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	Read      bool           `json:"read"`
	ReadAt    *time.Time     `json:"readAt,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}

// NotificationList is the response of GET /api/notifications.
type NotificationList struct {
	Notifications []Notification `json:"notifications"`
}

// NotificationRead is the response of POST /api/notifications/:id/read.
type NotificationRead struct {
	ID   string `json:"id"`
	Read bool   `json:"read"`
}

// TimelineEvent is the public view of one custody event. It never contains the
// item's private profile, document contents or exact coordinates.
type TimelineEvent struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Actor       string    `json:"actor"`
	ActorRole   string    `json:"actorRole,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	Notes       string    `json:"notes,omitempty"`
	EvidenceRef string    `json:"evidenceRef,omitempty"`
}

// ItemTimeline is the response of GET /api/items/:id/timeline.
type ItemTimeline struct {
	ItemID string          `json:"itemId"`
	Events []TimelineEvent `json:"events"`
}

// ToNotificationList maps the domain notifications onto the API response.
func ToNotificationList(notifications []platform.Notification) NotificationList {
	response := NotificationList{Notifications: make([]Notification, 0, len(notifications))}
	for _, notification := range notifications {
		response.Notifications = append(response.Notifications, Notification{
			ID:        notification.ID,
			Type:      notification.Type,
			Payload:   nonNilPayload(notification.Payload),
			Read:      notification.ReadAt != nil,
			ReadAt:    notification.ReadAt,
			CreatedAt: notification.CreatedAt,
		})
	}
	return response
}

// ToItemTimeline maps the domain events onto the API response.
func ToItemTimeline(itemID string, events []platform.CustodyEvent) ItemTimeline {
	response := ItemTimeline{ItemID: itemID, Events: make([]TimelineEvent, 0, len(events))}
	for _, event := range events {
		response.Events = append(response.Events, TimelineEvent{
			ID:          event.ID,
			Type:        event.Type,
			Actor:       event.Actor,
			ActorRole:   event.ActorRole,
			Timestamp:   event.Timestamp,
			Notes:       event.Notes,
			EvidenceRef: event.EvidenceRef,
		})
	}
	return response
}

func nonNilPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	return payload
}
