package claim

import (
	"encoding/json"
	"strings"
	"testing"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
)

// TestDocumentProfileIsRedactedBeforeD3 is the regression for the seeded
// identity-card profile: neither the document number nor the holder name may
// reach the D3 payload.
func TestDocumentProfileIsRedactedBeforeD3(t *testing.T) {
	item := itemdomain.Item{
		Category:  "document",
		Color:     "white",
		Condition: "good",
		RiskTier:  itemdomain.RiskSensitive,
		Sensitive: true,
		PrivateProfile: map[string]any{
			"document_type": "identity_card",
			"number":        "CA00000AA",
			"holder":        "Mario Rossi",
		},
	}

	attributes, private := expectedAttributes(item)
	for _, raw := range []string{"CA00000AA", "Mario Rossi"} {
		if strings.Contains(toStringForTest(attributes), raw) {
			t.Fatalf("expected attributes leak %q: %+v", raw, attributes)
		}
	}
	if private["number"] || private["holder"] {
		t.Fatalf("identity fields must not become private questions: %+v", private)
	}

	// Even if an upstream caller puts raw identity values in the state (for
	// example legacy rows), RedactedState must fail closed before marshalling.
	state := verification.State{
		Item: map[string]any{
			"color":            "white",
			"number":           "CA00000AA",
			"holder":           "Mario Rossi",
			"numero_documento": "CA00000AA",
			"nome":             "Mario Rossi",
			"intestatario":     "Mario Rossi",
			"telefono":         "333-1234567",
			"serial":           "SN-1234",
		},
		Searcher: map[string]any{"color": "white"},
	}
	payload, err := json.Marshal(verification.RedactedState(state))
	if err != nil {
		t.Fatalf("marshalling redacted state: %v", err)
	}
	encoded := string(payload)
	for _, raw := range []string{"CA00000AA", "Mario Rossi", "333-1234567"} {
		if strings.Contains(encoded, raw) {
			t.Fatalf("redacted D3 payload leaks %q: %s", raw, encoded)
		}
	}
	for _, boolean := range []string{`"number":true`, `"holder":true`, `"nome":true`, `"intestatario":true`} {
		if !strings.Contains(encoded, boolean) {
			t.Fatalf("redacted payload lost identity boolean %s: %s", boolean, encoded)
		}
	}
	if !strings.Contains(encoded, `"serial":"SN-1234"`) {
		t.Fatalf("redacted payload should keep discriminative attributes: %s", encoded)
	}
}

func toStringForTest(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}
