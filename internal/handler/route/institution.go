package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	institutiondomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/institution"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// maxInstitutionBody bounds the small JSON bodies of the institution routes.
const maxInstitutionBody = 64 << 10

// GetInstitutionPickups returns the pickup queue grouped by intermediary/zone,
// with sensitive items first.
func (h *WebAPIHandler) GetInstitutionPickups(ctx *reqcontext.RequestContext) {
	groups, err := h.Institution.ListPickups(ctx.Request.Context())
	if err != nil {
		respondWithInstitutionError(ctx, "GetInstitutionPickups", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToInstitutionPickupList(groups))
}

// PostInstitutionTransfer takes an item from an intermediary into institution
// custody.
func (h *WebAPIHandler) PostInstitutionTransfer(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	transferred, err := h.Institution.Transfer(
		ctx.Request.Context(), ctx.Param("id"), ctx.User.ID, time.Now().UTC(),
	)
	if err != nil {
		respondWithInstitutionError(ctx, "PostInstitutionTransfer", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToItemResponse(transferred))
}

// PostInstitutionItem enrolls an item handed directly to the PA. It accepts the
// same multipart shape as POST /api/items: a JSON `payload` field and repeated
// `photos` parts.
func (h *WebAPIHandler) PostInstitutionItem(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxRequestBody)
	parseErr := ctx.Request.ParseMultipartForm(maxMultipartMemory)
	defer func() {
		if form := ctx.Request.MultipartForm; form != nil {
			_ = form.RemoveAll()
		}
	}()
	if parseErr != nil {
		ctx.Logger.Error("PostInstitutionItem invalid multipart body", "err", parseErr)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be multipart/form-data with a payload field")
		return
	}

	var payload dto.OfficialItemPayload
	if err := json.NewDecoder(strings.NewReader(ctx.Request.FormValue("payload"))).Decode(&payload); err != nil {
		ctx.Logger.Error("PostInstitutionItem invalid payload", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "payload must be valid JSON")
		return
	}

	photos, err := h.saveUploadedPhotos(ctx)
	if err != nil {
		ctx.Logger.Error("PostInstitutionItem saving photos", "err", err)
		if errors.Is(err, errs.ErrUnavailable) {
			respondWithJSONError(ctx, http.StatusServiceUnavailable, "upload storage is unavailable")
			return
		}
		respondWithJSONError(ctx, http.StatusBadRequest, "photos must be JPEG, PNG or WebP images within the size limit")
		return
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		for _, path := range photos {
			if deleteErr := h.Items.DeletePhoto(path); deleteErr != nil {
				ctx.Logger.Warn("PostInstitutionItem cleaning up photo", "path", path, "err", deleteErr)
			}
		}
	}()

	created, err := h.Institution.CreateOfficialItem(ctx.Request.Context(), institutiondomain.CreateOfficialItemInput{
		InstitutionID:  ctx.User.ID,
		ActorID:        ctx.User.ID,
		Category:       payload.Category,
		Brand:          payload.Brand,
		Model:          payload.Model,
		Color:          payload.Color,
		Condition:      payload.Condition,
		PrivateProfile: payload.PrivateProfile,
		Photos:         photos,
		FoundAt:        payload.FoundAt,
		FoundLat:       payload.FoundLat,
		FoundLng:       payload.FoundLng,
		FoundZone:      payload.FoundZone,
		FoundNotes:     payload.FoundNotes,
	})
	if err == nil {
		committed = true
	}
	if err != nil {
		respondWithInstitutionError(ctx, "PostInstitutionItem", err)
		return
	}

	ctx.Header("Location", "/api/items/"+created.ID)
	ctx.JSON(http.StatusCreated, dto.ToItemResponse(created))
}

// GetInstitutionClaim returns the full verification report to the institution.
func (h *WebAPIHandler) GetInstitutionClaim(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	found, err := h.Claims.GetClaim(ctx.Request.Context(), ctx.Param("id"),
		claimdomain.Actor{ID: ctx.User.ID, Role: ctx.User.Role})
	if err != nil {
		respondWithInstitutionError(ctx, "GetInstitutionClaim", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToClaimReportResponse(found))
}

// PostInstitutionClaimReview records an institution review decision and returns
// the refreshed report so the operator sees why the claim reached its status.
func (h *WebAPIHandler) PostInstitutionClaimReview(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxInstitutionBody)
	var payload dto.ReviewClaimPayload
	if err := json.NewDecoder(ctx.Request.Body).Decode(&payload); err != nil {
		ctx.Logger.Error("PostInstitutionClaimReview invalid body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with a decision")
		return
	}

	reviewed, err := h.Institution.ReviewClaim(
		ctx.Request.Context(), ctx.Param("id"), payload.Decision, ctx.User.ID,
	)
	if err != nil {
		respondWithInstitutionError(ctx, "PostInstitutionClaimReview", err)
		return
	}

	report, err := h.Claims.GetClaim(ctx.Request.Context(), ctx.Param("id"),
		claimdomain.Actor{ID: ctx.User.ID, Role: ctx.User.Role})
	if err != nil {
		// The decision is committed; return the safe summary rather than
		// turning a successful review into an error.
		ctx.Logger.Error("PostInstitutionClaimReview reload report", "err", err)
		ctx.JSON(http.StatusOK, dto.ToClaimReportResponse(reviewed))
		return
	}
	ctx.JSON(http.StatusOK, dto.ToClaimReportResponse(report))
}

// PostInstitutionOffer offers an unclaimed item to its legal finder.
func (h *WebAPIHandler) PostInstitutionOffer(ctx *reqcontext.RequestContext) {
	offered, err := h.Institution.OfferUnclaimed(
		ctx.Request.Context(), ctx.Param("id"), time.Now().UTC(),
	)
	if err != nil {
		respondWithInstitutionError(ctx, "PostInstitutionOffer", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToItemResponse(offered))
}

// PostInstitutionCollect records that the legal finder collected an offered
// item.
func (h *WebAPIHandler) PostInstitutionCollect(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}
	collected, err := h.Institution.CollectByLegalFinder(
		ctx.Request.Context(), ctx.Param("id"), ctx.User.ID,
	)
	if err != nil {
		respondWithInstitutionError(ctx, "PostInstitutionCollect", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToItemResponse(collected))
}

// PostInstitutionDispose records the disposal of an offered item.
func (h *WebAPIHandler) PostInstitutionDispose(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}
	disposed, err := h.Institution.Dispose(ctx.Request.Context(), ctx.Param("id"), ctx.User.ID)
	if err != nil {
		respondWithInstitutionError(ctx, "PostInstitutionDispose", err)
		return
	}
	ctx.JSON(http.StatusOK, dto.ToItemResponse(disposed))
}

// respondWithInstitutionError maps domain sentinels to the shared HTTP error
// shape.
func respondWithInstitutionError(ctx *reqcontext.RequestContext, operation string, err error) {
	switch {
	case errors.Is(err, errs.ErrNotFound):
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
	case errors.Is(err, errs.ErrForbidden):
		respondWithJSONError(ctx, http.StatusForbidden, "forbidden")
	case errors.Is(err, errs.ErrConflict):
		respondWithJSONError(ctx, http.StatusConflict, "item is not in a valid institution state")
	case errors.Is(err, errs.ErrInvalidInput):
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid institution request")
	case errors.Is(err, errs.ErrUnavailable):
		respondWithJSONError(ctx, http.StatusServiceUnavailable, "service unavailable")
	default:
		ctx.Logger.Error(operation+" service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
	}
}
