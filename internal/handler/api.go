/*
Package handler exposes the main HTTP engine. All HTTP routing is wired here.
Business logic lives in the domain package; HTTP-specific concerns (Gin, middleware,
serialisation) live in this package and its sub-packages.

To use this package, create a new instance with New() passing a valid Config.
The resulting Router exposes Handler() which returns an http.Handler suitable for
a standard http.Server.

Example:

	apirouter, err := handler.New(handler.Config{
		Logger:        logger,
		NameService:   nameService,
		HealthService: healthService,
		AuthService:   authService,
	})
	if err != nil {
		return fmt.Errorf("error creating the API server instance: %w", err)
	}

	apiserver := http.Server{
		Addr:    cfg.Web.APIHost,
		Handler: apirouter.Handler(),
	}
	apiserver.ListenAndServe()

See cmd/webapi/main.go for a full usage example.
*/
package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/auth"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/health"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/domain/name"
	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/handler/middleware"
)

// Config provides dependencies and configuration to New.
type Config struct {
	CORS cors.Config

	// Logger where log entries are sent.
	Logger *slog.Logger

	// TrustedProxies is the list of trusted proxy IPs for Gin.
	TrustedProxies []string

	// NameService is the domain service for Name operations.
	NameService nameservice.ServiceName

	// HealthService is the domain service for health checks.
	HealthService health.HealthService

	// AuthService is the domain service for session authentication.
	AuthService auth.Service
}

// Router is the public interface for the HTTP handler layer.
type Router interface {
	// Handler returns an http.Handler that serves all registered API route.
	Handler() http.Handler

	// Close terminates any resource opened during the router lifecycle.
	Close() error
}

// New returns a new Router instance. Global middleware is registered here;
// route are registered lazily in Handler() — call Handler() exactly once.
func New(cfg Config) (Router, error) {
	if cfg.Logger == nil {
		return nil, errors.New("logger is required")
	}
	if cfg.NameService == nil {
		return nil, errors.New("name service is required")
	}
	if cfg.HealthService == nil {
		return nil, errors.New("health service is required")
	}
	if cfg.AuthService == nil {
		return nil, errors.New("auth service is required")
	}

	engine := gin.New()
	engine.RedirectTrailingSlash = false
	engine.RedirectFixedPath = false
	engine.HandleMethodNotAllowed = true
	if err := engine.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, fmt.Errorf("setting trusted proxies: %w", err)
	}

	// Global middleware
	engine.Use(cors.New(cfg.CORS))
	engine.Use(gin.Recovery())
	engine.Use(middleware.RequestContextLogger(cfg.Logger))

	return &_router{
		engine:        engine,
		baseLogger:    cfg.Logger,
		nameService:   cfg.NameService,
		healthService: cfg.HealthService,
		authService:   cfg.AuthService,
	}, nil
}

type _router struct {
	engine        *gin.Engine
	baseLogger    *slog.Logger
	nameService   nameservice.ServiceName
	healthService health.HealthService
	authService   auth.Service
}
