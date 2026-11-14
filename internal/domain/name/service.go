package nameservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

const maxNameLength = 255

// nameService is the concrete implementation of NameService.
type nameService struct {
	db repository.Database // access the repository layer with transaction support
}

// NewNameService returns a NameService backed by the provided Repository and Transactor.
func New(repo repository.Database) ServiceName {
	return &nameService{db: repo}
}
func (s *nameService) GetByID(ctx context.Context, id int64) (Name, error) {
	row, err := s.db.GetNameByID(ctx, id)
	if err != nil {
		return Name{}, fmt.Errorf("cannot find %d in db: %w", id, err)
	}
	return nameFromRow(row), nil
}
func (s *nameService) Create(ctx context.Context, value string) (Name, error) {
	if err := validateName(value); err != nil {
		return Name{}, fmt.Errorf("create: %w", err)
	}
	id, err := s.db.CreateName(ctx, value)
	if err != nil {
		return Name{}, fmt.Errorf("create: %w", err)
	}
	return nameFromRow(record.NameRecord{ID: id, Value: value}), nil
}
func (s *nameService) UpdateByID(ctx context.Context, id int64, value string) (Name, error) {
	if err := validateName(value); err != nil {
		return Name{}, fmt.Errorf("UpdateByID id=%d: %w", id, err)
	}
	if err := s.db.UpdateNameByID(ctx, id, value); err != nil {
		return Name{}, fmt.Errorf("UpdateByID id=%d: %w", id, err)
	}
	return nameFromRow(record.NameRecord{ID: id, Value: value}), nil
}
func (s *nameService) DeleteByID(ctx context.Context, id int64) error {
	if err := s.db.DeleteNameByID(ctx, id); err != nil {
		return fmt.Errorf("DeleteByID id=%d: %w", id, err)
	}
	return nil
}

// Replace atomically removes the Name identified by id and inserts a new one
// with newValue. Both operations run inside a single transaction so the database
// is never left in a half-updated state.
func (s *nameService) Replace(ctx context.Context, id int64, newValue string) (Name, error) {
	if err := validateName(newValue); err != nil {
		return Name{}, fmt.Errorf("replace id=%d: %w", id, err)
	}
	var created Name
	err := s.db.RunInTransaction(ctx, func(ctx context.Context, repoTx repository.Repository) error {
		// Verify the entity exists before deleting — surfaces ErrNotFound cleanly.
		if _, err := repoTx.GetNameByID(ctx, id); err != nil {
			return fmt.Errorf("replace id=%d: %w", id, err)
		}
		if err := repoTx.DeleteNameByID(ctx, id); err != nil {
			return fmt.Errorf("replace delete id=%d: %w", id, err)
		}
		newID, err := repoTx.CreateName(ctx, newValue)
		if err != nil {
			return fmt.Errorf("replace create: %w", err)
		}
		// We already know the id and value — no need for an extra GET round-trip.
		created = nameFromRow(record.NameRecord{ID: newID, Value: newValue})
		return nil
	})
	if err != nil {
		return Name{}, err
	}
	return created, nil
}

// validateName enforces domain-level invariants on a Name value.
func validateName(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("name cannot be empty or whitespace: %w", errs.ErrInvalidInput)
	}
	if len(value) > maxNameLength {
		return fmt.Errorf("name exceeds %d characters: %w", maxNameLength, errs.ErrInvalidInput)
	}
	return nil
}
