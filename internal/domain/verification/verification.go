/*
Package verification implements the deterministic verification engine used
before a claim can be approved: the risk tier, the probability that a searcher
really owns the item, the evidence collected and the resulting policy decision.

The package is transport-free and has no database dependency. The D3 decision
engine is reached through the item.Cataloger port, so tests can fake it without
any network access; when D3 is unavailable the deterministic local scorer keeps
the flow working.
*/
package verification

import (
	"fmt"
	"sort"
	"strings"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
)

// Risk tiers mirror item.RiskTier. They are repeated here because the policy
// is a verification concern and must stay usable without an item aggregate.
const (
	RiskLow       = itemdomain.RiskLow
	RiskMedium    = itemdomain.RiskMedium
	RiskHigh      = itemdomain.RiskHigh
	RiskSensitive = itemdomain.RiskSensitive
)

// Policy decisions. They are the only outcomes the claim service maps to a
// claim status; a decision never issues a pickup pass.
const (
	DecisionAutoApprove = "auto_approve"
	DecisionReview      = "review"
	DecisionReject      = "reject"
	DecisionMoreInfo    = "more_info"
)

// Flags that can force a decision regardless of the confidence score.
const (
	FlagPromptInjection     = "prompt_injection"
	FlagInconsistentAnswers = "inconsistent_answers"
	FlagMissingEvidence     = "missing_evidence"
	FlagIdentityMismatch    = "identity_mismatch"
)

// Model identifiers stored on every verification session.
const (
	ModelLocal = "local"
	ModelD3    = "d3"
)

// State is the verification state sent to D3 and to the local scorer.
//
// Item holds the expected attributes (never serialised to a searcher),
// Searcher holds the answers collected during the interview, IdentityMatch
// holds boolean document comparisons and Evidence lists the proof references.
type State struct {
	Item          map[string]any
	Searcher      map[string]any
	IdentityMatch map[string]bool
	Evidence      []string
	RiskTier      string
}

// Questions is the D3 question set keyed by attribute. The value type is the
// domain item.Question so the verification engine and the enrollment catalog
// share one D3 contract.
type Questions map[string]itemdomain.Question

// D3Result is the per-question probability output, normalised across the local
// scorer and D3 so the policy sees the same shape either way.
type D3Result struct {
	Answers      map[string]itemdomain.Answer
	Confidence   float64
	Rationale    string
	ModelVersion string
}

// PolicyDecision is the deterministic outcome of a verification session.
type PolicyDecision struct {
	Decision   string
	Confidence float64
	Reasons    []string
}

// SortedKeys returns the question keys in a deterministic order. It is useful
// for tests and for writing stable rationales.
func SortedKeys(questions Questions) []string {
	keys := make([]string, 0, len(questions))
	for key := range questions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// normalizeValue lower-cases and trims a scalar so comparisons are stable.
func normalizeValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(toString(value)))
}

// toString renders a scalar without importing fmt in every call site.
func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

// clamp bounds a probability to the [0, 1] interval.
func clamp(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}
