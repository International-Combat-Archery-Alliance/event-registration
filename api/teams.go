package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
)

func (a *API) PostEventsV1Teams(ctx context.Context, request PostEventsV1TeamsRequestObject) (PostEventsV1TeamsResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostEventsV1Teams")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Server-owned fields: clients cannot set identity, version, status, or
	// timestamps (all readOnly in the spec, enforced here).
	id := uuid.New()
	now := time.Now().UTC()
	request.Body.Id = &id
	request.Body.Version = ptr.Int(1)
	active := ACTIVE
	request.Body.Status = active
	request.Body.CreatedAt = &now
	request.Body.UpdatedAt = &now
	// Sanitize padding once; Validate rejects blank-after-trim, and the
	// reservation key derives from the same normalized form.
	request.Body.Name = strings.TrimSpace(request.Body.Name)
	request.Body.HomeCity = strings.TrimSpace(request.Body.HomeCity)

	team, err := apiTeamToTeam(*request.Body)
	if err != nil {
		span.RecordError(err)
		logger.Error("Failed to convert team into core type", "error", err)

		return PostEventsV1Teams400JSONResponse{
			Code:    InvalidBody,
			Message: "Failed to create the team",
		}, nil
	}

	if err := team.Validate(); err != nil {
		span.RecordError(err)
		logger.Error("Invalid team body", slog.String("error", err.Error()))

		return PostEventsV1Teams400JSONResponse{
			Code:    InvalidBody,
			Message: "Invalid team body",
		}, nil
	}

	if err := a.db.CreateTeam(ctx, team); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to create a team", "error", err)

		var teamErr *teams.Error
		if errors.As(err, &teamErr) && teamErr.Reason == teams.REASON_TEAM_NAME_TAKEN {
			return PostEventsV1Teams409JSONResponse{
				Code:    AlreadyExists,
				Message: "Team name is already taken",
			}, nil
		}

		return PostEventsV1Teams500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create the team",
		}, nil
	}

	logger.Info("created new team", slog.String("team-id", id.String()))

	return PostEventsV1Teams200JSONResponse(*request.Body), nil
}

func (a *API) GetEventsV1TeamsTeamId(ctx context.Context, request GetEventsV1TeamsTeamIdRequestObject) (GetEventsV1TeamsTeamIdResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "GetEventsV1TeamsTeamId")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	team, err := a.db.GetTeam(ctx, request.TeamId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch a team", "error", err)

		var teamErr *teams.Error
		if errors.As(err, &teamErr) && teamErr.Reason == teams.REASON_TEAM_DOES_NOT_EXIST {
			return GetEventsV1TeamsTeamId404JSONResponse{
				Code:    NotFound,
				Message: "Team does not exist",
			}, nil
		}

		return GetEventsV1TeamsTeamId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get team",
		}, nil
	}

	respTeam, err := teamToApiTeam(team)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to convert team into api type", "error", err)

		return GetEventsV1TeamsTeamId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get team",
		}, nil
	}
	return GetEventsV1TeamsTeamId200JSONResponse{Team: respTeam}, nil
}

func teamToApiTeam(team teams.Team) (Team, error) {
	apiStatus, err := teamStatusToApiStatus(team.Status)
	if err != nil {
		return Team{}, err
	}

	return Team{
		Id:              &team.ID,
		Version:         &team.Version,
		Name:            team.Name,
		HomeCity:        team.HomeCity,
		Status:          apiStatus,
		CaptainPlayerId: team.CaptainPlayerID,
		CreatedAt:       &team.CreatedAt,
		UpdatedAt:       &team.UpdatedAt,
	}, nil
}

func apiTeamToTeam(team Team) (teams.Team, error) {
	status := teams.TeamStatusActive
	if team.Status != "" {
		var err error
		status, err = apiStatusToTeamStatus(team.Status)
		if err != nil {
			return teams.Team{}, err
		}
	}

	var id uuid.UUID
	if team.Id != nil {
		id = *team.Id
	}
	version := 1
	if team.Version != nil {
		version = *team.Version
	}
	var createdAt, updatedAt time.Time
	if team.CreatedAt != nil {
		createdAt = *team.CreatedAt
	}
	if team.UpdatedAt != nil {
		updatedAt = *team.UpdatedAt
	}

	return teams.Team{
		ID:              id,
		Version:         version,
		Name:            team.Name,
		HomeCity:        team.HomeCity,
		Status:          status,
		CaptainPlayerID: team.CaptainPlayerId,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

func teamStatusToApiStatus(s teams.TeamStatus) (TeamStatus, error) {
	switch s {
	case teams.TeamStatusActive:
		return ACTIVE, nil
	case teams.TeamStatusArchived:
		return ARCHIVED, nil
	default:
		return TeamStatus(""), fmt.Errorf("unknown team status: %q", string(s))
	}
}

func apiStatusToTeamStatus(s TeamStatus) (teams.TeamStatus, error) {
	switch s {
	case ACTIVE:
		return teams.TeamStatusActive, nil
	case ARCHIVED:
		return teams.TeamStatusArchived, nil
	default:
		return teams.TeamStatus(""), fmt.Errorf("unknown team status: %q", string(s))
	}
}
