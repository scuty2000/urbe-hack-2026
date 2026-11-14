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
	return s.analyzeResult(ctx, result), nil
}

func (s *service) analyzeResult(ctx context.Context, result item.CatalogResult) Analysis {
	category := NormalizeCategory(result.Category)
	if category == "" {
		category = "altro"
	}
	if result.IsDocument {
		return documentAnalysis()
	}

	privatePrefill := fallbackPrivate(result)
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
			PhotoCount: 1,
		}
		planned, err := s.deps.Planner.Plan(ctx, plannerInput)
		if err == nil {
			plan = sanitizePlan(planned)
			source = "d3+deepseek"
		} else {
			warnings = append(warnings, "Suggerimenti automatici parziali: il piano foto usa le regole locali.")
		}
	} else {
		warnings = append(warnings, "Suggerimenti automatici parziali: il piano foto usa le regole locali.")
	}

	if len(plan.PhotoPlan) == 0 {
		plan.PhotoPlan = fallbackPlan(category)
	}
	if strings.TrimSpace(plan.PrivatePrefill) == "" {
		plan.PrivatePrefill = privatePrefill
	}

	prefill := Prefill{
		Category:  category,
		Color:     NormalizeColor(result.Color),
		Condition: NormalizeCondition(result.Condition),
	}
	if prefill.Color == "" {
		prefill.Color = NormalizeColor(plan.Prefill.Color)
	}
	if prefill.Condition == "" {
		prefill.Condition = NormalizeCondition(plan.Prefill.Condition)
	}
	if prefill.Condition == "" {
		prefill.Condition = "good"
	}

	_, sensitive := item.RiskForCategory(category)
	return Analysis{
		ObjectType:     category,
		PhotoPlan:      plan.PhotoPlan,
		Prefill:        prefill,
		PrivatePrefill: strings.TrimSpace(plan.PrivatePrefill),
		Warnings:       append(warnings, plan.Warnings...),
		Source:         source,
		Sensitive:      sensitive,
	}
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

func sanitizePlan(plan Plan) Plan {
	slots := make([]PhotoSlot, 0, len(plan.PhotoPlan))
	seen := make(map[string]bool, len(plan.PhotoPlan))
	for _, slot := range plan.PhotoPlan {
		slot.Key = strings.TrimSpace(slot.Key)
		slot.Label = strings.TrimSpace(slot.Label)
		slot.Instruction = strings.TrimSpace(slot.Instruction)
		if slot.Key == "" || slot.Label == "" || slot.Instruction == "" || seen[slot.Key] {
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
