package wizard

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

const maxPhotos = 4

var allowedMediaTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

// Analyze classifies the first photo and produces the additional-photo plan
// and card-1/private prefill. Provider failures degrade to deterministic
// fallbacks; only invalid input is an error.
func (s *service) Analyze(ctx context.Context, input AnalyzeInput) (Analysis, error) {
	if err := validatePhotos(input.Photos); err != nil {
		return Analysis{}, err
	}
	if s.deps.Items == nil {
		return manualAnalysis(), nil
	}

	result, err := s.deps.Items.Analyze(ctx, input.Photos, item.CatalogForm{})
	if err != nil || result.Source != item.CatalogSourceD3 {
		return manualAnalysis(), nil
	}
	return s.analyzeResult(ctx, result, input.Photos), nil
}

func (s *service) analyzeResult(ctx context.Context, result item.CatalogResult, photos []item.ImageInput) Analysis {
	category := NormalizeCategory(result.Category)
	if category == "" {
		category = "altro"
	}
	// The category and the document type are independent signals from D3's
	// `is_document` noul answer. A document category or a detected document
	// type must stop the photo plan even when the noul answer is missing or
	// below its threshold.
	if result.IsDocument || category == "documenti" || strings.TrimSpace(result.DocumentType) != "" {
		return documentAnalysis()
	}

	plan := Plan{}
	source := "d3+fallback"
	warnings := make([]string, 0, 2)
	if s.deps.Planner != nil {
		plannerInput := PlannerInput{
			Category:   result.Category,
			Color:      result.Color,
			Condition:  result.Condition,
			CaseColor:  result.CaseColor,
			HasCrack:   result.HasCrack,
			HasSticker: result.HasSticker,
			HasCase:    result.HasCase,
			IsDocument: result.IsDocument,
			PhotoCount: len(photos),
			Photos:     photos,
		}
		planned, err := s.deps.Planner.Plan(ctx, plannerInput)
		if err == nil {
			plan = sanitizePlan(planned)
			source = "d3+deepseek"
		} else {
			warnings = append(warnings, "Alcuni suggerimenti automatici non sono disponibili: puoi completare i campi manualmente.")
		}
	} else {
		warnings = append(warnings, "Alcuni suggerimenti automatici non sono disponibili: puoi completare i campi manualmente.")
	}

	prefill := Prefill{
		Category:  category,
		Brand:     optionalField(plan.Prefill.Brand, 80),
		Model:     optionalField(plan.Prefill.Model, 80),
		Color:     NormalizeColor(plan.Prefill.Color),
		Condition: NormalizeCondition(plan.Prefill.Condition),
	}
	if prefill.Category == "altro" {
		if refined := NormalizeCategory(plan.Prefill.Category); refined != "altro" {
			prefill.Category = refined
		}
	}
	if prefill.Category == "documenti" {
		return documentAnalysis()
	}
	if len(plan.PhotoPlan) == 0 {
		plan.PhotoPlan = fallbackPlan(prefill.Category)
	}
	if prefill.Color == "" {
		prefill.Color = NormalizeColor(result.Color)
	}
	if prefill.Condition == "" {
		prefill.Condition = NormalizeCondition(result.Condition)
	}
	if prefill.Condition == "" {
		prefill.Condition = "good"
	}

	privatePrefill := optionalField(plan.PrivatePrefill, 480)
	if privatePrefill == "" {
		privatePrefill = fallbackPrivate(result)
	}

	_, sensitive := item.RiskForCategory(prefill.Category)
	return Analysis{
		ObjectType:        prefill.Category,
		PhotoPlan:         plan.PhotoPlan,
		Prefill:           prefill,
		PrivatePrefill:    privatePrefill,
		PrivateAttributes: privateAttributes(result),
		Warnings:          append(warnings, plan.Warnings...),
		Source:            source,
		Sensitive:         sensitive,
	}
}

// privateAttributes are the structured D3 observations the claim engine can
// verify. They are stored in the item's private profile; free-text details are
// never used as verification questions.
func privateAttributes(result item.CatalogResult) map[string]any {
	attributes := map[string]any{}
	if color := NormalizeColor(result.CaseColor); color != "" {
		attributes["case_color"] = color
	}
	attributes["has_crack"] = result.HasCrack
	attributes["has_sticker"] = result.HasSticker
	attributes["has_case"] = result.HasCase
	return attributes
}

func documentAnalysis() Analysis {
	return Analysis{
		ObjectType: "documenti",
		Prefill:    Prefill{Category: "documenti"},
		Warnings: []string{
			"Documento: non aggiungere foto del contenuto. Consegna l'oggetto al punto di raccolta; la PA gestirà la parte sensibile.",
		},
		Source:     "d3",
		IsDocument: true,
		Sensitive:  true,
	}
}

func manualAnalysis() Analysis {
	return Analysis{
		ObjectType: "altro",
		PhotoPlan: []PhotoSlot{{
			Key:         "detail",
			Label:       "Dettaglio",
			Instruction: "Fotografa un dettaglio o un segno particolare dell'oggetto (facoltativo).",
			Required:    false,
		}},
		Warnings: []string{"Analisi automatica non disponibile: compila i campi manualmente. Puoi comunque inviare la segnalazione."},
		Source:   "manual",
	}
}

func validatePhotos(photos []item.ImageInput) error {
	if len(photos) == 0 {
		return fmt.Errorf("wizard analyze: at least one photo is required: %w", errs.ErrInvalidInput)
	}
	if len(photos) > maxPhotos {
		return fmt.Errorf("wizard analyze: at most %d photos are allowed: %w", maxPhotos, errs.ErrInvalidInput)
	}
	for _, photo := range photos {
		if len(photo.Data) == 0 {
			return fmt.Errorf("wizard analyze: empty photo: %w", errs.ErrInvalidInput)
		}
		mediaType := strings.TrimSpace(photo.MediaType)
		if mediaType == "" {
			mediaType = http.DetectContentType(photo.Data)
		}
		if _, ok := allowedMediaTypes[mediaType]; !ok {
			return fmt.Errorf("wizard analyze: unsupported media type %q: %w", mediaType, errs.ErrInvalidInput)
		}
	}
	return nil
}

func validSlotKey(key string) bool {
	if key == "" || len(key) > 40 {
		return false
	}
	switch key {
	case "__proto__", "prototype", "constructor":
		return false
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func sanitizePlan(plan Plan) Plan {
	slots := make([]PhotoSlot, 0, len(plan.PhotoPlan))
	seen := make(map[string]bool, len(plan.PhotoPlan))
	for _, slot := range plan.PhotoPlan {
		slot.Key = strings.TrimSpace(slot.Key)
		slot.Label = strings.TrimSpace(slot.Label)
		slot.Instruction = strings.TrimSpace(slot.Instruction)
		if !validSlotKey(slot.Key) || slot.Label == "" || slot.Instruction == "" || seen[slot.Key] {
			continue
		}
		seen[slot.Key] = true
		slots = append(slots, slot)
		if len(slots) == 3 {
			break
		}
	}
	warnings := make([]string, 0, len(plan.Warnings))
	for _, warning := range plan.Warnings {
		if warning = strings.TrimSpace(warning); warning != "" {
			warnings = append(warnings, warning)
		}
		if len(warnings) == 3 {
			break
		}
	}
	return Plan{
		PhotoPlan:      slots,
		Prefill:        plan.Prefill,
		PrivatePrefill: strings.TrimSpace(plan.PrivatePrefill),
		Warnings:       warnings,
	}
}
