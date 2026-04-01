package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
	_ "time/tzdata" // Embeds timezone data

	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/International-Combat-Archery-Alliance/captcha/cfturnstile"
	"github.com/International-Combat-Archery-Alliance/event-registration/api"
	"github.com/International-Combat-Archery-Alliance/event-registration/dynamo"
	"github.com/International-Combat-Archery-Alliance/payments/stripe"
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

	env := getApiEnvironment()

	signingKeys, currentKeyID, err := getJWTSigningKeys(ctx, env)
	if err != nil {
		logger.Error("failed to get JWT signing keys", slog.String("error", err.Error()))
		os.Exit(1)
	}

	tokenService := token.NewTokenService(
		signingKeys[currentKeyID],
		token.WithSigningKeys(signingKeys, currentKeyID),
	)

	cfSecretKey, err := getTurnstileSecretKey(ctx, env)
	if err != nil {
		logger.Error("failed to get turnstile secret key", slog.String("error", err.Error()))
		os.Exit(1)
	}
	cfTurnstileValidator := cfturnstile.NewValidator(http.DefaultClient, cfSecretKey)

	emailSender, err := createEmailSender(ctx, logger, env)
	if err != nil {
		logger.Error("failed to create email sender", slog.String("error", err.Error()))
		os.Exit(1)
	}

	stripeClient, err := makeStripeClient(ctx, env)
	if err != nil {
		logger.Error("failed to create stripe client", slog.String("error", err.Error()))
		os.Exit(1)
	}

	eventAPI := api.NewAPI(db, logger, env, tokenService, cfTurnstileValidator, emailSender, stripeClient)

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

func getGoogleServiceAccountJSON(ctx context.Context) ([]byte, error) {
	parameter, err := getParameterFromAWS(ctx, "/googleServiceAccount")
	if err != nil {
		return nil, fmt.Errorf("failed to get google service account from aws: %w", err)
	}

	return []byte(parameter), nil
}

func getStripeSecretKey(ctx context.Context, env api.Environment) (string, error) {
	if env == api.LOCAL {
		return getEnvOrDefault("STRIPE_SECRET_KEY", ""), nil
	}

	parameter, err := getParameterFromAWS(ctx, "/stripeSecretKey")
	if err != nil {
		return "", fmt.Errorf("failed to get stripe secret key: %w", err)
	}

	return parameter, nil
}

func getStripeEndpointSecret(ctx context.Context, env api.Environment) (string, error) {
	if env == api.LOCAL {
		return getEnvOrDefault("STRIPE_ENDPOINT_SECRET", ""), nil
	}

	parameter, err := getParameterFromAWS(ctx, "/stripeEndpointSecret")
	if err != nil {
		return "", fmt.Errorf("failed to get stripe endpoint secret: %w", err)
	}

	return parameter, nil
}

func makeStripeClient(ctx context.Context, env api.Environment) (*stripe.Client, error) {
	secretKey, err := getStripeSecretKey(ctx, env)
	if err != nil {
		return nil, err
	}
	endpointSecret, err := getStripeEndpointSecret(ctx, env)
	if err != nil {
		return nil, err
	}

	return stripe.NewClient(secretKey, endpointSecret), nil
}

// jwtSigningKeysData represents the JSON structure for signing keys
type jwtSigningKeysData struct {
	CurrentKey string            `json:"currentKey"`
	Keys       map[string]string `json:"keys"`
}

// getJWTSigningKeys retrieves the JWT signing keys from environment variable (local)
// or AWS Parameter Store (production)
func getJWTSigningKeys(ctx context.Context, env api.Environment) (map[string]token.SigningKey, string, error) {
	if env == api.LOCAL {
		// Local development: use environment variable
		key := os.Getenv("JWT_SIGNING_KEY")
		if key == "" {
			key = "local-development-signing-key-minimum-32-characters-long"
		}
		return map[string]token.SigningKey{
			"local": {ID: "local", Key: []byte(key)},
		}, "local", nil
	}

	// Production: retrieve from AWS Parameter Store
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	client := ssm.NewFromConfig(cfg)

	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String("/jwtSigningKeys"),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get JWT signing keys from Parameter Store: %w", err)
	}

	// Parse JSON response
	var data jwtSigningKeysData
	if err := json.Unmarshal([]byte(*result.Parameter.Value), &data); err != nil {
		return nil, "", fmt.Errorf("failed to parse JWT signing keys JSON: %w", err)
	}

	// Convert to map of SigningKey (keys are base64 encoded)
	signingKeys := make(map[string]token.SigningKey)
	for keyID, keyValue := range data.Keys {
		decodedKey, err := base64.StdEncoding.DecodeString(keyValue)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decode base64 key %q: %w", keyID, err)
		}
		signingKeys[keyID] = token.SigningKey{
			ID:  keyID,
			Key: decodedKey,
		}
	}

	// Validate that current key exists
	if _, ok := signingKeys[data.CurrentKey]; !ok {
		return nil, "", fmt.Errorf("current key ID %q not found in keys", data.CurrentKey)
	}

	return signingKeys, data.CurrentKey, nil
}
