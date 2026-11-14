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
	FlagPrivateMismatch     = "private_mismatch"
)

// FreeTextFields are private-profile keys that hold arbitrary prose. They are
// never used as verification questions: the model does not receive them and
// splitting them into expected values would make ordinary wording look like a
// secret. The wizard stores structured attributes for verification instead.
var FreeTextFields = map[string]bool{
	"notes": true, "note": true, "description": true, "descrizione": true,
	"detail": true, "details": true, "dettaglio": true,
	"comment": true, "comments": true,
}

// IsFreeTextField reports whether a normalized private-profile key is free
// text and must not become a verification question.
func IsFreeTextField(key string) bool {
	return FreeTextFields[strings.ToLower(strings.TrimSpace(key))]
}

// Probability bounds used to read one private answer as evidence.
const (
	// MatchProbability is the minimum probability of an ownership-grade match.
	MatchProbability = 0.75
	// MismatchProbability is the probability below which an answer is a clear
	// contradiction. Anything in between is undecided, not a mismatch.
	MismatchProbability = 0.25
)

// publicEvidenceKeys are listing attributes that are never shown to a
// searcher (search results expose only category, zone and time), so knowing
// them is ownership evidence: "una lattina di Red Bull zero zuccheri". Colour
// is left out on purpose: it is too easy to guess to count on its own.
var publicEvidenceKeys = []string{"brand", "model"}

// EvidenceKeys returns the keys whose answers count as ownership evidence: the
// private attributes plus the recorded brand and model.
func EvidenceKeys(attributes map[string]any, privateKeys map[string]bool) map[string]bool {
	keys := make(map[string]bool, len(privateKeys)+len(publicEvidenceKeys))
	for key := range privateKeys {
		keys[key] = true
	}
	for _, key := range publicEvidenceKeys {
		if value, ok := attributes[key]; ok && strings.TrimSpace(toString(value)) != "" {
			keys[key] = true
		}
	}
	return keys
}

// ApprovalKeys returns the keys of which at least one must match before an
// automatic approval. A low-risk item (a can, an umbrella) is approved on any
// evidence; anything more valuable needs a private detail, otherwise it goes
// to more_info or to in-person verification.
func ApprovalKeys(riskTier string, privateKeys, evidenceKeys map[string]bool) map[string]bool {
	if normalizeRisk(riskTier) == RiskLow {
		return evidenceKeys
	}
	return privateKeys
}

// PrivateConfidence averages the probabilities of the private attributes the
// searcher actually answered. Missing attributes are not mismatches: one
// correct high-signal detail is ownership evidence.
func PrivateConfidence(answers map[string]string, probabilities map[string]itemdomain.Answer, privateKeys map[string]bool) (float64, int) {
	sum := 0.0
	count := 0
	forEachPrivateAnswer(answers, probabilities, privateKeys, func(probability float64) {
		sum += probability
		count++
	})
	if count == 0 {
		return 0, 0
	}
	return sum / float64(count), count
}

// PrivateEvidence counts the answered private attributes that clearly matched
// and clearly contradicted the item profile.
func PrivateEvidence(answers map[string]string, probabilities map[string]itemdomain.Answer, privateKeys map[string]bool) (matches, mismatches int) {
	forEachPrivateAnswer(answers, probabilities, privateKeys, func(probability float64) {
		switch {
		case probability >= MatchProbability:
			matches++
		case probability < MismatchProbability:
			mismatches++
		}
	})
	return matches, mismatches
}

// PrivateEvidenceFlag turns the private evidence into the flag the policy
// understands. People misremember: a single wrong detail is not a rejection.
//
//   - right and wrong details together -> inconsistent_answers (human review);
//   - two or more wrong details and none right -> private_mismatch (reject);
//   - one wrong detail and none right -> no flag: the low confidence already
//     yields more_info, so the searcher can describe another detail.
func PrivateEvidenceFlag(matches, mismatches int) string {
	switch {
	case mismatches == 0:
		return ""
	case matches > 0:
		return FlagInconsistentAnswers
	case mismatches >= 2:
		return FlagPrivateMismatch
	default:
		return ""
	}
}

// HasPrivateMismatch reports at least one clear contradiction on a private,
// verifiable attribute.
func HasPrivateMismatch(answers map[string]string, probabilities map[string]itemdomain.Answer, privateKeys map[string]bool) bool {
	_, mismatches := PrivateEvidence(answers, probabilities, privateKeys)
	return mismatches > 0
}

func forEachPrivateAnswer(answers map[string]string, probabilities map[string]itemdomain.Answer, privateKeys map[string]bool, visit func(probability float64)) {
	if len(privateKeys) == 0 || len(answers) == 0 {
		return
	}
	for key := range privateKeys {
		if strings.TrimSpace(answers[key]) == "" {
			continue
		}
		answer, ok := probabilities[key]
		if !ok {
			continue
		}
		visit(AnswerProbability(answer))
	}
}

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
	// Context is what a human clerk would also look at. Only the DeepSeek
	// judge receives it: RedactedState (the D3 payload) leaves it out.
	Context JudgeContext
}

// JudgeContext is the judging material beyond the structured attributes:
// where and when the item was found, its free-text description and everything
// the searcher wrote. The statement is data to evaluate, never instructions.
type JudgeContext struct {
	FoundZone         string `json:"foundZone,omitempty"`
	FoundAt           string `json:"foundAt,omitempty"`
	ItemDescription   string `json:"itemDescription,omitempty"`
	SearcherStatement string `json:"searcherStatement,omitempty"`
}

// maxContextText bounds each free-text field sent to the judge.
const maxContextText = 1500

// ContextFor builds the judge context of an item and a searcher statement.
// The free-text description of a sensitive item (documents, cards) is never
// included: it may quote personal data.
func ContextFor(item itemdomain.Item, statement string) JudgeContext {
	context := JudgeContext{
		FoundZone:         strings.TrimSpace(item.FoundZone),
		SearcherStatement: truncate(strings.TrimSpace(statement), maxContextText),
	}
	if !item.FoundAt.IsZero() {
		context.FoundAt = item.FoundAt.UTC().Format("2006-01-02 15:04")
	}
	if !item.Sensitive && item.RiskTier != RiskSensitive {
		parts := make([]string, 0, 2)
		for _, key := range SortedKeys(freeTextQuestions(item.PrivateProfile)) {
			if text, ok := item.PrivateProfile[key].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		}
		context.ItemDescription = truncate(strings.Join(parts, " "), maxContextText)
	}
	return context
}

// freeTextQuestions lists the free-text keys of a profile as a question set,
// only to reuse SortedKeys for a deterministic order.
func freeTextQuestions(profile map[string]any) Questions {
	keys := Questions{}
	for key := range profile {
		if IsFreeTextField(key) {
			keys[key] = itemdomain.Question{}
		}
	}
	return keys
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}

// WithOverall folds the judge's overall ownership estimate into the evidence
// read from the single keys. The overall estimate uses everything the judge
// saw (statement, description, zone), so it can only raise the confidence; it
// counts as one matching piece of evidence when it is ownership-grade.
func WithOverall(result D3Result, confidence float64, matches int) (float64, int, bool) {
	if !result.HasOverall {
		return confidence, matches, false
	}
	overall := clamp(result.Overall)
	if overall > confidence {
		confidence = overall
	}
	if overall >= MatchProbability {
		return confidence, matches + 1, true
	}
	return confidence, matches, false
}

// GuardAutoApproval keeps an automatic approval only when a key that may
// approve matched, or when the overall estimate vouches for a low-risk item.
// Otherwise evidence without a private detail goes to in-person review, and
// no evidence at all asks for more information.
func GuardAutoApproval(policy PolicyDecision, riskTier string, approvalMatch, overallMatch bool, matches int) PolicyDecision {
	if policy.Decision != DecisionAutoApprove || approvalMatch {
		return policy
	}
	if overallMatch && normalizeRisk(riskTier) == RiskLow {
		return policy
	}
	if matches > 0 {
		policy.Decision = DecisionReview
		policy.Reasons = append(policy.Reasons, "evidence without a private detail; verify in person")
		return policy
	}
	policy.Decision = DecisionMoreInfo
	policy.Reasons = append(policy.Reasons, "no private attribute matched; more information required")
	return policy
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
	// Overall is the judge's estimate that the searcher owns the item, from
	// everything it saw. HasOverall is false for engines that do not give one.
	Overall    float64
	HasOverall bool
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
