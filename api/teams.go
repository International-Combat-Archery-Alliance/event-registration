package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// ==================== Event Team Endpoints ====================

func (a *API) GetEventsV1EventIdTeams(ctx context.Context, request GetEventsV1EventIdTeamsRequestObject) (GetEventsV1EventIdTeamsResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	limit := int32(10)
	if request.Params.Limit != nil {
		limit = int32(*request.Params.Limit)
	}

	result, err := a.db.GetEventTeamsForEvent(ctx, request.EventId, limit, request.Params.Cursor)
	if err != nil {
		logger.Error("Failed to get event teams from the DB", "error", err)

		var teamErr *teams.Error
		if errors.As(err, &teamErr) {
			switch teamErr.Reason {
			case teams.REASON_INVALID_CURSOR:
				return GetEventsV1EventIdTeams400JSONResponse{
					Code:    InvalidCursor,
					Message: "Passed in cursor is invalid",
				}, nil
			}
		}
		return GetEventsV1EventIdTeams500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get teams",
		}, nil
	}

	respTeams := []Team{}
	for _, v := range result.Data {
		respTeams = append(respTeams, eventTeamToApiTeam(v))
	}

	return GetEventsV1EventIdTeams200JSONResponse{
		Data:        respTeams,
		Cursor:      result.Cursor,
		HasNextPage: result.HasNextPage,
	}, nil
}

func (a *API) PostEventsV1EventIdTeams(ctx context.Context, request PostEventsV1EventIdTeamsRequestObject) (PostEventsV1EventIdTeamsResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	request.Body.EventId = &request.EventId
	eventTeam, err := apiTeamToEventTeam(*request.Body)
	if err != nil {
		logger.Error("Failed to convert team into core type", "error", err)
		return PostEventsV1EventIdTeams400JSONResponse{
			Code:    InvalidBody,
			Message: "Failed to create the team",
		}, nil
	}

	createdEventTeam, err := teams.CreateEventTeam(ctx, a.db, eventTeam)
	if err != nil {
		logger.Error("Failed to create an event team", "error", err)
		return PostEventsV1EventIdTeams500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create the team",
		}, nil
	}

	logger.Info("created new event team", slog.String("event-team-id", createdEventTeam.ID.String()))

	apiTeam := eventTeamToApiTeam(createdEventTeam)
	return PostEventsV1EventIdTeams200JSONResponse(apiTeam), nil
}

func (a *API) GetEventsV1EventIdTeamsTeamId(ctx context.Context, request GetEventsV1EventIdTeamsTeamIdRequestObject) (GetEventsV1EventIdTeamsTeamIdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	eventTeam, err := a.db.GetEventTeam(ctx, request.EventId, request.TeamId)
	if err != nil {
		logger.Error("Failed to fetch an event team", "error", err)

		var teamErr *teams.Error
		if errors.As(err, &teamErr) {
			switch teamErr.Reason {
			case teams.REASON_TEAM_DOES_NOT_EXIST:
				return GetEventsV1EventIdTeamsTeamId404JSONResponse{
					Code:    NotFound,
					Message: "Team does not exist",
				}, nil
			}
		}

		return GetEventsV1EventIdTeamsTeamId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get team",
		}, nil
	}

	return GetEventsV1EventIdTeamsTeamId200JSONResponse(eventTeamToApiTeam(eventTeam)), nil
}

func (a *API) PatchEventsV1EventIdTeamsTeamId(ctx context.Context, request PatchEventsV1EventIdTeamsTeamIdRequestObject) (PatchEventsV1EventIdTeamsTeamIdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	request.Body.Id = &request.TeamId
	request.Body.EventId = &request.EventId
	request.Body.Version = ptrInt(1)

	eventTeam, err := apiTeamToEventTeam(*request.Body)
	if err != nil {
		logger.Error("Invalid team body", slog.String("error", err.Error()))
		return PatchEventsV1EventIdTeamsTeamId400JSONResponse{
			Code:    InvalidBody,
			Message: "Invalid team body",
		}, nil
	}

	updatedEventTeam, err := teams.UpdateEventTeam(ctx, a.db, request.EventId, request.TeamId, eventTeam)
	if err != nil {
		logger.Error("failed to update event team", slog.String("error", err.Error()))

		var teamErr *teams.Error
		if errors.As(err, &teamErr) {
			switch teamErr.Reason {
			case teams.REASON_TEAM_DOES_NOT_EXIST:
				return PatchEventsV1EventIdTeamsTeamId404JSONResponse{
					Code:    NotFound,
					Message: "Team not found",
				}, nil
			}
		}

		return PatchEventsV1EventIdTeamsTeamId500JSONResponse{
			Code:    InternalError,
			Message: "Updating team failed",
		}, nil
	}

	return PatchEventsV1EventIdTeamsTeamId200JSONResponse(eventTeamToApiTeam(updatedEventTeam)), nil
}

func (a *API) PostEventsV1EventIdTeamsTeamIdPlayers(ctx context.Context, request PostEventsV1EventIdTeamsTeamIdPlayersRequestObject) (PostEventsV1EventIdTeamsTeamIdPlayersResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	player := apiTeamPlayerToTeamPlayer(*request.Body)

	err := teams.AddPlayerToEventTeam(ctx, a.db, request.EventId, request.TeamId, player)
	if err != nil {
		logger.Error("failed to add player to event team", slog.String("error", err.Error()))

		var teamErr *teams.Error
		if errors.As(err, &teamErr) {
			switch teamErr.Reason {
			case teams.REASON_TEAM_DOES_NOT_EXIST:
				return PostEventsV1EventIdTeamsTeamIdPlayers404JSONResponse{
					Code:    NotFound,
					Message: "Team not found",
				}, nil
			case teams.REASON_PLAYER_ALREADY_ASSIGNED:
				return PostEventsV1EventIdTeamsTeamIdPlayers400JSONResponse{
					Code:    AlreadyExists,
					Message: "Player is already assigned to a team",
				}, nil
			}
		}

		return PostEventsV1EventIdTeamsTeamIdPlayers500JSONResponse{
			Code:    InternalError,
			Message: "Failed to add player to team",
		}, nil
	}

	// Return the updated event team
	eventTeam, err := a.db.GetEventTeam(ctx, request.EventId, request.TeamId)
	if err != nil {
		logger.Error("Failed to fetch updated event team", "error", err)
		return PostEventsV1EventIdTeamsTeamIdPlayers500JSONResponse{
			Code:    InternalError,
			Message: "Failed to add player to team",
		}, nil
	}

	return PostEventsV1EventIdTeamsTeamIdPlayers200JSONResponse(eventTeamToApiTeam(eventTeam)), nil
}

// ==================== Helper Functions ====================

func eventTeamToApiTeam(eventTeam teams.EventTeam) Team {
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

	apiTeam := Team{
		Id:         &eventTeam.ID,
		Version:    &eventTeam.Version,
		EventId:    &eventTeam.EventID,
		Name:       eventTeam.Name,
		SourceType: sourceType,
		Players:    players,
		CreatedAt:  &createdAt,
	}

	if eventTeam.RegistrationID != nil {
		apiTeam.RegistrationId = eventTeam.RegistrationID
	}

	return apiTeam
}

func apiTeamToEventTeam(team Team) (teams.EventTeam, error) {
	players := make([]teams.TeamPlayer, len(team.Players))
	for i, p := range team.Players {
		var sourceType teams.PlayerSourceType
		switch p.SourceType {
		case PlayerSourceTypeTeamRegistration:
			sourceType = teams.PLAYER_SOURCE_TEAM_REGISTRATION
		case PlayerSourceTypeIndividualRegistration:
			sourceType = teams.PLAYER_SOURCE_INDIVIDUAL_REGISTRATION
		}

		var email *string
		if p.Email != nil {
			e := string(*p.Email)
			email = &e
		}

		players[i] = teams.TeamPlayer{
			PlayerInfo: registration.PlayerInfo{
				FirstName: p.FirstName,
				LastName:  p.LastName,
				Email:     email,
			},
			SourceType:     sourceType,
			RegistrationID: p.RegistrationId,
			AssignedAt:     p.AssignedAt,
		}
	}

	var sourceType teams.TeamSourceType
	switch team.SourceType {
	case TeamSourceTypeTeamRegistration:
		sourceType = teams.SOURCE_TEAM_REGISTRATION
	case TeamSourceTypeAdminCreated:
		sourceType = teams.SOURCE_ADMIN_CREATED
	case TeamSourceTypeMixed:
		sourceType = teams.SOURCE_MIXED
	}

	// TeamID is not in the API spec yet, so we use a zero UUID for now
	// This will need to be updated when the API spec is updated
	var teamID uuid.UUID
	if team.RegistrationId != nil {
		// Use registration ID as a proxy for team ID if available
		teamID = *team.RegistrationId
	}

	return teams.EventTeam{
		ID:             *team.Id,
		Version:        *team.Version,
		EventID:        *team.EventId,
		TeamID:         teamID,
		Name:           team.Name,
		SourceType:     sourceType,
		RegistrationID: team.RegistrationId,
		Players:        players,
		CreatedAt:      *team.CreatedAt,
	}, nil
}

func apiTeamPlayerToTeamPlayer(player TeamPlayer) teams.TeamPlayer {
	var sourceType teams.PlayerSourceType
	switch player.SourceType {
	case PlayerSourceTypeTeamRegistration:
		sourceType = teams.PLAYER_SOURCE_TEAM_REGISTRATION
	case PlayerSourceTypeIndividualRegistration:
		sourceType = teams.PLAYER_SOURCE_INDIVIDUAL_REGISTRATION
	}

	var email *string
	if player.Email != nil {
		e := string(*player.Email)
		email = &e
	}

	return teams.TeamPlayer{
		PlayerInfo: registration.PlayerInfo{
			FirstName: player.FirstName,
			LastName:  player.LastName,
			Email:     email,
		},
		SourceType:     sourceType,
		RegistrationID: player.RegistrationId,
		AssignedAt:     player.AssignedAt,
	}
}

func ptrInt(i int) *int {
	return &i
}
