package claim_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/claim"
	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
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

func TestDeepSeekEngineReturnsUnavailable(t *testing.T) {
	questions := verification.Questions{"case_color": itemdomain.Question{Type: "noul"}}
	state := verification.State{
		Item:     map[string]any{"case_color": "blu"},
		Searcher: map[string]any{"case_color": "blu"},
	}
	for name, engine := range map[string]claim.Engine{
		"provider down":  claim.DeepSeekEngine(&fakeCompleter{err: errors.New("provider down")}),
		"not configured": claim.DeepSeekEngine(nil),
		"garbage output": claim.DeepSeekEngine(&fakeCompleter{completion: deepseek.Completion{Content: "boh"}}),
	} {
		// No local fallback: no ownership decision without DeepSeek.
		if _, err := engine.Score(context.Background(), state, questions); !errors.Is(err, errs.ErrUnavailable) {
			t.Errorf("%s: Score() error = %v, want ErrUnavailable", name, err)
		}
	}
}

func TestDeepSeekEngineGivesTheJudgeFullContextAndReadsOverall(t *testing.T) {
	judge := &fakeCompleter{completion: deepseek.Completion{ToolCalls: []deepseek.ToolCall{{
		Function: deepseek.FunctionCall{
			Name:      "submit_ownership_scores",
			Arguments: `{"scores":{"model":0.9},"ownership":0.82,"rationale":"compatibile"}`,
		},
	}}}}
	state := verification.State{
		Item:     map[string]any{"model": "Zero / senza zuccheri"},
		Searcher: map[string]any{"model": "zero zuccheri"},
		Context: verification.JudgeContext{
			FoundZone:         "Testaccio",
			FoundAt:           "2026-11-14 11:51",
			ItemDescription:   "Lattina in alluminio argento con scritta ZERO ZUCCHERI",
			SearcherStatement: "oggi a Testaccio ho perso una lattina di redbull zero zuccheri",
		},
	}

	result, err := claim.DeepSeekEngine(judge).Score(context.Background(), state,
		verification.Questions{"model": itemdomain.Question{Type: "noul"}})
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}
	for _, want := range []string{`"foundZone":"Testaccio"`, `"foundAt":"2026-11-14 11:51"`, "ZERO ZUCCHERI", "ho perso una lattina di redbull"} {
		if !strings.Contains(judge.payload, want) {
			t.Fatalf("judge payload misses %s: %s", want, judge.payload)
		}
	}
	if !result.HasOverall || result.Overall != 0.82 {
		t.Fatalf("overall = %v (has=%v), want 0.82", result.Overall, result.HasOverall)
	}
}
