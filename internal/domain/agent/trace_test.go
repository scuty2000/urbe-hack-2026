package agent_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/trace"
)

func TestChatWritesTurnToolDecisionAndReplyToTrace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traces", "model-trace.jsonl")
	recorder, err := trace.Open(path)
	if err != nil {
		t.Fatalf("trace.Open() error = %v", err)
	}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "score_with_d3",
			`{"item_id":"item-phone","answers":{"case_color":"giallo"}}`)}},
		{Content: "Mi descriva un altro dettaglio."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Trace: recorder})

	if _, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ha un cinturino giallo"}}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening trace: %v", err)
	}
	defer func() { _ = file.Close() }()

	kinds := map[string]int{}
	turns := map[string]bool{}
	tools := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event trace.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("trace line is not JSON: %v: %s", err, scanner.Text())
		}
		kinds[event.Kind]++
		turns[event.TurnID] = true
		if event.Kind == trace.KindToolCall {
			tools = append(tools, event.Tool)
		}
	}
	if kinds[trace.KindChatTurn] != 1 || kinds[trace.KindReply] != 1 ||
		kinds[trace.KindToolCall] != 2 || kinds[trace.KindDecision] != 1 {
		t.Fatalf("event kinds = %v, want 1 chat_turn, 2 tool_call, 1 decision, 1 reply", kinds)
	}
	if len(tools) != 2 || tools[0] != "search_items" || tools[1] != "score_with_d3" {
		t.Fatalf("traced tools = %v", tools)
	}
	if len(turns) != 1 || turns[""] {
		t.Fatalf("turn ids = %v, want one shared non-empty id", turns)
	}
}
