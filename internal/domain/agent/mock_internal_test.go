package agent

import "testing"

// TestMockAnswersSeparatesColorFromCaseColor guards the demo phone flow: a
// sentence that mentions both the phone colour and the case colour must fill
// both answer keys. Assigning every colour to case_color leaves the public
// colour question unanswered and lowers the claim to "more_info".
func TestMockAnswersSeparatesColorFromCaseColor(t *testing.T) {
	answers := mockAnswers(
		"È un iPhone 17 nero con custodia blu, una crepa e un adesivo a forma di gatto",
	)

	if answers["category"] != "smartphone" {
		t.Fatalf("category = %q, want smartphone", answers["category"])
	}
	if answers["color"] != "nero" {
		t.Fatalf("color = %q, want nero", answers["color"])
	}
	if answers["case_color"] != "blu" {
		t.Fatalf("case_color = %q, want blu", answers["case_color"])
	}
	if answers["has_crack"] != "sì" {
		t.Fatalf("has_crack = %q, want sì", answers["has_crack"])
	}
	if answers["sticker"] != "gatto" {
		t.Fatalf("sticker = %q, want gatto", answers["sticker"])
	}
}

// TestMockAnswersAcceptsOnlyCaseColour keeps the short two-turn demo wording
// ("ha una custodia blu") working without inventing an object colour.
func TestMockAnswersAcceptsOnlyCaseColour(t *testing.T) {
	answers := mockAnswers("Ha una custodia blu")

	if answers["case_color"] != "blu" {
		t.Fatalf("case_color = %q, want blu", answers["case_color"])
	}
	if _, ok := answers["color"]; ok {
		t.Fatalf("color = %q, want no object colour", answers["color"])
	}
}
