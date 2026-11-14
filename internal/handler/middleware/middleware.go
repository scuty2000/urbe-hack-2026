package middleware

import (
	"log/slog"

	"github.com/tommaso-sgroi/fantastic-gin-dealcoholized/internal/repository"
)

type Middleware struct {
	Logger *slog.Logger
	DB     repository.Database
}
