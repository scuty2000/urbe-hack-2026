package wizard

import (
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
)

// fallbackPlan is the deterministic photo plan used when DeepSeek is
// unavailable or returns an unusable plan. It only asks for photos a finder can
// take without unlocking or opening private contents.
func fallbackPlan(category string) []PhotoSlot {
	switch category {
	case "smartphone":
		return []PhotoSlot{
			{Key: "back", Label: "Retro", Instruction: "Fotografa il retro del telefono, senza sbloccarlo.", Required: true},
			{Key: "case", Label: "Custodia o segno", Instruction: "Fotografa la custodia o un segno particolare visibile (facoltativo).", Required: false},
		}
	case "laptop":
		return []PhotoSlot{
			{Key: "open", Label: "Lato aperto", Instruction: "Se possibile, fotografa lo schermo e la tastiera senza accendere il dispositivo.", Required: false},
			{Key: "detail", Label: "Dettaglio", Instruction: "Fotografa un segno particolare o un adesivo (facoltativo).", Required: false},
		}
	case "portafoglio":
		return []PhotoSlot{
			{Key: "outside", Label: "Esterno", Instruction: "Fotografa il portafoglio chiuso.", Required: true},
			{Key: "inside", Label: "Interno", Instruction: "Se possibile, fotografa l'interno senza carte o documenti visibili (facoltativo).", Required: false},
		}
	case "chiavi":
		return []PhotoSlot{
			{Key: "bunch", Label: "Mazzo completo", Instruction: "Fotografa tutte le chiavi del mazzo.", Required: true},
			{Key: "chain", Label: "Portachiavi", Instruction: "Fotografa il portachiavi o un segno particolare (facoltativo).", Required: false},
		}
	case "zaino":
		return []PhotoSlot{
			{Key: "outside", Label: "Esterno", Instruction: "Fotografa lo zaino o la borsa chiusi.", Required: true},
			{Key: "detail", Label: "Dettaglio", Instruction: "Fotografa una tasca, una zip o un segno particolare (facoltativo).", Required: false},
		}
	case "documenti":
		return nil
	default:
		return []PhotoSlot{{
			Key:         "detail",
			Label:       "Dettaglio",
			Instruction: "Fotografa un dettaglio o un segno particolare dell'oggetto (facoltativo).",
			Required:    false,
		}}
	}
}

// fallbackPrivate writes the private-details draft from visible attributes.
func fallbackPrivate(result item.CatalogResult) string {
	if result.IsDocument {
		return ""
	}
	parts := make([]string, 0, 3)
	if color := NormalizeColor(result.CaseColor); color != "" {
		parts = append(parts, "custodia "+color)
	} else if result.HasCase {
		parts = append(parts, "custodia presente")
	}
	if result.HasCrack {
		parts = append(parts, "crepa o segni visibili")
	}
	if result.HasSticker {
		parts = append(parts, "adesivo presente")
	}
	if len(parts) == 0 {
		return ""
	}
	text := strings.Join(parts, ", ")
	runes := []rune(text)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes) + "."
}
