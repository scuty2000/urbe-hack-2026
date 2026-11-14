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
	h := routes.New(rt.nameService, rt.healthService, rt.authService, rt.itemService, rt.custodyService)

	rt.engine.GET("/names/:id", rt.wrap(h.GetNameByID))
	rt.engine.POST("/names", rt.wrap(h.PostNewName))
	rt.engine.PUT("/names/:id", rt.wrap(h.PutNameByID))
	rt.engine.DELETE("/names/:id", rt.wrap(h.DeleteNameByID))
	rt.engine.POST("/names/:id/replace", rt.wrap(h.PostReplaceNameByID))
	rt.engine.GET("/healthz", rt.wrap(h.Liveness))
	rt.engine.GET("/api/health", rt.wrap(h.Readiness))

	api := rt.engine.Group("/api")
	api.POST("/auth/login", rt.wrap(h.PostLogin))
	api.POST("/auth/logout", rt.wrap(h.PostLogout))
	api.GET("/auth/me", middleware.RequireAuth(rt.authService), rt.wrap(h.GetMe))

	api.POST("/items",
		middleware.RequireAnyRole(rt.authService, auth.RoleFinder, auth.RoleIntermediary),
		rt.wrap(h.PostItem))
	api.GET("/items",
		middleware.RequireRole(rt.authService, auth.RoleIntermediary),
		rt.wrap(h.ListItems))
	api.GET("/items/:id",
		middleware.RequireAnyRole(rt.authService, auth.RoleIntermediary, auth.RoleInstitution),
		rt.wrap(h.GetItem))
	api.POST("/items/:id/custody/accept",
		middleware.RequireRole(rt.authService, auth.RoleIntermediary),
		rt.wrap(h.PostCustodyAccept))
	api.POST("/items/:id/custody/refuse",
		middleware.RequireRole(rt.authService, auth.RoleIntermediary),
		rt.wrap(h.PostCustodyRefuse))
	api.POST("/institution/tick",
		middleware.RequireRole(rt.authService, auth.RoleAdmin),
		rt.wrap(h.PostInstitutionTick))
	api.GET("/uploads/:filename",
		middleware.RequireAnyRole(rt.authService, auth.RoleFinder, auth.RoleIntermediary, auth.RoleInstitution),
		rt.wrap(h.GetUpload))

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
