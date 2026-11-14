package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// maxCustodyBody bounds the small JSON bodies of the custody routes.
const maxCustodyBody = 64 << 10

// PostCustodyAccept acknowledges the handover for the authenticated
// intermediary. The actor and the current time are supplied by the HTTP layer,
// so the domain service stays deterministic.
func (h *WebAPIHandler) PostCustodyAccept(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	now := time.Now().UTC()
	accepted, err := h.Custody.Accept(ctx.Request.Context(), ctx.Param("id"), ctx.User.ID, now)
	if err != nil {
		respondWithCustodyError(ctx, "PostCustodyAccept", err)
		return
	}

	daysLeft, err := h.Custody.DaysLeft(ctx.Request.Context(), accepted.ItemID, now)
	if err != nil {
		ctx.Logger.Error("PostCustodyAccept days left", "err", err)
		daysLeft = 0
	}

	// Indexing is best-effort: a Qdrant/embeddings outage must never turn a
	// successful custody acceptance into an error.
	if h.Search != nil {
		item, getErr := h.Items.GetItem(ctx.Request.Context(), accepted.ItemID,
			itemdomain.Actor{ID: ctx.User.ID, Role: ctx.User.Role})
		if getErr != nil {
			ctx.Logger.Error("PostCustodyAccept load item for indexing", "err", getErr)
		} else if indexErr := h.Search.IndexItem(ctx.Request.Context(), item); indexErr != nil {
			ctx.Logger.Error("PostCustodyAccept index item", "err", indexErr)
		}
	}

	ctx.JSON(http.StatusOK, dto.ToCustodyResponse(accepted, daysLeft))
}

// PostCustodyRefuse rejects the handover for the authenticated intermediary.
func (h *WebAPIHandler) PostCustodyRefuse(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxCustodyBody)
	var payload dto.RefuseCustodyPayload
	if err := json.NewDecoder(ctx.Request.Body).Decode(&payload); err != nil {
		ctx.Logger.Error("PostCustodyRefuse invalid body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with a reason")
		return
	}

	refused, err := h.Custody.Refuse(
		ctx.Request.Context(), ctx.Param("id"), ctx.User.ID, payload.Reason,
	)
	if err != nil {
		respondWithCustodyError(ctx, "PostCustodyRefuse", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToItemResponse(refused))
}

// PostInstitutionTick applies the elapsed custody deadlines. It is the demo
// replacement for a scheduled job and is restricted to the admin role.
func (h *WebAPIHandler) PostInstitutionTick(ctx *reqcontext.RequestContext) {
	due, err := h.Custody.MarkPickupDue(ctx.Request.Context(), time.Now().UTC())
	if err != nil {
		ctx.Logger.Error("PostInstitutionTick service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}
	ctx.JSON(http.StatusOK, dto.ToTickResponse(due))
}

// respondWithCustodyError maps domain sentinels to the shared HTTP error shape.
func respondWithCustodyError(ctx *reqcontext.RequestContext, operation string, err error) {
	switch {
	case errors.Is(err, errs.ErrNotFound):
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
	case errors.Is(err, errs.ErrForbidden):
		respondWithJSONError(ctx, http.StatusForbidden, "forbidden")
	case errors.Is(err, errs.ErrConflict):
		respondWithJSONError(ctx, http.StatusConflict, "item is not pending custody")
	case errors.Is(err, errs.ErrInvalidInput):
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid custody request")
	default:
		ctx.Logger.Error(operation+" service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
	}
}
