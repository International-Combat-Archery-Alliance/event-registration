package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func (a *API) GetTeamsV1(ctx context.Context, request GetTeamsV1RequestObject) (GetTeamsV1ResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	limit := int32(10)
	if request.Params.Limit != nil {
		limit = int32(*request.Params.Limit)
	}

	result, err := a.db.GetTeams(ctx, limit, request.Params.Cursor)
	if err != nil {
		logger.Error("Failed to get teams from the DB", "error", err)

		var teamErr *teams.Error
		if errors.As(err, &teamErr) {
			switch teamErr.Reason {
			case teams.REASON_INVALID_CURSOR:
				return GetTeamsV1400JSONResponse{
					Code:    InvalidCursor,
					Message: "Passed in cursor is invalid",
				}, nil
			}
		}
		return GetTeamsV1500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get teams",
		}, nil
	}

	respTeams := []GlobalTeam{}
	for _, v := range result.Data {
		respTeams = append(respTeams, teamToApiGlobalTeam(v))
	}

	return GetTeamsV1200JSONResponse{
		Data:        respTeams,
		Cursor:      result.Cursor,
		HasNextPage: result.HasNextPage,
	}, nil
}

func (a *API) PostTeamsV1(ctx context.Context, request PostTeamsV1RequestObject) (PostTeamsV1ResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	team := apiGlobalTeamToTeam(*request.Body)

	createdTeam, err := teams.CreateTeam(ctx, a.db, team)
	if err != nil {
		logger.Error("Failed to create a team", "error", err)
		return PostTeamsV1500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create the team",
		}, nil
	}

	logger.Info("created new global team", slog.String("team-id", createdTeam.ID.String()))

	apiTeam := teamToApiGlobalTeam(createdTeam)
	return PostTeamsV1200JSONResponse(apiTeam), nil
}

func (a *API) GetTeamsV1TeamId(ctx context.Context, request GetTeamsV1TeamIdRequestObject) (GetTeamsV1TeamIdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	team, err := a.db.GetTeam(ctx, request.TeamId)
	if err != nil {
		logger.Error("Failed to fetch a team", "error", err)

		var teamErr *teams.Error
		if errors.As(err, &teamErr) {
			switch teamErr.Reason {
			case teams.REASON_TEAM_DOES_NOT_EXIST:
				return GetTeamsV1TeamId404JSONResponse{
					Code:    NotFound,
					Message: "Team does not exist",
				}, nil
			}
		}

		return GetTeamsV1TeamId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get team",
		}, nil
	}

	return GetTeamsV1TeamId200JSONResponse(teamToApiGlobalTeam(team)), nil
}

func (a *API) PatchTeamsV1TeamId(ctx context.Context, request PatchTeamsV1TeamIdRequestObject) (PatchTeamsV1TeamIdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	team := apiGlobalTeamToTeam(*request.Body)

	updatedTeam, err := teams.UpdateTeam(ctx, a.db, request.TeamId, team)
	if err != nil {
		logger.Error("failed to update team", slog.String("error", err.Error()))

		var teamErr *teams.Error
		if errors.As(err, &teamErr) {
			switch teamErr.Reason {
			case teams.REASON_TEAM_DOES_NOT_EXIST:
				return PatchTeamsV1TeamId404JSONResponse{
					Code:    NotFound,
					Message: "Team not found",
				}, nil
			}
		}

		return PatchTeamsV1TeamId500JSONResponse{
			Code:    InternalError,
			Message: "Updating team failed",
		}, nil
	}

	return PatchTeamsV1TeamId200JSONResponse(teamToApiGlobalTeam(updatedTeam)), nil
}

func (a *API) DeleteTeamsV1TeamId(ctx context.Context, request DeleteTeamsV1TeamIdRequestObject) (DeleteTeamsV1TeamIdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := a.db.DeleteTeam(ctx, request.TeamId)
	if err != nil {
		logger.Error("failed to delete team", slog.String("error", err.Error()))

		var teamErr *teams.Error
		if errors.As(err, &teamErr) {
			switch teamErr.Reason {
			case teams.REASON_TEAM_DOES_NOT_EXIST:
				return DeleteTeamsV1TeamId404JSONResponse{
					Code:    NotFound,
					Message: "Team not found",
				}, nil
			}
		}

		return DeleteTeamsV1TeamId500JSONResponse{
			Code:    InternalError,
			Message: "Deleting team failed",
		}, nil
	}

	return DeleteTeamsV1TeamId204Response{}, nil
}

func (a *API) GetTeamsV1TeamIdEvents(ctx context.Context, request GetTeamsV1TeamIdEventsRequestObject) (GetTeamsV1TeamIdEventsResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	limit := int32(10)
	if request.Params.Limit != nil {
		limit = int32(*request.Params.Limit)
	}

	result, err := a.db.GetEventTeamsByTeam(ctx, request.TeamId, limit, request.Params.Cursor)
	if err != nil {
		logger.Error("Failed to get event teams from the DB", "error", err)

		var teamErr *teams.Error
		if errors.As(err, &teamErr) {
			switch teamErr.Reason {
			case teams.REASON_INVALID_CURSOR:
				return GetTeamsV1TeamIdEvents400JSONResponse{
					Code:    InvalidCursor,
					Message: "Passed in cursor is invalid",
				}, nil
			}
		}
		return GetTeamsV1TeamIdEvents500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get event teams",
		}, nil
	}

	respEventTeams := []EventTeam{}
	for _, v := range result.Data {
		respEventTeams = append(respEventTeams, eventTeamToApiEventTeam(v))
	}

	return GetTeamsV1TeamIdEvents200JSONResponse{
		Data:        respEventTeams,
		Cursor:      result.Cursor,
		HasNextPage: result.HasNextPage,
	}, nil
}

func teamToApiGlobalTeam(team teams.Team) GlobalTeam {
	createdAt := team.CreatedAt
	return GlobalTeam{
		Id:        &team.ID,
		Name:      team.Name,
		CreatedAt: &createdAt,
	}
}

func apiGlobalTeamToTeam(team GlobalTeam) teams.Team {
	return teams.Team{
		ID:   *team.Id,
		Name: team.Name,
	}
}

func eventTeamToApiEventTeam(eventTeam teams.EventTeam) EventTeam {
	players := make([]TeamPlayer, len(eventTeam.Players))
	for i, p := range eventTeam.Players {
		var email *openapi_types.Email
		if p.PlayerInfo.Email != nil {
			e := openapi_types.Email(*p.PlayerInfo.Email)
			email = &e
		}
		players[i] = TeamPlayer{
			FirstName:      p.PlayerInfo.FirstName,
			LastName:       p.PlayerInfo.LastName,
			Email:          email,
			SourceType:     PlayerSourceType(p.SourceType.String()),
			RegistrationId: p.RegistrationID,
			AssignedAt:     p.AssignedAt,
		}
	}

	createdAt := eventTeam.CreatedAt

	var sourceType TeamSourceType
	switch eventTeam.SourceType {
	case teams.SOURCE_TEAM_REGISTRATION:
		sourceType = TeamSourceTypeTeamRegistration
	case teams.SOURCE_ADMIN_CREATED:
		sourceType = TeamSourceTypeAdminCreated
	case teams.SOURCE_MIXED:
		sourceType = TeamSourceTypeMixed
	}

	apiEventTeam := EventTeam{
		Id:         &eventTeam.ID,
		Version:    &eventTeam.Version,
		EventId:    &eventTeam.EventID,
		TeamId:     eventTeam.TeamID,
		Name:       eventTeam.Name,
		SourceType: sourceType,
		Players:    players,
		CreatedAt:  &createdAt,
	}

	if eventTeam.RegistrationID != nil {
		apiEventTeam.RegistrationId = eventTeam.RegistrationID
	}

	return apiEventTeam
}
