package routes

import (
	"context"

	agentdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
	feedomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/fee"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/health"
	institutiondomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/institution"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	nameservice "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/name"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	rewarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/reward"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	wizarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/wizard"
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

	// Auth is the domain service for session authentication.
	Auth auth.Service

	// Items is the domain service for item enrollment and reads.
	Items itemdomain.Service

	// Custody is the domain service for accept/refuse and pickup deadlines.
	Custody custodydomain.Service

	// Institution is the PA pickup, intake and unclaimed-item service.
	Institution institutiondomain.Service

	// Search is the hybrid search and index service.
	Search search.Service

	// Claims is the verification and claim service.
	Claims claimdomain.Service

	// Passes is the signed pickup-pass service.
	Passes passdomain.Service

	// Fees estimates PA custody fees and records mock payments.
	Fees feedomain.Service

	// Rewards records the legal finder's reward notice.
	Rewards rewarddomain.Service

	// Platform owns notifications, audit and the item timeline.
	Platform platform.Service

	// Agent is the searcher conversational orchestrator.
	Agent agentdomain.Service

	// Wizard is the finder photo-first enrollment service. Nil disables the
	// analyze/reverse/drop-points routes.
	Wizard wizarddomain.Service

	// DemoReset replaces the demo dataset. Nil disables the reset route.
	DemoReset func(context.Context) error

	// DemoResetSecret is the shared secret required by the reset route.
	DemoResetSecret string

	// chatLimiter enforces the per-searcher chat message rate.
	chatLimiter *chatLimiter

	// claimLimiter enforces the per-searcher claim creation rate.
	claimLimiter *chatLimiter

	// wizardLimiter bounds the expensive D3+DeepSeek analyze calls.
	wizardLimiter *chatLimiter
	// geoLimiter bounds reverse geocoding and drop-point lookups.
	geoLimiter *chatLimiter
}

// New creates a WebAPIHandler with the given domain services.
func New(
	names nameservice.ServiceName,
	health health.HealthService,
	authService auth.Service,
	items itemdomain.Service,
	custodyService custodydomain.Service,
	institutionService institutiondomain.Service,
	searchService search.Service,
	claimService claimdomain.Service,
	passService passdomain.Service,
	agentService agentdomain.Service,
	wizardService wizarddomain.Service,
	demoReset func(context.Context) error,
	demoResetSecret string,
	feeService feedomain.Service,
	rewardService rewarddomain.Service,
	platformService platform.Service,
) *WebAPIHandler {
	return &WebAPIHandler{
		Names:           names,
		Health:          health,
		Auth:            authService,
		Items:           items,
		Custody:         custodyService,
		Institution:     institutionService,
		Search:          searchService,
		Claims:          claimService,
		Passes:          passService,
		Agent:           agentService,
		Wizard:          wizardService,
		Fees:            feeService,
		Rewards:         rewardService,
		Platform:        platformService,
		DemoReset:       demoReset,
		DemoResetSecret: demoResetSecret,
		chatLimiter:     newChatLimiter(chatRateLimit, chatRateWindow),
		claimLimiter:    newChatLimiter(claimRateLimit, claimRateWindow),
		wizardLimiter:   newChatLimiter(wizardRateLimit, wizardRateWindow),
		geoLimiter:      newChatLimiter(geoRateLimit, wizardRateWindow),
	}
}
