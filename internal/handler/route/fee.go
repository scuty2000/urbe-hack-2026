package routes

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	rewarddomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/reward"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// maxFeeBody bounds the reward-notice JSON body. The payload only carries an
// optional integer amount.
const maxFeeBody = 4 << 10

// GetItemFee returns the current fee estimate for an item. The estimate is
// recalculated until it is paid; a direct return is always free.
func (h *WebAPIHandler) GetItemFee(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	fee, err := h.Fees.Calculate(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		respondWithFeeError(ctx, "GetItemFee", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToFeeResponse(fee))
}

// PostFeePay settles a fee through the mock provider. Only a searcher who owns
// a claim on the item may pay; the response is the local receipt.
func (h *WebAPIHandler) PostFeePay(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	payment, err := h.Fees.PayFor(ctx.Request.Context(), ctx.Param("id"), ctx.User.ID)
	if err != nil {
		respondWithFeeError(ctx, "PostFeePay", err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, dto.ToPaymentResponse(payment))
}

// PostRewardNotice records the legal finder's reward request. The amount is
// optional and the platform never charges it.
func (h *WebAPIHandler) PostRewardNotice(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	var payload dto.CreateRewardNoticePayload
	if err := decodeRewardBody(ctx, &payload); err != nil {
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with an optional amountCents")
		return
	}

	notice, err := h.Rewards.Request(
		ctx.Request.Context(), ctx.Param("id"), ctx.User.ID, payload.AmountCents,
	)
	if err != nil {
		respondWithFeeError(ctx, "PostRewardNotice", err)
		return
	}
	ctx.Header("Location", "/api/items/"+notice.ItemID+"/reward-notice")
	ctx.JSON(http.StatusCreated, dto.ToRewardNoticeResponse(notice))
}

// GetRewardNotice returns the reward notice to a searcher, intermediary or the
// institution. A searcher read marks it as shown before pickup.
func (h *WebAPIHandler) GetRewardNotice(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	notice, err := h.Rewards.GetFor(
		ctx.Request.Context(),
		ctx.Param("id"),
		rewarddomain.Viewer{ID: ctx.User.ID, Role: ctx.User.Role},
	)
	if err != nil {
		respondWithFeeError(ctx, "GetRewardNotice", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToRewardNoticeResponse(notice))
}

// decodeRewardBody accepts an empty body (no amount) or a small JSON object
// with only amountCents; unknown fields are rejected.
func decodeRewardBody(ctx *reqcontext.RequestContext, target *dto.CreateRewardNoticePayload) error {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxFeeBody)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

// respondWithFeeError maps fee/reward sentinels to the shared error shape. It
// never echoes private item data or an internal identifier.
func respondWithFeeError(ctx *reqcontext.RequestContext, operation string, err error) {
	switch {
	case errors.Is(err, errs.ErrNotFound):
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
	case errors.Is(err, errs.ErrForbidden):
		respondWithJSONError(ctx, http.StatusForbidden, "forbidden")
	case errors.Is(err, errs.ErrConflict):
		respondWithJSONError(ctx, http.StatusConflict, "fee cannot be paid")
	case errors.Is(err, errs.ErrInvalidInput):
		if strings.Contains(strings.ToLower(err.Error()), "amount") {
			respondWithJSONError(ctx, http.StatusBadRequest, "invalid reward amount")
			return
		}
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid fee request")
	default:
		ctx.Logger.Error(operation+" service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
	}
}
