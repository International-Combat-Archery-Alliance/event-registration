package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/email/mailerlite"
	"github.com/International-Combat-Archery-Alliance/email/mailersend"
	"github.com/International-Combat-Archery-Alliance/event-registration/api"
	"github.com/International-Combat-Archery-Alliance/event-registration/dynamo"
	"github.com/International-Combat-Archery-Alliance/payments/stripe"
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

func createEmailSender(logger *slog.Logger, env api.Environment, mailerSendAPIKey string) (email.Sender, error) {
	if env == api.LOCAL {
		return &emailLogger{logger: logger}, nil
	}
	return createProdMailerSendSender(mailerSendAPIKey)
}

func createProdMailerSendSender(apiKey string) (email.Sender, error) {
	return mailersend.NewMailerSendSender(apiKey), nil
}

var _ email.SubscriberManager = &subscriberLogger{}

type subscriberLogger struct {
	logger *slog.Logger
}

func (s *subscriberLogger) CreateGroup(ctx context.Context, name string) (string, error) {
	s.logger.Info("mailerlite group that would be created", slog.String("name", name))
	return "local-group-id", nil
}

func (s *subscriberLogger) AddSubscriberToGroup(ctx context.Context, email, name, groupID string) error {
	s.logger.Info("mailerlite subscriber that would be added", slog.String("email", email), slog.String("name", name), slog.String("groupID", groupID))
	return nil
}

func createSubscriberManager(logger *slog.Logger, env api.Environment, mailerLiteAPIKey string) (email.SubscriberManager, error) {
	if env == api.LOCAL {
		return &subscriberLogger{logger: logger}, nil
	}
	return mailerlite.NewMailerLiteManager(mailerLiteAPIKey), nil
}


