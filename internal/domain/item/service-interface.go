package item

import (
	"context"
	"io"
	"time"
)

// Deps are the outbound ports of the item service. All of them are optional:
// without D3 the catalog falls back to form values, and without storage the
// service still enrolls items (uploads are rejected by the HTTP layer).
type Deps struct {
	Storage Storage
	D3      Cataloger
	// AllowD3Images sends item photos to D3. It is false by default and always
	// false for document categories, regardless of this flag.
	AllowD3Images bool
	// Now is injectable for deterministic tests; production uses time.Now.
	Now func() time.Time
}

// CreateItemInput is the validated enrollment command. PrivateProfile and
// Photos are domain values; the service owns their JSON serialisation.
type CreateItemInput struct {
	Source         string
	FinderID       string
	IntermediaryID string
	LegalFinderID  string
	RightsAssigned bool
	Category       string
	Brand          string
	Model          string
	Color          string
	Condition      string
	PrivateProfile map[string]any
	Photos         []string
	FoundAt        time.Time
	FoundLat       float64
	FoundLng       float64
	FoundZone      string
	FoundNotes     string
}

// Service defines the item operations exposed to the HTTP layer.
type Service interface {
	// CreateItem enrolls an item, assigns the legal finder, creates its first
	// custody row and records the rights assignment atomically.
	// Returns ErrInvalidInput when a domain invariant is violated (missing
	// rights assignment, legal finder mismatch, unknown source).
	CreateItem(ctx context.Context, input CreateItemInput) (Item, error)

	// GetItem returns an item to an authorised intermediary or institution.
	// Returns ErrForbidden for other roles or for an intermediary that is not
	// the current custodian / legal finder.
	GetItem(ctx context.Context, id string, actor Actor) (Item, error)

	// ListItemsByCustodian returns the open items (pending_custody or
	// in_custody) currently held by custodianID.
	ListItemsByCustodian(ctx context.Context, custodianID string) ([]Item, error)

	// Catalog classifies photos with D3 over the form values. It never fails
	// because D3 is unavailable: Source is then "form".
	Catalog(ctx context.Context, photos []ImageInput, form CatalogForm) (CatalogResult, error)

	// SavePhoto stores one uploaded image and returns its relative path.
	SavePhoto(r io.Reader) (string, error)

	// DeletePhoto removes a stored photo. It is used to roll back uploads when
	// enrollment fails after the files were written.
	DeletePhoto(path string) error

	// OpenUpload returns the content type and bytes of a stored photo. The
	// filename is sanitised by the storage adapter.
	OpenUpload(filename string) (contentType string, data []byte, err error)
}
