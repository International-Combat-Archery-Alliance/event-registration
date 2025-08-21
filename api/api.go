//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	middleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/rs/cors"
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
		a.openapiValidateMiddleware(swagger),
		a.corsMiddleware(),
	)

	s := &http.Server{
		Handler: h,
		Addr:    net.JoinHostPort(host, port),
	}

	return s.ListenAndServe()
}

type middlewareFunc func(next http.Handler) http.Handler

func useMiddlewares(r *http.ServeMux, middlewares ...middlewareFunc) http.Handler {
	var s http.Handler
	s = r

	for _, mw := range middlewares {
		s = mw(s)
	}

	return s
}

func (a *API) openapiValidateMiddleware(swagger *openapi3.T) middlewareFunc {
	return middleware.OapiRequestValidatorWithOptions(swagger, &middleware.Options{
		ErrorHandlerWithOpts: func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts middleware.ErrorHandlerOpts) {
			var e Error

			var requestErr *openapi3filter.RequestError
			var secErr *openapi3filter.SecurityRequirementsError
			if errors.As(err, &requestErr) {
				e = Error{
					Message: err.Error(),
					Code:    InputValidationError,
				}
			} else if errors.As(err, &secErr) {
				e = Error{
					Message: err.Error(),
					Code:    AuthError,
				}
			} else {
				e = Error{
					Message: err.Error(),
					Code:    InternalError,
				}
			}
			jsonBody, err := json.Marshal(&e)
			if err != nil {
				a.logger.Error("failed to marshal input validation error resp", "error", err)
				jsonBody = []byte("{\"message\": \"input is invalid\", \"code\": \"InputValidationError\"")
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(opts.StatusCode)
			w.Write(jsonBody)
		},
	})
}

func (a *API) corsMiddleware() middlewareFunc {
	var serverCors *cors.Cors

	switch a.env {
	case LOCAL:
		serverCors = cors.AllowAll()
	case PROD:
		serverCors = cors.New(cors.Options{
			AllowedOrigins: []string{"https://icaa.world"},
			AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
			MaxAge:         300,
		})
	}

	return serverCors.Handler
}
