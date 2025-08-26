//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"google.golang.org/api/idtoken"
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

	googleIdVerifier googleAuthValidator
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(ctx context.Context, db DB, logger *slog.Logger, env Environment) (*API, error) {
	verifier, err := idtoken.NewValidator(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create google id verifier: %w", err)
	}

	return &API{
		db:               db,
		logger:           logger,
		env:              env,
		googleIdVerifier: verifier,
	}, nil
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
		// Executes from the bottom up
		a.openapiValidateMiddleware(swagger),
		a.corsMiddleware(),
		a.loggingMiddleware(),
	)

	s := &http.Server{
		Handler: h,
		Addr:    net.JoinHostPort(host, port),
	}

	return s.ListenAndServe()
}
