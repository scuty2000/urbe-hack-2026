package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

const (
	// maxMultipartMemory keeps small uploads in memory; larger ones spill to
	// temporary files.
	maxMultipartMemory = 16 << 20
	// maxRequestBody bounds the whole multipart request (payload + photos).
	// The per-file limit is enforced by the storage adapter.
	maxRequestBody = 80 << 20
	// maxPhotosPerItem bounds the number of files accepted in one request.
	maxPhotosPerItem = 8
)

// PostItem enrolls an item from a multipart request: a JSON `payload` field and
// repeated `photos` files. The role determines the source and the legal finder.
func (h *WebAPIHandler) PostItem(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxRequestBody)
	if err := ctx.Request.ParseMultipartForm(maxMultipartMemory); err != nil {
		ctx.Logger.Error("PostItem invalid multipart body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be multipart/form-data with a payload field")
		return
	}

	var payload dto.CreateItemPayload
	if err := json.NewDecoder(strings.NewReader(ctx.Request.FormValue("payload"))).Decode(&payload); err != nil {
		ctx.Logger.Error("PostItem invalid payload", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "payload must be valid JSON")
		return
	}

	input, ok := createItemInput(ctx.User, payload)
	if !ok {
		respondWithJSONError(ctx, http.StatusForbidden, "role cannot enroll items")
		return
	}

	photos, err := h.saveUploadedPhotos(ctx)
	if err != nil {
		ctx.Logger.Error("PostItem saving photos", "err", err)
		if errors.Is(err, errs.ErrUnavailable) {
			respondWithJSONError(ctx, http.StatusServiceUnavailable, "upload storage is unavailable")
			return
		}
		respondWithJSONError(ctx, http.StatusBadRequest, "photos must be JPEG, PNG or WebP images within the size limit")
		return
	}
	input.Photos = photos

	created, err := h.Items.CreateItem(ctx.Request.Context(), input)
	switch {
	case errors.Is(err, errs.ErrInvalidInput):
		ctx.Logger.Error("PostItem invalid input", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "rights assignment, intermediary and category are required")
		return
	case errors.Is(err, errs.ErrConflict):
		ctx.Logger.Error("PostItem conflict", "err", err)
		respondWithJSONError(ctx, http.StatusConflict, "item could not be enrolled")
		return
	case err != nil:
		ctx.Logger.Error("PostItem service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.Header("Location", "/api/items/"+created.ID)
	ctx.JSON(http.StatusCreated, dto.ToItemResponse(created))
}

// createItemInput applies the role rules from the controller decisions:
//   - finder: source finder_self, intermediary chosen by the finder;
//   - intermediary: source finder_mediated, self as finder and legal finder;
//   - institution: source operator (official intake, Task 11).
func createItemInput(user *auth.User, payload dto.CreateItemPayload) (itemdomain.CreateItemInput, bool) {
	input := itemdomain.CreateItemInput{
		FinderID:       user.ID,
		RightsAssigned: payload.RightsAssigned,
		Category:       payload.Category,
		Brand:          payload.Brand,
		Model:          payload.Model,
		Color:          payload.Color,
		Condition:      payload.Condition,
		PrivateProfile: payload.PrivateProfile,
		FoundAt:        payload.FoundAt,
		FoundLat:       payload.FoundLat,
		FoundLng:       payload.FoundLng,
		FoundZone:      payload.FoundZone,
		FoundNotes:     payload.FoundNotes,
	}

	switch user.Role {
	case auth.RoleFinder:
		input.Source = itemdomain.SourceFinderSelf
		input.IntermediaryID = payload.IntermediaryID
		input.LegalFinderID = payload.IntermediaryID
	case auth.RoleIntermediary:
		input.Source = itemdomain.SourceFinderMediated
		input.IntermediaryID = user.ID
		input.LegalFinderID = user.ID
	case auth.RoleInstitution:
		input.Source = itemdomain.SourceOperator
		input.LegalFinderID = user.ID
	default:
		return itemdomain.CreateItemInput{}, false
	}

	return input, true
}

// saveUploadedPhotos stores every `photos` part and returns their relative
// paths. A request with no photos is valid: photos are optional in the demo.
func (h *WebAPIHandler) saveUploadedPhotos(ctx *reqcontext.RequestContext) ([]string, error) {
	if ctx.Request.MultipartForm == nil {
		return nil, nil
	}
	files := ctx.Request.MultipartForm.File["photos"]
	if len(files) > maxPhotosPerItem {
		return nil, fmt.Errorf("too many photos (%d > %d): %w", len(files), maxPhotosPerItem, errs.ErrInvalidInput)
	}

	paths := make([]string, 0, len(files))
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			return nil, fmt.Errorf("opening photo %q: %w", header.Filename, err)
		}
		path, saveErr := h.Items.SavePhoto(file)
		closeErr := file.Close()
		if saveErr != nil {
			return nil, saveErr
		}
		if closeErr != nil {
			return nil, fmt.Errorf("closing photo %q: %w", header.Filename, closeErr)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// GetItem returns one item to its custodian, legal finder or an institution.
func (h *WebAPIHandler) GetItem(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	item, err := h.Items.GetItem(ctx.Request.Context(), ctx.Param("id"),
		itemdomain.Actor{ID: ctx.User.ID, Role: ctx.User.Role})
	switch {
	case errors.Is(err, errs.ErrNotFound):
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
		return
	case errors.Is(err, errs.ErrForbidden):
		respondWithJSONError(ctx, http.StatusForbidden, "forbidden")
		return
	case err != nil:
		ctx.Logger.Error("GetItem service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.JSON(http.StatusOK, dto.ToItemResponse(item))
}

// ListItems returns the items held by the authenticated intermediary.
// Only ?custodian=me is supported: an intermediary must not browse the registry.
func (h *WebAPIHandler) ListItems(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}
	if ctx.Query("custodian") != "me" {
		respondWithJSONError(ctx, http.StatusBadRequest, "custodian=me is the only supported query")
		return
	}

	items, err := h.Items.ListItemsByCustodian(ctx.Request.Context(), ctx.User.ID)
	if err != nil {
		ctx.Logger.Error("ListItems service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.JSON(http.StatusOK, dto.ToItemListResponse(items))
}

// GetUpload serves a stored photo to authenticated finders, intermediaries and
// institutions. The storage adapter rejects traversal; the role middleware
// keeps searchers out.
func (h *WebAPIHandler) GetUpload(ctx *reqcontext.RequestContext) {
	contentType, data, err := h.Items.OpenUpload(ctx.Param("filename"))
	switch {
	case errors.Is(err, errs.ErrNotFound):
		respondWithJSONError(ctx, http.StatusNotFound, "not found")
		return
	case errors.Is(err, errs.ErrInvalidInput):
		respondWithJSONError(ctx, http.StatusBadRequest, "invalid filename")
		return
	case errors.Is(err, errs.ErrUnavailable):
		respondWithJSONError(ctx, http.StatusServiceUnavailable, "upload storage is unavailable")
		return
	case err != nil:
		ctx.Logger.Error("GetUpload storage error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.Header("Cache-Control", "private, max-age=3600")
	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.Data(http.StatusOK, contentType, data)
}
