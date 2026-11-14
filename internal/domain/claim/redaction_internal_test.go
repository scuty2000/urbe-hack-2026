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
			"case_color":       "blu",
		},
		Searcher: map[string]any{"color": "white"},
	}
	payload, err := json.Marshal(verification.RedactedState(state))
	if err != nil {
		t.Fatalf("marshalling redacted state: %v", err)
	}
	encoded := string(payload)
	for _, raw := range []string{"CA00000AA", "Mario Rossi", "333-1234567", "SN-1234"} {
		if strings.Contains(encoded, raw) {
			t.Fatalf("redacted D3 payload leaks %q: %s", raw, encoded)
		}
	}
	for _, boolean := range []string{`"number":true`, `"holder":true`, `"nome":true`, `"intestatario":true`, `"serial":true`} {
		if !strings.Contains(encoded, boolean) {
			t.Fatalf("redacted payload lost identity boolean %s: %s", boolean, encoded)
		}
	}
	if !strings.Contains(encoded, `"case_color":"blu"`) {
		t.Fatalf("redacted payload should keep discriminative attributes: %s", encoded)
	}
}

// TestExtendedIdentityProfileIsRedactedBeforeD3 covers the cross-task review
// profile: owner/via/recapito/serial must be excluded while case_color stays a
// discriminative attribute.
func TestExtendedIdentityProfileIsRedactedBeforeD3(t *testing.T) {
	profile := map[string]any{
		"owner":      "Mario Rossi",
		"via":        "Via Roma 1",
		"recapito":   "3331234567",
		"serial":     "SN12345",
		"case_color": "blu",
	}
	item := itemdomain.Item{
		Category:       "smartphone",
		Color:          "black",
		RiskTier:       itemdomain.RiskHigh,
		PrivateProfile: profile,
	}

	attributes, private := expectedAttributes(item)
	if got := attributes["case_color"]; got != "blu" {
		t.Fatalf("case_color = %v, want it kept as a discriminative attribute", got)
	}
	for _, key := range []string{"owner", "via", "recapito", "serial"} {
		if _, ok := attributes[key]; ok {
			t.Fatalf("expected attributes contain identity key %q: %+v", key, attributes)
		}
		if private[key] {
			t.Fatalf("identity key %q must not be a private question: %+v", key, private)
		}
	}
	if !private["case_color"] {
		t.Fatalf("case_color must remain private evidence: %+v", private)
	}

	// Even if legacy code sends the raw profile as the state, RedactedState
	// must replace every identity value with a boolean.
	payload, err := json.Marshal(verification.RedactedState(verification.State{
		Item:     profile,
		Searcher: map[string]any{"case_color": "blu"},
	}))
	if err != nil {
		t.Fatalf("marshalling redacted state: %v", err)
	}
	encoded := string(payload)
	for _, raw := range []string{"Mario Rossi", "Via Roma 1", "3331234567", "SN12345"} {
		if strings.Contains(encoded, raw) {
			t.Fatalf("redacted D3 payload leaks %q: %s", raw, encoded)
		}
	}
	if !strings.Contains(encoded, `"case_color":"blu"`) {
		t.Fatalf("redacted payload lost case_color: %s", encoded)
	}
	for _, key := range []string{"owner", "via", "recapito", "serial"} {
		if !strings.Contains(encoded, `"`+key+`":true`) {
			t.Fatalf("redacted payload lost identity boolean %q: %s", key, encoded)
		}
	}
}

func toStringForTest(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}
