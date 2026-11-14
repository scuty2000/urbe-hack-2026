package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
// abort the request.
func (s *service) executeTool(ctx context.Context, searcherID string, call deepseek.ToolCall) toolOutcome {
	switch strings.TrimSpace(call.Function.Name) {
	case ToolSearchItems:
		return s.toolSearch(ctx, call)
	case ToolGetPrivateProfile:
		return s.toolPrivateProfile(ctx, call)
	case ToolScoreWithD3:
		return s.toolScore(ctx, call)
	case ToolCreateClaim:
		return s.toolCreateClaim(ctx, searcherID, call)
	case ToolEscalate:
		return s.toolEscalate()
	default:
		return toolError("unknown tool")
	}
}

func (s *service) toolSearch(ctx context.Context, call deepseek.ToolCall) toolOutcome {
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

func (s *service) toolPrivateProfile(ctx context.Context, call deepseek.ToolCall) toolOutcome {
	var args struct {
		ItemID string `json:"item_id"`
	}
	if err := decodeArguments(call, &args); err != nil {
		return toolError("invalid profile arguments")
	}
	item, err := s.deps.Items.ItemForAgent(ctx, strings.TrimSpace(args.ItemID))
	if err != nil {
		return toolError("item not found")
	}
	return toolOutcome{content: mustJSON(sanitizeProfile(item))}
}

// sanitizeProfile keeps public listing attributes and non-sensitive private
// attributes. Identity values become booleans and free text is dropped.
func sanitizeProfile(item itemdomain.Item) sanitizedProfile {
	profile := sanitizedProfile{
		ItemID:    item.ID,
		Category:  item.Category,
		Brand:     item.Brand,
		Model:     item.Model,
		Color:     item.Color,
		Condition: item.Condition,
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
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				documentType = strings.TrimSpace(text)
			}
			continue
		}
		if verification.IsIdentityField(normalized) {
			identity[normalized] = hasContent(value)
			continue
		}
		if !supportedValue(value) {
			continue
		}
		attributes[normalized] = value
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

// scoreResult is the only scoring output sent to the model. Expected values are
// never included.
type scoreResult struct {
	Confidence float64  `json:"confidence"`
	Decision   string   `json:"decision"`
	Reasons    []string `json:"reasons"`
}

func (s *service) toolScore(ctx context.Context, call deepseek.ToolCall) toolOutcome {
	var args struct {
		ItemID  string            `json:"item_id"`
		Answers map[string]string `json:"answers"`
	}
	if err := decodeArguments(call, &args); err != nil {
		return toolError("invalid score arguments")
	}
	item, err := s.deps.Items.ItemForAgent(ctx, strings.TrimSpace(args.ItemID))
	if err != nil {
		return toolError("item not found")
	}

	attributes, privateKeys := scoringAttributes(item)
	questions := questionsFor(attributes)
	state := verification.State{
		Item:          attributes,
		Searcher:      stringAnswers(args.Answers),
		IdentityMatch: identityMatches(item.PrivateProfile, args.Answers),
		RiskTier:      riskTierOf(item),
	}
	flags := []string{}
	if hasIdentityMismatch(state.IdentityMatch) {
		flags = append(flags, verification.FlagIdentityMismatch)
	}

	result, err := s.deps.Engine.Score(ctx, state, questions)
	if err != nil {
		return toolError("scoring is unavailable")
	}
	confidence := clamp01(result.Confidence)
	policy := verification.Decision(state.RiskTier, confidence, nil, flags)
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

func (s *service) toolCreateClaim(ctx context.Context, searcherID string, call deepseek.ToolCall) toolOutcome {
	var args struct {
		ItemID   string            `json:"item_id"`
		Answers  map[string]string `json:"answers"`
		Evidence []string          `json:"evidence"`
	}
	if err := decodeArguments(call, &args); err != nil {
		return toolError("invalid claim arguments")
	}
	created, err := s.deps.Claim.CreateClaim(ctx, claim.CreateClaimInput{
		ItemID:     strings.TrimSpace(args.ItemID),
		SearcherID: searcherID,
		Session: claim.Session{
			Answers: args.Answers,
		},
		Evidence: args.Evidence,
	})
	if err != nil {
		return toolError("claim could not be created")
	}
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

func (s *service) toolEscalate() toolOutcome {
	return toolOutcome{
		content:   mustJSON(map[string]any{"escalated": true}),
		escalated: true,
	}
}

// scoringAttributes mirrors the claim service: public listing attributes plus
// supported private scalars, with identity and free text excluded.
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
		if strings.TrimSpace(value) != "" {
			attributes[key] = value
		}
	}
	for key, value := range item.PrivateProfile {
		normalized := normalizeKey(key)
		if normalized == "" || profileMetadataKeys[normalized] || freeTextKeys[normalized] ||
			verification.IsIdentityField(normalized) || !supportedValue(value) {
			continue
		}
		attributes[normalized] = value
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

func supportedValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(typed) != ""
	case bool:
		return true
	case float64, float32, int, int64, int32:
		return true
	case []string:
		return len(typed) > 0
	case []any:
		if len(typed) == 0 {
			return false
		}
		for _, element := range typed {
			switch element.(type) {
			case string, bool, float64, float32, int, int64, int32:
			default:
				return false
			}
		}
		return true
	default:
		return false
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
