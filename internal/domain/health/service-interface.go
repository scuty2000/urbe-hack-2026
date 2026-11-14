package health

import "context"

// Status reports the operational status of the application dependencies.
// A component is true when its check succeeded or when the check is disabled.
type Status struct {
	OK         bool `json:"ok"`
	DB         bool `json:"db"`
	Qdrant     bool `json:"qdrant"`
	Embeddings bool `json:"embeddings"`
}

// HealthService reports the operational status of the application.
type HealthService interface {
	// Liveness returns a non-nil error when the application cannot serve requests
	// (e.g., the database is unreachable).
	Liveness(ctx context.Context) error

	// Readiness checks every optional dependency. It always returns a Status and
	// never fails the calling request.
	Readiness(ctx context.Context) Status
}

// Pinger is the minimal persistence contract needed for health checks.
type Pinger interface {
	Ping() error
}
