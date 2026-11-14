package item

import "strings"

// riskRule maps a set of keywords to a risk tier. Matching is substring-based
// and case-insensitive so "carta di credito", "identity-document" and
// "Passport" all produce the same tier. Rules are evaluated from most to least
// restrictive: a category containing both "phone" and "document" is sensitive.
type riskRule struct {
	tier     string
	keywords []string
}

var riskRules = []riskRule{
	{
		tier: RiskSensitive,
		keywords: []string{
			"document", "documents", "documento", "documenti", "identity",
			"identita", "carta d'identita", "carta d'identità", "carta identita",
			"passport", "passaporto", "credit card",
			"carta di credito", "debit card", "carta di debito", "bancomat",
			"key", "keys", "keychain", "chiave", "chiavi", "mazzo di chiavi",
			"medical", "medicale", "medico", "dispositivo medico", "sim",
			"sim card", "health card", "tessera sanitaria", "patente",
			"driving licence", "driving license",
		},
	},
	{
		tier: RiskHigh,
		keywords: []string{
			"phone", "telefono", "smartphone", "cellulare", "laptop",
			"portatile", "computer", "notebook", "wallet", "portafoglio",
			"jewel", "jewelry", "jewellery", "gioiello", "gioielli", "watch",
			"orologio", "camera", "fotocamera", "macchina fotografica", "tablet",
		},
	},
	{
		tier: RiskMedium,
		keywords: []string{
			"backpack", "zaino", "glasses", "occhiali", "headphones", "cuffie",
			"auricolari",
		},
	},
	{
		tier: RiskLow,
		keywords: []string{
			"umbrella", "ombrello", "bottle", "bottiglia", "book", "libro",
			"clothing", "abbigliamento", "vestiti", "vestito", "capo",
		},
	},
}

// defaultRiskTier is used for categories the deterministic policy does not
// recognise. Medium is the safe default: it requires more verification than
// low without forcing the 24h sensitive pickup path.
const defaultRiskTier = RiskMedium

// RiskForCategory returns the deterministic risk tier for a category and
// whether the item must follow the sensitive-item flow. The AI may suggest a
// category, but this policy is the authority: finders and intermediaries cannot
// lower the tier.
func RiskForCategory(category string) (tier string, sensitive bool) {
	normalized := strings.ToLower(strings.TrimSpace(category))
	if normalized == "" {
		return defaultRiskTier, false
	}

	for _, rule := range riskRules {
		for _, keyword := range rule.keywords {
			if containsWord(normalized, keyword) {
				return rule.tier, rule.tier == RiskSensitive
			}
		}
	}
	return defaultRiskTier, false
}

// containsWord reports whether keyword appears as a whole word (or phrase) in
// s. The boundary check matters: "headphones" must not trip the "phone" rule,
// and "smartphone" is matched by its own keyword instead.
func containsWord(s, keyword string) bool {
	if keyword == "" {
		return false
	}
	for offset := 0; offset <= len(s)-len(keyword); {
		index := strings.Index(s[offset:], keyword)
		if index < 0 {
			return false
		}
		index += offset
		beforeOK := index == 0 || !isWordByte(s[index-1])
		end := index + len(keyword)
		afterOK := end == len(s) || !isWordByte(s[end])
		if beforeOK && afterOK {
			return true
		}
		offset = index + 1
	}
	return false
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
