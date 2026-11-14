package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// Notification types owned by this package. Every hook must use one of these
// values: clients render a known copy from the type and never from free text.
const (
	NotificationMatch           = "match"
	NotificationCustodyAccepted = "custody_accepted"
	NotificationPickupDue       = "pickup_due"
	NotificationPassIssued      = "pass_issued"
	NotificationHandover        = "handover"
	NotificationTransfer        = "transfer"
	NotificationSensitive       = "sensitive"
	NotificationFeePaid         = "fee_paid"
)

// NotificationLimit is the maximum number of notifications returned to one
// user in a single request.
const NotificationLimit = 50

// Notification is the role-safe read model of an in-app message.
type Notification struct {
	ID        string
	UserID    string
	Type      string
	Payload   map[string]any
	ReadAt    *time.Time
	CreatedAt time.Time
}

// payloadKeys is the allowlist of notification payload fields. It deliberately
// contains only identifiers, statuses and amounts: a private profile, document
// data, exact coordinates, tokens and expected verification values can never
// be stored even if a future caller passes them by mistake.
var payloadKeys = map[string]string{
	"itemid":         "itemId",
	"claimid":        "claimId",
	"passid":         "passId",
	"feeid":          "feeId",
	"paymentid":      "paymentId",
	"custodyid":      "custodyId",
	"rewardnoticeid": "rewardNoticeId",
	"institutionid":  "institutionId",
	"intermediaryid": "intermediaryId",
	"custodianid":    "custodianId",
	"searcherid":     "searcherId",
	"finderid":       "finderId",
	"legalfinderid":  "legalFinderId",
	"recipientid":    "recipientId",
	"status":         "status",
	"type":           "type",
	"role":           "role",
	"action":         "action",
	"amountcents":    "amountCents",
	"currency":       "currency",
	"dueat":          "dueAt",
	"expiresat":      "expiresAt",
	"returnchannel":  "returnChannel",
	"sensitive":      "sensitive",
	"channel":        "channel",
	"publiccode":     "publicCode",
}

// Notify persists an in-app notification. The recipient does not have to be an
// existing account here; the hooks use NotifyAccount when that matters.
func (s *service) Notify(
	ctx context.Context,
	userID, notificationType string,
	payload map[string]any,
) error {
	userID = trimmed(userID)
	notificationType = trimmed(notificationType)
	if userID == "" || notificationType == "" {
		return fmt.Errorf("notify: recipient and type are required: %w", errs.ErrInvalidInput)
	}

	id, err := newID()
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	encoded, err := encodePayload(payload)
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}

	if err := s.db.CreateNotification(ctx, record.NotificationRecord{
		ID:          id,
		RecipientID: userID,
		Type:        notificationType,
		Payload:     encoded,
		CreatedAt:   s.now(),
	}); err != nil {
		return fmt.Errorf("notify %q: %w", userID, err)
	}
	return nil
}

// NotifyAccount persists a notification only for a known account. A missing
// account (for example a finder without CIE/SPID) is skipped silently.
func (s *service) NotifyAccount(
	ctx context.Context,
	userID, notificationType string,
	payload map[string]any,
) error {
	userID = trimmed(userID)
	if userID == "" {
		return nil
	}
	if _, err := s.db.GetUserByID(ctx, userID); err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("notify account %q: %w", userID, err)
	}
	return s.Notify(ctx, userID, notificationType, payload)
}

// NotifyRole notifies every account with the given role. It is best-effort at
// the caller: all recipients are attempted and the first failure is returned.
func (s *service) NotifyRole(
	ctx context.Context,
	role, notificationType string,
	payload map[string]any,
) error {
	role = trimmed(role)
	if role == "" {
		return fmt.Errorf("notify role: role is required: %w", errs.ErrInvalidInput)
	}
	users, err := s.db.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("notify role %q: %w", role, err)
	}

	var firstErr error
	for _, user := range users {
		if user.Role != role {
			continue
		}
		if err := s.Notify(ctx, user.ID, notificationType, payload); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ListNotifications returns the user's own notifications, newest first and
// capped at NotificationLimit.
func (s *service) ListNotifications(ctx context.Context, userID string) ([]Notification, error) {
	userID = trimmed(userID)
	if userID == "" {
		return nil, fmt.Errorf("list notifications: user id is required: %w", errs.ErrInvalidInput)
	}

	rows, err := s.db.ListNotificationsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list notifications %q: %w", userID, err)
	}
	if len(rows) > NotificationLimit {
		rows = rows[:NotificationLimit]
	}

	notifications := make([]Notification, 0, len(rows))
	for _, row := range rows {
		notifications = append(notifications, notificationFromRecord(row))
	}
	return notifications, nil
}

// MarkRead marks a notification read. Only the recipient may do so.
func (s *service) MarkRead(ctx context.Context, notificationID, userID string) error {
	notificationID = trimmed(notificationID)
	userID = trimmed(userID)
	if notificationID == "" || userID == "" {
		return fmt.Errorf("mark notification read: id and user id are required: %w", errs.ErrInvalidInput)
	}

	row, err := s.db.GetNotificationByID(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("mark notification read %q: %w", notificationID, err)
	}
	if row.RecipientID != userID {
		return fmt.Errorf("mark notification read %q: recipient mismatch: %w", notificationID, errs.ErrForbidden)
	}
	if err := s.db.MarkNotificationRead(ctx, notificationID, s.now()); err != nil {
		return fmt.Errorf("mark notification read %q: %w", notificationID, err)
	}
	return nil
}

// notificationFromRecord maps a persisted row and re-applies the payload
// allowlist, so a legacy or hand-written row cannot leak private data either.
func notificationFromRecord(row record.NotificationRecord) Notification {
	return Notification{
		ID:        row.ID,
		UserID:    row.RecipientID,
		Type:      row.Type,
		Payload:   decodePayload(row.Payload),
		ReadAt:    row.ReadAt,
		CreatedAt: row.CreatedAt,
	}
}

// encodePayload sanitises and serialises a notification payload.
func encodePayload(payload map[string]any) (string, error) {
	safe := sanitizeValues(payload, payloadKeys)
	encoded, err := json.Marshal(safe)
	if err != nil {
		return "", fmt.Errorf("encoding notification payload: %w", err)
	}
	return string(encoded), nil
}

// decodePayload decodes a stored payload, dropping non-object data and any key
// outside the allowlist.
func decodePayload(encoded string) map[string]any {
	if strings.TrimSpace(encoded) == "" {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		return map[string]any{}
	}
	return sanitizeValues(decoded, payloadKeys)
}

// sanitizeValues keeps only allowlisted keys with scalar values. Nested
// objects, arrays and unsupported values are dropped: IDs, statuses and
// amounts are the only payload shapes the product needs.
func sanitizeValues(values map[string]any, allowlist map[string]string) map[string]any {
	safe := make(map[string]any, len(values))
	for key, value := range values {
		canonical, ok := allowlist[strings.ToLower(strings.TrimSpace(key))]
		if !ok {
			continue
		}
		if scalar, ok := scalarValue(value); ok {
			safe[canonical] = scalar
		}
	}
	return safe
}

func scalarValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		return typed, true
	case bool:
		return typed, true
	case int:
		return typed, true
	case int32:
		return typed, true
	case int64:
		return typed, true
	case float32:
		return typed, true
	case float64:
		return typed, true
	case json.Number:
		return typed, true
	case time.Time:
		return typed.UTC().Format(time.RFC3339), true
	default:
		return nil, false
	}
}

// TryNotifyAccount calls NotifyAccount and logs a failed best-effort hook. The
// domain services call it after their transaction has committed.
func TryNotifyAccount(
	ctx context.Context,
	notifier Notifier,
	operation, userID, notificationType string,
	payload map[string]any,
) {
	if notifier == nil || trimmed(userID) == "" {
		return
	}
	if err := notifier.NotifyAccount(ctx, userID, notificationType, payload); err != nil {
		slog.Default().Warn("notification hook failed",
			slog.String("operation", operation),
			slog.String("recipient", userID),
			slog.String("type", notificationType),
			slog.Any("err", err),
		)
	}
}

// TryNotifyRole calls NotifyRole and logs a failed best-effort hook.
func TryNotifyRole(
	ctx context.Context,
	notifier Notifier,
	operation, role, notificationType string,
	payload map[string]any,
) {
	if notifier == nil || trimmed(role) == "" {
		return
	}
	if err := notifier.NotifyRole(ctx, role, notificationType, payload); err != nil {
		slog.Default().Warn("notification role hook failed",
			slog.String("operation", operation),
			slog.String("role", role),
			slog.String("type", notificationType),
			slog.Any("err", err),
		)
	}
}

func newID() (string, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
