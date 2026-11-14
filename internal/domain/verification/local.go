package verification

import (
	"fmt"
	"strings"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
)

const (
	// matchProbability is the deterministic probability assigned to an exact
	// attribute match.
	matchProbability = 0.95
	// mismatchProbability is assigned to a mismatch or a missing attribute.
	mismatchProbability = 0.1
	// partialProbability is assigned to an approximate match: one value
	// contains the other or they share a meaningful word ("blu" for "blu
	// scuro", "gatto" for "adesivo di un gatto"). It counts as a match.
	partialProbability = 0.8
	// undecidedProbability is assigned when the searcher only confirmed that a
	// detail exists ("sì, c'è un adesivo") without describing it. It is neither
	// a match nor a contradiction, so the policy asks for more information.
	undecidedProbability = 0.5
	// scoreExact is the score returned for a unique/exact free-text match.
	scoreExact = 0.9
	// scorePartial is the score returned when one value contains the other.
	scorePartial = 0.5
	// scoreMismatch is the score returned for unrelated free text.
	scoreMismatch = 0.1
	// matchThreshold is the probability above which an answer counts as a
	// match in the rationale.
	matchThreshold = 0.75
)

// LocalScore is the deterministic fallback scorer. It never performs network
// I/O: every question is answered by comparing the expected value with the
// searcher's answer after lower-casing and trimming.
//
// Weights are deliberately fixed: noul/choice attributes score 0.95 on a match
// and 0.1 on a mismatch, score attributes score 0.9 on an exact match, 0.5 on
// a partial match and 0.1 otherwise.
func LocalScore(state State, questions Questions) D3Result {
	result := D3Result{
		Answers:      make(map[string]itemdomain.Answer, len(questions)),
		ModelVersion: ModelLocal,
	}
	if len(questions) == 0 {
		result.Rationale = "local scorer: no questions"
		return result
	}

	keys := SortedKeys(questions)
	matches := 0
	var sum float64
	for _, key := range keys {
		expected, expectedOK := lookup(state.Item, key)
		actual, actualOK := lookup(state.Searcher, key)
		question := questions[key]

		switch question.Type {
		case "score":
			score := scoreValue(expected, actual, expectedOK && actualOK)
			result.Answers[key] = itemdomain.Answer{
				Type:       "score",
				Score:      score,
				Confidence: score,
			}
			sum += score
			if score >= scoreExact {
				matches++
			}
		case "choice":
			probability := matchValue(expected, actual, expectedOK && actualOK)
			answer := itemdomain.Answer{
				Type:       "choice",
				Confidence: probability,
			}
			if value := normalizeValue(actual); value != "" {
				answer.Choice = value
				answer.Probabilities = map[string]float64{value: probability}
			}
			result.Answers[key] = answer
			sum += probability
			if probability >= matchThreshold {
				matches++
			}
		default: // noul is the default question type
			probability := matchValue(expected, actual, expectedOK && actualOK)
			result.Answers[key] = itemdomain.Answer{
				Type:       "noul",
				Noul:       probability,
				Confidence: probability,
			}
			sum += probability
			if probability >= matchThreshold {
				matches++
			}
		}
	}

	result.Confidence = clamp(sum / float64(len(keys)))
	result.Rationale = fmt.Sprintf("local scorer: %d/%d attributes matched", matches, len(keys))
	return result
}

// matchValue returns 0.95 for a normalized match, 0.8 for an approximate one
// and 0.1 otherwise. Booleans accept yes/no synonyms and arrays accept
// membership, so seeded boolean and list attributes are answerable instead of
// capping every honest score. Comparison ignores accents, grammatical gender
// and common colour synonyms: "azzurra" matches "blu".
func matchValue(expected, actual any, bothPresent bool) float64 {
	if !bothPresent {
		return mismatchProbability
	}
	if expectedBool, ok := expected.(bool); ok {
		actualBool, parsed := parseBool(actual)
		if !parsed {
			// A description instead of yes/no ("ha una crepa in alto") confirms
			// the detail exists; "nessuna"/"senza" denies it.
			actualBool, parsed = describesPresence(actual)
		}
		if parsed && expectedBool == actualBool {
			return matchProbability
		}
		return mismatchProbability
	}

	best := mismatchProbability
	for _, candidate := range expectedValues(expected) {
		if probability := textMatch(candidate, normalizeValue(actual)); probability > best {
			best = probability
		}
	}
	if best > mismatchProbability {
		return best
	}
	// A bare "sì" against a descriptive attribute confirms presence only.
	if confirmed, parsed := parseBool(actual); parsed && confirmed {
		return undecidedProbability
	}
	return mismatchProbability
}

// expectedValues flattens a scalar or a list attribute to normalized strings.
func expectedValues(expected any) []string {
	switch values := expected.(type) {
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, normalizeValue(value))
		}
		return out
	case []string:
		out := make([]string, 0, len(values))
		for _, value := range values {
			out = append(out, normalizeValue(value))
		}
		return out
	default:
		return []string{normalizeValue(expected)}
	}
}

// textMatch compares two normalized values: exact (after folding) scores
// 0.95, containment or a shared meaningful word scores 0.8.
func textMatch(expected, actual string) float64 {
	if expected == "" || actual == "" {
		return mismatchProbability
	}
	left := foldTokens(expected)
	right := foldTokens(actual)
	if len(left) == 0 || len(right) == 0 {
		return mismatchProbability
	}
	if strings.Join(left, " ") == strings.Join(right, " ") {
		return matchProbability
	}
	seen := make(map[string]bool, len(left))
	for _, token := range left {
		seen[token] = true
	}
	for _, token := range right {
		if seen[token] {
			return partialProbability
		}
	}
	return mismatchProbability
}

// fillerWords carry no ownership signal and never count as a shared word.
var fillerWords = map[string]bool{
	"il": true, "lo": true, "la": true, "le": true, "gli": true, "un": true, "una": true, "uno": true,
	"di": true, "del": true, "della": true, "dei": true, "delle": true, "da": true, "in": true,
	"con": true, "su": true, "per": true, "che": true, "non": true, "e": true, "o": true, "a": true,
	"ha": true, "ho": true, "era": true, "the": true, "of": true, "and": true, "with": true,
	"colore": true, "color": true, "tipo": true,
}

// synonyms maps common variants to one canonical word. Colours dominate
// because they are the most frequent private attribute.
var synonyms = map[string]string{
	"black": "nero", "white": "bianco", "red": "rosso", "green": "verde",
	"blue": "blu", "azzurro": "blu", "celeste": "blu", "navy": "blu",
	"brown": "marrone", "yellow": "giallo", "pink": "rosa",
	"gray": "grigio", "grey": "grigio", "argento": "grigio", "silver": "grigio",
	"orange": "arancione", "purple": "viola", "gold": "oro", "dorato": "oro",
	"cat": "gatto", "dog": "cane", "cover": "custodia", "case": "custodia",
	"sticker": "adesivo", "crack": "crepa", "crepe": "crepa",
}

// foldTokens splits a value into comparable words: accents and filler words
// are removed, synonyms are canonicalized and the final vowel of longer words
// is dropped so "nera"/"nero" and "rossa"/"rosso" compare equal.
func foldTokens(value string) []string {
	fields := strings.FieldsFunc(stripAccents(value), func(r rune) bool {
		return isSeparator(r) || r == '\'' || r == '"' || r == '(' || r == ')' || r == '!' || r == '?'
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" || fillerWords[field] {
			continue
		}
		if canonical, ok := synonyms[field]; ok {
			field = canonical
		} else if canonical, ok := synonyms[stem(field)+"o"]; ok {
			field = canonical
		}
		tokens = append(tokens, stem(field))
	}
	return tokens
}

// stem drops the final vowel of words longer than three letters.
func stem(word string) string {
	if len(word) <= 3 {
		return word
	}
	switch word[len(word)-1] {
	case 'a', 'e', 'i', 'o':
		return word[:len(word)-1]
	default:
		return word
	}
}

var accentReplacer = strings.NewReplacer(
	"à", "a", "á", "a", "è", "e", "é", "e", "ì", "i", "í", "i",
	"ò", "o", "ó", "o", "ù", "u", "ú", "u",
)

func stripAccents(value string) string {
	return accentReplacer.Replace(value)
}

// describesPresence reads a free-text answer to a yes/no attribute: a negation
// denies the detail, any other description confirms it.
func describesPresence(value any) (bool, bool) {
	text := stripAccents(normalizeValue(value))
	if text == "" {
		return false, false
	}
	for _, token := range strings.FieldsFunc(text, isSeparator) {
		switch token {
		case "no", "non", "senza", "nessun", "nessuno", "nessuna", "niente", "without", "none":
			return false, true
		}
	}
	return true, true
}

// parseBool accepts the boolean spellings a searcher may type for a yes/no
// question, including Italian variants.
func parseBool(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case float64:
		switch v {
		case 1:
			return true, true
		case 0:
			return false, true
		}
	case int:
		switch v {
		case 1:
			return true, true
		case 0:
			return false, true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "y", "si", "sì", "1", "vero", "present", "presente", "has":
			return true, true
		case "false", "no", "n", "0", "falso", "absent", "assente":
			return false, true
		}
	}
	return false, false
}

// scoreValue implements the free-text score question: exact match 0.9,
// partial match 0.5, everything else 0.1.
func scoreValue(expected, actual any, bothPresent bool) float64 {
	if !bothPresent {
		return scoreMismatch
	}
	left := normalizeValue(expected)
	right := normalizeValue(actual)
	if left == "" || right == "" {
		return scoreMismatch
	}
	if left == right {
		return scoreExact
	}
	if strings.Contains(left, right) || strings.Contains(right, left) {
		return scorePartial
	}
	if sharesToken(left, right) || textMatch(left, right) > mismatchProbability {
		return scorePartial
	}
	return scoreMismatch
}

// sharesToken reports whether two free-text values share a meaningful token.
func sharesToken(left, right string) bool {
	seen := map[string]bool{}
	for _, token := range strings.FieldsFunc(left, isSeparator) {
		if len(token) >= 3 {
			seen[token] = true
		}
	}
	for _, token := range strings.FieldsFunc(right, isSeparator) {
		if len(token) >= 3 && seen[token] {
			return true
		}
	}
	return false
}

func isSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '-', '_', '/', ',', '.', ';', ':':
		return true
	default:
		return false
	}
}

// lookup returns the value for key, falling back to a case-insensitive match so
// private profiles written by hand still line up with generated questions.
func lookup(values map[string]any, key string) (any, bool) {
	if values == nil {
		return nil, false
	}
	if value, ok := values[key]; ok {
		return value, true
	}
	for candidate, value := range values {
		if strings.EqualFold(strings.TrimSpace(candidate), key) {
			return value, true
		}
	}
	return nil, false
}

// ConfidenceOf aggregates per-question probabilities from any D3 result.
func ConfidenceOf(questions Questions, answers map[string]itemdomain.Answer) float64 {
	if len(questions) == 0 {
		return 0
	}
	var sum float64
	for key := range questions {
		sum += AnswerProbability(answers[key])
	}
	return clamp(sum / float64(len(questions)))
}

// AnswerProbability extracts the match probability from a d3 answer.
func AnswerProbability(answer itemdomain.Answer) float64 {
	switch answer.Type {
	case "score":
		return clamp(answer.Score)
	case "choice":
		if answer.Choice != "" {
			if probability, ok := answer.Probabilities[answer.Choice]; ok {
				return clamp(probability)
			}
		}
		best := -1.0
		for _, probability := range answer.Probabilities {
			if probability > best {
				best = probability
			}
		}
		if best >= 0 {
			return clamp(best)
		}
		return clamp(answer.Confidence)
	case "noul":
		return clamp(answer.Noul)
	default:
		return clamp(answer.Confidence)
	}
}
