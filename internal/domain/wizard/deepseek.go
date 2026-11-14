package wizard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
)

const (
	plannerToolName = "submit_wizard_analysis"
	// maxPlannerPhotos bounds the images sent to the vision provider. The
	// first photo carries the brand/model cues; a second detail photo helps
	// with accessories and marks.
	maxPlannerPhotos = 2
	// maxPlannerImageBytes skips oversized uploads instead of failing the
	// whole analysis on a provider payload limit.
	maxPlannerImageBytes = 4 << 20
)

const plannerSystemPrompt = `Sei il regista del wizard di segnalazione di un oggetto smarrito.
Ricevi una o più foto dell'oggetto e una classificazione strutturata. Scrivi in
italiano e restituisci esclusivamente il tool submit_wizard_analysis con:
- photoPlan: massimo 3 foto aggiuntive, ognuna con key, label, instruction,
  required. La prima foto è già stata caricata: qui elenca solo le successive.
  Chiedi solo cose che un finder può fotografare senza sbloccare, aprire o
  mostrare dati personali. Mai contenuto di documenti, notifiche o interni di
  portafogli con carte visibili.
- prefill: categoria, marca, modello, colore e condizioni in italiano. Compila
  sempre marca e modello con la migliore ipotesi basata su logo, scritta, forma,
  colori e dettagli visibili. Se il modello esatto non è distinguibile, indica
  il modello o la famiglia più probabile (ad esempio "iPhone 15" o "iPhone").
  Lascia vuoto solo se la foto non contiene alcun indizio utile.
- privatePrefill: 1-3 frasi specifiche sui dettagli visibili che aiutano a
  riconoscere l'oggetto: colore e materiale di custodia o cinturino, graffi,
  crepe, adesivi, incisioni, segni d'uso. Descrivi solo ciò che vedi; non
  inventare. Vuota per i documenti.
- warnings: massimo 3 avvisi brevi.
Non includere nomi, telefoni, indirizzi, numeri di documento, targhe o dati
personali.`

// NewDeepSeekPlanner returns a planner backed by the DeepSeek client, or nil
// when no client is configured (the service will use its deterministic plan).
func NewDeepSeekPlanner(client *deepseek.Client) Planner {
	if client == nil {
		return nil
	}
	return &deepSeekPlanner{client: client}
}

type deepSeekPlanner struct {
	client *deepseek.Client
}

func (p *deepSeekPlanner) Plan(ctx context.Context, input PlannerInput) (Plan, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return Plan{}, fmt.Errorf("wizard planner: encoding input: %w", errs.ErrUnavailable)
	}
	content := []deepseek.ContentPart{deepseek.TextPart(string(inputJSON))}
	images := 0
	for _, photo := range input.Photos {
		if images >= maxPlannerPhotos {
			break
		}
		if len(photo.Data) == 0 || len(photo.Data) > maxPlannerImageBytes {
			continue
		}
		content = append(content, deepseek.ImagePart(photo.MediaType, photo.Data))
		images++
	}
	completion, err := p.client.Complete(ctx, []deepseek.Message{
		{Role: "system", Content: plannerSystemPrompt},
		{Role: "user", Content: content},
	}, []deepseek.Tool{plannerTool()})
	if err != nil {
		return Plan{}, err
	}
	for _, call := range completion.ToolCalls {
		if call.Function.Name == plannerToolName {
			return parsePlan(call.Function.Arguments)
		}
	}
	if strings.TrimSpace(completion.Content) != "" {
		return parsePlan(completion.Content)
	}
	return Plan{}, fmt.Errorf("wizard planner: empty provider response: %w", errs.ErrUnavailable)
}

func plannerTool() deepseek.Tool {
	return deepseek.Tool{
		Type: "function",
		Function: deepseek.ToolFunction{
			Name:        plannerToolName,
			Description: "Return the finder wizard photo plan and prefill.",
			Parameters: map[string]any{
				"type":     "object",
				"required": []string{"photoPlan", "prefill", "privatePrefill", "warnings"},
				"properties": map[string]any{
					"photoPlan": map[string]any{
						"type":     "array",
						"maxItems": 3,
						"items": map[string]any{
							"type":     "object",
							"required": []string{"key", "label", "instruction", "required"},
							"properties": map[string]any{
								"key":         map[string]any{"type": "string"},
								"label":       map[string]any{"type": "string"},
								"instruction": map[string]any{"type": "string"},
								"required":    map[string]any{"type": "boolean"},
							},
						},
					},
					"prefill": map[string]any{
						"type":     "object",
						"required": []string{"category", "brand", "model", "color", "condition"},
						"properties": map[string]any{
							"category":  map[string]any{"type": "string"},
							"brand":     map[string]any{"type": "string"},
							"model":     map[string]any{"type": "string"},
							"color":     map[string]any{"type": "string"},
							"condition": map[string]any{"type": "string"},
						},
					},
					"privatePrefill": map[string]any{"type": "string"},
					"warnings": map[string]any{
						"type":     "array",
						"maxItems": 3,
						"items":    map[string]any{"type": "string"},
					},
				},
			},
		},
	}
}

func parsePlan(raw string) (Plan, error) {
	raw = stripJSONFence(strings.TrimSpace(raw))
	if raw == "" {
		return Plan{}, fmt.Errorf("wizard planner: empty plan: %w", errs.ErrUnavailable)
	}
	var plan Plan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return Plan{}, fmt.Errorf("wizard planner: invalid plan JSON: %w", errs.ErrUnavailable)
	}
	return plan, nil
}

func stripJSONFence(raw string) string {
	if !strings.HasPrefix(raw, "```") {
		return raw
	}
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimPrefix(raw, "json")
	raw = strings.TrimPrefix(raw, "JSON")
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "```")
	return strings.TrimSpace(raw)
}
