package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

// TestRedactMessageUsesSeenItemsWithoutCandidates covers the turn that fetched a
// private profile but collected no search candidate in the reply: redaction
// must still run against every item touched by the turn.
func TestRedactMessageUsesSeenItemsWithoutCandidates(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	svc, ok := New(db, Deps{DeepSeekKey: ""}).(*service)
	if !ok {
		t.Fatal("New() did not return the concrete service")
	}

	turn := &turnState{
		allowed: map[string]bool{"item-phone": true},
		seen:    map[string]bool{"item-phone": true},
	}
	message := svc.redactMessage(context.Background(),
		"La custodia blu ha una crepa e un adesivo a forma di gatto.", turn)
	if message == "" {
		t.Fatal("redaction returned an empty message")
	}
	for _, secret := range []string{"blu", "gatto"} {
		if strings.Contains(message, secret) {
			t.Fatalf("redaction did not use saw items: message %q still contains %q", message, secret)
		}
	}
	// Free-text notes are never sent to the model, so their words must not
	// mangle the reply: the attribute names stay, only the structured values
	// are redacted.
	for _, public := range []string{"custodia", "crepa", "adesivo"} {
		if !strings.Contains(message, public) {
			t.Fatalf("over-redaction removed the ordinary word %q: %q", public, message)
		}
	}
}

func TestRedactMessageKeepsPublicObjectWords(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	svc, ok := New(db, Deps{DeepSeekKey: ""}).(*service)
	if !ok {
		t.Fatal("New() did not return the concrete service")
	}
	turn := &turnState{
		allowed: map[string]bool{"item-phone": true},
		seen:    map[string]bool{"item-phone": true},
	}
	// The screenshot scenario: the free-text note contains "orologio",
	// "watch" and "cinturino"; those are public object/attribute words and
	// must survive, while the structured case colour stays redacted.
	message := svc.redactMessage(context.Background(),
		"Ho trovato un orologio Apple Watch: che colore ha il cinturino? La custodia è blu.", turn)
	if !strings.Contains(message, "orologio") || !strings.Contains(message, "Apple Watch") || !strings.Contains(message, "cinturino") {
		t.Fatalf("over-redaction mangled the reply: %q", message)
	}
	if strings.Contains(message, "blu") {
		t.Fatalf("structured private value was not redacted: %q", message)
	}
}

func TestRedactMessageShortValueUsesWordBoundaries(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)

	svc, ok := New(db, Deps{DeepSeekKey: ""}).(*service)
	if !ok {
		t.Fatal("New() did not return the concrete service")
	}

	turn := &turnState{
		allowed: map[string]bool{"item-phone": true},
		seen:    map[string]bool{"item-phone": true},
	}
	got := svc.redactMessage(context.Background(),
		"Il colore è blu ma il bluetooth resta.", turn)
	want := "Il colore è " + redactionPlaceholder + " ma il bluetooth resta."
	if got != want {
		t.Fatalf("redaction = %q, want %q", got, want)
	}
	if strings.Contains(got, "è blu") {
		t.Fatalf("short private value was not redacted: %q", got)
	}
}
