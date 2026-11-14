package routes

import (
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/health"
	nameservice "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/name"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// HandlerFunc is the signature for all route handlers.
//
// Handlers receive a RequestContext that wraps Gin context plus per-request
// metadata (request UUID and request logger).
// Application-level dependencies are provided via the WebAPIHandler receiver.
type HandlerFunc func(*reqcontext.RequestContext)

// WebAPIHandler holds all application-level dependencies shared by route handlers.
// Per-request data arrives via the RequestContext parameter, not through this struct.
type WebAPIHandler struct {
	// Names is the domain service for Name CRUD operations.
	Names nameservice.ServiceName

	// Health is the domain service for liveness / readiness checks.
	Health health.HealthService
}

// New creates a WebAPIHandler with the given domain services.
func New(names nameservice.ServiceName, health health.HealthService) *WebAPIHandler {
	return &WebAPIHandler{
		Names:  names,
		Health: health,
	}
}
