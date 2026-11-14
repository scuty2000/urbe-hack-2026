/*
Package item contains the Item aggregate: enrollment of a found object, the
rights assignment to the legal finder, risk classification and the read models
used by the HTTP layer.

It has no dependency on Gin, SQL or any transport concern. The repository is
reached through repository.Database, while the D3 decision engine and the local
upload store are injected as small ports so tests can fake them without any
network or filesystem access.
*/
package item

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository/record"
)

// Sources record how an item entered the registry.
const (
	SourceFinderSelf     = "finder_self"
	SourceFinderMediated = "finder_mediated"
	SourceOperator       = "operator"
)

// Custody modes mirror the items.custody_mode column.
const (
	CustodyModeIntermediary = "intermediary"
	CustodyModeInstitution  = "institution"
)

// Item lifecycle statuses relevant to enrollment. The full state machine is
// implemented by later tasks; these constants keep the strings out of handlers.
const (
	StatusEnrolled        = "enrolled"
	StatusPendingCustody  = "pending_custody"
	StatusCustodyRejected = "custody_rejected"
	StatusInCustody       = "in_custody"
	StatusReturned        = "returned"
)

// Risk tiers, ordered from least to most restrictive.
const (
	RiskLow       = "low"
	RiskMedium    = "medium"
	RiskHigh      = "high"
	RiskSensitive = "sensitive"
)

// Custody event types used by this task. Later tasks append the remaining
// transitions defined by the design document.
const (
	EventEnrolled       = "enrolled"
	EventRightsAssigned = "rights_assigned"
)

// Actor roles used by the item domain. They intentionally mirror the auth
// package strings without importing it, so this package stays framework-free.
const (
	ActorRoleFinder       = "finder"
	ActorRoleSearcher     = "searcher"
	ActorRoleIntermediary = "intermediary"
	ActorRoleInstitution  = "institution"
	ActorRoleAdmin        = "admin"
)

// Actor is the authenticated caller asking for an item.
type Actor struct {
	ID   string
	Role string
}

// Item is the aggregate root of the lost-and-found registry.
//
// PrivateProfile holds unique identifying details and is never serialised by
// the HTTP DTOs. Photos are relative paths under the upload storage, not public
// filesystem locations.
type Item struct {
	ID               string
	PublicCode       string
	Source           string
	CustodyMode      string
	Status           string
	LegalFinderID    string
	RightsAssignedAt *time.Time
	Category         string
	Brand            string
	Model            string
	Color            string
	Condition        string
	PrivateProfile   map[string]any
	Photos           []string
	FoundAt          time.Time
	FoundLat         float64
	FoundLng         float64
	FoundZone        string
	FoundNotes       string
	CustodianID      string
	RiskTier         string
	Sensitive        bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// itemFromRecord maps a persisted row to the domain entity. JSON columns are
// owned by this package, so decoding failures are surfaced instead of being
// silently replaced with empty values.
func itemFromRecord(row record.ItemRecord) (Item, error) {
	privateProfile := map[string]any{}
	if row.PrivateProfile != "" {
		if err := json.Unmarshal([]byte(row.PrivateProfile), &privateProfile); err != nil {
			return Item{}, fmt.Errorf("decoding private profile for item %q: %w", row.ID, err)
		}
	}

	photos := []string{}
	if row.Photos != "" {
		if err := json.Unmarshal([]byte(row.Photos), &photos); err != nil {
			return Item{}, fmt.Errorf("decoding photos for item %q: %w", row.ID, err)
		}
	}

	return Item{
		ID:               row.ID,
		PublicCode:       row.PublicCode,
		Source:           row.Source,
		CustodyMode:      row.CustodyMode,
		Status:           row.Status,
		LegalFinderID:    row.LegalFinderID,
		RightsAssignedAt: row.RightsAssignedAt,
		Category:         row.Category,
		Brand:            row.Brand,
		Model:            row.Model,
		Color:            row.Color,
		Condition:        row.Condition,
		PrivateProfile:   privateProfile,
		Photos:           photos,
		FoundAt:          row.FoundAt,
		FoundLat:         row.FoundLat,
		FoundLng:         row.FoundLng,
		FoundZone:        row.FoundZone,
		FoundNotes:       row.FoundNotes,
		CustodianID:      row.CustodianID,
		RiskTier:         row.RiskTier,
		Sensitive:        row.Sensitive,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}, nil
}

// marshalPrivateProfile serialises the private profile, defaulting to an empty
// JSON object so the NOT NULL column is always valid JSON.
func marshalPrivateProfile(profile map[string]any) (string, error) {
	if len(profile) == 0 {
		return "{}", nil
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		return "", fmt.Errorf("encoding private profile: %w", err)
	}
	return string(encoded), nil
}

// marshalPhotos serialises the upload paths, defaulting to an empty JSON array.
func marshalPhotos(photos []string) (string, error) {
	if len(photos) == 0 {
		return "[]", nil
	}
	encoded, err := json.Marshal(photos)
	if err != nil {
		return "", fmt.Errorf("encoding photos: %w", err)
	}
	return string(encoded), nil
}
