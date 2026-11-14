package d3_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/item"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/d3"
)

type askRequest struct {
	Model     string                   `json:"model"`
	State     string                   `json:"state"`
	Images    []string                 `json:"images"`
	Questions map[string]item.Question `json:"questions"`
}

func TestClientAskSendsRequestAndParsesAnswers(t *testing.T) {
	t.Parallel()

	var got askRequest
	var gotAuth, gotMethod, gotPath, gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answers":{
			"category":{"type":"choice","choice":"smartphone","probabilities":{"smartphone":0.9},"confidence":0.9},
			"has_crack":{"type":"noul","noul":0.8,"confidence":0.7},
			"condition":{"type":"choice","probabilities":{"good":0.6,"worn":0.4}}
		}}`))
	}))
	defer server.Close()

	client := d3.New(server.URL, "test-key", "d3", time.Second)

	questions := map[string]item.Question{
		"category":  {Type: "choice", Criteria: map[string]string{"smartphone": "a phone"}},
		"has_crack": {Type: "noul"},
	}
	answers, err := client.Ask(context.Background(), "classify this item",
		[]string{"data:image/jpeg;base64,abc"}, questions)
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v1/systemone" {
		t.Fatalf("path = %q, want /v1/systemone", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("authorization = %q, want Bearer test-key", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", gotContentType)
	}
	if got.Model != "d3" || got.State != "classify this item" {
		t.Fatalf("model/state = %q/%q", got.Model, got.State)
	}
	if len(got.Images) != 1 || got.Images[0] != "data:image/jpeg;base64,abc" {
		t.Fatalf("images = %+v", got.Images)
	}
	if got.Questions["category"].Type != "choice" || got.Questions["has_crack"].Type != "noul" {
		t.Fatalf("questions = %+v", got.Questions)
	}

	if answers["category"].Choice != "smartphone" {
		t.Fatalf("category = %+v", answers["category"])
	}
	if answers["has_crack"].Noul != 0.8 {
		t.Fatalf("has_crack = %+v", answers["has_crack"])
	}
	// A choice without an explicit winner exposes its probabilities so the
	// catalog can pick the argmax.
	if answers["condition"].Probabilities["good"] != 0.6 {
		t.Fatalf("condition = %+v", answers["condition"])
	}
}

func TestClientAskOmitsAuthorizationWithoutKey(t *testing.T) {
	t.Parallel()

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	defer server.Close()

	client := d3.New(server.URL, "", "d3", time.Second)
	if _, err := client.Ask(context.Background(), "x", nil, nil); err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("authorization = %q, want empty", gotAuth)
	}
}

func TestClientAskUnavailable(t *testing.T) {
	t.Parallel()

	t.Run("empty url", func(t *testing.T) {
		t.Parallel()
		client := d3.New("", "k", "d3", time.Second)
		_, err := client.Ask(context.Background(), "x", nil, nil)
		if !errors.Is(err, errs.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer server.Close()

		client := d3.New(server.URL, "", "d3", time.Second)
		_, err := client.Ask(context.Background(), "x", nil, nil)
		if !errors.Is(err, errs.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not json"))
		}))
		defer server.Close()

		client := d3.New(server.URL, "", "d3", time.Second)
		_, err := client.Ask(context.Background(), "x", nil, nil)
		if !errors.Is(err, errs.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(150 * time.Millisecond)
			_, _ = w.Write([]byte(`{"answers":{}}`))
		}))
		defer server.Close()

		client := d3.New(server.URL, "", "d3", 20*time.Millisecond)
		_, err := client.Ask(context.Background(), "x", nil, nil)
		if !errors.Is(err, errs.ErrUnavailable) {
			t.Fatalf("err = %v, want ErrUnavailable", err)
		}
	})
}
