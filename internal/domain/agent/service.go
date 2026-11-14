package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
)

const (
	// fallbackMessage is returned when the provider produced no usable text.
	fallbackMessage = "Non riesco a completare la verifica in questa conversazione. Ho inoltrato la richiesta a un operatore."
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
// searcher never sees a 500 because of DeepSeek.
func (s *service) Chat(ctx context.Context, searcherID string, messages []Message) (Reply, error) {
	searcherID = strings.TrimSpace(searcherID)
	if searcherID == "" {
		return Reply{}, fmt.Errorf("agent chat: searcher id is required: %w", errs.ErrInvalidInput)
	}
	normalized, err := normalizeMessages(messages)
	if err != nil {
		return Reply{}, err
	}

	if s.deps.Client == nil {
		return s.mockChat(ctx, searcherID, normalized)
	}
	reply, err := s.llmChat(ctx, searcherID, normalized)
	if err != nil {
		return s.mockChat(ctx, searcherID, normalized)
	}
	return reply, nil
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
			if turn.used {
				return Reply{
					Message:    partialFailureMessage,
					Candidates: candidates,
					ClaimID:    claimID,
					Done:       claimID != "" || escalated,
				}, nil
			}
			return Reply{}, err
		}

		if len(completion.ToolCalls) == 0 {
			message := s.redactMessage(ctx, completion.Content, turn)
			if message == "" {
				message = fallbackMessage
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
			if len(outcome.candidates) > 0 {
				candidates = mergeCandidates(candidates, outcome.candidates)
			}
			if outcome.claimID != "" {
				claimID = outcome.claimID
			}
			if outcome.escalated {
				escalated = true
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

	return Reply{Message: fallbackMessage, Candidates: candidates, ClaimID: claimID, Done: claimID != ""}, nil
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
// Short tokens are skipped so redaction cannot mangle ordinary words.
func privateTokens(profile map[string]any) []string {
	tokens := make([]string, 0, len(profile))
	for _, value := range profile {
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

func appendToken(tokens *[]string, value string) {
	value = strings.TrimSpace(value)
	if len(value) < 4 {
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

// replaceAllFold replaces every case-insensitive occurrence of token.
func replaceAllFold(text, token, replacement string) string {
	if token == "" {
		return text
	}
	lowerText := strings.ToLower(text)
	lowerToken := strings.ToLower(token)
	var builder strings.Builder
	for {
		index := strings.Index(lowerText, lowerToken)
		if index < 0 {
			builder.WriteString(text)
			return builder.String()
		}
		builder.WriteString(text[:index])
		builder.WriteString(replacement)
		text = text[index+len(lowerToken):]
		lowerText = lowerText[index+len(lowerToken):]
	}
}
