package api

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/google/uuid"
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
			requestId := uuid.New()
			start := time.Now()

			requestLogger := a.logger.With(slog.String("request-id", requestId.String()))
			ctx := ctxWithRequestId(r.Context(), requestId)
			ctx = ctxWithLogger(ctx, requestLogger)

			loggingRW := newLoggingResponseWriter(w)

			// process the request
			next.ServeHTTP(loggingRW, r.WithContext(ctx))

			requestLogger.InfoContext(r.Context(),
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
		Options: openapi3filter.Options{
			AuthenticationFunc: func(ctx context.Context, ai *openapi3filter.AuthenticationInput) error {
				logger := getLoggerFromCtx(ctx)

				var token string

				switch ai.SecuritySchemeName {
				case "cookieAuth":
					authCookie, err := ai.RequestValidationInput.Request.Cookie(googleAuthJWTCookieKey)
					if err != nil {
						return fmt.Errorf("Auth token was not found in cookie %q", googleAuthJWTCookieKey)
					}
					token = authCookie.Value
				case "bearerAuth":
					authHeader := ai.RequestValidationInput.Request.Header.Get("Authorization")
					if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
						return fmt.Errorf("Auth token was not found in Authorization header")
					}
					token = strings.TrimPrefix(authHeader, "Bearer ")
				default:
					return fmt.Errorf("unsupported security scheme")
				}

				jwt, err := a.validateGoogleOauthToken(ctx, token, ai.Scopes)
				if err != nil {
					logger.Error("user attempted to hit an authenticated API without permissions", slog.String("error", err.Error()))

					return fmt.Errorf("failed to validate JWT")
				}

				loggerWithJwt := logger.With(slog.Any("user-email", jwt.Claims["email"]))
				ctx = ctxWithJWT(ctx, jwt)
				ctx = ctxWithLogger(ctx, loggerWithJwt)

				*ai.RequestValidationInput.Request = *ai.RequestValidationInput.Request.WithContext(ctx)

				return nil
			},
		},
		ErrorHandlerWithOpts: func(ctx context.Context, err error, w http.ResponseWriter, r *http.Request, opts middleware.ErrorHandlerOpts) {
			logger := getLoggerFromCtx(ctx)

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
				logger.Error("failed to marshal input validation error resp", "error", err)
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
		serverCors = cors.New(cors.Options{
			AllowedOrigins: []string{"http://localhost:4173", "http://localhost:5173"},
			AllowedMethods: []string{
				http.MethodHead,
				http.MethodGet,
				http.MethodPost,
				http.MethodPut,
				http.MethodPatch,
				http.MethodDelete,
			},
			AllowedHeaders:   []string{"*"},
			AllowCredentials: true,
		})
	case PROD:
		serverCors = cors.New(cors.Options{
			AllowedOrigins: []string{"https://icaa.world", "https://*-icaa-world.curly-sound-f2cd.workers.dev"},
			AllowedMethods: []string{
				http.MethodHead,
				http.MethodGet,
				http.MethodPost,
				http.MethodPut,
				http.MethodPatch,
				http.MethodDelete,
			},
			// TODO: revisit this
			AllowedHeaders:   []string{"*"},
			MaxAge:           300,
			AllowCredentials: true,
		})
	}

	return serverCors.Handler
}

//go:embed swagger-ui/*
var swaggerUI embed.FS

func (a *API) openapiRoutesMiddleware(spec *openapi3.T) middlewareFunc {
	openapiServer := http.NewServeMux()
	openapiServer.Handle("/events/swagger-ui/", http.StripPrefix("/events", http.FileServer(http.FS(swaggerUI))))
	openapiServer.HandleFunc("/events/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(spec)
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler, matchedPath := openapiServer.Handler(r)

			if matchedPath == "" {
				next.ServeHTTP(w, r)
				return
			}

			handler.ServeHTTP(w, r)
		})
	}
}

func (a *API) prodBaseNameHandling() middlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.env == PROD {
				urlWithBasePath, err := url.JoinPath("/events", r.URL.Path)
				if err != nil {
					a.logger.Error("url.JoinPath returned an error somehow?", slog.String("error", err.Error()))
				} else {
					r.URL.Path = urlWithBasePath
				}
			}

			next.ServeHTTP(w, r)
		})
	}
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
