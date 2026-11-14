/*
Package claim implements the ownership claim lifecycle: verification scoring,
the deterministic policy, persistence of the claim and its verification
session, and the custodian review.

The package has no HTTP dependency. Searchers create claims, the custodian (or
the institution) reviews them, and every write appends an item custody event and
an audit entry inside the same transaction. Pickup passes belong to a later
task: a claim is never handed over here.
*/
package claim

import (
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// Claim statuses owned by this package. The full design state machine also has
// draft/searching/verifying/scheduled/handed_over/closed, which later tasks
// will add; the policy only produces the four outcomes below.
const (
	StatusSubmitted   = "submitted"
	StatusUnderReview = "under_review"
	StatusApproved    = "approved"
	StatusRejected    = "rejected"
	StatusMoreInfo    = "more_info"
)

// Return channels derived from the item state: a return before the transfer to
// the PA is a free direct handover, afterwards it follows the PA tariff.
const (
	ReturnChannelIntermediaryDirect = "intermediary_direct"
	ReturnChannelPA                 = "pa"
)

// itemStatusTransferred is the item state after the custody deadline. It is not
// part of the enrollment constants because only the PA phase uses it.
const itemStatusTransferred = "transferred"

// Review decisions accepted by ReviewClaim.
const (
	DecisionApproved = "approved"
	DecisionRejected = "rejected"
)

// Custody event types appended by this package.
const (
	EventClaimCreated = "claim_created"
)

// Actor roles used for authorization. They mirror auth.Role* without importing
// the auth package, keeping the domain dependency direction clean.
const (
	ActorRoleFinder       = "finder"
	ActorRoleSearcher     = "searcher"
	ActorRoleIntermediary = "intermediary"
	ActorRoleInstitution  = "institution"
	ActorRoleAdmin        = "admin"
)

// Actor is the authenticated caller reading or reviewing a claim.
type Actor struct {
	ID   string
	Role string
}

// Claim is the domain read model. Report is only populated for the institution;
// the custodian receives the safe Evidence/Flags summary and the searcher only
// the status, confidence band and return channel.
type Claim struct {
	ID             string
	ItemID         string
	SearcherID     string
	Status         string
	ReturnChannel  string
	Confidence     float64
	ConfidenceBand string
	// Evidence and Flags are the safe review signals shared with the custodian
	// and the institution. They never contain the item's private profile.
	Evidence   []string
	Flags      []string
	ApprovalBy string
	ApprovedAt *time.Time
	CreatedAt  time.Time
	Report     *Report
}

// Report is the full verification report. It contains the questions asked, the
// searcher's answers, the per-attribute probabilities, rationale, model version,
// evidence and flags. It never contains the item's private profile.
type Report struct {
	Questions     map[string]itemdomain.Question
	Answers       map[string]string
	Probabilities map[string]itemdomain.Answer
	Confidence    float64
	Rationale     string
	ModelVersion  string
	Evidence      []string
	Flags         []string
	Decision      string
	Reasons       []string
}

// Session is the server-side verification input. Questions may be supplied by
// the agent; when empty the service derives them from the item's discriminative
// attributes. Answers are the searcher's free-text responses.
type Session struct {
	Questions verification.Questions
	Answers   map[string]string
	Evidence  []string
	Flags     []string
	// Statement is everything the searcher wrote in the chat. The judge reads
	// it as context; it is never stored.
	Statement string
}

// CreateClaimInput is the validated creation command. ReturnChannel is
// optional: the service derives it from the item status and rejects conflicts.
type CreateClaimInput struct {
	ItemID        string
	SearcherID    string
	ReturnChannel string
	Session       Session
	Evidence      []string
	Flags         []string
}

// claimFromRecord maps a persisted claim row to the domain read model.
func claimFromRecord(row record.ClaimRecord) Claim {
	return Claim{
		ID:             row.ID,
		ItemID:         row.ItemID,
		SearcherID:     row.SearcherID,
		Status:         row.Status,
		ReturnChannel:  row.ReturnChannel,
		Confidence:     row.Confidence,
		ConfidenceBand: ConfidenceBand(row.Confidence),
		ApprovalBy:     row.ApprovalBy,
		ApprovedAt:     row.ApprovedAt,
		CreatedAt:      row.CreatedAt,
	}
}

// statusForDecision maps the deterministic policy outcome to a claim status.
func statusForDecision(decision string) string {
	switch decision {
	case verification.DecisionAutoApprove:
		return StatusApproved
	case verification.DecisionReview:
		return StatusUnderReview
	case verification.DecisionReject:
		return StatusRejected
	default:
		return StatusMoreInfo
	}
}

// ConfidenceBand turns a raw confidence into a coarse band for the searcher.
func ConfidenceBand(confidence float64) string {
	switch {
	case confidence >= 0.75:
		return "high"
	case confidence >= 0.50:
		return "medium"
	default:
		return "low"
	}
}

// returnChannelForStatus derives the only legitimate channel for an item state.
// A sensitive item is never returned directly by the intermediary: it must be
// transferred to the PA first, so it always follows the PA channel.
func returnChannelForStatus(itemStatus string, sensitive bool) (string, bool) {
	switch itemStatus {
	case itemdomain.StatusInCustody:
		if sensitive {
			return ReturnChannelPA, true
		}
		return ReturnChannelIntermediaryDirect, true
	case itemStatusTransferred:
		return ReturnChannelPA, true
	default:
		return "", false
	}
}
