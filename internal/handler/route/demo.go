package routes

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

const (
	// demoResetHeader carries the shared secret for the demo reset route.
	demoResetHeader = "X-Demo-Reset-Secret"
	// maxDemoResetBody bounds the optional JSON body.
	maxDemoResetBody = 4 << 10
)

type demoResetPayload struct {
	Secret string `json:"secret"`
}

// demoResetResponse is the small confirmation returned after a successful seed.
type demoResetResponse struct {
	Status string `json:"status"`
}

// PostDemoReset truncates and re-seeds the demo dataset. The route is admin
// only (enforced by middleware) and additionally requires the shared secret
// configured through API_DEMO_RESET_SECRET, either in the X-Demo-Reset-Secret
// header or in the JSON body.
func (h *WebAPIHandler) PostDemoReset(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}
	if h.DemoReset == nil || strings.TrimSpace(h.DemoResetSecret) == "" {
		respondWithJSONError(ctx, http.StatusServiceUnavailable, "demo reset is not configured")
		return
	}

	secret := ctx.GetHeader(demoResetHeader)
	if secret == "" {
		body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, maxDemoResetBody))
		if err != nil {
			respondWithJSONError(ctx, http.StatusBadRequest, "could not read the request body")
			return
		}
		if len(strings.TrimSpace(string(body))) > 0 {
			var payload demoResetPayload
			if err := json.Unmarshal(body, &payload); err != nil {
				respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with a secret field")
				return
			}
			secret = payload.Secret
		}
	}

	if subtle.ConstantTimeCompare([]byte(secret), []byte(h.DemoResetSecret)) != 1 {
		respondWithJSONError(ctx, http.StatusForbidden, "forbidden")
		return
	}

	if err := h.DemoReset(ctx.Request.Context()); err != nil {
		ctx.Logger.Error("PostDemoReset seed error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}
	ctx.JSON(http.StatusOK, demoResetResponse{Status: "reset"})
}
