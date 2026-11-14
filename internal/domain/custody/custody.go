/*
Package custody implements the custody lifecycle of a found item: the
intermediary accepts or refuses the handover, the legal deadline is computed
(7 days, or 24 hours for sensitive items) and elapsed deadlines move the item
to a pickup-due status.

The package is transport-free: the HTTP layer supplies the authenticated actor
and the current time, so deadlines are deterministic in tests. Every transition
updates the item, the custody row, the custody event timeline and the audit log
inside a single transaction.
*/
package custody

import (
	"math"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// Custody event types appended by this package. They complement the
// enrollment events owned by the item domain.
const (
	EventAccepted        = "accepted"
	EventCustodyRejected = "custody_rejected"
	EventPickupScheduled = "pickup_scheduled"
)

// Audit actions written alongside the custody events.
const (
	auditActionAccepted        = "item.custody_accepted"
	auditActionRefused         = "item.custody_rejected"
	auditActionPickupScheduled = "item.pickup_scheduled"
)

// systemActor identifies transitions triggered by a scheduled job rather than
// by an authenticated user.
const systemActor = "system"

// Custody is the read model returned by Accept: the handover the intermediary
// just acknowledged, including the legal deadline.
type Custody struct {
	ID                string
	ItemID            string
	CustodianType     string
	CustodianID       string
	LegalFinderID     string
	ApprovalAuthority string
	MaxCustodyDays    int
	AcceptedAt        *time.Time
	DueAt             *time.Time
	ClosedAt          *time.Time
}

// custodyFromRecord maps a persisted custody row to the domain read model.
func custodyFromRecord(row record.CustodyRecord) Custody {
	return Custody{
		ID:                row.ID,
		ItemID:            row.ItemID,
		CustodianType:     row.CustodianType,
		CustodianID:       row.CustodianID,
		LegalFinderID:     row.LegalFinderID,
		ApprovalAuthority: row.ApprovalAuthority,
		MaxCustodyDays:    row.MaxCustodyDays,
		AcceptedAt:        row.AcceptedAt,
		DueAt:             row.DueAt,
		ClosedAt:          row.ClosedAt,
	}
}

// DaysLeft returns the whole days remaining before dueAt, rounding up so that
// any positive remainder still counts as a day. A missing or elapsed deadline
// yields 0.
func DaysLeft(dueAt *time.Time, now time.Time) int {
	if dueAt == nil {
		return 0
	}
	remaining := dueAt.Sub(now)
	if remaining <= 0 {
		return 0
	}
	return int(math.Ceil(remaining.Hours() / 24))
}
