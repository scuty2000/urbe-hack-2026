package wizard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
)

const plannerToolName = "submit_wizard_analysis"

const plannerSystemPrompt = `Sei il regista del wizard di segnalazione di un oggetto smarrito.
Ricevi solo una classificazione strutturata; non vedi foto e non devi inventare
marca o modello. Scrivi in italiano e restituisci esclusivamente il tool
submit_wizard_analysis con:
- photoPlan: massimo 3 foto aggiuntive, ognuna con key, label, instruction,
  required. La prima foto è già stata caricata: qui elenca solo le successive.
  Chiedi solo cose che un finder può fotografare senza sbloccare, aprire o
  mostrare dati personali. Mai contenuto di documenti, notifiche o interni di
  portafogli con carte visibili.
- prefill: categoria, colore e condizioni in italiano, coerenti con la
  classificazione.
- privatePrefill: una frase breve sui segni visibili (custodia, crepe, adesivi);
  vuota per i documenti.
- warnings: massimo 3 avvisi brevi.
Non includere nomi, telefoni, indirizzi, numeri di documento o dati personali.`

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
	completion, err := p.client.Complete(ctx, []deepseek.Message{
		{Role: "system", Content: plannerSystemPrompt},
		{Role: "user", Content: string(inputJSON)},
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
						"type": "object",
						"properties": map[string]any{
							"category":  map[string]any{"type": "string"},
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
