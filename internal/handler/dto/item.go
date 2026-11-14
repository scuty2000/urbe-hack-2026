package dto

import (
	"path"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
)

// CreateItemPayload is the JSON part of the multipart POST /api/items request.
// Photos travel as repeated `photos` file parts, never inside this payload.
type CreateItemPayload struct {
	IntermediaryID string         `json:"intermediaryId"`
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
	// RightsAssigned is the explicit transfer of the finder's legal status to
	// the intermediary. Enrollment is rejected unless it is true.
	RightsAssigned bool `json:"rightsAssigned"`
}

// ItemResponse is the role-safe representation of an item.
//
// It deliberately contains no private profile and no exact coordinates:
// searchers never reach this DTO, but finders receive it as their receipt and
// the zone is the only location information they need.
type ItemResponse struct {
	ID                string     `json:"id"`
	PublicCode        string     `json:"publicCode"`
	Source            string     `json:"source"`
	CustodyMode       string     `json:"custodyMode"`
	Status            string     `json:"status"`
	LegalFinderID     string     `json:"legalFinderId"`
	CustodianID       string     `json:"custodianId"`
	Category          string     `json:"category"`
	Brand             string     `json:"brand"`
	Model             string     `json:"model"`
	Color             string     `json:"color"`
	Condition         string     `json:"condition"`
	Photos            []string   `json:"photos"`
	FoundAt           time.Time  `json:"foundAt"`
	FoundZone         string     `json:"foundZone"`
	FoundNotes        string     `json:"foundNotes"`
	RiskTier          string     `json:"riskTier"`
	Sensitive         bool       `json:"sensitive"`
	RightsAssigned    bool       `json:"rightsAssigned"`
	RightsAssignedAt  *time.Time `json:"rightsAssignedAt,omitempty"`
	TransferredAt     *time.Time `json:"transferredAt,omitempty"`
	LegalTermStart    *time.Time `json:"legalTermStart,omitempty"`
	OfferedToFinderAt *time.Time `json:"offeredToFinderAt,omitempty"`
	DisposedAt        *time.Time `json:"disposedAt,omitempty"`
	// DueAt and DaysLeft describe the open custody deadline. They are absent
	// while the item is still pending custody or has no accepted handover.
	DueAt     *time.Time `json:"dueAt,omitempty"`
	DaysLeft  *int       `json:"daysLeft,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// ItemListResponse is the intermediary dashboard payload.
type ItemListResponse struct {
	Items []ItemResponse `json:"items"`
}

// ToItemResponse maps a domain item into the API response. Upload paths are
// converted to authenticated API URLs; raw storage paths never leak.
func ToItemResponse(it itemdomain.Item) ItemResponse {
	photos := make([]string, 0, len(it.Photos))
	for _, stored := range it.Photos {
		if filename := path.Base(stored); filename != "." && filename != "/" && filename != "" {
			photos = append(photos, "/api/uploads/"+filename)
		}
	}

	return ItemResponse{
		ID:                it.ID,
		PublicCode:        it.PublicCode,
		Source:            it.Source,
		CustodyMode:       it.CustodyMode,
		Status:            it.Status,
		LegalFinderID:     it.LegalFinderID,
		CustodianID:       it.CustodianID,
		Category:          it.Category,
		Brand:             it.Brand,
		Model:             it.Model,
		Color:             it.Color,
		Condition:         it.Condition,
		Photos:            photos,
		FoundAt:           it.FoundAt,
		FoundZone:         it.FoundZone,
		FoundNotes:        it.FoundNotes,
		RiskTier:          it.RiskTier,
		Sensitive:         it.Sensitive,
		RightsAssigned:    it.RightsAssignedAt != nil,
		RightsAssignedAt:  it.RightsAssignedAt,
		TransferredAt:     it.TransferredAt,
		LegalTermStart:    it.LegalTermStart,
		OfferedToFinderAt: it.OfferedToFinderAt,
		DisposedAt:        it.DisposedAt,
		DueAt:             it.DueAt,
		CreatedAt:         it.CreatedAt,
	}
}

// ToItemListResponse maps a slice of domain items.
func ToItemListResponse(items []itemdomain.Item) ItemListResponse {
	response := ItemListResponse{Items: make([]ItemResponse, 0, len(items))}
	for _, it := range items {
		response.Items = append(response.Items, ToItemResponse(it))
	}
	return response
}
