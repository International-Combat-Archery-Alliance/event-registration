package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
)

func (a *API) PostEventsV1AdminTestMailerlite(ctx context.Context, request PostEventsV1AdminTestMailerliteRequestObject) (PostEventsV1AdminTestMailerliteResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostEventsV1AdminTestMailerlite")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	groupName := "MailerLite Test Group"
	if request.Body.GroupName != nil {
		groupName = *request.Body.GroupName
	}

	logger.Info("creating test mailerlite group", slog.String("groupName", groupName))

	groupID, err := a.subscriberManager.CreateGroup(ctx, groupName)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("failed to create mailerlite group", slog.String("error", err.Error()))

		return PostEventsV1AdminTestMailerlite500JSONResponse{
			Code:    InternalError,
			Message: fmt.Sprintf("Failed to create test group: %v", err),
		}, nil
	}

	switch request.Body.RegistrationType {
	case ByIndividual:
		for i, email := range request.Body.Emails {
			reg := &registration.IndividualRegistration{
				ID:           uuid.New(),
				Version:      1,
				EventID:      uuid.New(),
				RegisteredAt: time.Now(),
				Email:        strings.ToLower(string(email)),
				HomeCity:     "Testville, USA",
				PlayerInfo: registration.PlayerInfo{
					FirstName: fmt.Sprintf("Test%d", i+1),
					LastName:  "User",
					Email:     ptr.String(strings.ToLower(string(email))),
				},
				Experience: registration.INTERMEDIATE,
				Paid:       false,
			}
			registration.AddToMailingList(ctx, a.subscriberManager, reg, groupID, logger)
		}
	case ByTeam:
		if request.Body.TeamName == nil {
			return PostEventsV1AdminTestMailerlite400JSONResponse{
				Code:    InvalidBody,
				Message: "teamName is required for ByTeam registrations",
			}, nil
		}
		if len(request.Body.Emails) < 1 {
			return PostEventsV1AdminTestMailerlite400JSONResponse{
				Code:    InvalidBody,
				Message: "at least one email is required for team registration (the captain)",
			}, nil
		}

		captainEmail := strings.ToLower(string(request.Body.Emails[0]))

		players := make([]registration.PlayerInfo, 0, len(request.Body.Emails)-1)
		for i, email := range request.Body.Emails[1:] {
			players = append(players, registration.PlayerInfo{
				FirstName: fmt.Sprintf("Player%d", i+1),
				LastName:  "Test",
				Email:     ptr.String(strings.ToLower(string(email))),
			})
		}

		reg := &registration.TeamRegistration{
			ID:           uuid.New(),
			Version:      1,
			EventID:      uuid.New(),
			RegisteredAt: time.Now(),
			HomeCity:     "Testville, USA",
			TeamName:     *request.Body.TeamName,
			CaptainEmail: captainEmail,
			Players:      players,
			Paid:         false,
		}
		registration.AddToMailingList(ctx, a.subscriberManager, reg, groupID, logger)
	default:
		return PostEventsV1AdminTestMailerlite400JSONResponse{
			Code:    InvalidBody,
			Message: fmt.Sprintf("Unknown registration type: %s", request.Body.RegistrationType),
		}, nil
	}

	logger.Info("test mailerlite group created and subscribers added",
		slog.String("groupId", groupID),
		slog.String("groupName", groupName),
		slog.Int("numEmails", len(request.Body.Emails)))

	return PostEventsV1AdminTestMailerlite200JSONResponse{
		GroupId:   &groupID,
		GroupName: &groupName,
	}, nil
}
