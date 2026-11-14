package deepseek_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
)

// chatRequestBody mirrors the OpenAI-compatible payload the client must send.
type chatRequestBody struct {
	Model       string             `json:"model"`
	Messages    []deepseek.Message `json:"messages"`
	Tools       []deepseek.Tool    `json:"tools"`
	ToolChoice  string             `json:"tool_choice"`
	Temperature float64            `json:"temperature"`
}

func TestClientSendsPathModelAuthAndTools(t *testing.T) {
	var captured chatRequestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		_, _ = w.Write([]byte(`{
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Ciao, come posso aiutarti?",
					"tool_calls": [{
						"id": "call-1",
						"type": "function",
						"function": {"name": "search_items", "arguments": "{\"query\":\"iphone\"}"}
					}]
				},
				"finish_reason": "tool_calls"
			}]
		}`))
	}))
	defer server.Close()

	client := deepseek.New(server.URL, "test-key", "deepseek-v4.1-flash", 2*time.Second)
	completion, err := client.Complete(context.Background(),
		[]deepseek.Message{{Role: "user", Content: "ho perso un iphone"}},
		[]deepseek.Tool{{
			Type: "function",
			Function: deepseek.ToolFunction{
				Name:        "search_items",
				Description: "search",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	if captured.Model != "deepseek-v4.1-flash" {
		t.Fatalf("model = %q", captured.Model)
	}
	if len(captured.Messages) != 1 || captured.Messages[0].Content != "ho perso un iphone" {
		t.Fatalf("messages = %+v", captured.Messages)
	}
	if len(captured.Tools) != 1 || captured.Tools[0].Function.Name != "search_items" {
		t.Fatalf("tools = %+v", captured.Tools)
	}
	if captured.ToolChoice != "auto" {
		t.Fatalf("tool_choice = %q, want auto", captured.ToolChoice)
	}
	if captured.Temperature != 0.2 {
		t.Fatalf("temperature = %v, want 0.2", captured.Temperature)
	}

	if completion.Content != "Ciao, come posso aiutarti?" {
		t.Fatalf("content = %q", completion.Content)
	}
	if len(completion.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", completion.ToolCalls)
	}
	call := completion.ToolCalls[0]
	if call.ID != "call-1" || call.Function.Name != "search_items" {
		t.Fatalf("tool call = %+v", call)
	}
	if call.Function.Arguments != `{"query":"iphone"}` {
		t.Fatalf("arguments = %q", call.Function.Arguments)
	}
}

func TestClientUnavailableOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := deepseek.New(server.URL, "test-key", "deepseek-v4.1-flash", time.Second)
	_, err := client.Complete(context.Background(), []deepseek.Message{{Role: "user", Content: "ciao"}}, nil)
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestClientUnavailableOnInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	client := deepseek.New(server.URL, "test-key", "deepseek-v4.1-flash", time.Second)
	_, err := client.Complete(context.Background(), []deepseek.Message{{Role: "user", Content: "ciao"}}, nil)
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestClientUnavailableWithoutKey(t *testing.T) {
	client := deepseek.New("http://127.0.0.1:1", "", "deepseek-v4.1-flash", time.Second)
	_, err := client.Complete(context.Background(), []deepseek.Message{{Role: "user", Content: "ciao"}}, nil)
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestClientUnavailableOnTransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := server.URL
	server.Close()

	client := deepseek.New(url, "test-key", "deepseek-v4.1-flash", 500*time.Millisecond)
	_, err := client.Complete(context.Background(), []deepseek.Message{{Role: "user", Content: "ciao"}}, nil)
	if !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}
