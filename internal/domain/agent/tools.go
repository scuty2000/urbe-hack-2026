package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
)

// profileMetadataKeys are private-profile bookkeeping values that are not
// discriminative attributes and are never sent to the model.
var profileMetadataKeys = map[string]bool{
	"catalog_source": true,
	"embedding":      true,
	"embeddings":     true,
	"photos":         true,
}

// questionAttributeKeys is an allowlist, not a denylist: only these
// non-sensitive private attributes may be forwarded to the model or the
// verification engine. Enrollment accepts arbitrary keys, so anything not
// listed here is dropped even when IsIdentityField does not recognise it
// (owner, via, recapito, serial, imei, ...).
var questionAttributeKeys = map[string]bool{
	"case_color": true,
	"has_crack":  true,
	"sticker":    true,
	"handle":     true,
	"contents":   true,
}

// identityDocumentKeys are the only identity fields whose presence is surfaced
// to the model as a boolean. Other identity keys (owner, via, recapito, serial,
// imei, ...) are dropped entirely: the model never needs their names, and the
// document report only needs the fields the document check asks about.
var identityDocumentKeys = map[string]bool{
	"holder":          true,
	"number":          true,
	"document_number": true,
	"documentnumber":  true,
	"name":            true,
	"owner_name":      true,
}

// freeTextKeys are private-profile values that may contain arbitrary prose
// (including a person's name) and are therefore excluded from the sanitized
// profile and from the scoring attributes.
var freeTextKeys = map[string]bool{
	"notes":       true,
	"note":        true,
	"description": true,
	"descrizione": true,
	"detail":      true,
	"details":     true,
	"dettaglio":   true,
	"comment":     true,
	"comments":    true,
}

// turnState scopes the tools to the items discovered by search in the current
// conversation turn and records every item whose private profile was touched,
// so the final message is redacted even without search candidates.
type turnState struct {
	allowed map[string]bool
	seen    map[string]bool
	// userText is the lower-cased concatenation of the searcher's messages.
	userText string
	// used reports whether any tool ran, so a provider failure never restarts
	// the mock after side effects.
	used bool
	// claimCreated prevents duplicate claims within a turn.
	claimCreated bool
}

func newTurnState(messages []Message) *turnState {
	return &turnState{
		allowed:  map[string]bool{},
		seen:     map[string]bool{},
		userText: strings.ToLower(userText(messages)),
	}
}

// toolOutcome is the result of one tool call.
type toolOutcome struct {
	content    string
	candidates []search.SearchResult
	claimID    string
	escalated  bool
}

// toolDefinitions declares the tools offered to the model.
func toolDefinitions() []deepseek.Tool {
	return []deepseek.Tool{
		{
			Type: "function",
			Function: deepseek.ToolFunction{
				Name:        ToolSearchItems,
				Description: "Cerca oggetti smarriti compatibili. Restituisce solo i campi pubblici dei candidati.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":    map[string]any{"type": "string", "description": "descrizione libera dell'oggetto"},
						"category": map[string]any{"type": "string", "description": "categoria opzionale"},
						"zone":     map[string]any{"type": "string", "description": "zona opzionale"},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: deepseek.ToolFunction{
				Name:        ToolGetPrivateProfile,
				Description: "Restituisce il profilo sanificato di un oggetto per formulare una domanda. I dati identificativi sono ridotti a booleani.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"item_id": map[string]any{"type": "string"},
					},
					"required": []string{"item_id"},
				},
			},
		},
		{
			Type: "function",
			Function: deepseek.ToolFunction{
				Name:        ToolScoreWithD3,
				Description: "Valuta le risposte rispetto all'oggetto. Restituisce solo confidenza, decisione e motivazioni, mai i valori attesi.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"item_id": map[string]any{"type": "string"},
						"answers": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{"type": "string"},
						},
					},
					"required": []string{"item_id", "answers"},
				},
			},
		},
		{
			Type: "function",
			Function: deepseek.ToolFunction{
				Name:        ToolCreateClaim,
				Description: "Crea la richiesta di verifica per il searcher autenticato. Non accetta una confidenza dal client.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"item_id": map[string]any{"type": "string"},
						"answers": map[string]any{
							"type":                 "object",
							"additionalProperties": map[string]any{"type": "string"},
						},
						"evidence": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
					},
					"required": []string{"item_id", "answers"},
				},
			},
		},
		{
			Type: "function",
			Function: deepseek.ToolFunction{
				Name:        ToolEscalate,
				Description: "Inoltra il caso a un operatore umano e termina la conversazione senza creare una richiesta.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"reason": map[string]any{"type": "string"},
					},
					"required": []string{"reason"},
				},
			},
		},
	}
}

// executeTool dispatches one model tool call. Tool failures are returned to the
// model as an error JSON object so the conversation can continue; they never
// abort the request. Every tool is scoped to the items discovered by
// search_items in this turn.
func (s *service) executeTool(ctx context.Context, searcherID string, call deepseek.ToolCall, state *turnState) toolOutcome {
	if state == nil {
		state = newTurnState(nil)
	}
	state.used = true
	switch strings.TrimSpace(call.Function.Name) {
	case ToolSearchItems:
		return s.toolSearch(ctx, call, state)
	case ToolGetPrivateProfile:
		return s.toolPrivateProfile(ctx, call, state)
	case ToolScoreWithD3:
		return s.toolScore(ctx, call, state)
	case ToolCreateClaim:
		return s.toolCreateClaim(ctx, searcherID, call, state)
	case ToolEscalate:
		return s.toolEscalate()
	default:
		return toolError("unknown tool")
	}
}

func (s *service) toolSearch(ctx context.Context, call deepseek.ToolCall, state *turnState) toolOutcome {
	var args struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		Zone     string `json:"zone"`
	}
	if err := decodeArguments(call, &args); err != nil {
		return toolError("invalid search arguments")
	}
	results, err := s.deps.Search.Search(ctx, search.Query{
		Text:     args.Query,
		Category: args.Category,
		Zone:     args.Zone,
		Limit:    searchLimit,
	})
	if err != nil {
		return toolError("search is unavailable")
	}
	if results == nil {
		results = []search.SearchResult{}
	}
	for _, result := range results {
		if result.ItemID == "" {
			continue
		}
		state.allowed[result.ItemID] = true
		state.seen[result.ItemID] = true
	}
	return toolOutcome{
		content:    mustJSON(map[string]any{"candidates": results}),
		candidates: results,
	}
}

// sanitizedProfile is the only item representation sent to the model for
// question generation.
type sanitizedProfile struct {
	ItemID     string           `json:"itemId"`
	Category   string           `json:"category,omitempty"`
	Brand      string           `json:"brand,omitempty"`
	Model      string           `json:"model,omitempty"`
	Color      string           `json:"color,omitempty"`
	Condition  string           `json:"condition,omitempty"`
	Attributes map[string]any   `json:"attributes,omitempty"`
	Document   *documentProfile `json:"document,omitempty"`
}

// documentProfile is the document-safe view: the document type plus boolean
// presence flags. Raw numbers and holder names never appear.
type documentProfile struct {
	DocumentType  string          `json:"document_type,omitempty"`
	IdentityMatch map[string]bool `json:"identity_match"`
}

func (s *service) toolPrivateProfile(ctx context.Context, call deepseek.ToolCall, state *turnState) toolOutcome {
	var args struct {
		ItemID string `json:"item_id"`
	}
	if err := decodeArguments(call, &args); err != nil {
		return toolError("invalid profile arguments")
	}
	itemID := strings.TrimSpace(args.ItemID)
	if !state.allowed[itemID] {
		return toolError("item not available in this turn")
	}
	item, err := s.deps.Items.ItemForAgent(ctx, itemID)
	if err != nil {
		return toolError("item not found")
	}
	state.seen[itemID] = true
	return toolOutcome{content: mustJSON(sanitizeProfile(item))}
}

// sanitizeProfile keeps public listing attributes and an allowlist of
// non-sensitive private attributes. Identity values become booleans, and every
// other key is dropped: the profile is written by enrollment, so an unknown key
// may well contain a name, a phone number or a document number.
func sanitizeProfile(item itemdomain.Item) sanitizedProfile {
	profile := sanitizedProfile{
		ItemID:    item.ID,
		Category:  safePublicText(item.Category),
		Brand:     safePublicText(item.Brand),
		Model:     safePublicText(item.Model),
		Color:     safePublicText(item.Color),
		Condition: safePublicText(item.Condition),
	}
	attributes := map[string]any{}
	identity := map[string]bool{}
	documentType := ""

	for key, value := range item.PrivateProfile {
		normalized := normalizeKey(key)
		if normalized == "" || profileMetadataKeys[normalized] || freeTextKeys[normalized] {
			continue
		}
		if normalized == "document_type" || normalized == "document" {
			if text, ok := value.(string); ok {
				documentType = safeText(text)
			}
			continue
		}
		if verification.IsIdentityField(normalized) {
			if identityDocumentKeys[normalized] {
				identity[normalized] = hasContent(value)
			}
			continue
		}
		if !questionAttributeKeys[normalized] {
			// Fail closed: unknown keys are never forwarded.
			continue
		}
		sanitized, ok := safeAttributeValue(value)
		if !ok {
			continue
		}
		attributes[normalized] = sanitized
	}

	if len(attributes) > 0 {
		profile.Attributes = attributes
	}
	if documentType != "" || len(identity) > 0 {
		profile.Document = &documentProfile{
			DocumentType:  documentType,
			IdentityMatch: identity,
		}
	}
	return profile
}

// safeAttributeValue accepts only booleans, short safe strings and lists of
// short safe strings. Numbers are dropped because a bare number in a private
// profile is more likely a document number, phone or serial than a question
// attribute.
func safeAttributeValue(value any) (any, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		text := safeText(typed)
		return text, text != ""
	case []string:
		values := make([]any, 0, len(typed))
		for _, element := range typed {
			text := safeText(element)
			if text == "" {
				continue
			}
			values = append(values, text)
		}
		if len(values) == 0 {
			return nil, false
		}
		return values, true
	case []any:
		values := make([]any, 0, len(typed))
		for _, element := range typed {
			text, ok := element.(string)
			if !ok {
				continue
			}
			safe := safeText(text)
			if safe == "" {
				continue
			}
			values = append(values, safe)
		}
		if len(values) == 0 {
			return nil, false
		}
		return values, true
	default:
		return nil, false
	}
}

// safeText rejects the value shapes that carry personal data: e-mail addresses,
// URLs, digit sequences (phones, documents, serials) and full names or postal
// addresses. The question attributes are short descriptive words.
func safeText(value string) string {
	text := strings.TrimSpace(value)
	if text == "" || len(text) > 80 {
		return ""
	}
	lower := strings.ToLower(text)
	if strings.ContainsAny(text, "@\n\r\t") {
		return ""
	}
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "www.") {
		return ""
	}
	if strings.IndexFunc(text, unicode.IsDigit) >= 0 {
		return ""
	}
	if containsAddressMarker(lower) {
		return ""
	}
	if looksLikeFullName(text) {
		return ""
	}
	return text
}

// safePublicText applies the personal-data shape checks to a public listing
// attribute while keeping legitimate model numbers ("iPhone 17").
func safePublicText(value string) string {
	text := strings.TrimSpace(value)
	if text == "" || len(text) > 80 {
		return ""
	}
	lower := strings.ToLower(text)
	if strings.ContainsAny(text, "@\n\r\t") {
		return ""
	}
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") || strings.Contains(lower, "www.") {
		return ""
	}
	if containsAddressMarker(lower) || looksLikeFullName(text) {
		return ""
	}
	return text
}

func containsAddressMarker(lower string) bool {
	for _, token := range strings.Fields(lower) {
		switch token {
		case "via", "viale", "piazza", "corso", "vicolo", "largo", "indirizzo",
			"residenza", "recapito", "street", "road", "avenue", "addr":
			return true
		}
	}
	return false
}

// looksLikeFullName reports whether two or more words start with a capital
// letter, which is how a first and last name usually appear.
func looksLikeFullName(text string) bool {
	capitalized := 0
	for _, token := range strings.Fields(text) {
		runes := []rune(token)
		if len(runes) == 0 {
			continue
		}
		if unicode.IsUpper(runes[0]) {
			capitalized++
		}
	}
	return capitalized >= 2
}

// scoreResult is the only scoring output sent to the model. Expected values are
// never included.
type scoreResult struct {
	Confidence float64  `json:"confidence"`
	Decision   string   `json:"decision"`
	Reasons    []string `json:"reasons"`
}

func (s *service) toolScore(ctx context.Context, call deepseek.ToolCall, state *turnState) toolOutcome {
	var args struct {
		ItemID  string            `json:"item_id"`
		Answers map[string]string `json:"answers"`
	}
	if err := decodeArguments(call, &args); err != nil {
		return toolError("invalid score arguments")
	}
	itemID := strings.TrimSpace(args.ItemID)
	if !state.allowed[itemID] {
		return toolError("item not available in this turn")
	}
	item, err := s.deps.Items.ItemForAgent(ctx, itemID)
	if err != nil {
		return toolError("item not found")
	}
	state.seen[itemID] = true

	attributes, privateKeys := scoringAttributes(item)
	questions := questionsFor(attributes)
	verificationState := verification.State{
		Item:          attributes,
		Searcher:      stringAnswers(args.Answers),
		IdentityMatch: identityMatches(item.PrivateProfile, args.Answers),
		RiskTier:      riskTierOf(item),
	}
	flags := []string{}
	if hasIdentityMismatch(verificationState.IdentityMatch) {
		flags = append(flags, verification.FlagIdentityMismatch)
	}

	result, err := s.deps.Engine.Score(ctx, verificationState, questions)
	if err != nil {
		return toolError("scoring is unavailable")
	}
	confidence := clamp01(result.Confidence)
	policy := verification.Decision(verificationState.RiskTier, confidence, nil, flags)
	// Mirror the claim service: an automatic approval needs at least one
	// matching private attribute.
	if policy.Decision == verification.DecisionAutoApprove && !hasPrivateMatch(result.Answers, privateKeys) {
		policy.Decision = verification.DecisionMoreInfo
		policy.Reasons = append(policy.Reasons, "no private attribute matched; more information required")
	}
	reasons := policy.Reasons
	if reasons == nil {
		reasons = []string{}
	}
	return toolOutcome{content: mustJSON(scoreResult{
		Confidence: confidence,
		Decision:   policy.Decision,
		Reasons:    reasons,
	})}
}

type claimToolResult struct {
	ClaimID        string `json:"claimId"`
	Status         string `json:"status"`
	ReturnChannel  string `json:"returnChannel"`
	ConfidenceBand string `json:"confidenceBand"`
}

func (s *service) toolCreateClaim(ctx context.Context, searcherID string, call deepseek.ToolCall, turn *turnState) toolOutcome {
	var args struct {
		ItemID   string            `json:"item_id"`
		Answers  map[string]string `json:"answers"`
		Evidence []string          `json:"evidence"`
	}
	if err := decodeArguments(call, &args); err != nil {
		return toolError("invalid claim arguments")
	}
	itemID := strings.TrimSpace(args.ItemID)
	if !turn.allowed[itemID] {
		return toolError("item not available in this turn")
	}
	if turn.claimCreated {
		return toolError("a claim has already been created in this turn")
	}

	// The model may have read expected values from the private profile. Only
	// answers the searcher actually wrote may reach the claim service, so an
	// invented answer cannot auto-approve a claim.
	answers := answersFromUserText(args.Answers, turn.userText)
	if len(answers) == 0 {
		return toolError("answers must come from the searcher's messages")
	}

	created, err := s.deps.Claim.CreateClaim(ctx, claim.CreateClaimInput{
		ItemID:     itemID,
		SearcherID: searcherID,
		Session: claim.Session{
			Answers: answers,
		},
		Evidence: args.Evidence,
	})
	if err != nil {
		return toolError("claim could not be created")
	}
	turn.claimCreated = true
	turn.seen[itemID] = true
	band := created.ConfidenceBand
	if band == "" {
		band = claim.ConfidenceBand(created.Confidence)
	}
	return toolOutcome{
		content: mustJSON(claimToolResult{
			ClaimID:        created.ID,
			Status:         created.Status,
			ReturnChannel:  created.ReturnChannel,
			ConfidenceBand: band,
		}),
		claimID: created.ID,
	}
}

// answersFromUserText drops every answer whose value does not appear verbatim
// (case-insensitive) in the searcher's messages.
func answersFromUserText(answers map[string]string, userText string) map[string]string {
	filtered := make(map[string]string, len(answers))
	if userText == "" {
		return filtered
	}
	for key, value := range answers {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if strings.Contains(userText, strings.ToLower(trimmed)) {
			filtered[key] = trimmed
		}
	}
	return filtered
}

func (s *service) toolEscalate() toolOutcome {
	return toolOutcome{
		content:   mustJSON(map[string]any{"escalated": true}),
		escalated: true,
	}
}

// scoringAttributes mirrors the claim service: public listing attributes plus
// the allowlisted private question attributes, with values sanitized exactly
// like the model profile. Identity, free text and unknown keys never reach the
// engine.
func scoringAttributes(item itemdomain.Item) (map[string]any, map[string]bool) {
	attributes := map[string]any{}
	private := map[string]bool{}
	public := map[string]string{
		"category":  item.Category,
		"brand":     item.Brand,
		"model":     item.Model,
		"color":     item.Color,
		"condition": item.Condition,
	}
	publicKeys := map[string]bool{
		"category": true, "brand": true, "model": true, "color": true, "condition": true,
	}
	for key, value := range public {
		if safe := safePublicText(value); safe != "" {
			attributes[key] = safe
		}
	}
	for key, value := range item.PrivateProfile {
		normalized := normalizeKey(key)
		if normalized == "" || profileMetadataKeys[normalized] || freeTextKeys[normalized] ||
			verification.IsIdentityField(normalized) || !questionAttributeKeys[normalized] {
			continue
		}
		sanitized, ok := safeAttributeValue(value)
		if !ok {
			continue
		}
		attributes[normalized] = sanitized
		if !publicKeys[normalized] {
			private[normalized] = true
		}
	}
	return attributes, private
}

// questionsFor maps supported values to the d3 question types used by the claim
// service: lists become choice questions, everything else stays noul.
func questionsFor(attributes map[string]any) verification.Questions {
	questions := make(verification.Questions, len(attributes)+1)
	for key, value := range attributes {
		switch typed := value.(type) {
		case []any:
			questions[key] = itemdomain.Question{Type: "choice", Criteria: choiceCriteria(typed)}
		case []string:
			questions[key] = itemdomain.Question{Type: "choice", Criteria: choiceCriteria(typed)}
		default:
			questions[key] = itemdomain.Question{Type: "noul"}
		}
	}
	if len(questions) == 0 {
		questions["description"] = itemdomain.Question{Type: "score"}
	}
	return questions
}

func choiceCriteria(values any) map[string]string {
	options := map[string]string{}
	appendOption := func(value any) {
		label := strings.TrimSpace(fmt.Sprint(value))
		if label != "" {
			options[label] = label
		}
	}
	switch typed := values.(type) {
	case []any:
		for _, value := range typed {
			appendOption(value)
		}
	case []string:
		for _, value := range typed {
			appendOption(value)
		}
	}
	if len(options) == 0 {
		return nil
	}
	return options
}

// identityMatches compares the identity fields the searcher answered. Missing
// answers are not mismatches.
func identityMatches(profile map[string]any, answers map[string]string) map[string]bool {
	if len(profile) == 0 || len(answers) == 0 {
		return nil
	}
	matches := map[string]bool{}
	for key, expected := range profile {
		if !verification.IsIdentityField(key) || !hasContent(expected) {
			continue
		}
		for answerKey, answerValue := range answers {
			if strings.EqualFold(strings.TrimSpace(answerKey), strings.TrimSpace(key)) {
				matches[key] = strings.EqualFold(strings.TrimSpace(fmt.Sprint(expected)), strings.TrimSpace(answerValue))
				break
			}
		}
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

func hasIdentityMismatch(matches map[string]bool) bool {
	for _, match := range matches {
		if !match {
			return true
		}
	}
	return false
}

func hasPrivateMatch(answers map[string]itemdomain.Answer, privateKeys map[string]bool) bool {
	for key := range privateKeys {
		answer, ok := answers[key]
		if ok && verification.AnswerProbability(answer) >= 0.75 {
			return true
		}
	}
	return false
}

func riskTierOf(item itemdomain.Item) string {
	if item.Sensitive {
		return itemdomain.RiskSensitive
	}
	switch strings.ToLower(strings.TrimSpace(item.RiskTier)) {
	case itemdomain.RiskLow, itemdomain.RiskMedium, itemdomain.RiskHigh, itemdomain.RiskSensitive:
		return strings.ToLower(strings.TrimSpace(item.RiskTier))
	default:
		return itemdomain.RiskMedium
	}
}

func stringAnswers(answers map[string]string) map[string]any {
	values := make(map[string]any, len(answers))
	for key, value := range answers {
		values[key] = value
	}
	return values
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func hasContent(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	default:
		return true
	}
}

func clamp01(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

// decodeArguments decodes a tool call's JSON arguments.
func decodeArguments(call deepseek.ToolCall, target any) error {
	raw := strings.TrimSpace(call.Function.Arguments)
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("decoding tool %q arguments: %w", call.Function.Name, err)
	}
	return nil
}

// toolError returns a generic error result. Internal error details are logged
// nowhere near the model to avoid accidental data disclosure.
func toolError(message string) toolOutcome {
	return toolOutcome{content: mustJSON(map[string]any{"error": message})}
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `{"error":"encoding failed"}`
	}
	return string(encoded)
}
