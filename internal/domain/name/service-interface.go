package nameservice

import "context"

// ServiceName defines the business operations available on Name entities.
// All methods accept a context.Context for cancellation and tracing propagation.
type ServiceName interface {
	// GetByID returns the Name with the given id.
	// Returns ErrNotFound when no such entity exists.
	GetByID(ctx context.Context, id int64) (Name, error)

	// Create validates and persists a new Name, returning the created entity.
	// Returns ErrInvalidInput when the name is empty or exceeds the allowed length.
	Create(ctx context.Context, value string) (Name, error)

	// UpdateByID replaces the value of the Name identified by id.
	// Returns ErrNotFound when no such entity exists.
	// Returns ErrInvalidInput when the new value fails validation.
	UpdateByID(ctx context.Context, id int64, value string) (Name, error)

	// DeleteByID removes the Name identified by id.
	// Returns ErrNotFound when no such entity exists.
	DeleteByID(ctx context.Context, id int64) error

	// Replace atomically deletes the Name identified by id and creates a new one
	// with newValue, returning the newly created entity.
	// Both operations run inside a single transaction: if either fails the database
	// is left unchanged.
	// Returns ErrNotFound when no entity with id exists.
	// Returns ErrInvalidInput when newValue fails validation.
	Replace(ctx context.Context, id int64, newValue string) (Name, error)
}
