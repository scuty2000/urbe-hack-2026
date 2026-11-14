package routes

import (
	"net/http"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// Liveness checks the application health. Replies 500 if the database is
// unreachable, 200 otherwise.
func (h *WebAPIHandler) Liveness(ctx *reqcontext.RequestContext) {
	if err := h.Health.Liveness(ctx.Request.Context()); err != nil {
		ctx.Logger.Error("liveness check failed", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "database unreachable")
		return
	}
	ctx.Status(http.StatusOK)
}
