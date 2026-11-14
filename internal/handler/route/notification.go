package routes

import (
	"errors"
	"net/http"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/platform"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// GetNotifications returns the authenticated user's own notifications, newest
// first. Another user's inbox is never reachable through this route.
func (h *WebAPIHandler) GetNotifications(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	notifications, err := h.Platform.ListNotifications(ctx.Request.Context(), ctx.User.ID)
	if err != nil {
		respondWithPlatformError(ctx, "GetNotifications", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToNotificationList(notifications))
}

// PostNotificationRead marks one of the authenticated user's notifications as
// read. A notification owned by another user is forbidden.
func (h *WebAPIHandler) PostNotificationRead(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	if err := h.Platform.MarkRead(ctx.Request.Context(), ctx.Param("id"), ctx.User.ID); err != nil {
		respondWithPlatformError(ctx, "PostNotificationRead", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NotificationRead{ID: ctx.Param("id"), Read: true})
}

// GetItemTimeline returns the safe custody timeline of one item. The platform
// service applies the role/ownership rules; the DTO only exposes the event
// type, actor, timestamp, notes and an evidence reference.
func (h *WebAPIHandler) GetItemTimeline(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	itemID := ctx.Param("id")
	events, err := h.Platform.TimelineFor(ctx.Request.Context(), itemID, platform.Actor{
		ID:   ctx.User.ID,
		Role: ctx.User.Role,
	})
	if err != nil {
		respondWithPlatformError(ctx, "GetItemTimeline", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToItemTimeline(itemID, events))
}

// respondWithPlatformError maps platform sentinels to the shared HTTP error
// shape.
func respondWithPlatformError(ctx *reqcontext.RequestContext, operation string, err error) {
	switch {
	case errors.Is(err, errs.ErrNotFound):
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
	case errors.Is(err, errs.ErrForbidden):
		respondWithJSONError(ctx, http.StatusForbidden, "forbidden")
	case errors.Is(err, errs.ErrInvalidInput):
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid request")
	default:
		ctx.Logger.Error(operation+" service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
	}
}
