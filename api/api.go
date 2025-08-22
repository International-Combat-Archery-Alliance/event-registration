//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
)

type Environment int

const (
	LOCAL Environment = iota
	PROD
)

type DB interface {
	events.Repository
	registration.Repository
}

type API struct {
	db     DB
	logger *slog.Logger
	env    Environment
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(db DB, logger *slog.Logger, env Environment) *API {
	return &API{
		db:     db,
		logger: logger,
		env:    env,
	}
}

func (a *API) ListenAndServe(host string, port string) error {
	swagger, err := GetSwagger()
	if err != nil {
		return fmt.Errorf("Error loading swagger spec: %w", err)
	}

	swagger.Servers = nil

	strictHandler := NewStrictHandler(a, []StrictMiddlewareFunc{})

	r := http.NewServeMux()

	HandlerFromMux(strictHandler, r)

	h := useMiddlewares(
		r,
		a.loggingMiddleware(),
		a.openapiValidateMiddleware(swagger),
		a.corsMiddleware(),
	)

	s := &http.Server{
		Handler: h,
		Addr:    net.JoinHostPort(host, port),
	}

	return s.ListenAndServe()
}
