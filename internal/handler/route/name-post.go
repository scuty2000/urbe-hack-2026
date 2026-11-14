package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

func (h *WebAPIHandler) PostNewName(ctx *reqcontext.RequestContext) {
	var namereq dto.CreateNameRequest

	if err := json.NewDecoder(ctx.Request.Body).Decode(&namereq); err != nil {
		ctx.Logger.Error("PostNewName invalid request body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be valid JSON with a \"name\" field")
		return
	}

	name, err := h.Names.Create(ctx.Request.Context(), namereq.Name)
	if errors.Is(err, errs.ErrInvalidInput) {
		ctx.Logger.Error("PostNewName invalid input", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "name must be a non-empty string of at most 255 characters")
		return
	} else if err != nil {
		ctx.Logger.Error("PostNewName service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.Header("Location", fmt.Sprintf("/names/%d", name.ID))
	ctx.JSON(http.StatusCreated, dto.ToNameResponse(name))
}
