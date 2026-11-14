package wizard

import (
	"strings"
	"unicode/utf8"
)

// optionalField cleans a free-text prefill field. Placeholder answers such as
// "non visibile" are dropped so the finder sees an empty field instead of a
// non-answer.
func optionalField(raw string, maxRunes int) string {
	value := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	switch strings.ToLower(value) {
	case "", "-", "n/d", "n/a", "nd", "unknown", "sconosciuto", "sconosciuta",
		"non visibile", "non riconoscibile", "non leggibile", "nessuna", "nessuno":
		return ""
	}
	if maxRunes > 0 && utf8.RuneCountInString(value) > maxRunes {
		runes := []rune(value)
		value = string(runes[:maxRunes])
	}
	return value
}

// NormalizeCategory maps D3's English categories (and the Italian form values)
// onto the shared UI catalog. Unknown values become "altro" rather than
// silently lowering a known high-risk category.
func NormalizeCategory(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "smartphone", "phone", "telefono", "cellulare":
		return "smartphone"
	case "laptop", "computer", "notebook computer":
		return "laptop"
	case "wallet", "portafoglio", "purse":
		return "portafoglio"
	case "keys", "chiavi", "keychain":
		return "chiavi"
	case "document", "documents", "documento", "documenti", "identity_document":
		return "documenti"
	case "jewelry", "jewellery", "gioielli", "gioiello":
		return "gioielli"
	case "watch", "orologio":
		return "orologio"
	case "glasses", "occhiali", "sunglasses":
		return "occhiali"
	case "backpack", "bag", "zaino", "borsa":
		return "zaino"
	case "headphones", "earbuds", "cuffie", "auricolari":
		return "cuffie"
	case "book", "libro", "quaderno":
		return "libro"
	case "bottle", "bottiglia", "borraccia", "thermos":
		return "bottiglia"
	case "clothing", "abbigliamento", "vestiti":
		return "abbigliamento"
	case "umbrella", "ombrello":
		return "ombrello"
	default:
		return "altro"
	}
}

// NormalizeColor maps the D3/Italian palette onto the UI values.
func NormalizeColor(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "black", "nero":
		return "nero"
	case "white", "bianco":
		return "bianco"
	case "gray", "grey", "silver", "grigio", "argento":
		return "grigio"
	case "red", "rosso":
		return "rosso"
	case "blue", "blu":
		return "blu"
	case "green", "verde":
		return "verde"
	case "yellow", "giallo":
		return "giallo"
	case "brown", "marrone":
		return "marrone"
	case "pink", "rosa":
		return "rosa"
	case "other", "multicolore", "multicolor":
		return "multicolore"
	default:
		return ""
	}
}

// NormalizeCondition maps the D3/Italian condition vocabulary onto the UI.
func NormalizeCondition(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "new", "like_new", "come nuovo", "come nuova", "ottimo", "ottime condizioni", "pari al nuovo":
		return "like_new"
	case "good", "buono", "buona", "buone condizioni", "in buono stato":
		return "good"
	case "worn", "used", "fair", "discreto", "discreta", "condizioni discrete", "usurato":
		return "fair"
	case "damaged", "poor", "da riparare", "danneggiato", "danneggiata", "rotto":
		return "poor"
	default:
		return ""
	}
}
