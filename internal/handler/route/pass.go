package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// maxPassBody bounds the small JSON bodies of the pass routes. A signed token
// is a few hundred bytes; 16 KiB leaves room for future fields without turning
// the scanner into an upload endpoint.
const maxPassBody = 16 << 10

// PostPass issues a pickup pass for the authenticated searcher's approved
// claim. The raw token is returned once and is never logged.
func (h *WebAPIHandler) PostPass(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	var payload dto.CreatePassPayload
	if err := decodePassBody(ctx, &payload); err != nil {
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with a claimId")
		return
	}

	now := time.Now().UTC()
	issued, err := h.Passes.Issue(ctx.Request.Context(), payload.ClaimID, ctx.User.ID, now)
	if err != nil {
		respondWithPassError(ctx, "PostPass", err)
		return
	}

	ctx.JSON(http.StatusCreated, dto.ToPassResponse(issued))
}

// PostPassValidate checks a scanned token for the authenticated custodian. An
// invalid pass is a normal scanner outcome, so it is HTTP 200 with valid=false
// and a coarse reason; only a malformed request or an unavailable service is
// an error status.
func (h *WebAPIHandler) PostPassValidate(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	var payload dto.PassTokenPayload
	if err := decodePassBody(ctx, &payload); err != nil || strings.TrimSpace(payload.Token) == "" {
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with a token")
		return
	}

	validation, err := h.Passes.Validate(
		ctx.Request.Context(), payload.Token, ctx.User.ID, time.Now().UTC(),
	)
	if err != nil {
		if errors.Is(err, errs.ErrUnavailable) {
			respondWithJSONError(ctx, http.StatusServiceUnavailable, "pass validation is unavailable")
			return
		}
		if reason := passdomain.ReasonOf(err); reason != "" {
			// Never surface the underlying error: it can contain internal ids.
			ctx.JSON(http.StatusOK, dto.ToPassValidationResponse(validation))
			return
		}
		if errors.Is(err, errs.ErrInvalidInput) {
			respondWithJSONError(ctx, http.StatusBadRequest, "invalid pass token")
			return
		}
		ctx.Logger.Error("PostPassValidate service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.JSON(http.StatusOK, dto.ToPassValidationResponse(validation))
}

// PostPassRedeem validates the scanned token and performs the handover. The
// response contains only the safe item summary.
func (h *WebAPIHandler) PostPassRedeem(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	var payload dto.PassTokenPayload
	if err := decodePassBody(ctx, &payload); err != nil || strings.TrimSpace(payload.Token) == "" {
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with a token")
		return
	}

	now := time.Now().UTC()
	item, err := h.Passes.Redeem(
		ctx.Request.Context(), payload.Token, ctx.User.ID, ctx.User.ID, now,
	)
	if err != nil {
		respondWithPassError(ctx, "PostPassRedeem", err)
		return
	}

	ctx.JSON(http.StatusOK, dto.PassRedemptionResponse{
		Item:       dto.ToPassItemSummary(item),
		ReturnedAt: now,
	})
}

// decodePassBody enforces a bounded body and rejects unknown fields.
func decodePassBody(ctx *reqcontext.RequestContext, target any) error {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxPassBody)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// respondWithPassError maps pass sentinels to the shared HTTP error shape. It
// never echoes the token or an internal id.
func respondWithPassError(ctx *reqcontext.RequestContext, operation string, err error) {
	switch {
	case errors.Is(err, errs.ErrUnavailable):
		respondWithJSONError(ctx, http.StatusServiceUnavailable, "pickup passes are unavailable")
	case errors.Is(err, errs.ErrForbidden):
		respondWithJSONError(ctx, http.StatusForbidden, "forbidden")
	case errors.Is(err, errs.ErrNotFound):
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
	case errors.Is(err, errs.ErrConflict):
		respondWithJSONError(ctx, http.StatusConflict, "pass cannot be used")
	case errors.Is(err, errs.ErrInvalidInput):
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid pass")
	default:
		ctx.Logger.Error(operation+" service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
	}
}
