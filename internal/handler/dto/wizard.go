package dto

import (
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/wizard"
)

// WizardAnalyzePayload is the small JSON part of the multipart analyze call.
type WizardAnalyzePayload struct {
	Slot string `json:"slot"`
	Note string `json:"note"`
}

// PhotoPlanSlotResponse is one additional photo the finder should take.
type PhotoPlanSlotResponse struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Instruction string `json:"instruction"`
	Required    bool   `json:"required"`
}

// WizardPrefillResponse is the editable card-1 prefill.
type WizardPrefillResponse struct {
	Category  string `json:"category"`
	Color     string `json:"color"`
	Condition string `json:"condition"`
}

// WizardAnalysisResponse is the finder wizard analysis.
type WizardAnalysisResponse struct {
	ObjectType        string                  `json:"objectType"`
	Confidence        float64                 `json:"confidence"`
	PhotoPlan         []PhotoPlanSlotResponse `json:"photoPlan"`
	Prefill           WizardPrefillResponse   `json:"prefill"`
	PrivatePrefill    string                  `json:"privatePrefill"`
	PrivateAttributes map[string]any          `json:"privateAttributes"`
	Warnings          []string                `json:"warnings"`
	Source            string                  `json:"source"`
	IsDocument        bool                    `json:"isDocument"`
	Sensitive         bool                    `json:"sensitive"`
}

// ReverseGeocodeRequest is the POST body for the zone lookup.
type ReverseGeocodeRequest struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// ReverseGeocodeResponse is the zone lookup result.
type ReverseGeocodeResponse struct {
	Zone      string `json:"zone"`
	PlaceName string `json:"placeName"`
	Source    string `json:"source"`
}

// DropPointsRequest is the POST body for the walking-distance ranking. Lat/Lng
// are optional: without them the service returns the active points in
// alphabetical order for a manual choice.
type DropPointsRequest struct {
	Lat   *float64 `json:"lat"`
	Lng   *float64 `json:"lng"`
	Limit int      `json:"limit"`
}

// DropPointResponse is one active partner with walking distance.
type DropPointResponse struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Address         string  `json:"address"`
	Hours           string  `json:"hours"`
	DistanceMeters  float64 `json:"distanceMeters"`
	DurationSeconds float64 `json:"durationSeconds"`
	WalkingMinutes  int     `json:"walkingMinutes"`
	Approximate     bool    `json:"approximate"`
	Source          string  `json:"source"`
}

// DropPointsResponse is the ranked list.
type DropPointsResponse struct {
	Points      []DropPointResponse `json:"points"`
	Source      string              `json:"source"`
	Approximate bool                `json:"approximate"`
}

// ToWizardAnalysisResponse maps the domain analysis.
func ToWizardAnalysisResponse(analysis wizard.Analysis) WizardAnalysisResponse {
	plan := make([]PhotoPlanSlotResponse, 0, len(analysis.PhotoPlan))
	for _, slot := range analysis.PhotoPlan {
		plan = append(plan, PhotoPlanSlotResponse{
			Key:         slot.Key,
			Label:       slot.Label,
			Instruction: slot.Instruction,
			Required:    slot.Required,
		})
	}
	return WizardAnalysisResponse{
		ObjectType: analysis.ObjectType,
		Confidence: analysis.Confidence,
		PhotoPlan:  plan,
		Prefill: WizardPrefillResponse{
			Category:  analysis.Prefill.Category,
			Color:     analysis.Prefill.Color,
			Condition: analysis.Prefill.Condition,
		},
		PrivatePrefill:    analysis.PrivatePrefill,
		PrivateAttributes: analysis.PrivateAttributes,
		Warnings:          nonNilStrings(analysis.Warnings),
		Source:            analysis.Source,
		IsDocument:        analysis.IsDocument,
		Sensitive:         analysis.Sensitive,
	}
}

// ToDropPointsResponse maps the ranked points.
func ToDropPointsResponse(points []wizard.DropPoint) DropPointsResponse {
	response := DropPointsResponse{Points: make([]DropPointResponse, 0, len(points)), Source: "none"}
	for i, point := range points {
		if i == 0 {
			response.Source = point.Source
			response.Approximate = point.Approximate
		}
		response.Points = append(response.Points, DropPointResponse{
			ID:              point.ID,
			Name:            point.Name,
			Address:         point.Address,
			Hours:           point.Hours,
			DistanceMeters:  point.DistanceMeters,
			DurationSeconds: point.DurationSeconds,
			WalkingMinutes:  point.WalkingMinutes,
			Approximate:     point.Approximate,
			Source:          point.Source,
		})
	}
	return response
}
