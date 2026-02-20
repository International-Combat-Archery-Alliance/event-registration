package api

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/google/uuid"
)

func (a *API) GetEventsV1EventIdGames(ctx context.Context, request GetEventsV1EventIdGamesRequestObject) (GetEventsV1EventIdGamesResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Check if schedule is public
	event, err := a.db.GetEvent(ctx, request.EventId)
	if err != nil {
		logger.Error("Failed to get event", "error", err)
		return GetEventsV1EventIdGames500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get games",
		}, nil
	}

	if !event.SchedulePublic {
		return GetEventsV1EventIdGames403JSONResponse{
			Code:    AuthError,
			Message: "Schedule is not public",
		}, nil
	}

	limit := int32(10)
	if request.Params.Limit != nil {
		limit = int32(*request.Params.Limit)
	}

	result, err := a.db.GetGamesForEvent(ctx, request.EventId, limit, request.Params.Cursor)
	if err != nil {
		logger.Error("Failed to get games from the DB", "error", err)

		var gameErr *games.Error
		if errors.As(err, &gameErr) {
			switch gameErr.Reason {
			case games.REASON_INVALID_CURSOR:
				return GetEventsV1EventIdGames400JSONResponse{
					Code:    InvalidCursor,
					Message: "Passed in cursor is invalid",
				}, nil
			}
		}
		return GetEventsV1EventIdGames500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get games",
		}, nil
	}

	respGames := []Game{}
	for _, v := range result.Data {
		respGames = append(respGames, gameToApiGame(v))
	}

	return GetEventsV1EventIdGames200JSONResponse{
		Data:        respGames,
		Cursor:      result.Cursor,
		HasNextPage: result.HasNextPage,
	}, nil
}

func (a *API) PostEventsV1EventIdGames(ctx context.Context, request PostEventsV1EventIdGamesRequestObject) (PostEventsV1EventIdGamesResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	request.Body.EventId = &request.EventId
	game, err := apiGameToGame(*request.Body)
	if err != nil {
		logger.Error("Failed to convert game into core type", "error", err)
		return PostEventsV1EventIdGames400JSONResponse{
			Code:    InvalidBody,
			Message: "Failed to create the game",
		}, nil
	}

	createdGame, err := games.CreateGame(ctx, a.db, game)
	if err != nil {
		logger.Error("Failed to create a game", "error", err)
		return PostEventsV1EventIdGames500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create the game",
		}, nil
	}

	logger.Info("created new game", slog.String("game-id", createdGame.ID.String()))

	apiGame := gameToApiGame(createdGame)
	return PostEventsV1EventIdGames200JSONResponse(apiGame), nil
}

func (a *API) GetEventsV1EventIdGamesGameId(ctx context.Context, request GetEventsV1EventIdGamesGameIdRequestObject) (GetEventsV1EventIdGamesGameIdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Check if schedule is public
	event, err := a.db.GetEvent(ctx, request.EventId)
	if err != nil {
		logger.Error("Failed to get event", "error", err)
		return GetEventsV1EventIdGamesGameId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get game",
		}, nil
	}

	if !event.SchedulePublic {
		return GetEventsV1EventIdGamesGameId403JSONResponse{
			Code:    AuthError,
			Message: "Schedule is not public",
		}, nil
	}

	game, err := a.db.GetGame(ctx, request.EventId, request.GameId)
	if err != nil {
		logger.Error("Failed to fetch a game", "error", err)

		var gameErr *games.Error
		if errors.As(err, &gameErr) {
			switch gameErr.Reason {
			case games.REASON_GAME_DOES_NOT_EXIST:
				return GetEventsV1EventIdGamesGameId404JSONResponse{
					Code:    NotFound,
					Message: "Game does not exist",
				}, nil
			}
		}

		return GetEventsV1EventIdGamesGameId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get game",
		}, nil
	}

	return GetEventsV1EventIdGamesGameId200JSONResponse(gameToApiGame(game)), nil
}

func (a *API) PatchEventsV1EventIdGamesGameId(ctx context.Context, request PatchEventsV1EventIdGamesGameIdRequestObject) (PatchEventsV1EventIdGamesGameIdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	request.Body.Id = &request.GameId
	request.Body.EventId = &request.EventId
	request.Body.Version = ptrInt(1)

	game, err := apiGameToGame(*request.Body)
	if err != nil {
		logger.Error("Invalid game body", slog.String("error", err.Error()))
		return PatchEventsV1EventIdGamesGameId400JSONResponse{
			Code:    InvalidBody,
			Message: "Invalid game body",
		}, nil
	}

	// Check if this is a result update
	if request.Body.Status == Completed {
		// This is a result recording
		if request.Body.Team1Score == nil || request.Body.Team2Score == nil || request.Body.WinnerId == nil {
			return PatchEventsV1EventIdGamesGameId400JSONResponse{
				Code:    InvalidBody,
				Message: "Missing required result fields",
			}, nil
		}

		var roundResults []RoundResult
		if request.Body.RoundResults != nil {
			roundResults = *request.Body.RoundResults
		}

		result := games.GameResult{
			Team1Score:   *request.Body.Team1Score,
			Team2Score:   *request.Body.Team2Score,
			WinnerID:     *request.Body.WinnerId,
			RoundResults: apiRoundResultsToRoundResults(roundResults),
		}

		// Get admin email from context (assuming it's set by auth middleware)
		recordedBy := "admin@example.com" // TODO: Get from context

		updatedGame, err := games.RecordGameResult(ctx, a.db, request.EventId, request.GameId, result, recordedBy)
		if err != nil {
			logger.Error("failed to record game result", slog.String("error", err.Error()))

			var gameErr *games.Error
			if errors.As(err, &gameErr) {
				switch gameErr.Reason {
				case games.REASON_GAME_DOES_NOT_EXIST:
					return PatchEventsV1EventIdGamesGameId404JSONResponse{
						Code:    NotFound,
						Message: "Game not found",
					}, nil
				case games.REASON_GAME_ALREADY_HAS_RESULT:
					return PatchEventsV1EventIdGamesGameId400JSONResponse{
						Code:    AlreadyExists,
						Message: "Game already has results recorded",
					}, nil
				}
			}

			return PatchEventsV1EventIdGamesGameId500JSONResponse{
				Code:    InternalError,
				Message: "Recording game result failed",
			}, nil
		}

		// Recalculate standings
		err = a.db.(interface {
			RecalculateStandings(context.Context, uuid.UUID) error
		}).RecalculateStandings(ctx, request.EventId)
		if err != nil {
			logger.Error("failed to recalculate standings", slog.String("error", err.Error()))
		}

		return PatchEventsV1EventIdGamesGameId200JSONResponse(gameToApiGame(updatedGame)), nil
	}

	// Regular game update
	updatedGame, err := games.UpdateGame(ctx, a.db, request.EventId, request.GameId, game)
	if err != nil {
		logger.Error("failed to update game", slog.String("error", err.Error()))

		var gameErr *games.Error
		if errors.As(err, &gameErr) {
			switch gameErr.Reason {
			case games.REASON_GAME_DOES_NOT_EXIST:
				return PatchEventsV1EventIdGamesGameId404JSONResponse{
					Code:    NotFound,
					Message: "Game not found",
				}, nil
			case games.REASON_CANNOT_MODIFY_COMPLETED_GAME:
				return PatchEventsV1EventIdGamesGameId400JSONResponse{
					Code:    InputValidationError,
					Message: "Cannot modify a completed game",
				}, nil
			}
		}

		return PatchEventsV1EventIdGamesGameId500JSONResponse{
			Code:    InternalError,
			Message: "Updating game failed",
		}, nil
	}

	return PatchEventsV1EventIdGamesGameId200JSONResponse(gameToApiGame(updatedGame)), nil
}

func (a *API) DeleteEventsV1EventIdGamesGameId(ctx context.Context, request DeleteEventsV1EventIdGamesGameIdRequestObject) (DeleteEventsV1EventIdGamesGameIdResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := games.DeleteGame(ctx, a.db, request.EventId, request.GameId)
	if err != nil {
		logger.Error("failed to delete game", slog.String("error", err.Error()))

		var gameErr *games.Error
		if errors.As(err, &gameErr) {
			switch gameErr.Reason {
			case games.REASON_GAME_DOES_NOT_EXIST:
				return DeleteEventsV1EventIdGamesGameId404JSONResponse{
					Code:    NotFound,
					Message: "Game not found",
				}, nil
			case games.REASON_CANNOT_DELETE_COMPLETED_GAME:
				return DeleteEventsV1EventIdGamesGameId400JSONResponse{
					Code:    InputValidationError,
					Message: "Cannot delete a completed game",
				}, nil
			}
		}

		return DeleteEventsV1EventIdGamesGameId500JSONResponse{
			Code:    InternalError,
			Message: "Deleting game failed",
		}, nil
	}

	return DeleteEventsV1EventIdGamesGameId204Response{}, nil
}

func gameToApiGame(game games.Game) Game {
	var roundResultsSlice []RoundResult
	if game.RoundResults != nil {
		roundResultsSlice = make([]RoundResult, len(game.RoundResults))
		for i, r := range game.RoundResults {
			roundResultsSlice[i] = RoundResult{
				RoundNumber:  r.RoundNumber,
				WinnerTeamId: r.WinnerTeamID,
			}
		}
	}
	roundResults := &roundResultsSlice

	var status GameStatus
	switch game.Status {
	case games.STATUS_SCHEDULED:
		status = Scheduled
	case games.STATUS_IN_PROGRESS:
		status = InProgress
	case games.STATUS_COMPLETED:
		status = Completed
	}

	return Game{
		Id:            &game.ID,
		Version:       &game.Version,
		EventId:       &game.EventID,
		Team1Id:       game.Team1ID,
		Team2Id:       game.Team2ID,
		ScheduledTime: game.ScheduledTime,
		Location:      game.Location,
		Status:        status,
		Team1Score:    game.Team1Score,
		Team2Score:    game.Team2Score,
		WinnerId:      game.WinnerID,
		RoundResults:  roundResults,
		RecordedAt:    game.RecordedAt,
		RecordedBy:    game.RecordedBy,
	}
}

func apiGameToGame(game Game) (games.Game, error) {
	var status games.GameStatus
	switch game.Status {
	case Scheduled:
		status = games.STATUS_SCHEDULED
	case InProgress:
		status = games.STATUS_IN_PROGRESS
	case Completed:
		status = games.STATUS_COMPLETED
	}

	var roundResults []games.RoundResult
	if game.RoundResults != nil {
		roundResults = make([]games.RoundResult, len(*game.RoundResults))
		for i, r := range *game.RoundResults {
			roundResults[i] = games.RoundResult{
				RoundNumber:  r.RoundNumber,
				WinnerTeamID: r.WinnerTeamId,
			}
		}
	}

	return games.Game{
		ID:            *game.Id,
		Version:       *game.Version,
		EventID:       *game.EventId,
		Team1ID:       game.Team1Id,
		Team2ID:       game.Team2Id,
		ScheduledTime: game.ScheduledTime,
		Location:      game.Location,
		Status:        status,
		Team1Score:    game.Team1Score,
		Team2Score:    game.Team2Score,
		WinnerID:      game.WinnerId,
		RoundResults:  roundResults,
		RecordedAt:    game.RecordedAt,
		RecordedBy:    game.RecordedBy,
	}, nil
}

func apiRoundResultsToRoundResults(roundResults []RoundResult) []games.RoundResult {
	result := make([]games.RoundResult, len(roundResults))
	for i, r := range roundResults {
		result[i] = games.RoundResult{
			RoundNumber:  r.RoundNumber,
			WinnerTeamID: r.WinnerTeamId,
		}
	}
	return result
}
