package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/agent"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/seed"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/testutil"
)

// scriptedLLM replays a fixed completion sequence and records every call.
type scriptedLLM struct {
	mu    sync.Mutex
	steps []deepseek.Completion
	err   error
	calls [][]deepseek.Message
	tools [][]deepseek.Tool
}

func (f *scriptedLLM) Complete(_ context.Context, messages []deepseek.Message, tools []deepseek.Tool) (deepseek.Completion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]deepseek.Message(nil), messages...))
	f.tools = append(f.tools, append([]deepseek.Tool(nil), tools...))
	if f.err != nil {
		return deepseek.Completion{}, f.err
	}
	if len(f.steps) == 0 {
		return deepseek.Completion{Content: "Come posso aiutarti?"}, nil
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	return step, nil
}

func (f *scriptedLLM) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *scriptedLLM) call(index int) []deepseek.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[index]
}

// recordingClaims counts CreateClaim invocations and keeps the command.
type recordingClaims struct {
	mu      sync.Mutex
	calls   int
	input   claim.CreateClaimInput
	created claim.Claim
	err     error
}

func (r *recordingClaims) CreateClaim(_ context.Context, input claim.CreateClaimInput) (claim.Claim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.input = input
	if r.err != nil {
		return claim.Claim{}, r.err
	}
	return r.created, nil
}

func (r *recordingClaims) GetClaim(context.Context, string, claim.Actor) (claim.Claim, error) {
	return claim.Claim{}, errs.ErrNotFound
}

func (r *recordingClaims) ReviewClaim(context.Context, string, string, string) (claim.Claim, error) {
	return claim.Claim{}, errs.ErrNotFound
}

func toolCall(id, name, arguments string) deepseek.ToolCall {
	return deepseek.ToolCall{
		ID:   id,
		Type: "function",
		Function: deepseek.FunctionCall{
			Name:      name,
			Arguments: arguments,
		},
	}
}

func newSeededAgent(t *testing.T, deps agent.Deps) agent.Service {
	t.Helper()
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	return agent.New(db, deps)
}

func firstToolResult(t *testing.T, messages []deepseek.Message) string {
	t.Helper()
	for _, message := range messages {
		if message.Role == "tool" {
			return message.Content
		}
	}
	t.Fatal("no tool message recorded")
	return ""
}

var privateTokens = []string{
	"basso-sinistra",
	"blu",
	"gatto",
	"crepa",
	"CA00000AA",
	"Mario Rossi",
	"41.9012",
}

func assertNoPrivateLeak(t *testing.T, label, value string) {
	t.Helper()
	for _, token := range privateTokens {
		if strings.Contains(value, token) {
			t.Fatalf("%s leaks private value %q: %s", label, token, value)
		}
	}
}

func TestMockAgentDoesNotLeakPrivateData(t *testing.T) {
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: ""})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	assertNoPrivateLeak(t, "message", reply.Message)
	if len(reply.Candidates) == 0 {
		t.Fatal("expected candidates")
	}
	if reply.Done {
		t.Fatal("a question turn must not be done")
	}

	encoded, err := json.Marshal(reply)
	if err != nil {
		t.Fatalf("marshalling reply: %v", err)
	}
	assertNoPrivateLeak(t, "reply", string(encoded))
	for _, candidate := range reply.Candidates {
		if candidate.ItemID == "" || candidate.FoundZone == "" {
			t.Fatalf("candidate is missing public fields: %+v", candidate)
		}
	}
}

func TestMockAgentCreatesClaimFromDetails(t *testing.T) {
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: ""})

	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Descrivi un dettaglio che solo il proprietario conosce."},
		{Role: "user", Content: "è nero, ha una crepa e un adesivo a forma di gatto"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if reply.ClaimID == "" {
		t.Fatalf("expected a claim, got %+v", reply)
	}
	if !reply.Done {
		t.Fatal("claim creation must mark the conversation done")
	}
	if len(reply.Candidates) == 0 {
		t.Fatal("expected candidates")
	}
	assertNoPrivateLeak(t, "message", reply.Message)
}

func TestToolLoopSearchesThenAnswers(t *testing.T) {
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone San Lorenzo"}`)}},
		{Content: "Ho trovato un candidato compatibile. Puoi descrivermi un segno particolare?"},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ho perso un iphone"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if len(reply.Candidates) == 0 {
		t.Fatal("expected candidates from search_items")
	}
	if !strings.Contains(reply.Message, "candidato") {
		t.Fatalf("final message not used: %q", reply.Message)
	}
	if reply.Done {
		t.Fatal("question turn must not be done")
	}
	assertNoPrivateLeak(t, "message", reply.Message)

	// The system prompt is always first and asks one question at a time.
	first := llm.call(0)
	if len(first) < 2 || first[0].Role != "system" {
		t.Fatalf("first call messages = %+v", first)
	}
	if !strings.Contains(strings.ToLower(first[0].Content), "una sola domanda") {
		t.Fatalf("system prompt missing the one-question rule: %q", first[0].Content)
	}

	// The tool result is sent back as a tool message and carries the item id.
	second := llm.call(1)
	toolResult := firstToolResult(t, second)
	if !strings.Contains(toolResult, "item-phone") {
		t.Fatalf("tool result = %s", toolResult)
	}
	assertNoPrivateLeak(t, "tool result", toolResult)

	// Every declared tool is offered to the model.
	names := map[string]bool{}
	for _, tool := range llm.tools[0] {
		names[tool.Function.Name] = true
	}
	for _, want := range []string{"search_items", "get_private_profile", "score_with_d3", "create_claim", "escalate"} {
		if !names[want] {
			t.Fatalf("tool %q not declared: %+v", want, names)
		}
	}
}

func TestPrivateProfileToolSanitizesDocuments(t *testing.T) {
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "get_private_profile", `{"item_id":"item-document"}`)}},
		{Content: "Che tipo di documento hai perso?"},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	if _, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ho perso un documento"}}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	result := firstToolResult(t, llm.call(1))
	if !strings.Contains(result, "identity_card") {
		t.Fatalf("document type missing from sanitized profile: %s", result)
	}
	if !strings.Contains(result, "identity_match") {
		t.Fatalf("identity booleans missing from sanitized profile: %s", result)
	}
	for _, secret := range []string{"CA00000AA", "Mario Rossi", `"holder":"`, `"number":"`} {
		if strings.Contains(result, secret) {
			t.Fatalf("sanitized profile leaks %q: %s", secret, result)
		}
	}
}

func TestScoreToolReturnsNoExpectedValues(t *testing.T) {
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "score_with_d3",
			`{"item_id":"item-phone","answers":{"color":"nero","has_crack":"sì"}}`)}},
		{Content: "Grazie, verifico le informazioni."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	if _, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "è nero e ha una crepa"}}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	result := firstToolResult(t, llm.call(1))
	var decoded map[string]any
	if err := json.Unmarshal([]byte(result), &decoded); err != nil {
		t.Fatalf("decoding score result %q: %v", result, err)
	}
	if len(decoded) != 3 {
		t.Fatalf("score result exposes extra fields: %s", result)
	}
	for _, key := range []string{"confidence", "decision", "reasons"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("score result missing %q: %s", key, result)
		}
	}
	for _, secret := range []string{"Apple", "iPhone", "nero", "blu", "gatto", "crepa"} {
		if strings.Contains(result, secret) {
			t.Fatalf("score result leaks expected value %q: %s", secret, result)
		}
	}
}

func TestCreateClaimToolUsesServerSearcher(t *testing.T) {
	claims := &recordingClaims{created: claim.Claim{
		ID:             "claim-test",
		ItemID:         "item-phone",
		Status:         claim.StatusUnderReview,
		ReturnChannel:  claim.ReturnChannelIntermediaryDirect,
		ConfidenceBand: "medium",
	}}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "create_claim",
			`{"item_id":"item-phone","answers":{"color":"nero"},"evidence":["ricevuta"]}`)}},
		{Content: "Ho inviato la richiesta di verifica."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "è nero, ho la ricevuta"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if claims.calls != 1 {
		t.Fatalf("claim service called %d times, want 1", claims.calls)
	}
	if claims.input.SearcherID != "searcher-demo" {
		t.Fatalf("searcher = %q, want the authenticated searcher", claims.input.SearcherID)
	}
	if claims.input.ItemID != "item-phone" {
		t.Fatalf("item = %q", claims.input.ItemID)
	}
	if claims.input.Session.Answers["color"] != "nero" {
		t.Fatalf("answers = %+v", claims.input.Session.Answers)
	}
	if len(claims.input.Evidence) != 1 || claims.input.Evidence[0] != "ricevuta" {
		t.Fatalf("evidence = %+v", claims.input.Evidence)
	}
	if reply.ClaimID != "claim-test" {
		t.Fatalf("reply claim id = %q", reply.ClaimID)
	}
	if !reply.Done {
		t.Fatal("claim creation must mark the conversation done")
	}
}

func TestEscalateStopsWithoutClaim(t *testing.T) {
	claims := &recordingClaims{}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "escalate", `{"reason":"caso ambiguo"}`)}},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "non so cosa fare"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if claims.calls != 0 {
		t.Fatal("escalate must not create a claim")
	}
	if !reply.Done || reply.ClaimID != "" {
		t.Fatalf("reply = %+v", reply)
	}
	if !strings.Contains(strings.ToLower(reply.Message), "operatore") {
		t.Fatalf("escalation message = %q", reply.Message)
	}
}

func TestClientFailureFallsBackToMock(t *testing.T) {
	llm := &scriptedLLM{err: errs.ErrUnavailable}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if len(reply.Candidates) == 0 {
		t.Fatal("fallback must search and return candidates")
	}
	assertNoPrivateLeak(t, "fallback message", reply.Message)
}

func TestToolLoopRoundCap(t *testing.T) {
	steps := make([]deepseek.Completion, 0, 10)
	for i := 0; i < 10; i++ {
		steps = append(steps, deepseek.Completion{
			ToolCalls: []deepseek.ToolCall{toolCall("call", "search_items", `{"query":"iphone"}`)},
		})
	}
	llm := &scriptedLLM{steps: steps}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, MaxToolRounds: 5})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "iphone"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if got := llm.callCount(); got != 5 {
		t.Fatalf("LLM calls = %d, want the 5 round cap", got)
	}
	if strings.TrimSpace(reply.Message) == "" {
		t.Fatal("round cap must still return a safe message")
	}
}

func TestFinalMessageRedactsPrivateDetails(t *testing.T) {
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{Content: "La custodia blu ha una crepa e un adesivo a forma di gatto."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "iphone"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	for _, secret := range []string{"crepa", "gatto", "adesivo"} {
		if strings.Contains(reply.Message, secret) {
			t.Fatalf("final message leaks %q: %q", secret, reply.Message)
		}
	}
}

func TestRejectsTooManyMessages(t *testing.T) {
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: ""})
	messages := make([]agent.Message, 0, 30)
	for i := 0; i < 30; i++ {
		messages = append(messages, agent.Message{Role: "user", Content: "ciao"})
	}
	if _, err := svc.Chat(context.Background(), "searcher-demo", messages); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}
