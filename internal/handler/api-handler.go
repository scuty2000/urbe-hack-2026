package handler

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/middleware"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/route"
)

// Handler registers all route and returns the http.Handler for the API.
// Call this method exactly once after New().
func (rt *_router) Handler() http.Handler {
	h := routes.New(
		rt.nameService,
		rt.healthService,
		rt.authService,
		rt.itemService,
		rt.custodyService,
		rt.institutionService,
		rt.searchService,
		rt.claimService,
		rt.passService,
		rt.agentService,
		rt.demoReset,
		rt.demoResetSecret,
		rt.feeService,
		rt.rewardService,
		rt.platformService,
	)

	rt.engine.GET("/names/:id", middleware.RequireRole(rt.authService, auth.RoleAdmin), rt.wrap(h.GetNameByID))
	rt.engine.POST("/names", middleware.RequireRole(rt.authService, auth.RoleAdmin), rt.wrap(h.PostNewName))
	rt.engine.PUT("/names/:id", middleware.RequireRole(rt.authService, auth.RoleAdmin), rt.wrap(h.PutNameByID))
	rt.engine.DELETE("/names/:id", middleware.RequireRole(rt.authService, auth.RoleAdmin), rt.wrap(h.DeleteNameByID))
	rt.engine.POST("/names/:id/replace", middleware.RequireRole(rt.authService, auth.RoleAdmin), rt.wrap(h.PostReplaceNameByID))
	rt.engine.GET("/healthz", rt.wrap(h.Liveness))
	rt.engine.GET("/api/health", rt.wrap(h.Readiness))

	api := rt.engine.Group("/api")
	api.POST("/auth/login", rt.wrap(h.PostLogin))
	api.POST("/auth/logout", rt.wrap(h.PostLogout))
	api.GET("/auth/me", middleware.RequireAuth(rt.authService), rt.wrap(h.GetMe))

	api.GET("/notifications",
		middleware.RequireAuth(rt.authService),
		rt.wrap(h.GetNotifications))
	api.POST("/notifications/:id/read",
		middleware.RequireAuth(rt.authService),
		rt.wrap(h.PostNotificationRead))

	api.POST("/claims",
		middleware.RequireRole(rt.authService, auth.RoleSearcher),
		rt.wrap(h.PostClaim))
	api.POST("/chat",
		middleware.RequireRole(rt.authService, auth.RoleSearcher),
		rt.wrap(h.PostChat))
	api.GET("/claims/:id",
		middleware.RequireAnyRole(rt.authService, auth.RoleSearcher, auth.RoleIntermediary, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.GetClaim))
	api.POST("/claims/:id/review",
		middleware.RequireAnyRole(rt.authService, auth.RoleIntermediary, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.PostClaimReview))

	api.POST("/passes",
		middleware.RequireRole(rt.authService, auth.RoleSearcher),
		rt.wrap(h.PostPass))
	api.POST("/passes/validate",
		middleware.RequireAnyRole(rt.authService, auth.RoleIntermediary, auth.RoleInstitution),
		rt.wrap(h.PostPassValidate))
	api.POST("/passes/redeem",
		middleware.RequireAnyRole(rt.authService, auth.RoleIntermediary, auth.RoleInstitution),
		rt.wrap(h.PostPassRedeem))

	api.GET("/items/:id/fee",
		middleware.RequireAnyRole(rt.authService, auth.RoleSearcher, auth.RoleIntermediary, auth.RoleInstitution),
		rt.wrap(h.GetItemFee))
	api.POST("/fees/:id/pay",
		middleware.RequireRole(rt.authService, auth.RoleSearcher),
		rt.wrap(h.PostFeePay))
	api.POST("/items/:id/reward-notice",
		middleware.RequireRole(rt.authService, auth.RoleIntermediary),
		rt.wrap(h.PostRewardNotice))
	api.GET("/items/:id/reward-notice",
		middleware.RequireAnyRole(rt.authService, auth.RoleSearcher, auth.RoleIntermediary, auth.RoleInstitution),
		rt.wrap(h.GetRewardNotice))

	api.POST("/items",
		middleware.RequireAnyRole(rt.authService, auth.RoleFinder, auth.RoleIntermediary),
		rt.wrap(h.PostItem))
	api.GET("/items",
		middleware.RequireRole(rt.authService, auth.RoleIntermediary),
		rt.wrap(h.ListItems))
	api.GET("/items/:id",
		middleware.RequireAnyRole(rt.authService, auth.RoleIntermediary, auth.RoleInstitution),
		rt.wrap(h.GetItem))
	api.GET("/items/:id/timeline",
		middleware.RequireAnyRole(rt.authService, auth.RoleFinder, auth.RoleIntermediary, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.GetItemTimeline))
	api.POST("/items/:id/custody/accept",
		middleware.RequireRole(rt.authService, auth.RoleIntermediary),
		rt.wrap(h.PostCustodyAccept))
	api.POST("/items/:id/custody/refuse",
		middleware.RequireRole(rt.authService, auth.RoleIntermediary),
		rt.wrap(h.PostCustodyRefuse))
	api.POST("/institution/tick",
		middleware.RequireRole(rt.authService, auth.RoleAdmin),
		rt.wrap(h.PostInstitutionTick))
	api.GET("/institution/pickups",
		middleware.RequireAnyRole(rt.authService, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.GetInstitutionPickups))
	api.POST("/institution/items",
		middleware.RequireAnyRole(rt.authService, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.PostInstitutionItem))
	api.POST("/institution/items/:id/transfer",
		middleware.RequireAnyRole(rt.authService, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.PostInstitutionTransfer))
	api.POST("/institution/items/:id/offer",
		middleware.RequireAnyRole(rt.authService, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.PostInstitutionOffer))
	api.POST("/institution/items/:id/collect",
		middleware.RequireAnyRole(rt.authService, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.PostInstitutionCollect))
	api.POST("/institution/items/:id/dispose",
		middleware.RequireAnyRole(rt.authService, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.PostInstitutionDispose))
	api.GET("/institution/claims/:id",
		middleware.RequireAnyRole(rt.authService, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.GetInstitutionClaim))
	api.POST("/institution/claims/:id/review",
		middleware.RequireAnyRole(rt.authService, auth.RoleInstitution, auth.RoleAdmin),
		rt.wrap(h.PostInstitutionClaimReview))
	api.GET("/uploads/:filename",
		middleware.RequireAnyRole(rt.authService, auth.RoleFinder, auth.RoleIntermediary, auth.RoleInstitution),
		rt.wrap(h.GetUpload))
	api.POST("/demo/reset",
		middleware.RequireRole(rt.authService, auth.RoleAdmin),
		rt.wrap(h.PostDemoReset))

	return rt.engine
}

// wrap adapts a framework-agnostic route handler to Gin.
func (rt *_router) wrap(fn routes.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqCtx := reqcontext.GetRequestContext(c)
		if reqCtx.Logger == nil {
			reqCtx.Logger = rt.baseLogger.With(
				slog.String("WARNING", "request context is missing logger, falling back to base logger"),
				slog.String("request-id", reqCtx.ReqUUID.String()),
			)
		}
		fn(reqCtx)
	}
}
