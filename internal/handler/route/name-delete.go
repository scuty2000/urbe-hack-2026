package routes

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

func (h *WebAPIHandler) DeleteNameByID(ctx *reqcontext.RequestContext) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.Logger.Error("DeleteNameByID invalid id", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.Names.DeleteByID(ctx.Request.Context(), id); errors.Is(err, errs.ErrNotFound) {
		ctx.Logger.Error("DeleteNameByID name not found", "id", id)
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		ctx.Logger.Error("DeleteNameByID service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.Status(http.StatusNoContent) // 204 — deleted, no-body
}
