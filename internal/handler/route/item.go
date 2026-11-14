package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	custodydomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/custody"
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
	parseErr := ctx.Request.ParseMultipartForm(maxMultipartMemory)
	// Multipart parts larger than maxMultipartMemory spill to os.TempDir();
	// RemoveAll must run on every path, including parse errors.
	defer func() {
		if form := ctx.Request.MultipartForm; form != nil {
			_ = form.RemoveAll()
		}
	}()
	if parseErr != nil {
		ctx.Logger.Error("PostItem invalid multipart body", "err", parseErr)
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

	// Validate the domain invariants before writing any file: a rejected
	// enrollment (for example missing rights assignment) must not leave
	// orphan uploads on disk.
	if err := itemdomain.ValidateCreateInput(input); err != nil {
		ctx.Logger.Error("PostItem invalid input", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "rights assignment and a valid intermediary are required")
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

	// Roll back the files if anything after this point fails.
	committed := false
	defer func() {
		if committed {
			return
		}
		for _, path := range photos {
			if deleteErr := h.Items.DeletePhoto(path); deleteErr != nil {
				ctx.Logger.Warn("PostItem cleaning up photo", "path", path, "err", deleteErr)
			}
		}
	}()

	created, err := h.Items.CreateItem(ctx.Request.Context(), input)
	if err == nil {
		committed = true
	}
	switch {
	case errors.Is(err, errs.ErrInvalidInput):
		ctx.Logger.Error("PostItem invalid input", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "rights assignment and a valid intermediary are required")
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
//   - intermediary: source finder_mediated, self as finder and legal finder.
//
// Official operator intake (source=operator) is deliberately not exposed here;
// it belongs to Task 11, but the domain branch stays available for it.
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

	ctx.JSON(http.StatusOK, itemResponseWithDeadline(item, time.Now().UTC()))
}

// itemResponseWithDeadline maps an item and computes daysLeft from the deadline
// stored on its custody row. The policy itself is never duplicated here.
func itemResponseWithDeadline(it itemdomain.Item, now time.Time) dto.ItemResponse {
	response := dto.ToItemResponse(it)
	if it.DueAt != nil {
		daysLeft := custodydomain.DaysLeft(it.DueAt, now)
		response.DaysLeft = &daysLeft
	}
	return response
}

func itemListResponseWithDeadlines(items []itemdomain.Item, now time.Time) dto.ItemListResponse {
	response := dto.ItemListResponse{Items: make([]dto.ItemResponse, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, itemResponseWithDeadline(item, now))
	}
	return response
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

	ctx.JSON(http.StatusOK, itemListResponseWithDeadlines(items, time.Now().UTC()))
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
