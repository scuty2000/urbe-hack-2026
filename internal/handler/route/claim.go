package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// maxClaimBody bounds the small JSON body of the claim routes.
const maxClaimBody = 64 << 10

// claimRateLimit is the number of claims a searcher may create per window.
const claimRateLimit = 10

// claimRateWindow is the claim rate-limit window.
const claimRateWindow = time.Minute

// PostClaim creates a claim for the authenticated searcher. The handler never
// accepts a raw confidence: the service builds the verification session and
// applies the deterministic policy.
func (h *WebAPIHandler) PostClaim(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}
	if !h.claimLimiter.allow(ctx.User.ID) {
		respondWithJSONError(ctx, http.StatusTooManyRequests, "too many claims, try again later")
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxClaimBody)
	decoder := json.NewDecoder(ctx.Request.Body)
	// unknown fields (including a forged confidence) are rejected outright.
	decoder.DisallowUnknownFields()
	var payload dto.CreateClaimPayload
	if err := decoder.Decode(&payload); err != nil {
		ctx.Logger.Error("PostClaim invalid body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with an itemId")
		return
	}

	created, err := h.Claims.CreateClaim(ctx.Request.Context(), claimdomain.CreateClaimInput{
		ItemID:        payload.ItemID,
		SearcherID:    ctx.User.ID,
		ReturnChannel: payload.ReturnChannel,
		Session: claimdomain.Session{
			Answers: payload.Answers,
		},
		Evidence: payload.Evidence,
		Flags:    payload.Flags,
	})
	if err != nil {
		respondWithClaimError(ctx, "PostClaim", err)
		return
	}

	ctx.Header("Location", "/api/claims/"+created.ID)
	ctx.JSON(http.StatusCreated, dto.ToClaimOwnerResponse(created))
}

// GetClaim returns a claim in the caller's role-safe shape. The searcher owner
// gets the owner summary, the custodian/legal finder gets the same summary plus
// safe evidence and flags, and only the institution/admin gets the full report.
func (h *WebAPIHandler) GetClaim(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	found, err := h.Claims.GetClaim(
		ctx.Request.Context(),
		ctx.Param("id"),
		claimdomain.Actor{ID: ctx.User.ID, Role: ctx.User.Role},
	)
	if err != nil {
		respondWithClaimError(ctx, "GetClaim", err)
		return
	}

	switch ctx.User.Role {
	case auth.RoleInstitution, auth.RoleAdmin:
		ctx.JSON(http.StatusOK, dto.ToClaimReportResponse(found))
	case auth.RoleIntermediary:
		ctx.JSON(http.StatusOK, dto.ToClaimSummaryResponse(found))
	default:
		ctx.JSON(http.StatusOK, dto.ToClaimOwnerResponse(found))
	}
}

// PostClaimReview records the custodian, legal finder or institution decision.
// It returns the safe summary (never the verification report), so the
// intermediary can refresh its claim card without seeing the questions.
func (h *WebAPIHandler) PostClaimReview(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxClaimBody)
	var payload dto.ReviewClaimPayload
	if err := json.NewDecoder(ctx.Request.Body).Decode(&payload); err != nil {
		ctx.Logger.Error("PostClaimReview invalid body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with a decision")
		return
	}

	reviewed, err := h.Claims.ReviewClaim(
		ctx.Request.Context(), ctx.Param("id"), payload.Decision, ctx.User.ID,
	)
	if err != nil {
		respondWithClaimError(ctx, "PostClaimReview", err)
		return
	}

	// Reload so the response carries the refreshed status and the safe
	// evidence/flags; a reload failure must not turn a recorded decision into an
	// error.
	refreshed, err := h.Claims.GetClaim(ctx.Request.Context(), ctx.Param("id"),
		claimdomain.Actor{ID: ctx.User.ID, Role: ctx.User.Role})
	if err != nil {
		ctx.Logger.Error("PostClaimReview reload", "err", err)
		ctx.JSON(http.StatusOK, dto.ToClaimSummaryResponse(reviewed))
		return
	}
	ctx.JSON(http.StatusOK, dto.ToClaimSummaryResponse(refreshed))
}

// respondWithClaimError maps domain sentinels to the shared HTTP error shape.
func respondWithClaimError(ctx *reqcontext.RequestContext, operation string, err error) {
	switch {
	case errors.Is(err, errs.ErrNotFound):
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
	case errors.Is(err, errs.ErrForbidden):
		respondWithJSONError(ctx, http.StatusForbidden, "forbidden")
	case errors.Is(err, errs.ErrConflict):
		respondWithJSONError(ctx, http.StatusConflict, "item is not claimable")
	case errors.Is(err, errs.ErrInvalidInput):
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid claim request")
	default:
		ctx.Logger.Error(operation+" service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
	}
}
