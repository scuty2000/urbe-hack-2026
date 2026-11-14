/*
Package pass implements single-use pickup passes with a signed QR token.

A pass is only ever created from an approved claim. The token is a base64url
payload signed with HMAC-SHA256; the database stores only its SHA-256 hash, so a
database leak cannot be turned into a valid QR code. Validation is deliberately
fail-closed: without a configured secret no pass can be issued, validated or
redeemed.

The package has no HTTP dependency. The handler maps the returned domain errors
and reason codes onto the API response.
*/
package pass

import (
	"errors"
	"fmt"
	"time"
)

// Pass lifecycle statuses. `pending_payment` passes cannot be redeemed until
// the PA fee is settled; `expired` and `revoked` are terminal.
const (
	StatusPendingPayment = "pending_payment"
	StatusIssued         = "issued"
	StatusValidated      = "validated"
	StatusRedeemed       = "redeemed"
	StatusExpired        = "expired"
	StatusRevoked        = "revoked"
)

// Return channels derived from the item state. They mirror the claim package
// without importing it, so the two domains stay independent.
const (
	ReturnChannelIntermediaryDirect = "intermediary_direct"
	ReturnChannelPA                 = "pa"
)

// itemStatusTransferred is the item state after the custody deadline.
const itemStatusTransferred = "transferred"

// Custody event types appended by this package.
const (
	EventPassIssued = "pass_issued"
	EventHandover   = "handover"
)

// Audit actions appended by this package.
const (
	auditActionIssued   = "pass.issued"
	auditActionRedeemed = "pass.redeemed"
	auditActionRevoked  = "pass.revoked"
)

// Validation reasons returned to the scanner. They are deliberately coarse:
// they never reveal private profile data or exact coordinates.
const (
	ReasonInvalidToken     = "invalid_token"
	ReasonExpired          = "expired"
	ReasonWrongCustodian   = "wrong_custodian"
	ReasonUnpaid           = "unpaid"
	ReasonRevoked          = "revoked"
	ReasonAlreadyRedeemed  = "already_redeemed"
	ReasonItemNotInCustody = "item_not_in_custody"
	ReasonClaimNotApproved = "claim_not_approved"
)

// defaultTTL is used when the configuration does not provide PassTTL.
const defaultTTL = 30 * time.Minute

// ItemSummary is the only item view a pass exposes. It is a domain-level safe
// summary: no private profile, evidence or exact coordinates. The HTTP DTO maps
// this type directly, so no future caller can serialise the item aggregate by
// accident.
type ItemSummary struct {
	ID          string
	PublicCode  string
	Category    string
	Brand       string
	Model       string
	Color       string
	Status      string
	CustodianID string
}

// Pass is the issuing-side read model. Token is the raw signed value and is
// returned exactly once, to the claim owner; it is never stored or logged.
type Pass struct {
	ID               string
	ClaimID          string
	ItemID           string
	SearcherID       string
	CustodianID      string
	Token            string
	Status           string
	ReturnChannel    string
	IssuedAt         time.Time
	ExpiresAt        time.Time
	IdentityRequired bool
	Instructions     []string
	Item             *ItemSummary
}

// Validation is the result of scanning a token. Item is only populated for a
// successful validation and is the same safe summary returned with a pass.
type Validation struct {
	Valid         bool
	Reason        string
	PassID        string
	ClaimID       string
	ItemID        string
	CustodianID   string
	ReturnChannel string
	Status        string
	ExpiresAt     time.Time
	Item          *ItemSummary
}

// FeeStatus is the fee/payment snapshot used by Issue and Validate. Task 10
// replaces the default repository checker with the full fee service.
type FeeStatus struct {
	// Required reports that an amount is due before the handover.
	Required bool
	// Paid reports that the required amount has been settled.
	Paid bool
	// AmountCents is the outstanding/charged amount, for instructions only.
	AmountCents int64
	FeeID       string
	PaymentID   string
}

// ReasonError attaches a machine-readable validation reason to a sentinel
// error so the HTTP layer can return `{valid:false, reason}` while callers can
// still use errors.Is against errs.ErrForbidden / ErrConflict / ErrInvalidInput.
type ReasonError struct {
	Reason string
	Err    error
}

func (e *ReasonError) Error() string {
	if e.Reason == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("pass %s: %v", e.Reason, e.Err)
}

func (e *ReasonError) Unwrap() error { return e.Err }

// ReasonOf extracts the validation reason from an error chain.
func ReasonOf(err error) string {
	var reason *ReasonError
	if errors.As(err, &reason) {
		return reason.Reason
	}
	return ""
}

// instructions are the human-readable pickup notes returned with a pass. They
// contain the custodian id (needed to know where to go) but no item secrets.
func instructions(p Pass) []string {
	lines := []string{
		"Show this QR code at the pickup point: " + p.CustodianID,
		"This pass is single-use and expires at " + p.ExpiresAt.Format(time.RFC3339) + ".",
		"Do not share this code: anyone holding it can collect the item.",
	}
	if p.ReturnChannel == ReturnChannelPA {
		lines = append(lines, "The public administration fee must be paid before the handover.")
	}
	if p.IdentityRequired {
		lines = append(lines, "Bring a valid identity document for the custodian's check.")
	}
	return lines
}
