package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/errs"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/dto"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/reqcontext"
)

// RequireAuth returns a middleware that requires a valid lf_session cookie.
// On success it stores the authenticated user in the request context; on
// failure it aborts the request with 401 and a JSON error body.
func RequireAuth(authService auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := authenticateRequest(c, authService); !ok {
			return
		}
		c.Next()
	}
}

// RequireRole returns a middleware that requires a valid session whose user has
// the given role. It aborts with 401 when the session is missing or invalid and
// with 403 when the authenticated user has the wrong role.
//
// RequireRole authenticates on its own, but when it runs after RequireAuth it
// reuses the user already stored in the request context instead of loading the
// session twice.
func RequireRole(authService auth.Service, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := authenticateRequest(c, authService)
		if !ok {
			return
		}
		if user.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, dto.ErrorResponse{Error: "forbidden"})
			return
		}
		c.Next()
	}
}

// authenticateRequest resolves the session cookie to a user, reusing a user
// already attached by an earlier middleware. It writes the error response and
// returns false when the request must not continue.
func authenticateRequest(c *gin.Context, authService auth.Service) (auth.User, bool) {
	rc := reqcontext.GetRequestContext(c)
	if rc.User != nil {
		return *rc.User, true
	}

	token, err := c.Cookie(auth.CookieName)
	if err != nil || token == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "authentication required"})
		return auth.User{}, false
	}

	user, err := authService.Authenticate(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, errs.ErrUnauthorized) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "invalid or expired session"})
			return auth.User{}, false
		}
		// Any other error is an infrastructure failure (e.g. the database is
		// unavailable): log it and report 500 instead of mislabelling it as a
		// bad session. Mirrors PostLogin's error mapping.
		if rc.Logger != nil {
			rc.Logger.Error("authenticate session", "err", err)
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "internal server error"})
		return auth.User{}, false
	}

	rc.User = &user
	return user, true
}
