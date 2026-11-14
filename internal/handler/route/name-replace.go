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

// PostReplaceNameByID handles POST /name/:id/replace.
// It atomically deletes the existing name and creates a new one in a single transaction.
func (h *WebAPIHandler) PostReplaceNameByID(ctx *reqcontext.RequestContext) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil {
		ctx.Logger.Error("PostReplaceNameByID invalid id", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid id")
		return
	}

	var req dto.CreateNameRequest
	if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil {
		ctx.Logger.Error("PostReplaceNameByID invalid request body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be valid JSON with a \"name\" field")
		return
	}

	n, err := h.Names.Replace(ctx.Request.Context(), id, req.Name)
	if errors.Is(err, errs.ErrNotFound) {
		ctx.Logger.Error("PostReplaceNameByID name not found", "id", id)
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
		return
	} else if errors.Is(err, errs.ErrInvalidInput) {
		ctx.Logger.Error("PostReplaceNameByID invalid input", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "name must be a non-empty string of at most 255 characters")
		return
	} else if err != nil {
		ctx.Logger.Error("PostReplaceNameByID service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.JSON(http.StatusCreated, dto.ToNameResponse(n))
}
