package search

import (
	"strings"
	"unicode"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
)

// publicText is the fallback haystack. It mirrors the fields sent to the
// embedding model; private profile, notes and coordinates are never included.
func publicText(item itemdomain.Item) string {
	return indexText(item)
}

// tokenAliases bridges the Italian demo language with the English catalog
// values and vice versa, so "iphone nero" matches a smartphone whose color is
// stored as "black" (and the seeded "nero").
var tokenAliases = map[string][]string{
	"nero":        {"black"},
	"nera":        {"black"},
	"neri":        {"black"},
	"nere":        {"black"},
	"bianco":      {"white"},
	"bianca":      {"white"},
	"blu":         {"blue"},
	"azzurro":     {"blue"},
	"rosso":       {"red"},
	"rossa":       {"red"},
	"verde":       {"green"},
	"giallo":      {"yellow"},
	"marrone":     {"brown"},
	"grigio":      {"gray"},
	"argento":     {"silver"},
	"rosa":        {"pink"},
	"telefono":    {"smartphone"},
	"cellulare":   {"smartphone"},
	"portafoglio": {"wallet"},
	"ombrello":    {"umbrella"},
	"documento":   {"document"},
	"documenti":   {"document"},
	"zaino":       {"backpack"},
	"cuffie":      {"headphones"},
	"auricolari":  {"headphones"},
	"occhiali":    {"glasses"},
	"orologio":    {"watch"},
	"chiavi":      {"keys"},
}

// tokenSet lowercases, strips punctuation and expands the known aliases.
func tokenSet(text string) map[string]bool {
	tokens := make(map[string]bool)
	for _, token := range strings.Fields(strings.ToLower(text)) {
		token = strings.TrimFunc(token, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		})
		if token == "" {
			continue
		}
		tokens[token] = true
		for _, alias := range tokenAliases[token] {
			tokens[alias] = true
		}
	}
	return tokens
}

// overlapScore is the fraction of query tokens found in the haystack.
func overlapScore(query, haystack map[string]bool) float64 {
	if len(query) == 0 {
		return 0
	}
	matched := 0
	for token := range query {
		if haystack[token] {
			matched++
		}
	}
	return float64(matched) / float64(len(query))
}
