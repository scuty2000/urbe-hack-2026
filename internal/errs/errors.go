/*
Package errs defines application-level sentinel errors shared across all layers.

It is a leaf package: it has no internal imports, so both the domain and the
repository can import it without creating import cycles.

Dependency graph:

	handler    -> errs
	domain     -> errs
	repository -> errs   (no import of domain - direction stays clean)

Always check with errors.Is(), never with ==, because errors are typically
wrapped with fmt.Errorf("context: %w", errs.ErrNotFound).
*/
package errs

import "errors"

var (
	// ErrNotFound is returned when the requested resource does not exist.
	ErrNotFound = errors.New("not found")

	// ErrInvalidInput is returned when caller-supplied data fails validation.
	ErrInvalidInput = errors.New("invalid input")

	// ErrConflict is returned when an operation conflicts with the current
	// state of the resource (e.g., duplicate entry, optimistic lock failure).
	ErrConflict = errors.New("conflict")

	// ErrUnauthorized is returned when authentication is missing or invalid.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden is returned when the caller is authenticated but not allowed.
	ErrForbidden = errors.New("forbidden")

	// ErrUnavailable is returned when a required dependency is temporarily unavailable.
	ErrUnavailable = errors.New("unavailable")

	// ErrTimeout is returned when an operation exceeds its deadline.
	ErrTimeout = errors.New("timeout")

	// ErrInternal is returned for unexpected internal failures.
	ErrInternal = errors.New("internal error")
)
