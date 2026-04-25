package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/email/gmail"
	"github.com/International-Combat-Archery-Alliance/event-registration/api"
	"github.com/International-Combat-Archery-Alliance/event-registration/dynamo"
	"github.com/International-Combat-Archery-Alliance/payments/stripe"
	"github.com/International-Combat-Archery-Alliance/telemetry"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func makeDB(ctx context.Context) (api.DB, error) {
	if isLocal() {
		return makeLocalDB(ctx)
	}
	return makeProdDB(ctx)
}

func makeLocalDB(ctx context.Context) (api.DB, error) {
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
		return nil, fmt.Errorf("failed to create local dynamo client: %w", err)
	}

	telemetry.InstrumentAWSConfig(&cfg)

	dynamoClient := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://dynamodb:8000")
	})

	return dynamo.NewDB(dynamoClient, os.Getenv("DYNAMO_TABLE_NAME")), nil
}

func makeProdDB(ctx context.Context) (api.DB, error) {
	cfg, err := loadAWSConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create production dynamo client: %w", err)
	}

	dynamoClient := dynamodb.NewFromConfig(cfg)
	return dynamo.NewDB(dynamoClient, os.Getenv("DYNAMO_TABLE_NAME")), nil
}

func makeStripeClient(secretKey, endpointSecret string, httpClient *http.Client) *stripe.Client {
	return stripe.NewClient(secretKey, endpointSecret, stripe.WithHTTPClient(httpClient))
}

var _ email.Sender = &emailLogger{}

type emailLogger struct {
	logger *slog.Logger
}

func (el *emailLogger) SendEmail(ctx context.Context, e email.Email) error {
	el.logger.Info("email that would be sent", slog.Any("email", e))
	return nil
}

func createEmailSender(logger *slog.Logger, env api.Environment, googleServiceAccount []byte) (email.Sender, error) {
	if env == api.LOCAL {
		return &emailLogger{logger: logger}, nil
	}
	return createProdGmailSender(googleServiceAccount)
}

func createProdGmailSender(creds []byte) (email.Sender, error) {
	return gmail.NewGmailSender(context.Background(), creds, "andrew.mellen@icaa.world")
}


