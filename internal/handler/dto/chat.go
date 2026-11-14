package dto

import (
	agentdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
)

// ChatPayload is the JSON body of POST /api/chat. It carries only the
// conversation: the searcher identity comes from the session and the
// verification session is built server-side.
type ChatPayload struct {
	Messages []ChatMessage `json:"messages"`
}

// ChatMessage is one conversation turn sent by the browser.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse is the role-safe orchestrator reply. Candidates carry only the
// public search fields; the private profile, exact coordinates and expected
// values are never serialised.
type ChatResponse struct {
	Message    string                `json:"message"`
	Candidates []search.SearchResult `json:"candidates"`
	ClaimID    string                `json:"claimId"`
	Done       bool                  `json:"done"`
}

// ToAgentMessages maps the transport payload to domain messages.
func ToAgentMessages(messages []ChatMessage) []agentdomain.Message {
	converted := make([]agentdomain.Message, 0, len(messages))
	for _, message := range messages {
		converted = append(converted, agentdomain.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	return converted
}

// ToChatResponse maps a domain reply to the HTTP response.
func ToChatResponse(reply agentdomain.Reply) ChatResponse {
	candidates := reply.Candidates
	if candidates == nil {
		candidates = []search.SearchResult{}
	}
	return ChatResponse{
		Message:    reply.Message,
		Candidates: candidates,
		ClaimID:    reply.ClaimID,
		Done:       reply.Done,
	}
}
