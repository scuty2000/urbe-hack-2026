package sqlite_test

import (
	"strings"
	"testing"

	platformsqlite "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/sqlite"
)

func TestDSNAppendsLockingParameters(t *testing.T) {
	dsn := platformsqlite.DSN("db/lostfound.db")

	if !strings.HasPrefix(dsn, "db/lostfound.db?") {
		t.Fatalf("DSN() = %q, want the filename followed by a query string", dsn)
	}
	for _, want := range []string{"_txlock=immediate", "_busy_timeout=5000", "_journal_mode=WAL"} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("DSN() = %q, missing %q", dsn, want)
		}
	}
}

func TestDSNKeepsExistingQueryString(t *testing.T) {
	dsn := platformsqlite.DSN("file:test.db?cache=shared")

	if !strings.HasPrefix(dsn, "file:test.db?cache=shared&") {
		t.Fatalf("DSN() = %q, want the existing query string preserved", dsn)
	}
	if strings.Contains(dsn, "??") || strings.Contains(dsn, "&&") {
		t.Fatalf("DSN() = %q has duplicated separators", dsn)
	}
	if !strings.Contains(dsn, "_txlock=immediate") {
		t.Fatalf("DSN() = %q missing _txlock=immediate", dsn)
	}
}

func TestDSNHandlesTrailingSeparator(t *testing.T) {
	dsn := platformsqlite.DSN("file:test.db?")

	if strings.Contains(dsn, "?&") {
		t.Fatalf("DSN() = %q has a dangling separator", dsn)
	}
	if !strings.Contains(dsn, "_busy_timeout=5000") {
		t.Fatalf("DSN() = %q missing _busy_timeout", dsn)
	}
}
