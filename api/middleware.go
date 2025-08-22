package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	middleware "github.com/oapi-codegen/nethttp-middleware"
	"github.com/rs/cors"
)

type middlewareFunc func(next http.Handler) http.Handler

func useMiddlewares(r *http.ServeMux, middlewares ...middlewareFunc) http.Handler {
	var s http.Handler
	s = r

	for _, mw := range middlewares {
		s = mw(s)
	}

	return s
}

func (a *API) loggingMiddleware() middlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			loggingRW := newLoggingResponseWriter(w)

			// process the request
			next.ServeHTTP(loggingRW, r)

			a.logger.InfoContext(r.Context(),
				"Access log",
				slog.String("latency", formatDuration(time.Since(start))),
				slog.Int64("request-content-length", r.ContentLength),
				slog.Int("resp-body-size", loggingRW.responseSize),
				slog.String("host", r.Host),
				slog.String("method", r.Method),
				slog.Int("status-code", loggingRW.statusCode),
				slog.String("path", r.URL.Path),
			)
		})
	}
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

// formatDuration formats a duration to one decimal point.
func formatDuration(d time.Duration) string {
	div := time.Duration(10)
	switch {
	case d > time.Second:
		d = d.Round(time.Second / div)
	case d > time.Millisecond:
		d = d.Round(time.Millisecond / div)
	case d > time.Microsecond:
		d = d.Round(time.Microsecond / div)
	case d > time.Nanosecond:
		d = d.Round(time.Nanosecond / div)
	}
	return d.String()
}
