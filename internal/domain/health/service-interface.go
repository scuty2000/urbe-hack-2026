package health

import "context"

// Status reports the operational status of the application dependencies.
// A component is true when its check succeeded or when the check is disabled.
type Status struct {
	OK         bool   `json:"ok"`
	DB         bool   `json:"db"`
	Qdrant     bool   `json:"qdrant"`
	Embeddings bool   `json:"embeddings"`
	// D3 reports that the decision engine is enabled and configured. It is a
	// configuration fact, not a liveness probe: D3 has no health endpoint.
	D3 bool `json:"d3"`
	// AgentMode is the configured conversational agent: "deepseek" when the
	// provider is enabled and keyed, "mock" otherwise. The UI uses it to avoid
	// claiming an AI mode the backend is not actually running.
	AgentMode string `json:"agentMode"`
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
