package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

const (
	// maxChatBody bounds the small JSON body of the chat route.
	maxChatBody = 32 << 10
	// chatRateLimit is the number of messages allowed per searcher per window.
	chatRateLimit = 20
	// chatRateWindow is the rate-limit window.
	chatRateWindow = time.Minute
)

// PostChat runs one orchestrator turn for the authenticated searcher.
//
// The body carries only the conversation. The handler never accepts a
// confidence, a private profile or an item claim decision from the client: the
// agent builds the verification session server-side.
func (h *WebAPIHandler) PostChat(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxChatBody)
	decoder := json.NewDecoder(ctx.Request.Body)
	// Unknown fields (including a forged confidence) are rejected outright.
	decoder.DisallowUnknownFields()
	var payload dto.ChatPayload
	if err := decoder.Decode(&payload); err != nil {
		ctx.Logger.Error("PostChat invalid body", "err", err)
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be JSON with a messages array")
		return
	}
	// The limit is per conversation turn, not per message: the client sends the
	// full transcript every time, so charging each historical message would
	// exhaust the budget during a normal interview.
	if !h.chatLimiter.allow(ctx.User.ID) {
		respondWithJSONError(ctx, http.StatusTooManyRequests, "too many chat messages, try again later")
		return
	}

	reply, err := h.Agent.Chat(ctx.Request.Context(), ctx.User.ID, dto.ToAgentMessages(payload.Messages))
	if err != nil {
		if errors.Is(err, errs.ErrInvalidInput) {
			respondWithJSONError(ctx, http.StatusBadRequest, "invalid chat messages")
			return
		}
		ctx.Logger.Error("PostChat service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	ctx.JSON(http.StatusOK, dto.ToChatResponse(reply))
}

// chatLimiter is a small in-memory fixed-window limiter keyed by searcher id.
// It is intentionally process-local: the MVP runs one instance and the limit is
// a guard against runaway chat loops, not a billing control.
type chatLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	now      func() time.Time
	counters map[string]chatWindow
}

type chatWindow struct {
	start time.Time
	count int
}

func newChatLimiter(limit int, window time.Duration) *chatLimiter {
	return &chatLimiter{
		limit:    limit,
		window:   window,
		now:      time.Now,
		counters: map[string]chatWindow{},
	}
}

// allow records one conversation turn for key and reports whether it is within
// the window. A request that would exceed the limit is not consumed.
func (l *chatLimiter) allow(key string) bool {
	if l == nil || key == "" {
		return true
	}
	now := l.now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.counters) > 1024 {
		l.prune(now)
	}
	window, ok := l.counters[key]
	if !ok || now.Sub(window.start) >= l.window {
		l.counters[key] = chatWindow{start: now, count: 1}
		return true
	}
	if window.count+1 > l.limit {
		return false
	}
	window.count++
	l.counters[key] = window
	return true
}

// prune drops expired windows. It runs under the limiter mutex.
func (l *chatLimiter) prune(now time.Time) {
	for key, window := range l.counters {
		if now.Sub(window.start) >= l.window {
			delete(l.counters, key)
		}
	}
}
