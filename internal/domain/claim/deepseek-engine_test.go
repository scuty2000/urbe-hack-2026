package claim_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
)

type fakeCompleter struct {
	completion deepseek.Completion
	err        error
	payload    string
}

func (f *fakeCompleter) Complete(_ context.Context, messages []deepseek.Message, _ []deepseek.Tool) (deepseek.Completion, error) {
	if len(messages) > 1 {
		f.payload, _ = messages[1].Content.(string)
	}
	return f.completion, f.err
}

func TestDeepSeekEngineUsesJudgeScores(t *testing.T) {
	judge := &fakeCompleter{completion: deepseek.Completion{ToolCalls: []deepseek.ToolCall{{
		Function: deepseek.FunctionCall{
			Name:      "submit_ownership_scores",
			Arguments: `{"scores":{"case_color":0.85,"contents":0.9},"rationale":"compatibile"}`,
		},
	}}}}
	questions := verification.Questions{
		"case_color": {Type: "noul"},
		"contents":   {Type: "choice"},
		"has_crack":  {Type: "noul"},
	}
	state := verification.State{
		Item:     map[string]any{"case_color": "nero", "contents": []any{"bancomat"}, "has_crack": false, "owner_name": "Mario Rossi"},
		Searcher: map[string]any{"case_color": "trasparente", "contents": "bancomat"},
	}

	result, err := claim.DeepSeekEngine(judge).Score(context.Background(), state, questions)
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}
	if result.ModelVersion != claim.ModelDeepSeek {
		t.Fatalf("model = %q, want deepseek", result.ModelVersion)
	}
	if got := verification.AnswerProbability(result.Answers["case_color"]); got != 0.85 {
		t.Fatalf("case_color = %v, want 0.85", got)
	}
	if got := verification.AnswerProbability(result.Answers["contents"]); got != 0.9 {
		t.Fatalf("contents = %v, want 0.9 (choice)", got)
	}
	if got := verification.AnswerProbability(result.Answers["has_crack"]); got != 0.5 {
		t.Fatalf("unscored key = %v, want neutral 0.5", got)
	}
	if strings.Contains(judge.payload, "Mario Rossi") {
		t.Fatalf("identity value sent to DeepSeek: %s", judge.payload)
	}
}

func TestDeepSeekEngineFallsBackToLocalScorer(t *testing.T) {
	judge := &fakeCompleter{err: errors.New("provider down")}
	questions := verification.Questions{"case_color": itemdomain.Question{Type: "noul"}}
	state := verification.State{
		Item:     map[string]any{"case_color": "blu"},
		Searcher: map[string]any{"case_color": "blu"},
	}

	result, err := claim.DeepSeekEngine(judge).Score(context.Background(), state, questions)
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}
	if result.ModelVersion != verification.ModelLocal || !strings.Contains(result.Rationale, "local fallback") {
		t.Fatalf("result = %+v, want the local fallback", result)
	}
	if got := verification.AnswerProbability(result.Answers["case_color"]); got < 0.9 {
		t.Fatalf("case_color = %v, want a local exact match", got)
	}
}
