//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/International-Combat-Archery-Alliance/captcha"
	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/middleware"
	"github.com/International-Combat-Archery-Alliance/payments"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
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
	tracer trace.Tracer

	tokenService     *token.TokenService
	captchaValidator captcha.Validator
	emailSender      email.Sender
	checkoutManager  payments.CheckoutManager
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(
	db DB,
	logger *slog.Logger,
	env Environment,
	tokenService *token.TokenService,
	captchaValidator captcha.Validator,
	emailSender email.Sender,
	checkoutManager payments.CheckoutManager,
) *API {
	return &API{
		db:               db,
		logger:           logger,
		env:              env,
		tracer:           otel.Tracer("github.com/International-Combat-Archery-Alliance/event-registration/api"),
		tokenService:     tokenService,
		captchaValidator: captchaValidator,
		emailSender:      emailSender,
		checkoutManager:  checkoutManager,
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

	swaggerUIMiddleware, err := middleware.HostSwaggerUI("/events", swagger)
	if err != nil {
		return fmt.Errorf("failed to create swagger ui middleware: %w", err)
	}

	// Setup CORS middleware
	corsConfig := middleware.DefaultCorsConfig()
	corsConfig.IsProduction = a.env == PROD
	corsMiddleware := middleware.CorsMiddleware(corsConfig)

	middlewares := []middleware.MiddlewareFunc{
		// Executes from the bottom up
		a.openapiValidateMiddleware(swagger),
		corsMiddleware,
		a.stripeRegistrationPaymentWebhookMiddleware("/events/v1/registration/webhook"),
		swaggerUIMiddleware,
		middleware.AccessLogging(a.logger),
	}

	if a.env == PROD {
		middlewares = append(middlewares, middleware.BaseNamePrefix(a.logger, "/events"))
	}

	h := middleware.UseMiddlewares(r, middlewares...)
	h = otelhttp.NewHandler(h, "")

	s := &http.Server{
		Handler: h,
		Addr:    net.JoinHostPort(host, port),
	}

	return s.ListenAndServe()
}

func (a *API) getLoggerOrBaseLogger(ctx context.Context) *slog.Logger {
	logger, ok := middleware.GetLoggerFromCtx(ctx)
	if !ok {
		a.logger.Error("tried to get logger and it wasn't in the context")
		return a.logger
	}
	return logger
}
