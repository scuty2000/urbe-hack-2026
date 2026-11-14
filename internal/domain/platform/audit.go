package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// auditMetadataKeys is the allowlist of audit metadata fields. Audit entries
// describe what changed, never who the item belongs to: private profile values,
// document contents, exact coordinates, tokens and expected verification
// values are dropped.
var auditMetadataKeys = map[string]string{
	"itemid":         "itemId",
	"claimid":        "claimId",
	"passid":         "passId",
	"feeid":          "feeId",
	"paymentid":      "paymentId",
	"custodyid":      "custodyId",
	"rewardnoticeid": "rewardNoticeId",
	"institutionid":  "institutionId",
	"institution":    "institution",
	"intermediary":   "intermediary",
	"intermediaryid": "intermediaryId",
	"custodianid":    "custodianId",
	"searcherid":     "searcherId",
	"finderid":       "finderId",
	"legalfinderid":  "legalFinderId",
	"status":         "status",
	"from":           "from",
	"to":             "to",
	"source":         "source",
	"role":           "role",
	"action":         "action",
	"publiccode":     "publicCode",
	"amountcents":    "amountCents",
	"currency":       "currency",
	"sensitive":      "sensitive",
	"returnchannel":  "returnChannel",
	"dueat":          "dueAt",
	"expiresat":      "expiresAt",
	"revoked":        "revoked",
	"provider":       "provider",
	"paymentstatus":  "paymentStatus",
}

// Audit appends an entry to the append-only audit trail. It is the hook for
// actions that are not already audited inside a business transaction; existing
// transaction-local entries must not be duplicated.
func (s *service) Audit(
	ctx context.Context,
	actorID, action, entity, entityID string,
	metadata map[string]any,
) error {
	actorID = trimmed(actorID)
	action = trimmed(action)
	entity = trimmed(entity)
	entityID = trimmed(entityID)
	if actorID == "" || action == "" || entity == "" || entityID == "" {
		return fmt.Errorf("audit: actor, action, entity and entity id are required: %w", errs.ErrInvalidInput)
	}

	id, err := newID()
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	encoded, err := encodeMetadata(metadata)
	if err != nil {
		return fmt.Errorf("audit: %w", err)
	}

	if err := s.db.CreateAuditLog(ctx, record.AuditLogRecord{
		ID:        id,
		ActorID:   actorID,
		Action:    action,
		Entity:    entity,
		EntityID:  entityID,
		Metadata:  encoded,
		Timestamp: s.now(),
	}); err != nil {
		return fmt.Errorf("audit %q: %w", action, err)
	}
	return nil
}

func encodeMetadata(metadata map[string]any) (string, error) {
	safe := sanitizeValues(metadata, auditMetadataKeys)
	encoded, err := json.Marshal(safe)
	if err != nil {
		return "", fmt.Errorf("encoding audit metadata: %w", err)
	}
	return string(encoded), nil
}

// TryAudit calls Audit and logs a failed best-effort hook.
func TryAudit(
	ctx context.Context,
	auditor Auditor,
	operation, actorID, action, entity, entityID string,
	metadata map[string]any,
) {
	if auditor == nil {
		return
	}
	if err := auditor.Audit(ctx, actorID, action, entity, entityID, metadata); err != nil {
		logAuditFailure(operation, action, err)
	}
}

func logAuditFailure(operation, action string, err error) {
	slog.Default().Warn("audit hook failed",
		slog.String("operation", operation),
		slog.String("action", action),
		slog.Any("err", err),
	)
}
