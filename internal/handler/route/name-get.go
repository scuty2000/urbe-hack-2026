package routes

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

func (h *WebAPIHandler) GetNameByID(ctx *reqcontext.RequestContext) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.Logger.Error("GetNameByID invalid id", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid id")
		return
	}

	name, err := h.Names.GetByID(ctx.Request.Context(), id)
	if errors.Is(err, errs.ErrNotFound) {
		ctx.Logger.Error("GetNameByID name not found", "id", id)
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		ctx.Logger.Error("GetNameByID service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}
	ctx.JSON(http.StatusOK, dto.ToNameResponse(name))
}
