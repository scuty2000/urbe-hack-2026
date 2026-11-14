package verification

import (
	"context"
	"encoding/json"
	"fmt"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
)

// D3Score asks the D3 decision engine for per-question probabilities.
//
// The state is converted to a redacted JSON document first: identity values
// (name, date of birth, document number, ...) are replaced by booleans, so the
// provider never receives personal data. If D3 is unavailable or returns an
// error, the deterministic local scorer answers instead; the caller can keep
// working without masking the degradation from the audit trail.
func D3Score(
	ctx context.Context,
	client itemdomain.Cataloger,
	state State,
	questions Questions,
	images []string,
) (D3Result, error) {
	if client == nil {
		return localFallback(state, questions, "d3 is not configured"), nil
	}

	payload, err := json.Marshal(RedactedState(state))
	if err != nil {
		return D3Result{}, fmt.Errorf("verification d3: encoding state: %w", err)
	}

	answers, err := client.Ask(ctx, string(payload), images, map[string]itemdomain.Question(questions))
	if err != nil {
		return localFallback(state, questions, "d3 unavailable"), nil
	}

	if answers == nil {
		answers = map[string]itemdomain.Answer{}
	}
	return D3Result{
		Answers:      answers,
		Confidence:   ConfidenceOf(questions, answers),
		Rationale:    "d3 judge",
		ModelVersion: ModelD3,
	}, nil
}

// localFallback runs the local scorer and records why D3 was not used.
func localFallback(state State, questions Questions, reason string) D3Result {
	result := LocalScore(state, questions)
	result.Rationale = reason + " (local fallback): " + result.Rationale
	return result
}
