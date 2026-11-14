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
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/search"
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
	// errAt makes Complete fail on the 1-based call number errAt (0 = never).
	errAt int
	calls [][]deepseek.Message
	tools [][]deepseek.Tool
}

func (f *scriptedLLM) Complete(_ context.Context, messages []deepseek.Message, tools []deepseek.Tool) (deepseek.Completion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]deepseek.Message(nil), messages...))
	f.tools = append(f.tools, append([]deepseek.Tool(nil), tools...))
	if f.err != nil && (f.errAt == 0 || len(f.calls) == f.errAt) {
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

func phonePrivateAnswers() map[string]string {
	return map[string]string{
		"category":   "smartphone",
		"brand":      "Apple",
		"model":      "iPhone 17",
		"color":      "nero",
		"condition":  "good",
		"case_color": "blu",
		"has_crack":  "sì",
		"sticker":    "gatto",
	}
}

const phonePrivateText = "è uno smartphone apple iphone 17 nero good, custodia blu, sì crepa, adesivo gatto"

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
			content, _ := message.Content.(string)
			return content
		}
	}
	t.Fatal("no tool message recorded")
	return ""
}

// toolResultFor returns the last tool result for the named tool.
func toolResultFor(t *testing.T, messages []deepseek.Message, name string) string {
	t.Helper()
	result := ""
	found := false
	for _, message := range messages {
		if message.Role == "tool" && message.Name == name {
			result, _ = message.Content.(string)
			found = true
		}
	}
	if !found {
		t.Fatalf("no tool message for %q recorded", name)
	}
	return result
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
		{Role: "user", Content: "è nero, ha una custodia blu, una crepa e un adesivo a forma di gatto"},
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
	prompt, _ := first[0].Content.(string)
	if !strings.Contains(strings.ToLower(prompt), "una sola domanda") {
		t.Fatalf("system prompt missing the one-question rule: %q", prompt)
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
	for _, want := range []string{"search_items", "get_private_profile", "score_answers", "create_claim", "escalate"} {
		if !names[want] {
			t.Fatalf("tool %q not declared: %+v", want, names)
		}
	}
}

func TestPrivateProfileToolSanitizesDocuments(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	// item-document is not searchable; put the document fields on the searchable
	// phone to exercise the document view through the scoped tool.
	if err := db.GormDB().Exec(
		`UPDATE items SET private_profile = ? WHERE id = ?`,
		`{"document_type":"identity_card","number":"CA00000AA","holder":"Mario Rossi","case_color":"blu"}`,
		"item-phone",
	).Error; err != nil {
		t.Fatalf("updating profile: %v", err)
	}

	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "get_private_profile", `{"item_id":"item-phone"}`)}},
		{Content: "Che tipo di documento hai perso?"},
	}}
	svc := agent.New(db, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	if _, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ho perso un iphone"}}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	result := toolResultFor(t, llm.call(2), "get_private_profile")
	if !strings.Contains(result, "identity_card") {
		t.Fatalf("document type missing from sanitized profile: %s", result)
	}
	if !strings.Contains(result, "identity_match") {
		t.Fatalf("identity booleans missing from sanitized profile: %s", result)
	}
	if !strings.Contains(result, "case_color") || !strings.Contains(result, "blu") {
		t.Fatalf("allowlisted private attribute missing: %s", result)
	}
	for _, secret := range []string{"CA00000AA", "Mario Rossi", `"holder":"`, `"number":"`} {
		if strings.Contains(result, secret) {
			t.Fatalf("sanitized profile leaks %q: %s", secret, result)
		}
	}
}

func TestPrivateProfileToolAllowlistDropsUnknownPII(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed.Demo(t, db)
	profile := `{"owner":"Mario Rossi","recapito":"3331234567","serial":"ABC123","imei":"123456789012345",` +
		`"case_color":"blu","handle":"legno chiaro","sticker":"Mario Rossi","contents":["bancomat"]}`
	if err := db.GormDB().Exec(
		`UPDATE items SET private_profile = ? WHERE id = ?`, profile, "item-phone",
	).Error; err != nil {
		t.Fatalf("updating profile: %v", err)
	}

	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "get_private_profile", `{"item_id":"item-phone"}`)}},
		{Content: "Mario Rossi ha una custodia blu."},
	}}
	svc := agent.New(db, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	if _, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ho perso un iphone"}}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	result := toolResultFor(t, llm.call(2), "get_private_profile")
	if !strings.Contains(result, `"case_color":"blu"`) {
		t.Fatalf("allowlisted attribute missing: %s", result)
	}
	if !strings.Contains(result, `"handle":"legno chiaro"`) {
		t.Fatalf("allowlisted attribute missing: %s", result)
	}
	if !strings.Contains(result, `"contents":["bancomat"]`) {
		t.Fatalf("sanitized list attribute missing: %s", result)
	}
	// sticker carries a full name: the value shape must reject it even though
	// the key is allowlisted.
	if strings.Contains(result, `"sticker"`) {
		t.Fatalf("unsafe value forwarded for allowlisted key: %s", result)
	}
	for _, secret := range []string{"Mario Rossi", "3331234567", "ABC123", "123456789012345", "owner", "recapito", "serial", "imei"} {
		if strings.Contains(result, secret) {
			t.Fatalf("sanitized profile leaks %q: %s", secret, result)
		}
	}
}

func TestToolsRejectItemsNotDiscoveredBySearch(t *testing.T) {
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "get_private_profile", `{"item_id":"item-document"}`)}},
		{Content: "Non ho informazioni su questo oggetto."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ho perso un documento"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	result := firstToolResult(t, llm.call(1))
	if !strings.Contains(result, "not available") {
		t.Fatalf("unscoped profile access was not rejected: %s", result)
	}
	if strings.Contains(result, "identity_card") || strings.Contains(result, "Mario Rossi") {
		t.Fatalf("unscoped profile leaked document data: %s", result)
	}
	if reply.ClaimID != "" || len(reply.Candidates) != 0 {
		t.Fatalf("unscoped turn returned item data: %+v", reply)
	}
}

func TestClaimAnswersMustComeFromUserText(t *testing.T) {
	claims := &recordingClaims{created: claim.Claim{ID: "claim-laundered", Status: claim.StatusApproved}}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "create_claim",
			`{"item_id":"item-phone","answers":{"case_color":"blu"}}`)}},
		{Content: "Non riesco a verificare, prova a descrivere l'oggetto."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ho perso un iphone"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if claims.calls != 0 {
		t.Fatalf("claim service called with model-invented answers %d times", claims.calls)
	}
	if reply.ClaimID != "" {
		t.Fatalf("reply carries a claim from invented answers: %q", reply.ClaimID)
	}
	result := toolResultFor(t, llm.call(2), "create_claim")
	if !strings.Contains(result, "error") {
		t.Fatalf("invented answers were not rejected: %s", result)
	}
}

type fakeSearch struct {
	result search.SearchResult
}

func (f fakeSearch) Search(context.Context, search.Query) ([]search.SearchResult, error) {
	return []search.SearchResult{f.result}, nil
}
func (fakeSearch) IndexItem(context.Context, itemdomain.Item) error { return nil }
func (fakeSearch) DeleteItem(context.Context, string) error         { return nil }

type fakeItemReader struct {
	item itemdomain.Item
}

func (f fakeItemReader) ItemForAgent(context.Context, string) (itemdomain.Item, error) {
	return f.item, nil
}

func TestPreflightSingleWrongPrivateDetailAsksAgain(t *testing.T) {
	claims := &recordingClaims{}
	llm := &scriptedLLM{}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Descrivi un dettaglio."},
		{Role: "user", Content: "ha un cinturino giallo"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	// People misremember: one wrong detail asks for another one instead of
	// closing the conversation.
	if reply.Done || !strings.Contains(strings.ToLower(reply.Message), "non basta ancora") {
		t.Fatalf("reply = %+v, want another-detail question", reply)
	}
	if claims.calls != 0 || reply.ClaimID != "" {
		t.Fatalf("claim forwarded: calls=%d id=%q", claims.calls, reply.ClaimID)
	}
	if len(llm.calls) != 0 {
		t.Fatalf("LLM was called %d times; the deterministic gate should ask", len(llm.calls))
	}
}

func TestPreflightSeveralWrongPrivateDetailsCloseBeforeLLM(t *testing.T) {
	claims := &recordingClaims{}
	llm := &scriptedLLM{}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Descrivi un dettaglio."},
		{Role: "user", Content: "ha una custodia gialla ed è senza crepe"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !reply.Done || !strings.Contains(strings.ToLower(reply.Message), "non posso verificare") {
		t.Fatalf("reply = %+v, want a terminal rejection", reply)
	}
	if claims.calls != 0 || reply.ClaimID != "" {
		t.Fatalf("claim forwarded: calls=%d id=%q", claims.calls, reply.ClaimID)
	}
	if len(llm.calls) != 0 {
		t.Fatalf("LLM was called %d times after a clear contradiction", len(llm.calls))
	}
}

func TestPreflightMixedPrivateDetailsGoToHumanReview(t *testing.T) {
	llm := &scriptedLLM{}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Descrivi un dettaglio."},
		{Role: "user", Content: "ha una custodia blu ed è senza crepe"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	// One right and one wrong detail: low but not null confidence. The claim
	// goes to the custodian, who verifies in person without a pickup pass.
	if !reply.Done || reply.ClaimID == "" || !strings.Contains(reply.Message, "di persona") {
		t.Fatalf("reply = %+v, want in-person verification", reply)
	}
}

func TestPreflightApproximateColourMatches(t *testing.T) {
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: ""})

	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Descrivi un dettaglio."},
		{Role: "user", Content: "ha una cover azzurra"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !reply.Done || reply.ClaimID == "" {
		t.Fatalf("reply = %+v, want a created claim for a colour synonym", reply)
	}
}

func TestPreflightCorrectPrivateDetailCreatesClaimBeforeLLM(t *testing.T) {
	llm := &scriptedLLM{}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Descrivi un dettaglio."},
		{Role: "user", Content: "ha una custodia blu"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !reply.Done || reply.ClaimID == "" {
		t.Fatalf("reply = %+v, want a created claim", reply)
	}
	if len(llm.calls) != 0 {
		t.Fatalf("LLM was called %d times; the deterministic gate should decide", len(llm.calls))
	}
}

func TestPreflightNoPrivateAttributesGoesToHumanReview(t *testing.T) {
	claims := &recordingClaims{created: claim.Claim{ID: "claim-watch", Status: claim.StatusUnderReview}}
	llm := &scriptedLLM{}
	db := testutil.NewTestDB(t)
	svc := agent.New(db, agent.Deps{
		DeepSeekKey: "test-key",
		Client:      llm,
		Claim:       claims,
		Search:      fakeSearch{result: search.SearchResult{ItemID: "watch-x", Category: "orologio", FoundZone: "Testaccio"}},
		Items: fakeItemReader{item: itemdomain.Item{
			ID: "watch-x", Category: "orologio", Status: itemdomain.StatusInCustody,
			RiskTier:       itemdomain.RiskHigh,
			PrivateProfile: map[string]any{"details": "orologio con cinturino nero"},
		}},
	})
	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un orologio in zona Testaccio"},
		{Role: "assistant", Content: "Descrivi un dettaglio."},
		{Role: "user", Content: "ha un cinturino giallo"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	// The item has nothing to verify against: that is not the searcher's
	// fault, so the request reaches a human instead of being refused.
	if !reply.Done || reply.ClaimID != "claim-watch" || !strings.Contains(reply.Message, "in verifica") {
		t.Fatalf("reply = %+v, want a claim under review", reply)
	}
	if claims.calls != 1 || len(llm.calls) != 0 {
		t.Fatalf("claims=%d llm=%d, want one deterministic claim", claims.calls, len(llm.calls))
	}
	if len(claims.input.Flags) != 1 || claims.input.Flags[0] != "missing_evidence" {
		t.Fatalf("flags = %v, want missing_evidence to force human review", claims.input.Flags)
	}
}

func TestPreflightUndecidedAsksWithoutEscalating(t *testing.T) {
	claims := &recordingClaims{}
	llm := &scriptedLLM{}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Com'è fatta la custodia?"},
		{Role: "user", Content: "ha una custodia"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if reply.Done || reply.ClaimID != "" {
		t.Fatalf("undecided ownership was forwarded: %+v", reply)
	}
	if claims.calls != 0 || len(llm.calls) != 0 {
		t.Fatalf("claims=%d llm=%d, want a deterministic question", claims.calls, len(llm.calls))
	}
	if !strings.Contains(strings.ToLower(reply.Message), "non basta ancora") {
		t.Fatalf("message = %q, want another-detail question", reply.Message)
	}
}

func TestScoreSingleWrongPrivateDetailKeepsAsking(t *testing.T) {
	claims := &recordingClaims{}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "score_answers",
			`{"item_id":"item-phone","answers":{"case_color":"giallo"}}`)}},
		{Content: "Mi descriva un altro dettaglio."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ha un cinturino giallo"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if reply.Done || reply.Message != "Mi descriva un altro dettaglio." {
		t.Fatalf("reply = %+v, want the conversation to continue", reply)
	}
	if claims.calls != 0 || reply.ClaimID != "" {
		t.Fatalf("claim was forwarded: calls=%d id=%q", claims.calls, reply.ClaimID)
	}
	if result := toolResultFor(t, llm.call(2), "score_answers"); !strings.Contains(result, `"more_info"`) {
		t.Fatalf("score result = %s, want more_info", result)
	}
}

func TestScoreSeveralWrongPrivateDetailsCloseWithoutClaim(t *testing.T) {
	claims := &recordingClaims{}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "score_answers",
			`{"item_id":"item-phone","answers":{"case_color":"giallo","sticker":"cane"}}`)}},
		{Content: "Continuiamo a provare."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "ha un cinturino giallo e un adesivo di un cane"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !reply.Done || !strings.Contains(strings.ToLower(reply.Message), "non posso verificare") {
		t.Fatalf("reply = %+v, want a terminal ownership rejection", reply)
	}
	if claims.calls != 0 || reply.ClaimID != "" {
		t.Fatalf("claim was forwarded: calls=%d id=%q", claims.calls, reply.ClaimID)
	}
}

func TestScoreNoPrivateAttributesGoesToHumanReview(t *testing.T) {
	claims := &recordingClaims{created: claim.Claim{ID: "claim-watch", Status: claim.StatusUnderReview}}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"orologio"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "score_answers",
			`{"item_id":"watch-x","answers":{"color":"nero"}}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-3", "create_claim",
			`{"item_id":"watch-x","answers":{"color":"nero"}}`)}},
		{Content: "La richiesta sarà verificata dal custode."},
	}}
	db := testutil.NewTestDB(t)
	svc := agent.New(db, agent.Deps{
		DeepSeekKey: "test-key",
		Client:      llm,
		Claim:       claims,
		Search:      fakeSearch{result: search.SearchResult{ItemID: "watch-x", Category: "orologio", FoundZone: "Testaccio"}},
		Items: fakeItemReader{item: itemdomain.Item{
			ID: "watch-x", Category: "orologio", Status: itemdomain.StatusInCustody,
			RiskTier:       itemdomain.RiskHigh,
			PrivateProfile: map[string]any{"details": "orologio Apple Watch con cinturino nero"},
		}},
	})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "è nero"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !reply.Done || reply.ClaimID != "claim-watch" {
		t.Fatalf("reply = %+v, want a claim under human review", reply)
	}
	if result := toolResultFor(t, llm.call(2), "score_answers"); !strings.Contains(result, `"review"`) {
		t.Fatalf("score result = %s, want review", result)
	}
	if claims.calls != 1 || len(claims.input.Flags) != 1 || claims.input.Flags[0] != "missing_evidence" {
		t.Fatalf("claims=%d flags=%v, want one claim flagged missing_evidence", claims.calls, claims.input.Flags)
	}
}

func TestMockAgentAsksAgainOnFeminineWrongPrivateDetail(t *testing.T) {
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: ""})

	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Com'è fatta la custodia?"},
		{Role: "user", Content: "ha una custodia gialla"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if reply.Done || !strings.Contains(strings.ToLower(reply.Message), "non basta ancora") {
		t.Fatalf("reply = %+v, want another-detail question", reply)
	}
	if reply.ClaimID != "" {
		t.Fatalf("wrong private detail created a claim: %q", reply.ClaimID)
	}
}

func TestEscalateIsBlockedWhileOwnershipIsUndecided(t *testing.T) {
	claims := &recordingClaims{}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "score_answers",
			`{"item_id":"item-phone","answers":{"color":"nero"}}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-3", "escalate", `{}`)}},
		{Content: "Mi descriva un altro dettaglio della custodia."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "è nero"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if reply.Done || reply.ClaimID != "" || claims.calls != 0 {
		t.Fatalf("undecided ownership was forwarded: reply=%+v claims=%d", reply, claims.calls)
	}
	result := toolResultFor(t, llm.call(3), "escalate")
	if !strings.Contains(result, "undecided") {
		t.Fatalf("escalate was not blocked: %s", result)
	}
}

func TestCreateClaimToolRefusesWeakEvidence(t *testing.T) {
	claims := &recordingClaims{created: claim.Claim{ID: "should-not-exist", Status: claim.StatusMoreInfo}}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "create_claim",
			`{"item_id":"item-phone","answers":{"color":"nero"}}`)}},
		{Content: "Servono altri dettagli."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "è nero"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if claims.calls != 0 {
		t.Fatalf("CreateClaim calls = %d, want 0 on weak evidence", claims.calls)
	}
	if reply.ClaimID != "" {
		t.Fatalf("reply carries a weak claim: %q", reply.ClaimID)
	}
}

func TestProviderFailureAfterClaimReturnsPartialReply(t *testing.T) {
	claims := &recordingClaims{created: claim.Claim{ID: "claim-partial", Status: claim.StatusUnderReview}}
	llm := &scriptedLLM{
		err:   errs.ErrUnavailable,
		errAt: 2,
		steps: []deepseek.Completion{
			{ToolCalls: []deepseek.ToolCall{
				toolCall("call-1", "search_items", `{"query":"iphone"}`),
				toolCall("call-2", "create_claim", `{"item_id":"item-phone","answers":{"category":"smartphone","brand":"Apple","model":"iPhone 17","color":"nero","condition":"good","case_color":"blu","has_crack":"sì","sticker":"gatto"}}`),
			}},
		},
	}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: phonePrivateText}})
	if err != nil {
		t.Fatalf("Chat() restarted the mock after a side effect: %v", err)
	}
	if claims.calls != 1 {
		t.Fatalf("CreateClaim calls = %d, want 1", claims.calls)
	}
	if reply.ClaimID != "claim-partial" || !reply.Done {
		t.Fatalf("partial reply lost the claim: %+v", reply)
	}
	if strings.TrimSpace(reply.Message) == "" {
		t.Fatal("partial reply has no message")
	}
}

func TestMultipleCreateClaimCallsCreateOneClaim(t *testing.T) {
	claims := &recordingClaims{created: claim.Claim{ID: "claim-once", Status: claim.StatusUnderReview}}
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{
			toolCall("call-1", "search_items", `{"query":"iphone"}`),
			toolCall("call-2", "create_claim", `{"item_id":"item-phone","answers":{"category":"smartphone","brand":"Apple","model":"iPhone 17","color":"nero","condition":"good","case_color":"blu","has_crack":"sì","sticker":"gatto"}}`),
			toolCall("call-3", "create_claim", `{"item_id":"item-phone","answers":{"category":"smartphone","brand":"Apple","model":"iPhone 17","color":"nero","condition":"good","case_color":"blu","has_crack":"sì","sticker":"gatto"}}`),
		}},
		{Content: "Richiesta registrata."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: phonePrivateText}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if claims.calls != 1 {
		t.Fatalf("CreateClaim calls = %d, want 1", claims.calls)
	}
	if reply.ClaimID != "claim-once" {
		t.Fatalf("reply claim id = %q", reply.ClaimID)
	}
}

func TestScoreToolReturnsNoExpectedValues(t *testing.T) {
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "score_answers",
			`{"item_id":"item-phone","answers":{"color":"nero","has_crack":"sì"}}`)}},
		{Content: "Grazie, verifico le informazioni."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	if _, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "è nero e ha una crepa"}}); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	result := toolResultFor(t, llm.call(2), "score_answers")
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
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "create_claim",
			`{"item_id":"item-phone","answers":{"category":"smartphone","brand":"Apple","model":"iPhone 17","color":"nero","condition":"good","case_color":"blu","has_crack":"sì","sticker":"gatto"},"evidence":["ricevuta"]}`)}},
		{Content: "Ho inviato la richiesta di verifica."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: phonePrivateText + ", ho la ricevuta"}})
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
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "get_private_profile", `{"item_id":"item-phone"}`)}},
		{Content: "La custodia blu ha una crepa e un adesivo a forma di gatto."},
	}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "iphone"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	// The sanitized profile exposes case_color and sticker, so echoing those
	// values must be redacted. "crepa"/"adesivo" are not actual values (the
	// profile only says has_crack: true) and stay as ordinary prose.
	for _, secret := range []string{"blu", "gatto"} {
		if strings.Contains(reply.Message, secret) {
			t.Fatalf("final message leaks %q: %q", secret, reply.Message)
		}
	}
	for _, ordinary := range []string{"crepa", "adesivo"} {
		if !strings.Contains(reply.Message, ordinary) {
			t.Fatalf("over-redaction removed %q: %q", ordinary, reply.Message)
		}
	}
}

func TestRejectsTooManyMessages(t *testing.T) {
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: ""})
	messages := make([]agent.Message, 0, 61)
	for i := 0; i < 61; i++ {
		messages = append(messages, agent.Message{Role: "user", Content: "ciao"})
	}
	if _, err := svc.Chat(context.Background(), "searcher-demo", messages); !errors.Is(err, errs.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

// Regression: a claim approved on the last tool round used to end with the
// generic "forwarded to an operator" fallback, contradicting the claim.
func TestClaimOnLastToolRoundRepliesWithClaimOutcome(t *testing.T) {
	llm := &scriptedLLM{steps: []deepseek.Completion{
		{ToolCalls: []deepseek.ToolCall{toolCall("call-1", "search_items", `{"query":"iphone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-2", "search_items", `{"query":"iphone nero"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-3", "get_private_profile", `{"item_id":"item-phone"}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-4", "score_answers",
			`{"item_id":"item-phone","answers":{"case_color":"blu"}}`)}},
		{ToolCalls: []deepseek.ToolCall{toolCall("call-5", "create_claim",
			`{"item_id":"item-phone","answers":{"case_color":"blu"}}`)}},
	}}
	claims := &recordingClaims{created: claim.Claim{ID: "claim-1", Status: claim.StatusApproved}}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims, MaxToolRounds: 5})

	reply, err := svc.Chat(context.Background(), "searcher-demo",
		[]agent.Message{{Role: "user", Content: "iphone con custodia blu"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !reply.Done || reply.ClaimID != "claim-1" || !strings.Contains(reply.Message, "approvata") {
		t.Fatalf("reply = %+v, want the approved-claim message", reply)
	}
	if strings.Contains(reply.Message, "operatore") {
		t.Fatalf("reply contradicts the approved claim: %q", reply.Message)
	}
}

// An honest searcher whose details stay undecided (e.g. the catalog recorded a
// transparent case as black) must reach a human instead of looping forever.
func TestPreflightUndecidedTooLongGoesToHumanReview(t *testing.T) {
	claims := &recordingClaims{created: claim.Claim{ID: "claim-review", Status: claim.StatusUnderReview}}
	llm := &scriptedLLM{}
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: "test-key", Client: llm, Claim: claims})

	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Descrivi un dettaglio."},
		{Role: "user", Content: "ha un cinturino giallo"},
		{Role: "assistant", Content: "Non basta ancora."},
		{Role: "user", Content: "era nero"},
		{Role: "assistant", Content: "Non basta ancora."},
		{Role: "user", Content: "non ricordo altro"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !reply.Done || reply.ClaimID != "claim-review" || !strings.Contains(reply.Message, "in verifica") {
		t.Fatalf("reply = %+v, want a claim under human review", reply)
	}
	found := false
	for _, flag := range claims.input.Flags {
		if flag == "missing_evidence" {
			found = true
		}
	}
	if !found || len(llm.calls) != 0 {
		t.Fatalf("flags=%v llm=%d, want missing_evidence and no LLM call", claims.input.Flags, len(llm.calls))
	}
}

func TestMockAnswersRecognisesTransparentCase(t *testing.T) {
	svc := newSeededAgent(t, agent.Deps{DeepSeekKey: ""})
	reply, err := svc.Chat(context.Background(), "searcher-demo", []agent.Message{
		{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
		{Role: "assistant", Content: "Descrivi un dettaglio."},
		{Role: "user", Content: "cover trasparente con ring bianco"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	// Seeded phone: case_color blu, no has_case. One wrong colour alone asks
	// again; it must not be read as "bianco".
	if reply.Done {
		t.Fatalf("reply = %+v, want another-detail question", reply)
	}
}

func TestInPersonVerificationOnlyWithPartialEvidence(t *testing.T) {
	cases := []struct {
		name      string
		answer    string
		wantPlace bool
	}{
		// case_color right, has_crack wrong: low confidence, some evidence.
		{"partial evidence reveals the pickup point", "ha una custodia blu ed è senza crepe", true},
		// Only wrong answers after many turns: total uncertainty.
		{"total uncertainty keeps it private", "ha un cinturino giallo", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newSeededAgent(t, agent.Deps{DeepSeekKey: ""})
			messages := []agent.Message{
				{Role: "user", Content: "ho perso un iphone in zona San Lorenzo"},
				{Role: "assistant", Content: "Descrivi un dettaglio."},
				{Role: "user", Content: tc.answer},
			}
			if !tc.wantPlace {
				messages = append(messages,
					agent.Message{Role: "assistant", Content: "Non basta ancora."},
					agent.Message{Role: "user", Content: "non ricordo"},
					agent.Message{Role: "assistant", Content: "Non basta ancora."},
					agent.Message{Role: "user", Content: "davvero non ricordo"},
				)
			}
			reply, err := svc.Chat(context.Background(), "searcher-demo", messages)
			if err != nil {
				t.Fatalf("Chat() error = %v", err)
			}
			if !reply.Done || reply.ClaimID == "" {
				t.Fatalf("reply = %+v, want a claim under review", reply)
			}
			revealed := strings.Contains(reply.Message, "Bar X") || strings.Contains(reply.Message, "Via dei Volsci")
			if revealed != tc.wantPlace {
				t.Fatalf("pickup point revealed = %v, want %v: %q", revealed, tc.wantPlace, reply.Message)
			}
			if tc.wantPlace && !strings.Contains(reply.Message, "non serve il pass") {
				t.Fatalf("message = %q, want the no-pass in-person explanation", reply.Message)
			}
		})
	}
}
