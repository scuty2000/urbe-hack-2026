package item

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

const (
	// intermediaryCustodyDays is the legal maximum before handing the item to
	// the PA.
	intermediaryCustodyDays = 7
	// sensitiveCustodyDays is the shorter window for documents, keys, SIMs...
	sensitiveCustodyDays = 1
	// publicCodeAttempts bounds retries on a (extremely unlikely) unique
	// constraint collision.
	publicCodeAttempts = 3
)

// service implements Service.
type service struct {
	db   repository.Database
	deps Deps
}

// New returns an item Service backed by db and the injected ports.
func New(db repository.Database, deps Deps) Service {
	return &service{db: db, deps: deps}
}

func (s *service) now() time.Time {
	if s.deps.Now != nil {
		return s.deps.Now().UTC()
	}
	return time.Now().UTC()
}

// CreateItem enrolls an item and assigns its legal finder. The item, its
// custody row, the rights_assigned event and the audit entry are committed in
// one transaction: no item can exist without a recorded rights assignment.
func (s *service) CreateItem(ctx context.Context, input CreateItemInput) (Item, error) {
	input = normalizeInput(input)
	if input.FoundAt.IsZero() {
		input.FoundAt = s.now()
	}
	input.FoundAt = input.FoundAt.UTC()

	if err := validateInput(input); err != nil {
		return Item{}, fmt.Errorf("create item: %w", err)
	}

	// Let D3 confirm/populate the catalog. When D3 is configured we always ask
	// it: the document signal can raise an item to the sensitive tier even when
	// the form is complete. When D3 is absent or down the catalog returns the
	// form values unchanged, so this is always safe.
	var catalogResult CatalogResult
	if s.deps.D3 != nil {
		result, err := s.Catalog(ctx, imageInputs(input.Photos), CatalogForm{
			Category:  input.Category,
			Color:     input.Color,
			Condition: input.Condition,
		})
		if err != nil {
			return Item{}, fmt.Errorf("create item catalog: %w", err)
		}
		catalogResult = result
		if input.Category == "" {
			input.Category = result.Category
		}
		if input.Color == "" {
			input.Color = result.Color
		}
		if input.Condition == "" {
			input.Condition = result.Condition
		}

		// Persist the catalog provenance and the document type alongside the
		// private profile. Both are internal and never serialised by DTOs.
		if input.PrivateProfile == nil {
			input.PrivateProfile = map[string]any{}
		}
		if result.DocumentType != "" {
			input.PrivateProfile["document_type"] = result.DocumentType
		}
		input.PrivateProfile["catalog_source"] = result.Source
	}
	if input.Category == "" {
		input.Category = "other"
	}

	riskTier, sensitive := RiskForCategory(input.Category)
	// A document detected by D3 is sensitive regardless of the category the
	// finder picked (e.g. a passport in a book-shaped form). Form/category risk
	// still applies when D3 is absent or down.
	if s.deps.D3 != nil && (catalogResult.IsDocument || catalogResult.DocumentType != "") {
		riskTier = RiskSensitive
		sensitive = true
	}

	var lastErr error
	for attempt := 0; attempt < publicCodeAttempts; attempt++ {
		item, err := s.persist(ctx, input, riskTier, sensitive)
		if err == nil {
			return item, nil
		}
		lastErr = err
		if !isConflict(err) {
			return Item{}, err
		}
	}
	return Item{}, fmt.Errorf("create item after %d attempts: %w", publicCodeAttempts, lastErr)
}

// persist writes one enrollment attempt inside a transaction.
func (s *service) persist(ctx context.Context, input CreateItemInput, riskTier string, sensitive bool) (Item, error) {
	now := s.now()

	itemID, err := newID()
	if err != nil {
		return Item{}, fmt.Errorf("create item id: %w", err)
	}
	custodyID, err := newID()
	if err != nil {
		return Item{}, fmt.Errorf("create custody id: %w", err)
	}
	eventID, err := newID()
	if err != nil {
		return Item{}, fmt.Errorf("create event id: %w", err)
	}
	auditID, err := newID()
	if err != nil {
		return Item{}, fmt.Errorf("create audit id: %w", err)
	}
	publicCode, err := newPublicCode()
	if err != nil {
		return Item{}, fmt.Errorf("create public code: %w", err)
	}

	privateProfile, err := marshalPrivateProfile(input.PrivateProfile)
	if err != nil {
		return Item{}, fmt.Errorf("create item: %w", err)
	}
	photos, err := marshalPhotos(input.Photos)
	if err != nil {
		return Item{}, fmt.Errorf("create item: %w", err)
	}

	custodyMode, custodianID := custodyFor(input)
	rightsAssignedAt := now

	row := record.ItemRecord{
		ID:               itemID,
		PublicCode:       publicCode,
		Source:           input.Source,
		CustodyMode:      custodyMode,
		Status:           StatusPendingCustody,
		LegalFinderID:    input.LegalFinderID,
		RightsAssignedAt: &rightsAssignedAt,
		Category:         input.Category,
		Brand:            input.Brand,
		Model:            input.Model,
		Color:            input.Color,
		Condition:        input.Condition,
		PrivateProfile:   privateProfile,
		Photos:           photos,
		FoundAt:          input.FoundAt,
		FoundLat:         input.FoundLat,
		FoundLng:         input.FoundLng,
		FoundZone:        input.FoundZone,
		FoundNotes:       input.FoundNotes,
		CustodianID:      custodianID,
		RiskTier:         riskTier,
		Sensitive:        sensitive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	maxCustodyDays := intermediaryCustodyDays
	if sensitive {
		maxCustodyDays = sensitiveCustodyDays
	}
	custody := record.CustodyRecord{
		ID:                custodyID,
		ItemID:            itemID,
		CustodianType:     custodyMode,
		CustodianID:       custodianID,
		LegalFinderID:     input.LegalFinderID,
		RightsAssignedAt:  &rightsAssignedAt,
		ApprovalAuthority: custodyMode,
		MaxCustodyDays:    maxCustodyDays,
	}
	event := record.CustodyEventRecord{
		ID:        eventID,
		ItemID:    itemID,
		Type:      EventRightsAssigned,
		ActorID:   input.FinderID,
		Timestamp: now,
		Notes:     fmt.Sprintf("legal finder assigned to %s", input.LegalFinderID),
	}
	metadata, err := json.Marshal(map[string]string{
		"source":     input.Source,
		"publicCode": publicCode,
	})
	if err != nil {
		return Item{}, fmt.Errorf("create item audit metadata: %w", err)
	}
	audit := record.AuditLogRecord{
		ID:        auditID,
		ActorID:   input.FinderID,
		Action:    "item.enrolled",
		Entity:    "item",
		EntityID:  itemID,
		Metadata:  string(metadata),
		Timestamp: now,
	}

	if err := s.db.RunInTransaction(ctx, func(ctx context.Context, tx repository.Repository) error {
		if err := tx.CreateItem(ctx, row); err != nil {
			return err
		}
		if err := tx.CreateCustody(ctx, custody); err != nil {
			return err
		}
		if err := tx.CreateCustodyEvent(ctx, event); err != nil {
			return err
		}
		return tx.CreateAuditLog(ctx, audit)
	}); err != nil {
		return Item{}, fmt.Errorf("create item: %w", err)
	}

	return itemFromRecord(row)
}

// GetItem returns the item when the actor is the current custodian, its legal
// finder, or an institution operator. Everyone else — including finders and
// searchers — is forbidden.
func (s *service) GetItem(ctx context.Context, id string, actor Actor) (Item, error) {
	row, err := s.db.GetItemByID(ctx, id)
	if err != nil {
		return Item{}, fmt.Errorf("get item %q: %w", id, err)
	}

	switch actor.Role {
	case ActorRoleInstitution:
		// Institutions may read any item they are operationally responsible for.
	case ActorRoleIntermediary:
		if actor.ID == "" || (actor.ID != row.CustodianID && actor.ID != row.LegalFinderID) {
			return Item{}, fmt.Errorf("get item %q: %w", id, errs.ErrForbidden)
		}
	default:
		return Item{}, fmt.Errorf("get item %q: %w", id, errs.ErrForbidden)
	}

	item, err := itemFromRecord(row)
	if err != nil {
		return Item{}, fmt.Errorf("get item %q: %w", id, err)
	}
	return item, nil
}

// ListItemsByCustodian returns the items currently held by custodianID. Items
// that already left custody are intentionally omitted: the intermediary
// dashboard only shows open work.
func (s *service) ListItemsByCustodian(ctx context.Context, custodianID string) ([]Item, error) {
	if strings.TrimSpace(custodianID) == "" {
		return nil, fmt.Errorf("list items: custodian id is required: %w", errs.ErrInvalidInput)
	}

	rows, err := s.db.ListItemsByCustodian(ctx, custodianID)
	if err != nil {
		return nil, fmt.Errorf("list items for custodian %q: %w", custodianID, err)
	}

	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		if row.Status != StatusPendingCustody && row.Status != StatusInCustody {
			continue
		}
		item, err := itemFromRecord(row)
		if err != nil {
			return nil, fmt.Errorf("list items for custodian %q: %w", custodianID, err)
		}
		items = append(items, item)
	}
	return items, nil
}

// SavePhoto stores one uploaded image through the configured storage port.
func (s *service) SavePhoto(r io.Reader) (string, error) {
	if s.deps.Storage == nil {
		return "", fmt.Errorf("save photo: storage is not configured: %w", errs.ErrUnavailable)
	}
	path, err := s.deps.Storage.Save(r)
	if err != nil {
		return "", fmt.Errorf("save photo: %w", err)
	}
	return path, nil
}

// DeletePhoto rolls back one stored upload.
func (s *service) DeletePhoto(path string) error {
	if s.deps.Storage == nil {
		return fmt.Errorf("delete photo: storage is not configured: %w", errs.ErrUnavailable)
	}
	if err := s.deps.Storage.Delete(path); err != nil {
		return fmt.Errorf("delete photo %q: %w", path, err)
	}
	return nil
}

// OpenUpload returns a stored photo and its detected content type. The storage
// adapter is responsible for rejecting path traversal.
func (s *service) OpenUpload(filename string) (string, []byte, error) {
	if s.deps.Storage == nil {
		return "", nil, fmt.Errorf("open upload: storage is not configured: %w", errs.ErrUnavailable)
	}
	data, err := s.deps.Storage.Read(filename)
	if err != nil {
		return "", nil, fmt.Errorf("open upload %q: %w", filename, err)
	}
	return http.DetectContentType(data), data, nil
}

// normalizeInput trims user-supplied strings and drops empty photo entries.
func normalizeInput(input CreateItemInput) CreateItemInput {
	input.Source = strings.TrimSpace(input.Source)
	input.FinderID = strings.TrimSpace(input.FinderID)
	input.IntermediaryID = strings.TrimSpace(input.IntermediaryID)
	input.LegalFinderID = strings.TrimSpace(input.LegalFinderID)
	input.Category = strings.TrimSpace(input.Category)
	input.Brand = strings.TrimSpace(input.Brand)
	input.Model = strings.TrimSpace(input.Model)
	input.Color = strings.TrimSpace(input.Color)
	input.Condition = strings.TrimSpace(input.Condition)
	input.FoundZone = strings.TrimSpace(input.FoundZone)
	input.FoundNotes = strings.TrimSpace(input.FoundNotes)

	photos := make([]string, 0, len(input.Photos))
	for _, photo := range input.Photos {
		if trimmed := strings.TrimSpace(photo); trimmed != "" {
			photos = append(photos, trimmed)
		}
	}
	input.Photos = photos

	if input.PrivateProfile == nil {
		input.PrivateProfile = map[string]any{}
	}
	return input
}

// ValidateCreateInput normalizes the enrollment command and enforces its
// domain invariants without touching the database or writing any photo. The
// HTTP layer calls it before saving uploads so a rejected enrollment leaves no
// orphan files.
func ValidateCreateInput(input CreateItemInput) error {
	return validateInput(normalizeInput(input))
}

// validateInput enforces the enrollment invariants that do not depend on the
// caller's role: rights must be assigned, the legal finder must be the
// intermediary, and operator intake must name the institution.
func validateInput(input CreateItemInput) error {
	if !input.RightsAssigned {
		return fmt.Errorf("rights assignment is required: %w", errs.ErrInvalidInput)
	}
	if input.FinderID == "" {
		return fmt.Errorf("finder id is required: %w", errs.ErrInvalidInput)
	}
	if input.LegalFinderID == "" {
		return fmt.Errorf("legal finder id is required: %w", errs.ErrInvalidInput)
	}

	switch input.Source {
	case SourceOperator:
		// Official intake: the institution is the legal finder. Task 11 adds
		// the rest of the operator flow; the path stays here so items created
		// by an operator already point at the PA.
		return nil
	case SourceFinderSelf, SourceFinderMediated:
		if input.IntermediaryID == "" {
			return fmt.Errorf("intermediary id is required: %w", errs.ErrInvalidInput)
		}
		if input.IntermediaryID != input.LegalFinderID {
			return fmt.Errorf("legal finder must be the intermediary: %w", errs.ErrInvalidInput)
		}
		return nil
	default:
		return fmt.Errorf("unknown item source %q: %w", input.Source, errs.ErrInvalidInput)
	}
}

// custodyFor resolves the custody mode and custodian from the source.
func custodyFor(input CreateItemInput) (mode, custodianID string) {
	if input.Source == SourceOperator {
		return CustodyModeInstitution, input.LegalFinderID
	}
	return CustodyModeIntermediary, input.IntermediaryID
}

// imageInputs turns stored relative paths into catalog inputs.
func imageInputs(paths []string) []ImageInput {
	images := make([]ImageInput, 0, len(paths))
	for _, path := range paths {
		images = append(images, ImageInput{Path: path})
	}
	return images
}

func newID() (string, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// newPublicCode returns "LF-" plus 8 uppercase hex characters. The code is a
// receipt handle, not a secret: it is safe to show to the finder.
func newPublicCode() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("LF-%X", buf), nil
}

func isConflict(err error) bool {
	return errors.Is(err, errs.ErrConflict)
}
