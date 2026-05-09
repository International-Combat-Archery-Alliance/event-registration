package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
)

func (a *API) PostEventsV1AdminTestEmail(ctx context.Context, request PostEventsV1AdminTestEmailRequestObject) (PostEventsV1AdminTestEmailResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostEventsV1AdminTestEmail")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	targetEmail := string(request.Body.Email)
	logger.Info("sending test email", slog.String("email", targetEmail))

	event := newFakeEvent()
	reg := newFakeRegistration(event.ID, targetEmail)

	err := registration.SendRegistrationConfirmationEmail(ctx, a.emailSender, "ICAA <info@icaa.world>", reg, event)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to send test email", slog.String("error", err.Error()))

		return PostEventsV1AdminTestEmail500JSONResponse{
			Code:    InternalError,
			Message: fmt.Sprintf("Failed to send test email: %v", err),
		}, nil
	}

	return PostEventsV1AdminTestEmail200Response{}, nil
}

func newFakeEvent() events.Event {
	nyc, _ := time.LoadLocation("America/New_York")
	now := time.Now()

	return events.Event{
		ID:      uuid.New(),
		Version: 1,
		Name:    "ICAA Test Event 2026",
		EventLocation: events.Location{
			Name: "Test Community Center",
			LocAddress: events.Address{
				Street:     "123 Main St",
				City:       "Testville",
				State:      "MA",
				PostalCode: "02101",
				Country:    "USA",
			},
		},
		TimeZone:              nyc,
		StartTime:             now.Add(14 * 24 * time.Hour),
		EndTime:               now.Add(14*24*time.Hour + 6*time.Hour),
		RegistrationCloseTime: now.Add(13 * 24 * time.Hour),
		RegistrationOptions: []events.EventRegistrationOption{
			{
				RegType: events.BY_INDIVIDUAL,
				Price:   money.New(5000, "USD"),
			},
		},
		AllowedTeamSizeRange: events.Range{Min: 1, Max: 5},
		RulesDocLink:         ptr.String("https://assets.icaa.world/2183e906-c915-4b44-bc9e-fe83fb30457c.pdf"),
	}
}

func newFakeRegistration(eventID uuid.UUID, email string) *registration.IndividualRegistration {
	now := time.Now()

	return &registration.IndividualRegistration{
		ID:           uuid.New(),
		Version:      1,
		EventID:      eventID,
		RegisteredAt: now,
		Email:        email,
		HomeCity:     "Testville, USA",
		PlayerInfo: registration.PlayerInfo{
			FirstName: "Jane",
			LastName:  "Archer",
			Email:     ptr.String(email),
		},
		Experience: registration.INTERMEDIATE,
		Paid:       false,
	}
}
