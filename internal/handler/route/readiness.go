package routes

import (
	"net/http"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// Readiness reports the status of the application dependencies. It always
// replies 200: the JSON body carries a boolean per dependency so monitoring
// can distinguish partial outages without treating the endpoint as down.
func (h *WebAPIHandler) Readiness(ctx *reqcontext.RequestContext) {
	ctx.JSON(http.StatusOK, h.Health.Readiness(ctx.Request.Context()))
}
