/*
Package institution implements the public-administration (PA) side of the
lost-and-found lifecycle: the pickup queue at the intermediaries, the transfer
of custody to the institution, official operator intake, claim review and the
unclaimed-item flow (offer to the legal finder, collection or disposal).

The package is transport-free. The HTTP layer supplies the authenticated
operator and the current time, so every deadline and legal term is
deterministic in tests. Every write appends a custody event and an audit entry
in the same transaction as the state change.

Open pickup passes are revoked by the pass domain after a transfer; the search
index is refreshed from the returned item. Neither operation is part of the
transfer transaction, because both domains own their own writes.
*/
package institution

import (
	"time"
)

// Custody event types appended by this package. They complement the events
// owned by the custody, claim and pass domains.
const (
	EventAccepted        = "accepted"
	EventTransferred     = "transferred"
	EventOfferedToFinder = "offered_to_finder"
	EventFinderCollected = "finder_collected"
	EventDisposed        = "disposed"
)

// Audit actions written alongside the institution events. They are exported so
// tests and the audit timeline can assert the public-administration trail.
const (
	AuditActionOfficialIntake = "item.official_intake"
	AuditActionTransferred    = "item.transferred"
	AuditActionOffered        = "item.offered_to_finder"
	AuditActionCollected      = "item.finder_collected"
	AuditActionDisposed       = "item.disposed"
)

// PickupGroup is one intermediary/zone bucket of the PA pickup queue.
// Sensitive reports are always returned before ordinary ones.
type PickupGroup struct {
	IntermediaryID string
	Zone           string
	Sensitive      bool
	Items          []PickupItem
}

// PickupItem is the role-safe summary shown in the pickup queue. It never
// carries the private profile, the exact coordinates or free-text notes.
type PickupItem struct {
	ID             string
	PublicCode     string
	Category       string
	Brand          string
	Model          string
	Color          string
	Status         string
	FoundZone      string
	IntermediaryID string
	Sensitive      bool
	DueAt          *time.Time
	DaysOverdue    int
}

// CreateOfficialItemInput is the validated official intake command. The
// institution is both the legal finder and the custodian; the caller cannot
// override the source or the custody mode.
type CreateOfficialItemInput struct {
	// InstitutionID is the PA that takes the item into custody.
	InstitutionID string
	// ActorID is the operator performing the intake (event/audit actor).
	// It defaults to InstitutionID when empty.
	ActorID string

	Category       string
	Brand          string
	Model          string
	Color          string
	Condition      string
	PrivateProfile map[string]any
	Photos         []string

	FoundAt    time.Time
	FoundLat   float64
	FoundLng   float64
	FoundZone  string
	FoundNotes string
}
