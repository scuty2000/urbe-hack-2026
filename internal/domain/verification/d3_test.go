package verification_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	itemdomain "github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/verification"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/d3"
)

type d3Request struct {
	Model     string                         `json:"model"`
	State     string                         `json:"state"`
	Questions map[string]itemdomain.Question `json:"questions"`
}

func TestD3ScoreUsesRemoteAnswersAndRedactsIdentity(t *testing.T) {
	t.Parallel()

	var got d3Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decoding d3 request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answers":{"color":{"type":"noul","noul":0.91,"confidence":0.91}}}`))
	}))
	defer server.Close()

	client := d3.New(server.URL, "test-key", "d3-test", time.Second)
	state := verification.State{
		Item: map[string]any{
			"color": "nero",
			"name":  "Giulia Ferrari",
			"dob":   "1990-01-01",
		},
		Searcher: map[string]any{
			"color": "nero",
			"name":  "Marco Bianchi",
		},
		RiskTier: itemdomain.RiskLow,
	}
	questions := verification.Questions{"color": {Type: "noul"}}

	result, err := verification.D3Score(context.Background(), client, state, questions, nil)
	if err != nil {
		t.Fatalf("D3Score() error = %v", err)
	}
	if got := result.Answers["color"].Noul; got != 0.91 {
		t.Fatalf("noul = %v, want the remote answer 0.91", got)
	}
	if result.ModelVersion != verification.ModelD3 {
		t.Fatalf("modelVersion = %q, want %q", result.ModelVersion, verification.ModelD3)
	}

	// The request state must never contain the raw identity values.
	for _, secret := range []string{"Giulia Ferrari", "Marco Bianchi", "1990-01-01"} {
		if strings.Contains(got.State, secret) {
			t.Fatalf("raw identity value %q leaked to d3: %s", secret, got.State)
		}
	}
	if !strings.Contains(got.State, `"name":true`) {
		t.Fatalf("identity field should be represented as a boolean, got state %s", got.State)
	}
}

func TestD3ScoreFallsBackWhenUnavailable(t *testing.T) {
	t.Parallel()

	// An empty base URL makes the platform client return ErrUnavailable.
	client := d3.New("", "", "", time.Second)
	state := verification.State{
		Item:     map[string]any{"color": "nero"},
		Searcher: map[string]any{"color": "nero"},
	}
	result, err := verification.D3Score(
		context.Background(), client, state,
		verification.Questions{"color": {Type: "noul"}}, nil,
	)
	if err != nil {
		t.Fatalf("D3Score() error = %v, want a local fallback", err)
	}
	if result.ModelVersion != verification.ModelLocal {
		t.Fatalf("modelVersion = %q, want %q", result.ModelVersion, verification.ModelLocal)
	}
	if got := result.Answers["color"].Noul; got < 0.8 {
		t.Fatalf("fallback noul = %v, want the local match probability", got)
	}
	if !strings.Contains(result.Rationale, "fallback") {
		t.Fatalf("rationale = %q, want it to mention the fallback", result.Rationale)
	}
}

func TestD3ScoreFallsBackOnServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	client := d3.New(server.URL, "", "", time.Second)
	state := verification.State{
		Item:     map[string]any{"color": "nero"},
		Searcher: map[string]any{"color": "rosso"},
	}
	result, err := verification.D3Score(
		context.Background(), client, state,
		verification.Questions{"color": {Type: "noul"}}, nil,
	)
	if err != nil {
		t.Fatalf("D3Score() error = %v, want a local fallback", err)
	}
	if result.ModelVersion != verification.ModelLocal {
		t.Fatalf("modelVersion = %q, want %q", result.ModelVersion, verification.ModelLocal)
	}
	if got := result.Answers["color"].Noul; got >= 0.5 {
		t.Fatalf("fallback mismatch noul = %v, want a local mismatch", got)
	}
}
