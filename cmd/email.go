package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/email/awsses"
	"github.com/International-Combat-Archery-Alliance/event-registration/api"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
)

var _ email.Sender = &EmailLogger{}

// email.Sender that logs out the email contents for local dev
// also will write it to a file
type EmailLogger struct {
	logger *slog.Logger
}

func (el *EmailLogger) SendEmail(ctx context.Context, e email.Email) error {
	el.logger.Info("email that would be sent", slog.Any("email", e))

	return nil
}

func createProdEmailSender(ctx context.Context) (*awsses.AWSSESSender, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws config: %w", err)
	}

	sesClient := sesv2.NewFromConfig(cfg)
	sender := awsses.NewAWSSESSender(sesClient)

	return sender, nil
}

func createEmailSender(ctx context.Context, logger *slog.Logger, env api.Environment) (email.Sender, error) {
	if env == api.LOCAL {
		return &EmailLogger{logger: logger}, nil
	}

	return createProdEmailSender(ctx)
}
