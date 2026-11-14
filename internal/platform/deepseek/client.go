/*
Package deepseek is a small OpenAI-compatible client for the DeepSeek chat
completions API.

The orchestrator agent uses it with tool definitions: the client sends the
conversation and the declared tools, and returns the assistant content plus any
tool calls. It deliberately has no retry logic and no logging: the agent falls
back to its deterministic mock as soon as the client reports
errs.ErrUnavailable, so a provider outage never blocks the demo.

No provider response is trusted for policy decisions: the model only proposes
tool calls, and every tool is executed server-side.
*/
package deepseek

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
)

const (
	// chatPath is the OpenAI-compatible chat completions endpoint.
	chatPath = "/chat/completions"
	// maxResponseBytes bounds the response body read from the provider.
	maxResponseBytes = 1 << 20
	// defaultTimeout is used when the caller passes a non-positive timeout.
	defaultTimeout = 30 * time.Second
	// temperature keeps the interview focused and reproducible.
	temperature = 0.2
)

// Message is one OpenAI-compatible chat message. Assistant messages may carry
// tool calls; tool messages reference the call they answer. Content is a
// string for text-only turns and a []ContentPart for multimodal turns.
type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ContentPart is one part of an OpenAI-compatible multimodal message.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL is the image payload of a ContentPart. URL may be an https URL or
// a data URL.
type ImageURL struct {
	URL string `json:"url"`
}

// TextPart returns a text content part.
func TextPart(text string) ContentPart {
	return ContentPart{Type: "text", Text: text}
}

// ImagePart returns a data-URL image content part so the provider can see the
// photo without a public URL.
func ImagePart(mediaType string, data []byte) ContentPart {
	mediaType = strings.TrimSpace(mediaType)
	if mediaType == "" {
		mediaType = "image/jpeg"
	}
	return ContentPart{
		Type:     "image_url",
		ImageURL: &ImageURL{URL: "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)},
	}
}

// ToolCall is one function invocation requested by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall is the name and JSON-encoded arguments of a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool declares a function the model may call.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction is the OpenAI-compatible function description.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Completion is the assistant turn returned by the provider.
type Completion struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
}

// Client calls a DeepSeek chat completions endpoint.
type Client struct {
	baseURL string
	key     string
	model   string
	http    *http.Client
}

// New returns a DeepSeek client. An empty baseURL or key makes every call
// return errs.ErrUnavailable, which lets the agent use its mock without a
// separate feature flag.
func New(baseURL, key, model string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		key:     strings.TrimSpace(key),
		model:   strings.TrimSpace(model),
		http:    &http.Client{Timeout: timeout},
	}
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  string    `json:"tool_choice,omitempty"`
	Temperature float64   `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// Complete sends one chat completion request. Any transport, status or decoding
// failure is reported as errs.ErrUnavailable so the agent can fall back.
func (c *Client) Complete(ctx context.Context, messages []Message, tools []Tool) (Completion, error) {
	if c == nil || c.baseURL == "" || c.key == "" {
		return Completion{}, fmt.Errorf("deepseek completion: endpoint is not configured: %w", errs.ErrUnavailable)
	}

	payload, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    messages,
		Tools:       tools,
		ToolChoice:  toolChoice(tools),
		Temperature: temperature,
	})
	if err != nil {
		return Completion{}, fmt.Errorf("deepseek completion: encoding request: %w", errs.ErrUnavailable)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+chatPath, bytes.NewReader(payload))
	if err != nil {
		return Completion{}, fmt.Errorf("deepseek completion: building request: %w", errs.ErrUnavailable)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)

	resp, err := c.http.Do(req)
	if err != nil {
		return Completion{}, fmt.Errorf("deepseek completion: request failed: %w: %w", errs.ErrUnavailable, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Completion{}, fmt.Errorf("deepseek completion: unexpected status %d: %w", resp.StatusCode, errs.ErrUnavailable)
	}

	var decoded chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&decoded); err != nil {
		return Completion{}, fmt.Errorf("deepseek completion: decoding response: %w", errs.ErrUnavailable)
	}
	if len(decoded.Choices) == 0 {
		return Completion{}, fmt.Errorf("deepseek completion: empty choices: %w", errs.ErrUnavailable)
	}

	choice := decoded.Choices[0]
	return Completion{
		Content:      strings.TrimSpace(choice.Message.Content),
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
	}, nil
}

// toolChoice asks the model to decide between answering and calling a tool.
// The field is omitted when no tools are declared, because some compatible
// providers reject tool_choice without tools.
func toolChoice(tools []Tool) string {
	if len(tools) == 0 {
		return ""
	}
	return "auto"
}
