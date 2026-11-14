package dto

import (
	"time"

	claimdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
)

// CreateClaimPayload is the JSON body of POST /api/claims. It deliberately has
// no confidence field: the service recomputes the score from the item profile
// and the searcher answers, so a client can never forge an approval.
type CreateClaimPayload struct {
	ItemID        string            `json:"itemId"`
	ReturnChannel string            `json:"returnChannel"`
	Answers       map[string]string `json:"answers"`
	Evidence      []string          `json:"evidence"`
	Flags         []string          `json:"flags"`
}

// ClaimOwnerResponse is what a searcher sees: status, confidence band and
// pickup channel. It contains no raw confidence and no verification report.
type ClaimOwnerResponse struct {
	ID             string    `json:"id"`
	ItemID         string    `json:"itemId"`
	Status         string    `json:"status"`
	ConfidenceBand string    `json:"confidenceBand"`
	ReturnChannel  string    `json:"returnChannel"`
	CreatedAt      time.Time `json:"createdAt"`
}

// ClaimReport is the full verification report shown to the institution. It
// never contains the item's private profile values.
type ClaimReport struct {
	Questions     map[string]itemdomain.Question `json:"questions"`
	Answers       map[string]string              `json:"answers"`
	Probabilities map[string]itemdomain.Answer   `json:"probabilities"`
	Confidence    float64                        `json:"confidence"`
	Rationale     string                         `json:"rationale"`
	ModelVersion  string                         `json:"modelVersion"`
	Evidence      []string                       `json:"evidence"`
	Flags         []string                       `json:"flags"`
	Decision      string                         `json:"decision"`
	Reasons       []string                       `json:"reasons"`
}

// ClaimReportResponse is the institution/admin claim read model.
type ClaimReportResponse struct {
	ClaimOwnerResponse
	Confidence float64      `json:"confidence"`
	Report     *ClaimReport `json:"report,omitempty"`
}

// ToClaimOwnerResponse maps the searcher-safe claim view.
func ToClaimOwnerResponse(c claimdomain.Claim) ClaimOwnerResponse {
	return ClaimOwnerResponse{
		ID:             c.ID,
		ItemID:         c.ItemID,
		Status:         c.Status,
		ConfidenceBand: c.ConfidenceBand,
		ReturnChannel:  c.ReturnChannel,
		CreatedAt:      c.CreatedAt,
	}
}

// ClaimSummaryResponse is the intermediary custodian view: the owner fields plus
// the safe evidence and flags. It deliberately has no questions, answers,
// probabilities, rationale or confidence.
type ClaimSummaryResponse struct {
	ClaimOwnerResponse
	Evidence []string `json:"evidence"`
	Flags    []string `json:"flags"`
}

// ToClaimSummaryResponse maps the safe custodian view.
func ToClaimSummaryResponse(c claimdomain.Claim) ClaimSummaryResponse {
	return ClaimSummaryResponse{
		ClaimOwnerResponse: ToClaimOwnerResponse(c),
		Evidence:           nonNilStrings(c.Evidence),
		Flags:              nonNilStrings(c.Flags),
	}
}

// ToClaimReportResponse maps the full institution view.
func ToClaimReportResponse(c claimdomain.Claim) ClaimReportResponse {
	response := ClaimReportResponse{
		ClaimOwnerResponse: ToClaimOwnerResponse(c),
		Confidence:         c.Confidence,
	}
	if c.Report == nil {
		return response
	}
	report := ClaimReport{
		Questions:     nonNilQuestionMap(c.Report.Questions),
		Answers:       nonNilStringMap(c.Report.Answers),
		Probabilities: nonNilAnswerMap(c.Report.Probabilities),
		Confidence:    c.Report.Confidence,
		Rationale:     c.Report.Rationale,
		ModelVersion:  c.Report.ModelVersion,
		Evidence:      nonNilStrings(c.Report.Evidence),
		Flags:         nonNilStrings(c.Report.Flags),
		Decision:      c.Report.Decision,
		Reasons:       nonNilStrings(c.Report.Reasons),
	}
	response.Report = &report
	return response
}

func nonNilQuestionMap(values map[string]itemdomain.Question) map[string]itemdomain.Question {
	if values == nil {
		return map[string]itemdomain.Question{}
	}
	return values
}

func nonNilAnswerMap(values map[string]itemdomain.Answer) map[string]itemdomain.Answer {
	if values == nil {
		return map[string]itemdomain.Answer{}
	}
	return values
}

func nonNilStringMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
