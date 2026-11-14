package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/health"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

type fakeHealthService struct {
	status health.Status
}

func (f fakeHealthService) Liveness(context.Context) error { return nil }

func (f fakeHealthService) Readiness(context.Context) health.Status { return f.status }

func TestReadinessHandler(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		status health.Status
	}{
		{
			name:   "healthy",
			status: health.Status{OK: true, DB: true, Qdrant: true, Embeddings: true},
		},
		{
			name:   "partial outage still returns 200",
			status: health.Status{OK: false, DB: true, Qdrant: false, Embeddings: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			ginCtx, _ := gin.CreateTestContext(recorder)
			ginCtx.Request = httptest.NewRequest(http.MethodGet, "/api/health", nil)

			handler := &WebAPIHandler{Health: fakeHealthService{status: tt.status}}
			handler.Readiness(&reqcontext.RequestContext{Context: ginCtx})

			if recorder.Code != http.StatusOK {
				t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
			}

			var got health.Status
			if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
				t.Fatalf("decoding response: %v", err)
			}
			if got != tt.status {
				t.Fatalf("response = %+v, want %+v", got, tt.status)
			}
		})
	}
}
