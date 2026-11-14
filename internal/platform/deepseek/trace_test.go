package deepseek_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/trace"
)

func TestCompleteWritesModelOutputToTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ciao","tool_calls":[{"id":"c1","type":"function","function":{"name":"search_items","arguments":"{\"query\":\"iphone\"}"}}]},"finish_reason":"tool_calls"}]}`))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "model-trace.jsonl")
	recorder, err := trace.Open(path)
	if err != nil {
		t.Fatalf("trace.Open() error = %v", err)
	}
	client := deepseek.New(server.URL, "key", "test-model", time.Second).WithTrace(recorder, "agent")
	if _, err := client.Complete(context.Background(), []deepseek.Message{{Role: "user", Content: "hi"}}, nil); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	_ = recorder.Close()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading trace: %v", err)
	}
	line := string(raw)
	for _, want := range []string{`"kind":"model_output"`, `"source":"agent"`, `"model":"test-model"`, `"content":"ciao"`, `"search_items"`, `"finishReason":"tool_calls"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("trace line misses %s: %s", want, line)
		}
	}
	if strings.Count(line, "\n") != 1 {
		t.Fatalf("want exactly one JSONL line, got %q", line)
	}
}
