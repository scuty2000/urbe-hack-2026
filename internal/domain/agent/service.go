package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/gofrs/uuid"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/trace"
)

const (
	// wrapUpInstruction asks the model for a plain reply when it produced no
	// text or used up its tool rounds. There is no canned fallback sentence:
	// the searcher always gets the model's own words, or an error.
	wrapUpInstruction = "Hai esaurito gli strumenti per questo turno. Rispondi ora all'utente, in italiano e in modo breve, senza usare strumenti: digli a che punto è la verifica in base ai risultati ottenuti e fai una sola domanda utile per proseguire. Non rivelare valori attesi o dettagli privati."
	// escalationMessage is returned when the model escalates.
	escalationMessage = "Ho inoltrato la richiesta a un operatore. Riceverai aggiornamenti in questa conversazione."
	// partialFailureMessage is returned when the provider fails after a tool
	// already ran. The mock is not restarted, so a created claim is not
	// duplicated.
	partialFailureMessage = "La conversazione si è interrotta. Se una richiesta è stata registrata, la trovi tra le tue richieste; riprova tra poco per continuare."
	// maxRedactedItems bounds the private-profile lookups performed before
	// returning a model-generated message.
	maxRedactedItems = 10
	// redactionPlaceholder replaces private values found in a model message.
	redactionPlaceholder = "[dettaglio riservato]"
)

// service implements Service.
type service struct {
	deps   Deps
	rounds int
}

// Chat runs one conversation turn. Without a configured client the deterministic
// mock answers; any provider failure also falls back to the mock, so the
// searcher never sees a 500 because of DeepSeek. The turn (messages, model
// outputs, tool calls, ownership decisions and the reply) is appended to the
// JSONL trace when one is configured.
func (s *service) Chat(ctx context.Context, searcherID string, messages []Message) (Reply, error) {
	searcherID = strings.TrimSpace(searcherID)
	if searcherID == "" {
		return Reply{}, fmt.Errorf("agent chat: searcher id is required: %w", errs.ErrInvalidInput)
	}
	normalized, err := normalizeMessages(messages)
	if err != nil {
		return Reply{}, err
	}

	ctx = trace.WithTurnID(ctx, newTurnID())
	s.deps.Trace.Record(ctx, trace.Event{
		Kind:   trace.KindChatTurn,
		Source: "agent",
		Data:   map[string]any{"searcherId": searcherID, "messages": traceMessages(normalized)},
	})
	reply, path, err := s.route(ctx, searcherID, normalized)
	s.deps.Trace.Record(ctx, trace.Event{
		Kind:   trace.KindReply,
		Source: "agent",
		Error:  trace.ErrString(err),
		Output: map[string]any{
			"message":    reply.Message,
			"candidates": len(reply.Candidates),
			"claimId":    reply.ClaimID,
			"done":       reply.Done,
		},
		Data: map[string]any{"path": path},
	})
	return reply, err
}

// route picks who answers the turn and reports it for the trace: the
// deterministic ownership gate, the model or the mock.
func (s *service) route(ctx context.Context, searcherID string, normalized []Message) (Reply, string, error) {
	if s.deps.Offline {
		// Tests only: the keyword gate and the keyword agent stand in for
		// DeepSeek. Production never extracts answers with keywords: the
		// conversation is always driven by the model, the decision by the
		// judge and the policy.
		if reply, handled, err := s.preflightChat(ctx, searcherID, normalized); err != nil {
			return Reply{}, "offline", err
		} else if handled {
			return reply, "offline", nil
		}
		reply, err := s.mockChat(ctx, searcherID, normalized)
		return reply, "offline", err
	}

	if s.deps.Client == nil {
		return Reply{}, "llm", fmt.Errorf("agent chat: deepseek is not configured: %w", errs.ErrUnavailable)
	}
	// No local fallback: a DeepSeek failure is an error for the caller.
	reply, err := s.llmChat(ctx, searcherID, normalized)
	if err != nil {
		return Reply{}, "llm", fmt.Errorf("agent chat: %w", err)
	}
	return reply, "llm", nil
}

// maxUndecidedUserTurns is the number of searcher messages after which an
// undecided ownership check stops asking and goes to human review. The catalog
// can be wrong (a transparent case recorded as black), so an honest searcher
// must never be stuck in an endless "tell me more" loop.
const maxUndecidedUserTurns = 4

func undecidedLimitReached(messages []Message) bool {
	return len(messagesFromUser(messages)) >= maxUndecidedUserTurns
}

func newTurnID() string {
	id, err := uuid.NewV4()
	if err != nil {
		return ""
	}
	return id.String()
}

func traceMessages(messages []Message) []map[string]string {
	out := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		out = append(out, map[string]string{"role": message.Role, "content": message.Content})
	}
	return out
}

// maxEvaluatedCandidates bounds the candidates scored by the deterministic
// gate. The searcher's object is not always the first search result.
const maxEvaluatedCandidates = 3

// candidateEvaluation is the ownership evaluation of one search candidate.
type candidateEvaluation struct {
	itemID     string
	evaluation ownershipEvaluation
}

// bestCandidate scores the top candidates and returns the most favourable
// one, so a detail that contradicts the first result does not close the
// conversation when it fits the second. Search order breaks ties. A scoring
// outage is returned as an error: no decision is taken without the judge.
func (s *service) bestCandidate(ctx context.Context, results []search.SearchResult, answers map[string]string, statement string) (candidateEvaluation, bool, error) {
	best := candidateEvaluation{}
	bestRank := -1
	for index, result := range results {
		if index >= maxEvaluatedCandidates {
			break
		}
		item, err := s.deps.Items.ItemForAgent(ctx, result.ItemID)
		if err != nil {
			continue
		}
		evaluation, err := s.evaluateOwnership(ctx, item, answers, statement)
		if errors.Is(err, errs.ErrUnavailable) {
			return candidateEvaluation{}, false, err
		}
		if err != nil {
			continue
		}
		if rank := decisionRank(evaluation); rank > bestRank {
			best = candidateEvaluation{itemID: result.ItemID, evaluation: evaluation}
			bestRank = rank
		}
	}
	return best, bestRank >= 0, nil
}

// decisionRank orders evaluations from least to most favourable: a rejection,
// an item nobody can verify automatically, an undecided case and a verified
// (approved or reviewable) one.
func decisionRank(evaluation ownershipEvaluation) int {
	switch {
	case evaluation.Decision == verification.DecisionReject:
		return 0
	case evaluation.unverifiable():
		return 1
	case evaluation.Decision == verification.DecisionMoreInfo:
		return 2
	default:
		return 3
	}
}

// preflightChat is the deterministic ownership gate. After the first exchange,
// it searches with the searcher's own words, evaluates the extracted private
// answers with the configured engine (the DeepSeek judge, local scorer as fallback) against the top
// candidates and either creates the claim, asks for more information, or
// closes the conversation when every candidate clearly contradicts the answers.
// An item without verifiable private attributes becomes a claim under human
// review instead of a refusal.
func (s *service) preflightChat(ctx context.Context, searcherID string, messages []Message) (Reply, bool, error) {
	users := messagesFromUser(messages)
	if len(users) < 2 {
		return Reply{}, false, nil
	}
	text := userText(messages)
	answers := mockAnswers(text)
	if len(answers) == 0 {
		return Reply{}, false, nil
	}
	zone := mockZone(users[0].Content)
	results, err := s.deps.Search.Search(ctx, search.Query{
		Text:     text,
		Category: answers["category"],
		Zone:     zone,
		Limit:    searchLimit,
	})
	if err != nil || len(results) == 0 {
		return Reply{}, false, nil
	}
	best, ok, err := s.bestCandidate(ctx, results, answers, text)
	if err != nil {
		return Reply{}, false, err
	}
	if !ok {
		return Reply{}, false, nil
	}
	if best.evaluation.Decision == verification.DecisionReject {
		return Reply{
			Message:    terminalMismatchMessage,
			Candidates: results,
			Done:       true,
		}, true, nil
	}
	flags := best.evaluation.claimFlags()
	if best.evaluation.Decision == verification.DecisionMoreInfo {
		if !undecidedLimitReached(messages) {
			// The server asks, so the model cannot turn an undecided case
			// into an escalation or a forwarded claim.
			return Reply{
				Message:    moreInfoMessage,
				Candidates: results,
			}, true, nil
		}
		// The searcher kept answering without a decision: a human decides
		// instead of asking forever. missing_evidence forces review.
		flags = append(flags, verification.FlagMissingEvidence)
	}

	created, err := s.deps.Claim.CreateClaim(ctx, claim.CreateClaimInput{
		ItemID:     best.itemID,
		SearcherID: searcherID,
		Session:    claim.Session{Answers: answers, Statement: text},
		Flags:      flags,
	})
	if errors.Is(err, errs.ErrUnavailable) {
		return Reply{}, false, err
	}
	if err != nil {
		return Reply{}, false, nil
	}
	if created.Status == claim.StatusRejected {
		return Reply{
			Message:    terminalMismatchMessage,
			Candidates: results,
			ClaimID:    created.ID,
			Done:       true,
		}, true, nil
	}
	if created.Status == claim.StatusMoreInfo {
		return Reply{}, false, nil
	}
	return Reply{
		Message:    s.claimMessage(ctx, best.itemID, created, best.evaluation),
		Candidates: results,
		ClaimID:    created.ID,
		Done:       true,
	}, true, nil
}

// normalizeMessages validates the client conversation and enforces the role
// boundary: only the user and the assistant may speak, and a system prompt can
// never be injected by the caller.
func normalizeMessages(messages []Message) ([]Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("agent chat: at least one message is required: %w", errs.ErrInvalidInput)
	}
	if len(messages) > maxClientMessages {
		return nil, fmt.Errorf("agent chat: at most %d messages are allowed: %w", maxClientMessages, errs.ErrInvalidInput)
	}
	normalized := make([]Message, 0, len(messages))
	hasUser := false
	for _, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		content := strings.TrimSpace(message.Content)
		if role != "user" && role != "assistant" {
			return nil, fmt.Errorf("agent chat: role %q is not allowed: %w", message.Role, errs.ErrInvalidInput)
		}
		if content == "" {
			return nil, fmt.Errorf("agent chat: message content is required: %w", errs.ErrInvalidInput)
		}
		if role == "user" {
			hasUser = true
		}
		normalized = append(normalized, Message{Role: role, Content: content})
	}
	if !hasUser {
		return nil, fmt.Errorf("agent chat: a user message is required: %w", errs.ErrInvalidInput)
	}
	return normalized, nil
}

// llmChat drives the tool loop: send the conversation, execute the requested
// tools server-side, append the tool results and repeat up to the round cap.
func (s *service) llmChat(ctx context.Context, searcherID string, messages []Message) (Reply, error) {
	conversation := make([]deepseek.Message, 0, maxConversationMessages)
	conversation = append(conversation, deepseek.Message{Role: "system", Content: systemPrompt})
	for _, message := range messages {
		conversation = append(conversation, deepseek.Message{Role: message.Role, Content: message.Content})
	}

	var candidates []search.SearchResult
	claimID := ""
	escalated := false
	turn := newTurnState(messages)

	for round := 0; round < s.rounds; round++ {
		conversation = trimConversation(conversation)
		completion, err := s.deps.Client.Complete(ctx, conversation, toolDefinitions())
		if err != nil {
			// Never restart the mock after a tool ran: a claim may already
			// exist, and the mock would create a duplicate.
			// A claim already created in this turn is kept and reported;
			// otherwise the provider failure is the caller's error.
			if claimID != "" {
				return Reply{
					Message:    partialFailureMessage,
					Candidates: candidates,
					ClaimID:    claimID,
					Done:       true,
				}, nil
			}
			return Reply{}, err
		}

		if len(completion.ToolCalls) == 0 {
			// The model answered in plain text: that is the reply.
			message := s.redactMessage(ctx, completion.Content, turn)
			if message == "" {
				if message, err = s.wrapUp(ctx, conversation, turn); err != nil {
					return Reply{}, err
				}
			}
			return Reply{
				Message:    message,
				Candidates: candidates,
				ClaimID:    claimID,
				Done:       claimID != "" || escalated,
			}, nil
		}

		conversation = append(conversation, deepseek.Message{
			Role:      "assistant",
			Content:   completion.Content,
			ToolCalls: completion.ToolCalls,
		})
		for _, call := range completion.ToolCalls {
			outcome := s.executeTool(ctx, searcherID, call, turn)
			if outcome.err != nil {
				return Reply{}, outcome.err
			}
			if len(outcome.candidates) > 0 {
				candidates = mergeCandidates(candidates, outcome.candidates)
			}
			if outcome.claimID != "" {
				claimID = outcome.claimID
			}
			if outcome.escalated {
				escalated = true
			}
			if outcome.terminal {
				return Reply{
					Message:    outcome.terminalMessage,
					Candidates: candidates,
					ClaimID:    claimID,
					Done:       true,
				}, nil
			}
			conversation = append(conversation, deepseek.Message{
				Role:       "tool",
				Content:    outcome.content,
				ToolCallID: call.ID,
				Name:       call.Function.Name,
			})
		}
		if escalated {
			return Reply{
				Message:    escalationMessage,
				Candidates: candidates,
				ClaimID:    claimID,
				Done:       true,
			}, nil
		}
	}

	message, err := s.wrapUp(ctx, conversation, turn)
	if err != nil {
		return Reply{}, err
	}
	return Reply{Message: message, Candidates: candidates, ClaimID: claimID, Done: claimID != "" || escalated}, nil
}

// wrapUp asks the model for a final plain-text reply without tools. An empty
// answer is a provider failure, never replaced by a canned sentence.
func (s *service) wrapUp(ctx context.Context, conversation []deepseek.Message, turn *turnState) (string, error) {
	conversation = append(trimConversation(conversation), deepseek.Message{Role: "system", Content: wrapUpInstruction})
	completion, err := s.deps.Client.Complete(ctx, conversation, nil)
	if err != nil {
		return "", err
	}
	message := s.redactMessage(ctx, completion.Content, turn)
	if message == "" {
		return "", fmt.Errorf("agent chat: the model returned no reply: %w", errs.ErrUnavailable)
	}
	return message, nil
}

// trimConversation caps the provider conversation, keeping the system prompt
// and the most recent turns.
func trimConversation(conversation []deepseek.Message) []deepseek.Message {
	if len(conversation) <= maxConversationMessages {
		return conversation
	}
	trimmed := make([]deepseek.Message, 0, maxConversationMessages)
	trimmed = append(trimmed, conversation[0])
	keep := maxConversationMessages - 1
	trimmed = append(trimmed, conversation[len(conversation)-keep:]...)
	return trimmed
}

// mergeCandidates appends unseen candidates, preserving order and de-duplicating
// by item id.
func mergeCandidates(existing, added []search.SearchResult) []search.SearchResult {
	seen := make(map[string]bool, len(existing))
	for _, candidate := range existing {
		seen[candidate.ItemID] = true
	}
	for _, candidate := range added {
		if candidate.ItemID == "" || seen[candidate.ItemID] {
			continue
		}
		seen[candidate.ItemID] = true
		existing = append(existing, candidate)
	}
	return existing
}

// redactMessage removes private values of every item touched in the turn from a
// model-generated message. It always runs, even when no search candidate was
// collected: a turn that fetched a profile without search must not echo it.
func (s *service) redactMessage(ctx context.Context, message string, turn *turnState) string {
	message = strings.TrimSpace(message)
	if message == "" || turn == nil || len(turn.seen) == 0 {
		return message
	}

	tokens := make([]string, 0, 16)
	inspected := 0
	for itemID := range turn.seen {
		if inspected >= maxRedactedItems {
			break
		}
		inspected++
		item, err := s.deps.Items.ItemForAgent(ctx, itemID)
		if err != nil {
			continue
		}
		tokens = append(tokens, privateTokens(item.PrivateProfile)...)
	}
	for _, token := range tokens {
		message = replaceAllFold(message, token, redactionPlaceholder)
	}
	return message
}

// privateTokens extracts the values that must never be visible to a searcher.
// Values shorter than two characters and common stopwords are skipped so
// redaction cannot mangle ordinary prose.
func privateTokens(profile map[string]any) []string {
	tokens := make([]string, 0, len(profile))
	for key, value := range profile {
		// Only the structured attributes exposed to the model need a reply
		// backstop. Free-text notes/details are never sent to the model
		// (freeTextKeys in tools.go); splitting them into words would redact
		// public object names and ordinary question words, producing nonsense
		// such as "[dettaglio riservato] ritrovato in zona Testaccio".
		if !questionAttributeKeys[normalizeKey(key)] {
			continue
		}
		collectTokens(value, &tokens)
	}
	return tokens
}

func collectTokens(value any, tokens *[]string) {
	switch typed := value.(type) {
	case string:
		appendToken(tokens, typed)
		for _, part := range strings.FieldsFunc(typed, isSeparator) {
			appendToken(tokens, part)
		}
	case []string:
		for _, element := range typed {
			collectTokens(element, tokens)
		}
	case []any:
		for _, element := range typed {
			collectTokens(element, tokens)
		}
	}
}

// redactionStopwords are too common to redact without mangling normal prose.
// They are compared case-insensitively after trimming.
var redactionStopwords = map[string]bool{
	"si": true, "sì": true, "no": true, "ok": true,
	"il": true, "lo": true, "la": true, "un": true, "una": true,
	"di": true, "da": true, "in": true, "con": true, "su": true,
	"per": true, "e": true, "o": true, "a": true,
}

func appendToken(tokens *[]string, value string) {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) < 2 {
		return
	}
	if redactionStopwords[strings.ToLower(value)] {
		return
	}
	hasWord := false
	for _, r := range runes {
		if isWordRune(r) {
			hasWord = true
			break
		}
	}
	if !hasWord {
		return
	}
	*tokens = append(*tokens, value)
}

func isSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '-', '_', '/', ',', '.', ';', ':':
		return true
	default:
		return false
	}
}

// isWordRune reports whether r is a letter or digit for word-boundary purposes.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// replaceAllFold replaces every case-insensitive whole-word occurrence of
// token. A two-character private value such as "blu" is redacted, while the
// longer word "bluetooth" is left intact.
func replaceAllFold(text, token, replacement string) string {
	if token == "" {
		return text
	}
	lowerText := []rune(strings.ToLower(text))
	lowerToken := []rune(strings.ToLower(token))
	if len(lowerToken) == 0 {
		return text
	}
	original := []rune(text)

	var builder strings.Builder
	last := 0
	for i := 0; i+len(lowerToken) <= len(lowerText); {
		match := true
		for j := range lowerToken {
			if lowerText[i+j] != lowerToken[j] {
				match = false
				break
			}
		}
		if !match {
			i++
			continue
		}
		beforeOK := i == 0 || !isWordRune(lowerText[i-1])
		afterIndex := i + len(lowerToken)
		afterOK := afterIndex == len(lowerText) || !isWordRune(lowerText[afterIndex])
		if !beforeOK || !afterOK {
			i++
			continue
		}
		builder.WriteString(string(original[last:i]))
		builder.WriteString(replacement)
		last = afterIndex
		i = afterIndex
	}
	if last == 0 {
		return text
	}
	builder.WriteString(string(original[last:]))
	return builder.String()
}
