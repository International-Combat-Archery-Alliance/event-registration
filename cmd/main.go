package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/International-Combat-Archery-Alliance/auth/google"
	"github.com/International-Combat-Archery-Alliance/captcha/cfturnstile"
	"github.com/International-Combat-Archery-Alliance/event-registration/api"
	"github.com/International-Combat-Archery-Alliance/event-registration/dynamo"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	db, err := makeDB(ctx)
	if err != nil {
		logger.Error("Error creating db client", "error", err)
		os.Exit(1)
	}

	googleAuthValidator, err := google.NewValidator(ctx)
	if err != nil {
		logger.Error("failed to create google auth validator", slog.String("error", err.Error()))
		os.Exit(1)
	}

	env := getApiEnvironment()

	cfSecretKey, err := getTurnstileSecretKey(ctx, env)
	if err != nil {
		logger.Error("failed to get turnstile secret key", slog.String("error", err.Error()))
		os.Exit(1)
	}
	cfTurnstileValidator := cfturnstile.NewValidator(http.DefaultClient, cfSecretKey)

	eventAPI := api.NewAPI(db, logger, env, googleAuthValidator, cfTurnstileValidator)

	serverSettings := getServerSettingsFromEnv()
	err = eventAPI.ListenAndServe(serverSettings.Host, serverSettings.Port)
	if err != nil {
		logger.Error("error running server", "error", err)
		os.Exit(1)
	}
	logger.Info("shutting down")
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

func getEnvOrDefault(key string, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}

	return defaultVal
}

func makeDB(ctx context.Context) (api.DB, error) {
	var dynamoClient *dynamodb.Client
	var err error
	if isLocal() {
		dynamoClient, err = createLocalDynamoClient(ctx)
	} else {
		dynamoClient, err = createProdDynamoClient(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamo client: %w", err)
	}

	database := dynamo.NewDB(dynamoClient, os.Getenv("DYNAMO_TABLE_NAME"))
	return database, nil
}

func isLocal() bool {
	return getEnvOrDefault("AWS_SAM_LOCAL", "false") == "true"
}

func getApiEnvironment() api.Environment {
	if isLocal() {
		return api.LOCAL
	}
	return api.PROD
}

func createLocalDynamoClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("localhost"),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID: "local", SecretAccessKey: "local", SessionToken: "",
				Source: "Mock credentials used above for local instance",
			},
		}),
	)
	if err != nil {
		return nil, err
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://dynamodb:8000")
	}), nil
}

func createProdDynamoClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return dynamodb.NewFromConfig(cfg), nil
}

func getParameterFromAWS(ctx context.Context, parameterName string) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to load SDK config: %w", err)
	}

	client := ssm.NewFromConfig(cfg)

	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(parameterName),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get parameter %s: %w", parameterName, err)
	}

	return *result.Parameter.Value, nil
}

func getTurnstileSecretKey(ctx context.Context, env api.Environment) (string, error) {
	if env == api.LOCAL {
		// This is the test key to always have success
		return "1x0000000000000000000000000000000AA", nil
	}

	parameter, err := getParameterFromAWS(ctx, "/cfTurnstileSecretKey")
	if err != nil {
		return "", fmt.Errorf("failed to get turnstile key from aws: %w", err)
	}

	return parameter, nil
}
