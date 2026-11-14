package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// PostLogin authenticates a demo user by id, creates a server-side session and
// sets the session cookie.
func (h *WebAPIHandler) PostLogin(ctx *reqcontext.RequestContext) {
	var req dto.LoginRequest
	if err := json.NewDecoder(ctx.Request.Body).Decode(&req); err != nil || strings.TrimSpace(req.UserID) == "" {
		respondWithJSONError(ctx, http.StatusBadRequest, "body must be valid JSON with a \"userId\" field")
		return
	}

	token, user, err := h.Auth.Login(ctx.Request.Context(), strings.TrimSpace(req.UserID))
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			respondWithJSONError(ctx, http.StatusUnauthorized, "unknown user")
			return
		}
		ctx.Logger.Error("PostLogin service error", "err", err)
		respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
		return
	}

	setSessionCookie(ctx.Writer, token)
	ctx.JSON(http.StatusOK, dto.ToUserResponse(user))
}

// PostLogout deletes the current session, if any, and clears the session
// cookie. It is idempotent and does not require an authenticated session.
func (h *WebAPIHandler) PostLogout(ctx *reqcontext.RequestContext) {
	if token, err := ctx.Cookie(auth.CookieName); err == nil && token != "" {
		if err := h.Auth.Logout(ctx.Request.Context(), token); err != nil {
			ctx.Logger.Error("PostLogout service error", "err", err)
			respondWithJSONError(ctx, http.StatusInternalServerError, "internal server error")
			return
		}
	}

	clearSessionCookie(ctx.Writer)
	ctx.Status(http.StatusNoContent)
}

// GetMe returns the user authenticated by the RequireAuth middleware.
func (h *WebAPIHandler) GetMe(ctx *reqcontext.RequestContext) {
	if ctx.User == nil {
		respondWithJSONError(ctx, http.StatusUnauthorized, "authentication required")
		return
	}
	ctx.JSON(http.StatusOK, dto.ToUserResponse(*ctx.User))
}

// setSessionCookie writes the session cookie. Secure is false because the MVP
// runs on plain HTTP locally; enable it once the app is served over TLS.
func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(auth.SessionTTL / time.Second),
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie expires the session cookie in the browser.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
}
