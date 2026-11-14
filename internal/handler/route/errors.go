package routes

import (
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// respondWithJSONError writes a consistent JSON error response.
func respondWithJSONError(ctx *reqcontext.RequestContext, status int, msg string) {
	ctx.JSON(status, dto.ErrorResponse{Error: msg})
}
