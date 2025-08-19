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
	"github.com/getkin/kin-openapi/openapi3filter"
	middleware "github.com/oapi-codegen/nethttp-middleware"
)

type DB interface {
	events.Repository
	registration.Repository
}

type API struct {
	db     DB
	logger *slog.Logger
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(db DB, logger *slog.Logger) *API {
	return &API{
		db:     db,
		logger: logger,
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

	oapiValidationMW := middleware.OapiRequestValidatorWithOptions(swagger, &middleware.Options{
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

	h := useMiddlewares(
		r,
		oapiValidationMW,
	)

	s := &http.Server{
		Handler: h,
		Addr:    net.JoinHostPort(host, port),
	}

	return s.ListenAndServe()
}

func useMiddlewares(r *http.ServeMux, middlewares ...func(next http.Handler) http.Handler) http.Handler {
	var s http.Handler
	s = r

	for _, mw := range middlewares {
		s = mw(s)
	}

	return s
}
