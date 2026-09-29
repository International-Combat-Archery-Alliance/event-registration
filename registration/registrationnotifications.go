package registration

import (
	"context"
	"log/slog"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
)

func SendRegistrationNotifications(ctx context.Context, emailSender email.Sender, subscriberManager email.SubscriberManager, reg Registration, event events.Event, logger *slog.Logger) error {
	ctx, span := tracer.Start(ctx, "SendRegistrationNotifications")
	defer span.End()

	emailErr := SendRegistrationConfirmationEmail(ctx, emailSender, email.Address{Name: "ICAA", Address: "info@icaa.world"}, reg, event)

	if event.MailingListGroupID != nil {
		AddToMailingList(ctx, subscriberManager, reg, *event.MailingListGroupID, logger)
	}

	if emailErr != nil {
		span.RecordError(emailErr)
		span.SetStatus(codes.Error, emailErr.Error())
	}
	return emailErr
}

// NotifyRegistration fetches the registration's event and sends its
// notifications. Every failure is logged, none is returned.
func NotifyRegistration(ctx context.Context, eventRepo events.Repository, emailSender email.Sender, subscriberManager email.SubscriberManager, reg Registration, logger *slog.Logger) {
	ctx, span := tracer.Start(ctx, "NotifyRegistration")
	defer span.End()

	event, err := eventRepo.GetEvent(ctx, reg.GetEventID())
	if err != nil {
		span.RecordError(err)
		logger.Error("Failed to get event for registration notifications", slog.String("error", err.Error()), slog.String("email", reg.GetEmail()))
		return
	}

	if err := SendRegistrationNotifications(ctx, emailSender, subscriberManager, reg, event, logger); err != nil {
		span.RecordError(err)
		logger.Error("Failed to send registration notifications", slog.String("error", err.Error()), slog.String("email", reg.GetEmail()))
	}
}

// ConfirmPaidRegistration marks an unpaid registration as paid and sends its
// notifications. alreadyPaid reports the registration was paid before this
// call, in which case nothing is written or sent. Notification failures are
// logged inside NotifyRegistration and never fail the result.
func ConfirmPaidRegistration(ctx context.Context, eventId uuid.UUID, email string, registrationRepo Repository, eventRepo events.Repository, emailSender email.Sender, subscriberManager email.SubscriberManager, logger *slog.Logger) (Registration, bool, error) {
	ctx, span := tracer.Start(ctx, "ConfirmPaidRegistration")
	defer span.End()

	reg, alreadyPaid, err := MarkRegistrationAsPaid(ctx, eventId, email, registrationRepo)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, false, err
	}

	if !alreadyPaid {
		NotifyRegistration(ctx, eventRepo, emailSender, subscriberManager, reg, logger)
	}
	return reg, alreadyPaid, nil
}
