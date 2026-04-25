package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // Embeds timezone data

	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/International-Combat-Archery-Alliance/captcha/cfturnstile"
	"github.com/International-Combat-Archery-Alliance/event-registration/api"
	"github.com/International-Combat-Archery-Alliance/telemetry"
	"go.opentelemetry.io/otel"
	"golang.org/x/sync/errgroup"
)

var tracer = otel.Tracer("github.com/International-Combat-Archery-Alliance/event-registration/cmd")

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	logger.Info("starting up")
	if err := run(logger); err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	eventAPI, traceShutdown, err := setupApi(logger)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := traceShutdown(shutdownCtx); err != nil {
			logger.Error("failed to shutdown telemetry", "error", err)
		}
	}()
	if err != nil {
		return err
	}

	serverSettings := getServerSettingsFromEnv()

	sigCtx, sigStop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer sigStop()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- eventAPI.ListenAndServe(serverSettings.Host, serverSettings.Port)
	}()

	select {
	case <-sigCtx.Done():
		logger.Info("shutting down gracefully")
		return nil
	case err := <-serverErrCh:
		if err != nil {
			logger.Error("error running server", "error", err)
			return err
		}
		return nil
	}
}

func setupApi(logger *slog.Logger) (*api.API, func(context.Context) error, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	env := getApiEnvironment()

	traceShutdown := func(context.Context) error { return nil }

	// -----------------------------------------------------------------------
	// Phase 1: New Relic license key → telemetry init (sequential dependency)
	// -----------------------------------------------------------------------

	licenseKey, err := getNewRelicLicenseKey(ctx, env)
	if err != nil {
		return nil, traceShutdown, fmt.Errorf("new relic license key: %w", err)
	}

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "otlp.nr-data.net:4317"
	}

	traceShutdown, flushTraces, err := telemetry.Init(ctx, telemetry.Options{
		ServiceName: "event-registration",
		Endpoint:    endpoint,
		APIKey:      licenseKey,
		Lambda:      telemetry.LambdaInfoFromEnv(),
	})
	if err != nil {
		return nil, traceShutdown, fmt.Errorf("telemetry init: %w", err)
	}

	ctx, startupSpan := tracer.Start(ctx, "startup")
	defer startupSpan.End()

	httpClient := telemetry.InstrumentedHTTPClient()

	// -----------------------------------------------------------------------
	// Phase 2: Fetch app config and create DB in parallel
	// -----------------------------------------------------------------------

	var (
		cfg *AppConfig
		db  api.DB
	)

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		ctx, span := tracer.Start(gCtx, "init-config")
		defer span.End()

		var err error
		cfg, err = fetchAppConfig(ctx, env)
		if err != nil {
			span.RecordError(err)
		}
		return err
	})

	g.Go(func() error {
		ctx, span := tracer.Start(gCtx, "init-db")
		defer span.End()

		var err error
		db, err = makeDB(ctx)
		if err != nil {
			span.RecordError(err)
		}
		return err
	})

	if err := g.Wait(); err != nil {
		startupSpan.RecordError(err)
		startupSpan.End()
		return nil, traceShutdown, err
	}

	// -----------------------------------------------------------------------
	// Phase 3: Wire up services (all instant after config is loaded)
	// -----------------------------------------------------------------------

	tokenService := token.NewTokenService(
		cfg.JWTSigningKeys[cfg.JWTCurrentKeyID],
		token.WithSigningKeys(cfg.JWTSigningKeys, cfg.JWTCurrentKeyID),
	)

	cfTurnstileValidator := cfturnstile.NewValidator(httpClient, cfg.TurnstileSecretKey)

	emailSender, err := createEmailSender(logger, env, cfg.GoogleServiceAccount)
	if err != nil {
		startupSpan.RecordError(err)
		startupSpan.End()
		return nil, traceShutdown, fmt.Errorf("email sender: %w", err)
	}

	stripeClient := makeStripeClient(cfg.StripeSecretKey, cfg.StripeEndpointSecret, httpClient)

	eventAPI := api.NewAPI(db, logger, env, tokenService, cfTurnstileValidator, emailSender, stripeClient, flushTraces)

	return eventAPI, traceShutdown, nil
}

type ServerSettings struct {
	Host string
	Port string
}

func getServerSettingsFromEnv() ServerSettings {
	return ServerSettings{
		Host: getEnvOrDefault("HOST", "0.0.0.0"),
		Port: getEnvOrDefault("PORT", "8080"),
	}
}
