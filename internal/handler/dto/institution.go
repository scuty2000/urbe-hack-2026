package dto

import (
	"time"

	institutiondomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/institution"
)

// OfficialItemPayload is the JSON part of the multipart POST
// /api/institution/items request. The institution is derived from the session:
// the payload cannot choose a different source or legal finder.
type OfficialItemPayload struct {
	Category       string         `json:"category"`
	Brand          string         `json:"brand"`
	Model          string         `json:"model"`
	Color          string         `json:"color"`
	Condition      string         `json:"condition"`
	PrivateProfile map[string]any `json:"privateProfile"`
	FoundAt        time.Time      `json:"foundAt"`
	FoundLat       float64        `json:"foundLat"`
	FoundLng       float64        `json:"foundLng"`
	FoundZone      string         `json:"foundZone"`
	FoundNotes     string         `json:"foundNotes"`
}

// InstitutionPickupItem is the safe item summary in the PA pickup queue. It
// has no private profile, no notes and no exact coordinates.
type InstitutionPickupItem struct {
	ID             string     `json:"id"`
	PublicCode     string     `json:"publicCode"`
	Category       string     `json:"category"`
	Brand          string     `json:"brand,omitempty"`
	Model          string     `json:"model,omitempty"`
	Color          string     `json:"color,omitempty"`
	Status         string     `json:"status"`
	FoundZone      string     `json:"foundZone"`
	IntermediaryID string     `json:"intermediaryId"`
	Sensitive      bool       `json:"sensitive"`
	DueAt          *time.Time `json:"dueAt,omitempty"`
	DaysOverdue    int        `json:"daysOverdue"`
}

// InstitutionPickupGroup groups the pickup queue by holding intermediary/zone.
type InstitutionPickupGroup struct {
	IntermediaryID string                  `json:"intermediaryId"`
	Zone           string                  `json:"zone"`
	Sensitive      bool                    `json:"sensitive"`
	Count          int                     `json:"count"`
	Items          []InstitutionPickupItem `json:"items"`
}

// InstitutionPickupList is the PA pickup queue response.
type InstitutionPickupList struct {
	Groups []InstitutionPickupGroup `json:"groups"`
	Total  int                      `json:"total"`
}

// ReviewClaimPayload is the JSON body of
// POST /api/institution/claims/{id}/review.
type ReviewClaimPayload struct {
	Decision string `json:"decision"`
}

// ToInstitutionPickupList maps the pickup groups and counts the items.
func ToInstitutionPickupList(groups []institutiondomain.PickupGroup) InstitutionPickupList {
	response := InstitutionPickupList{Groups: make([]InstitutionPickupGroup, 0, len(groups))}
	for _, group := range groups {
		items := make([]InstitutionPickupItem, 0, len(group.Items))
		for _, item := range group.Items {
			items = append(items, InstitutionPickupItem{
				ID:             item.ID,
				PublicCode:     item.PublicCode,
				Category:       item.Category,
				Brand:          item.Brand,
				Model:          item.Model,
				Color:          item.Color,
				Status:         item.Status,
				FoundZone:      item.FoundZone,
				IntermediaryID: item.IntermediaryID,
				Sensitive:      item.Sensitive,
				DueAt:          item.DueAt,
				DaysOverdue:    item.DaysOverdue,
			})
		}
		response.Groups = append(response.Groups, InstitutionPickupGroup{
			IntermediaryID: group.IntermediaryID,
			Zone:           group.Zone,
			Sensitive:      group.Sensitive,
			Count:          len(items),
			Items:          items,
		})
		response.Total += len(items)
	}
	return response
}
