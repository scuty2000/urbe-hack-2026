/*
Package sqlite centralises the SQLite connection string so the API server,
tests and future tooling share the same locking policy.

Two DSN parameters matter for the custody transitions:

  - `_txlock=immediate` makes database/sql start every transaction with
    `BEGIN IMMEDIATE`. A deferred transaction upgrades its read snapshot to a
    write lock only at the first UPDATE; under WAL the losing concurrent writer
    then fails with `SQLITE_BUSY_SNAPSHOT`, which `busy_timeout` does not retry.
  - `_busy_timeout=5000` makes lock acquisition wait instead of failing
    immediately when two writers overlap.

`_journal_mode=WAL` sets WAL on every new connection. The explicit PRAGMA calls
in main and testutil remain as backstops.
*/
package sqlite

import "strings"

const (
	txLockImmediate = "_txlock=immediate"
	busyTimeout     = "_busy_timeout=5000"
	journalMode     = "_journal_mode=WAL"
)

// DSN returns filename with the application's SQLite parameters appended.
// Filenames that already contain a query string keep it and receive the
// parameters as additional pairs.
func DSN(filename string) string {
	params := txLockImmediate + "&" + busyTimeout + "&" + journalMode
	switch {
	case strings.HasSuffix(filename, "?") || strings.HasSuffix(filename, "&"):
		return filename + params
	case strings.Contains(filename, "?"):
		return filename + "&" + params
	default:
		return filename + "?" + params
	}
}
