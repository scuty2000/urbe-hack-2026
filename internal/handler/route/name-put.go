package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

func (h *WebAPIHandler) PutNameByID(ctx *reqcontext.RequestContext) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.Logger.Error("PutNameByID invalid id", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid id")
		return
	}

	var req dto.CreateNameRequest
	if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil {
		ctx.Logger.Error("PutNameByID invalid request body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be valid JSON with a \"name\" field")
		return
	}

	n, err := h.Names.UpdateByID(ctx.Request.Context(), id, req.Name)
	if errors.Is(err, errs.ErrNotFound) {
		ctx.Logger.Error("PutNameByID name not found", "id", id)
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, errs.ErrInvalidInput) {
		ctx.Logger.Error("PutNameByID invalid input", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "name must be a non-empty string of at most 255 characters")
		return
	}
	if err != nil {
		ctx.Logger.Error("PutNameByID service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.JSON(http.StatusOK, dto.ToNameResponse(n))
}
