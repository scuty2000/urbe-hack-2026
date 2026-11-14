package wizard_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/wizard"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/platform/deepseek"
)

func TestDeepSeekPlannerParsesToolCall(t *testing.T) {
	t.Parallel()

	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		var decoded map[string]any
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		encoded, _ := json.Marshal(decoded)
		requestBody = string(encoded)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"submit_wizard_analysis","arguments":"{\"photoPlan\":[{\"key\":\"back\",\"label\":\"Retro\",\"instruction\":\"Fotografa il retro\",\"required\":true}],\"prefill\":{\"category\":\"smartphone\",\"color\":\"nero\",\"condition\":\"good\"},\"privatePrefill\":\"Custodia blu.\",\"warnings\":[]}"}}]}}]}`))
	}))
	defer server.Close()

	planner := wizard.NewDeepSeekPlanner(deepseek.New(server.URL, "test-key", "test-model", time.Second))
	plan, err := planner.Plan(context.Background(), wizard.PlannerInput{Category: "smartphone", Color: "black", PhotoCount: 1})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.PhotoPlan) != 1 || plan.PhotoPlan[0].Key != "back" || !plan.PhotoPlan[0].Required {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.PrivatePrefill != "Custodia blu." {
		t.Fatalf("privatePrefill = %q", plan.PrivatePrefill)
	}
	if !strings.Contains(requestBody, "submit_wizard_analysis") {
		t.Fatalf("request does not declare the planner tool: %s", requestBody)
	}
	if strings.Contains(requestBody, "image/jpeg") || strings.Contains(requestBody, "base64") {
		t.Fatalf("request leaks image data: %s", requestBody)
	}
}

func TestDeepSeekPlannerAcceptsContentJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"photoPlan\":[],\"prefill\":{\"category\":\"wallet\"},\"privatePrefill\":\"\",\"warnings\":[\"Nota\"]}"}}]}`))
	}))
	defer server.Close()

	planner := wizard.NewDeepSeekPlanner(deepseek.New(server.URL, "key", "model", time.Second))
	plan, err := planner.Plan(context.Background(), wizard.PlannerInput{Category: "wallet"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Prefill.Category != "wallet" || len(plan.Warnings) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestDeepSeekPlannerRejectsBadJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"not json"}}]}`))
	}))
	defer server.Close()

	planner := wizard.NewDeepSeekPlanner(deepseek.New(server.URL, "key", "model", time.Second))
	if _, err := planner.Plan(context.Background(), wizard.PlannerInput{}); !errors.Is(err, errs.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestDeepSeekPlannerWithoutClientIsNil(t *testing.T) {
	t.Parallel()
	if planner := wizard.NewDeepSeekPlanner(nil); planner != nil {
		t.Fatalf("NewDeepSeekPlanner(nil) = %v, want nil", planner)
	}
}
