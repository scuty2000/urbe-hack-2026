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
	for _, secret := range []string{"crepa", "gatto", "adesivo", "custodia"} {
		if strings.Contains(message, secret) {
			t.Fatalf("redaction did not use saw items: message %q still contains %q", message, secret)
		}
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
