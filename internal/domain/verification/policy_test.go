package verification_test

import (
	"testing"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
)

func TestLocalScoreMatchingAnswers(t *testing.T) {
	t.Parallel()

	state := verification.State{
		Item:     map[string]any{"color": "Nero", "case": "blu", "serial": "SN-1234"},
		Searcher: map[string]any{"color": " nero ", "case": "blu", "serial": "sn-1234"},
	}
	questions := verification.Questions{
		"color":  {Type: "noul"},
		"case":   {Type: "noul"},
		"serial": {Type: "score"},
	}

	result := verification.LocalScore(state, questions)

	if got := result.Answers["color"].Noul; got < 0.8 {
		t.Fatalf("color noul = %v, want a match probability >= 0.8", got)
	}
	if got := result.Answers["case"].Noul; got < 0.8 {
		t.Fatalf("case noul = %v, want a match probability >= 0.8", got)
	}
	if got := result.Answers["serial"].Score; got != 0.9 {
		t.Fatalf("serial score = %v, want 0.9 for an exact match", got)
	}
	if result.Confidence < 0.8 {
		t.Fatalf("confidence = %v, want >= 0.8", result.Confidence)
	}
	if result.ModelVersion != verification.ModelLocal {
		t.Fatalf("modelVersion = %q, want %q", result.ModelVersion, verification.ModelLocal)
	}
}

func TestLocalScoreMismatchingAnswers(t *testing.T) {
	t.Parallel()

	state := verification.State{
		Item:     map[string]any{"color": "nero"},
		Searcher: map[string]any{"color": "rosso"},
	}
	result := verification.LocalScore(state, verification.Questions{"color": {Type: "noul"}})
	if got := result.Answers["color"].Noul; got >= 0.5 {
		t.Fatalf("mismatch noul = %v, want < 0.5", got)
	}
	if result.Confidence >= 0.5 {
		t.Fatalf("confidence = %v, want < 0.5 for a mismatch", result.Confidence)
	}
}

func TestLocalScoreBooleanAndListAttributes(t *testing.T) {
	t.Parallel()

	state := verification.State{
		Item: map[string]any{
			"has_crack": true,
			"contents":  []any{"carta di identità", "bancomat"},
		},
		Searcher: map[string]any{
			"has_crack": "yes",
			"contents":  "bancomat",
		},
	}
	questions := verification.Questions{
		"has_crack": {Type: "noul"},
		"contents":  {Type: "choice"},
	}

	result := verification.LocalScore(state, questions)
	if got := result.Answers["has_crack"].Noul; got < 0.8 {
		t.Fatalf("has_crack noul = %v, want a yes/no match", got)
	}
	if got := result.Answers["contents"].Confidence; got < 0.8 {
		t.Fatalf("contents confidence = %v, want a list membership match", got)
	}
	if result.Confidence < 0.8 {
		t.Fatalf("confidence = %v, want >= 0.8", result.Confidence)
	}
}

func TestIsIdentityFieldFailsClosed(t *testing.T) {
	t.Parallel()

	identity := []string{
		"number", "holder", "numero_documento", "nome", "intestatario",
		"telefono", "contact_email", "documentNumber", "date_of_birth",
		"codice_fiscale", "residenza", "serial_number", "serial", "seriale",
		"owner", "proprietario", "via", "recapito", "imei", "targa", "plate",
		"iban", "conto", "account", "tessera", "patente", "username", "login",
		"nickname",
	}
	for _, key := range identity {
		if !verification.IsIdentityField(key) {
			t.Fatalf("IsIdentityField(%q) = false, want redacted", key)
		}
	}
	for _, key := range []string{"case_color", "has_crack", "sticker", "notes", "contents", "detail", "handle"} {
		if verification.IsIdentityField(key) {
			t.Fatalf("IsIdentityField(%q) = true, want a discriminative item attribute", key)
		}
	}
}

func TestLocalScoreScorePartialAndExact(t *testing.T) {
	t.Parallel()

	questions := verification.Questions{
		"exact":   {Type: "score"},
		"partial": {Type: "score"},
		"wrong":   {Type: "score"},
	}
	state := verification.State{
		Item: map[string]any{
			"exact":   "iPhone 17 Pro",
			"partial": "iPhone 17 Pro Max",
			"wrong":   "Samsung Galaxy",
		},
		Searcher: map[string]any{
			"exact":   "iphone 17 pro",
			"partial": "iPhone 17",
			"wrong":   "Nokia",
		},
	}

	result := verification.LocalScore(state, questions)
	if got := result.Answers["exact"].Score; got != 0.9 {
		t.Fatalf("exact score = %v, want 0.9", got)
	}
	if got := result.Answers["partial"].Score; got != 0.5 {
		t.Fatalf("partial score = %v, want 0.5", got)
	}
	if got := result.Answers["wrong"].Score; got != 0.1 {
		t.Fatalf("wrong score = %v, want 0.1", got)
	}
}

func TestDecisionThresholds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		riskTier   string
		confidence float64
		want       string
	}{
		{"low just above threshold", verification.RiskLow, 0.75, verification.DecisionAutoApprove},
		{"medium above threshold", verification.RiskMedium, 0.90, verification.DecisionAutoApprove},
		{"medium below threshold", verification.RiskMedium, 0.60, verification.DecisionMoreInfo},
		{"high above threshold", verification.RiskHigh, 0.90, verification.DecisionReview},
		{"high at threshold", verification.RiskHigh, 0.85, verification.DecisionReview},
		{"high below threshold", verification.RiskHigh, 0.50, verification.DecisionMoreInfo},
		{"sensitive always reviewed", verification.RiskSensitive, 0.99, verification.DecisionReview},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := verification.Decision(tc.riskTier, tc.confidence, nil, nil).Decision; got != tc.want {
				t.Fatalf("Decision(%q, %v) = %q, want %q", tc.riskTier, tc.confidence, got, tc.want)
			}
		})
	}
}

func TestDecisionFlagsForceReviewOrReject(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		flags []string
		want  string
	}{
		{"prompt injection", []string{verification.FlagPromptInjection}, verification.DecisionReject},
		{"identity mismatch", []string{verification.FlagIdentityMismatch}, verification.DecisionReject},
		{"inconsistent answers", []string{verification.FlagInconsistentAnswers}, verification.DecisionReview},
		{"missing evidence", []string{verification.FlagMissingEvidence}, verification.DecisionReview},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := verification.Decision(verification.RiskLow, 0.99, []string{"receipt"}, tc.flags)
			if got.Decision != tc.want {
				t.Fatalf("Decision with flags %v = %q, want %q", tc.flags, got.Decision, tc.want)
			}
			if len(got.Reasons) == 0 {
				t.Fatal("expected at least one reason for a flag-forced decision")
			}
		})
	}
}

func TestDecisionRejectWinsOverReview(t *testing.T) {
	t.Parallel()

	flags := []string{verification.FlagMissingEvidence, verification.FlagPromptInjection}
	if got := verification.Decision(verification.RiskLow, 0.99, nil, flags).Decision; got != verification.DecisionReject {
		t.Fatalf("decision = %q, want reject", got)
	}
}

func TestRedactIdentityKeepsOnlyBooleans(t *testing.T) {
	t.Parallel()

	redacted := verification.RedactIdentity(map[string]any{
		"name":           "Giulia Ferrari",
		"date_of_birth":  "1990-01-01",
		"documentNumber": "CA12345",
		"email":          "",
	})

	if !redacted["name"] || !redacted["date_of_birth"] || !redacted["documentNumber"] {
		t.Fatalf("redacted identity = %+v, want present fields true", redacted)
	}
	if redacted["email"] {
		t.Fatalf("empty email should map to false: %+v", redacted)
	}
	for key := range redacted {
		if _, ok := any(redacted[key]).(bool); !ok {
			t.Fatalf("redacted[%q] is not a bool", key)
		}
	}
}

func TestQuestionsUseD3Types(t *testing.T) {
	t.Parallel()

	var questions verification.Questions = map[string]itemdomain.Question{
		"color": {Type: "noul"},
	}
	if questions["color"].Type != "noul" {
		t.Fatalf("question type = %q", questions["color"].Type)
	}
}
