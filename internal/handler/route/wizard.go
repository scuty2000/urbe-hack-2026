package routes

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/wizard"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/mapbox"
)

const (
	wizardRateLimit  = 20
	geoRateLimit     = 60
	wizardRateWindow = time.Minute
)

const (
	// maxWizardPhotos bounds the analyze request.
	maxWizardPhotos = 4
	// maxWizardPhotoBytes is the per-photo limit, matching the upload policy.
	maxWizardPhotoBytes = 8 << 20
	// maxWizardBody bounds the whole multipart request.
	maxWizardBody = 40 << 20
	// maxWizardJSONBody bounds the reverse/drop-points JSON bodies.
	maxWizardJSONBody = 16 << 10
)

var wizardMediaTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

// PostWizardAnalyze classifies the first photo and returns the photo plan and
// prefill. It persists nothing; the final POST /api/items remains the write.
func (h *WebAPIHandler) PostWizardAnalyze(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.Wizard == nil {
		respondWithJSONError(ctx, http.StatusServiceUnavailable, "wizard is not configured")
		return
	}
	if !h.wizardLimiter.allow(ctx.User.ID) {
		respondWithJSONError(ctx, http.StatusTooManyRequests, "too many analyses, wait a minute")
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxWizardBody)
	parseErr := ctx.Request.ParseMultipartForm(8 << 20)
	defer func() {
		if form := ctx.Request.MultipartForm; form != nil {
			_ = form.RemoveAll()
		}
	}()
	if parseErr != nil {
		ctx.Logger.Error("PostWizardAnalyze invalid multipart body", "err", parseErr)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be multipart/form-data")
		return
	}

	var payload dto.WizardAnalyzePayload
	if raw := strings.TrimSpace(ctx.Request.FormValue("payload")); raw != "" {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			ctx.Logger.Error("PostWizardAnalyze invalid payload", "err", err)
			respondWithJSONError(ctx, http.StatusBadRequest, "payload must be valid JSON")
			return
		}
	}

	files := ctx.Request.MultipartForm.File["photos"]
	if len(files) == 0 {
		respondWithJSONError(ctx, http.StatusBadRequest, "at least one photo is required")
		return
	}
	if len(files) > maxWizardPhotos {
		respondWithJSONError(ctx, http.StatusBadRequest, "at most four photos are allowed")
		return
	}

	photos := make([]itemdomain.ImageInput, 0, len(files))
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			ctx.Logger.Error("PostWizardAnalyze opening photo", "err", err)
			respondWithJSONError(ctx, http.StatusBadRequest, "invalid photo")
			return
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxWizardPhotoBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			ctx.Logger.Error("PostWizardAnalyze reading photo", "readErr", readErr, "closeErr", closeErr)
			respondWithJSONError(ctx, http.StatusBadRequest, "invalid photo")
			return
		}
		if len(data) > maxWizardPhotoBytes {
			respondWithJSONError(ctx, http.StatusRequestEntityTooLarge, "photo is too large")
			return
		}
		mediaType := http.DetectContentType(data)
		if _, ok := wizardMediaTypes[mediaType]; !ok {
			respondWithJSONError(ctx, http.StatusBadRequest, "photos must be JPEG, PNG or WebP")
			return
		}
		photos = append(photos, itemdomain.ImageInput{Data: data, MediaType: mediaType})
	}

	analysis, err := h.Wizard.Analyze(ctx.Request.Context(), wizard.AnalyzeInput{
		Photos: photos,
		Slot:   payload.Slot,
		Note:   payload.Note,
	})
	if err != nil {
		respondWithWizardError(ctx, "PostWizardAnalyze", err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, dto.ToWizardAnalysisResponse(analysis))
}

// PostGeoReverse resolves the browser position to a zone. A provider failure is
// a 200 with an empty zone and source "unavailable".
func (h *WebAPIHandler) PostGeoReverse(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.Wizard == nil {
		respondWithJSONError(ctx, http.StatusServiceUnavailable, "wizard is not configured")
		return
	}
	if !h.geoLimiter.allow(ctx.User.ID) {
		respondWithJSONError(ctx, http.StatusTooManyRequests, "too many requests, wait a minute")
		return
	}
	var payload dto.ReverseGeocodeRequest
	if !decodeWizardJSON(ctx, &payload) {
		return
	}
	if !validCoordinate(payload.Lat, payload.Lng) {
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid coordinates")
		return
	}

	place, err := h.Wizard.Reverse(ctx.Request.Context(), mapbox.Point{Lat: payload.Lat, Lng: payload.Lng})
	if err != nil {
		respondWithWizardError(ctx, "PostGeoReverse", err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, dto.ReverseGeocodeResponse{
		Zone:      place.Zone,
		PlaceName: place.PlaceName,
		Source:    place.Source,
	})
}

// PostDropPoints returns the top active partners by walking time.
func (h *WebAPIHandler) PostDropPoints(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.Wizard == nil {
		respondWithJSONError(ctx, http.StatusServiceUnavailable, "wizard is not configured")
		return
	}
	if !h.geoLimiter.allow(ctx.User.ID) {
		respondWithJSONError(ctx, http.StatusTooManyRequests, "too many requests, wait a minute")
		return
	}
	var payload dto.DropPointsRequest
	if !decodeWizardJSON(ctx, &payload) {
		return
	}
	var at *mapbox.Point
	if payload.Lat != nil || payload.Lng != nil {
		if payload.Lat == nil || payload.Lng == nil || !validCoordinate(*payload.Lat, *payload.Lng) {
			respondWithJSONError(ctx, http.StatusBadRequest, "invalid coordinates")
			return
		}
		at = &mapbox.Point{Lat: *payload.Lat, Lng: *payload.Lng}
	}

	points, err := h.Wizard.Nearest(ctx.Request.Context(), at, payload.Limit)
	if err != nil {
		respondWithWizardError(ctx, "PostDropPoints", err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, dto.ToDropPointsResponse(points))
}

func decodeWizardJSON(ctx *reqcontext.RequestContext, target any) bool {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxWizardJSONBody)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		ctx.Logger.Error("wizard invalid JSON body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be valid JSON")
		return false
	}
	return true
}

func validCoordinate(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

func respondWithWizardError(ctx *reqcontext.RequestContext, operation string, err error) {
	switch {
	case errors.Is(err, errs.ErrInvalidInput):
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid wizard request")
	case errors.Is(err, errs.ErrUnauthorized):
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
	default:
		ctx.Logger.Error(operation+" service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
	}
}
