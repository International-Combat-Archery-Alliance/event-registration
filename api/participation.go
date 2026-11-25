package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
)

func (a *API) PostEventsV1EventIdTeamsTeamIdSeed(ctx context.Context, request PostEventsV1EventIdTeamsTeamIdSeedRequestObject) (PostEventsV1EventIdTeamsTeamIdSeedResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostEventsV1EventIdTeamsTeamIdSeed")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	event, err := a.db.GetEvent(ctx, request.EventId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch event for participation seed", "error", err)

		var eventErr *events.Error
		if errors.As(err, &eventErr) && eventErr.Reason == events.REASON_EVENT_DOES_NOT_EXIST {
			return PostEventsV1EventIdTeamsTeamIdSeed404JSONResponse{
				Code:    NotFound,
				Message: "Event not found",
			}, nil
		}
		return PostEventsV1EventIdTeamsTeamIdSeed500JSONResponse{
			Code:    InternalError,
			Message: "Failed to seed participation",
		}, nil
	}

	if event.Status.NormalizeDefault() == events.EventStatusFinalized {
		return PostEventsV1EventIdTeamsTeamIdSeed409JSONResponse{
			Code:    Conflict,
			Message: "Event is FINALIZED; standings locked",
		}, nil
	}

	if _, err := a.db.GetTeam(ctx, request.TeamId); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch team for participation seed", "error", err)

		var teamErr *teams.Error
		if errors.As(err, &teamErr) && teamErr.Reason == teams.REASON_TEAM_DOES_NOT_EXIST {
			return PostEventsV1EventIdTeamsTeamIdSeed404JSONResponse{
				Code:    NotFound,
				Message: "Team not found",
			}, nil
		}
		return PostEventsV1EventIdTeamsTeamIdSeed500JSONResponse{
			Code:    InternalError,
			Message: "Failed to seed participation",
		}, nil
	}

	// MVP bypass: CONFIRMED directly with an empty snapshot, no payment
	// lifecycle. Revisit in Stage C (RFC-0001 §6).
	participation := teams.Participation{
		EventID:           request.EventId,
		TeamID:            request.TeamId,
		Status:            teams.ParticipationStatusConfirmed,
		RosterSnapshot:    []uuid.UUID{},
		RosterSizeAtEvent: 0,
		Version:           1,
	}
	history := teams.TeamHistory{
		TeamID:    request.TeamId,
		EventID:   request.EventId,
		EventName: event.Name,
		EventDate: event.StartTime,
		Status:    teams.ParticipationStatusConfirmed,
		Version:   1,
	}

	if err := a.db.SeedParticipation(ctx, participation, history); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to seed participation", "error", err)

		var teamErr *teams.Error
		if errors.As(err, &teamErr) && teamErr.Reason == teams.REASON_PARTICIPATION_ALREADY_EXISTS {
			return PostEventsV1EventIdTeamsTeamIdSeed409JSONResponse{
				Code:    AlreadyExists,
				Message: "Team already participates in this event",
			}, nil
		}
		return PostEventsV1EventIdTeamsTeamIdSeed500JSONResponse{
			Code:    InternalError,
			Message: "Failed to seed participation",
		}, nil
	}

	respParticipation, err := participationToApiParticipation(participation)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to convert participation into api type", "error", err)

		return PostEventsV1EventIdTeamsTeamIdSeed500JSONResponse{
			Code:    InternalError,
			Message: "Failed to seed participation",
		}, nil
	}
	return PostEventsV1EventIdTeamsTeamIdSeed200JSONResponse{Participation: respParticipation}, nil
}

func participationToApiParticipation(p teams.Participation) (Participation, error) {
	apiStatus, err := participationStatusToApiStatus(p.Status)
	if err != nil {
		return Participation{}, err
	}

	snapshot := make([]uuid.UUID, 0, len(p.RosterSnapshot))
	snapshot = append(snapshot, p.RosterSnapshot...)

	return Participation{
		EventId:           &p.EventID,
		TeamId:            &p.TeamID,
		Status:            apiStatus,
		RosterSnapshot:    snapshot,
		RosterSizeAtEvent: p.RosterSizeAtEvent,
		Version:           &p.Version,
	}, nil
}

func participationStatusToApiStatus(s teams.ParticipationStatus) (ParticipationStatus, error) {
	switch s {
	case teams.ParticipationStatusRegistered:
		return REGISTERED, nil
	case teams.ParticipationStatusConfirmed:
		return CONFIRMED, nil
	case teams.ParticipationStatusWithdrawn:
		return WITHDRAWN, nil
	case teams.ParticipationStatusDNS:
		return DNS, nil
	default:
		return ParticipationStatus(""), fmt.Errorf("unknown participation status: %q", string(s))
	}
}
