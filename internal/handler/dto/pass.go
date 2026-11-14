package dto

import (
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	passdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/pass"
)

// CreatePassPayload is the JSON body of POST /api/passes. The claim owner is
// always the authenticated searcher: no actor or item id is accepted here.
type CreatePassPayload struct {
	ClaimID string `json:"claimId"`
}

// PassTokenPayload is the JSON body of the validate/redeem scanner routes.
type PassTokenPayload struct {
	Token string `json:"token"`
}

// PassItemSummary is the only item view returned with a pass. It deliberately
// contains no private profile, no evidence and no exact coordinates.
type PassItemSummary struct {
	ID          string `json:"id"`
	PublicCode  string `json:"publicCode"`
	Category    string `json:"category"`
	Brand       string `json:"brand,omitempty"`
	Model       string `json:"model,omitempty"`
	Color       string `json:"color,omitempty"`
	Status      string `json:"status"`
	CustodianID string `json:"custodianId"`
}

// PassResponse is the issuing response. Token is returned exactly once, to the
// searcher; it never appears in any other response.
type PassResponse struct {
	PassID        string          `json:"passId"`
	ClaimID       string          `json:"claimId"`
	ItemID        string          `json:"itemId"`
	Token         string          `json:"token"`
	Status        string          `json:"status"`
	ReturnChannel string          `json:"returnChannel"`
	ExpiresAt     time.Time       `json:"expiresAt"`
	Instructions  []string        `json:"instructions"`
	Item          PassItemSummary `json:"item"`
}

// PassValidationResponse is the scanner's read-only check. A failed check is
// still HTTP 200 with valid=false and a coarse reason.
type PassValidationResponse struct {
	Valid     bool             `json:"valid"`
	Reason    string           `json:"reason,omitempty"`
	ExpiresAt *time.Time       `json:"expiresAt,omitempty"`
	Item      *PassItemSummary `json:"item,omitempty"`
}

// PassRedemptionResponse is returned after a successful handover.
type PassRedemptionResponse struct {
	Item       PassItemSummary `json:"item"`
	ReturnedAt time.Time       `json:"returnedAt"`
}

// ToPassItemSummary maps the safe subset of an item returned by redemption.
func ToPassItemSummary(it itemdomain.Item) PassItemSummary {
	return PassItemSummary{
		ID:          it.ID,
		PublicCode:  it.PublicCode,
		Category:    it.Category,
		Brand:       it.Brand,
		Model:       it.Model,
		Color:       it.Color,
		Status:      it.Status,
		CustodianID: it.CustodianID,
	}
}

// toPassItemSummary maps the domain-level safe pass summary.
func toPassItemSummary(summary passdomain.ItemSummary) PassItemSummary {
	return PassItemSummary{
		ID:          summary.ID,
		PublicCode:  summary.PublicCode,
		Category:    summary.Category,
		Brand:       summary.Brand,
		Model:       summary.Model,
		Color:       summary.Color,
		Status:      summary.Status,
		CustodianID: summary.CustodianID,
	}
}

// ToPassResponse maps the issuing read model.
func ToPassResponse(p passdomain.Pass) PassResponse {
	response := PassResponse{
		PassID:        p.ID,
		ClaimID:       p.ClaimID,
		ItemID:        p.ItemID,
		Token:         p.Token,
		Status:        p.Status,
		ReturnChannel: p.ReturnChannel,
		ExpiresAt:     p.ExpiresAt,
		Instructions:  nonNilStrings(p.Instructions),
	}
	if p.Item != nil {
		response.Item = toPassItemSummary(*p.Item)
	}
	return response
}

// ToPassValidationResponse maps a validation result, including the safe item
// summary only when the pass is valid.
func ToPassValidationResponse(v passdomain.Validation) PassValidationResponse {
	response := PassValidationResponse{Valid: v.Valid, Reason: v.Reason}
	if !v.Valid || v.Item == nil {
		return response
	}
	expiresAt := v.ExpiresAt
	response.ExpiresAt = &expiresAt
	summary := toPassItemSummary(*v.Item)
	response.Item = &summary
	return response
}
