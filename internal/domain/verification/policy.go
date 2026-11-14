package verification

import (
	"strings"
)

const (
	// autoApproveThreshold is the minimum confidence for low/medium risk items.
	autoApproveThreshold = 0.75
	// highReviewThreshold is the minimum confidence for high risk items.
	highReviewThreshold = 0.85
)

// Decision applies the deterministic policy: probabilities + risk tier +
// evidence + flags. It never calls a model and never mutates state.
//
//   - low/medium with confidence >= 0.75 -> auto_approve;
//   - high with confidence >= 0.85 -> review;
//   - sensitive always -> review;
//   - everything below the threshold -> more_info;
//   - rejection/review flags override the confidence.
func Decision(riskTier string, confidence float64, evidence []string, flags []string) PolicyDecision {
	confidence = clamp(confidence)
	tier := normalizeRisk(riskTier)
	decision := PolicyDecision{Confidence: confidence}

	reasons := make([]string, 0, 4)
	forceReject := false
	forceReview := false
	for _, flag := range flags {
		switch normalizeFlag(flag) {
		case FlagPromptInjection:
			forceReject = true
			reasons = append(reasons, "prompt injection detected")
		case FlagIdentityMismatch:
			forceReject = true
			reasons = append(reasons, "identity document does not match")
		case FlagInconsistentAnswers:
			forceReview = true
			reasons = append(reasons, "answers are inconsistent")
		case FlagMissingEvidence:
			forceReview = true
			reasons = append(reasons, "required evidence is missing")
		case "":
			// Ignore blank flags.
		default:
			// An unknown flag is conservative: a human must look at it.
			forceReview = true
			reasons = append(reasons, "unrecognised verification flag")
		}
	}

	switch {
	case forceReject:
		decision.Decision = DecisionReject
	case forceReview:
		decision.Decision = DecisionReview
	case tier == RiskSensitive:
		decision.Decision = DecisionReview
		reasons = append(reasons, "sensitive item requires human review")
	case tier == RiskHigh:
		if confidence >= highReviewThreshold {
			decision.Decision = DecisionReview
			reasons = append(reasons, "high risk tier requires human review")
		} else {
			decision.Decision = DecisionMoreInfo
			reasons = append(reasons, "confidence below the high-risk threshold")
		}
	default: // low / medium
		if confidence >= autoApproveThreshold {
			decision.Decision = DecisionAutoApprove
			reasons = append(reasons, "confidence above the automatic threshold")
		} else {
			decision.Decision = DecisionMoreInfo
			reasons = append(reasons, "confidence below the automatic threshold")
		}
	}

	if len(evidence) == 0 && decision.Decision == DecisionAutoApprove {
		// Evidence is optional for low/medium auto-approval: the design only
		// mandates it for high-risk interviews, which are always reviewed.
		reasons = append(reasons, "no evidence provided")
	}

	decision.Reasons = reasons
	return decision
}

// normalizeRisk maps unknown or empty tiers to high: a missing classification
// must never make approval easier.
func normalizeRisk(riskTier string) string {
	switch strings.ToLower(strings.TrimSpace(riskTier)) {
	case RiskLow:
		return RiskLow
	case RiskMedium:
		return RiskMedium
	case RiskSensitive:
		return RiskSensitive
	case RiskHigh:
		return RiskHigh
	default:
		return RiskHigh
	}
}

// normalizeFlag lower-cases and replaces separators so "Prompt Injection",
// "prompt-injection" and "prompt_injection" are the same flag.
func normalizeFlag(flag string) string {
	normalized := strings.ToLower(strings.TrimSpace(flag))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return normalized
}

// identityKeys are the exact attribute names that identify a person and must
// never be sent to an external model in clear text.
var identityKeys = map[string]bool{
	"name":            true,
	"full_name":       true,
	"first_name":      true,
	"last_name":       true,
	"surname":         true,
	"owner_name":      true,
	"nome":            true,
	"cognome":         true,
	"holder":          true,
	"cardholder":      true,
	"intestatario":    true,
	"titolare":        true,
	"proprietario":    true,
	"dob":             true,
	"date_of_birth":   true,
	"birth_date":      true,
	"document":        true,
	"document_type":   true,
	"document_number": true,
	"documentnumber":  true,
	"number":          true,
	"numero":          true,
	"id_number":       true,
	"identity_number": true,
	"fiscal_code":     true,
	"codice_fiscale":  true,
	"tax_code":        true,
	"email":           true,
	"mail":            true,
	"phone":           true,
	"telefono":        true,
	"tel":             true,
	"address":         true,
	"indirizzo":       true,
	"place_of_birth":  true,
	"ssn":             true,
	"cf":              true,
}

// identityFragments make the denylist fail closed for compound and localised
// keys: "number" catches document_number and "numero_documento", "holder"
// catches card_holder, "nome" catches owner_nome, and so on. Unknown keys that
// contain one of these fragments are redacted rather than forwarded.
var identityFragments = []string{
	"name",
	"nome",
	"cognome",
	"holder",
	"intestatario",
	"titolare",
	"proprietar",
	"document",
	"documento",
	"number",
	"numero",
	"email",
	"mail",
	"phone",
	"telefon",
	"telephon",
	"mobile",
	"cellulare",
	"address",
	"indirizzo",
	"residenza",
	"contact",
	"contatt",
	"birth",
	"nascita",
	"fiscal",
	"fiscale",
	"ssn",
	"codice_fiscale",
}

// isIdentityField reports whether key describes a person rather than an item.
func isIdentityField(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	if identityKeys[normalized] {
		return true
	}
	for _, fragment := range identityFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

// IsIdentityField reports whether an attribute identifies a person and must be
// redacted (or excluded from questions) before any external model sees it.
func IsIdentityField(key string) bool { return isIdentityField(key) }

// RedactIdentity maps identity values to booleans. The raw value (name, date
// of birth, document number, ...) never leaves the process.
func RedactIdentity(values map[string]any) map[string]bool {
	redacted := make(map[string]bool, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		redacted[key] = hasValue(value)
	}
	return redacted
}

// hasValue reports whether a raw value is present.
func hasValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case bool:
		return v
	default:
		return true
	}
}

// RedactedState returns the JSON-ready map sent to D3: identity values are
// replaced by booleans and item/searcher attributes are split accordingly.
func RedactedState(state State) map[string]any {
	itemAttributes, itemIdentity := splitIdentity(state.Item)
	searcherAttributes, searcherIdentity := splitIdentity(state.Searcher)

	return map[string]any{
		"item":             itemAttributes,
		"searcher":         searcherAttributes,
		"itemIdentity":     RedactIdentity(itemIdentity),
		"searcherIdentity": RedactIdentity(searcherIdentity),
		"identityMatch":    state.IdentityMatch,
		"evidence":         state.Evidence,
		"riskTier":         state.RiskTier,
	}
}

// splitIdentity separates item attributes from person-identifying fields.
func splitIdentity(values map[string]any) (map[string]any, map[string]any) {
	attributes := make(map[string]any, len(values))
	identity := make(map[string]any, len(values))
	for key, value := range values {
		if isIdentityField(key) {
			identity[key] = value
			continue
		}
		attributes[key] = value
	}
	return attributes, identity
}
