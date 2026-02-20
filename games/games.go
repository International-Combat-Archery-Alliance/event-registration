//go:generate go tool stringer -type=GameStatus

package games

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type GameStatus int

const (
	STATUS_SCHEDULED GameStatus = iota
	STATUS_IN_PROGRESS
	STATUS_COMPLETED
)

type Game struct {
	ID            uuid.UUID
	Version       int
	EventID       uuid.UUID
	Team1ID       uuid.UUID
	Team2ID       uuid.UUID
	ScheduledTime time.Time
	Location      string
	Status        GameStatus

	// Results (only populated when Status is STATUS_COMPLETED)
	Team1Score   *int
	Team2Score   *int
	WinnerID     *uuid.UUID
	RoundResults []RoundResult
	RecordedAt   *time.Time
	RecordedBy   *string
}

type RoundResult struct {
	RoundNumber  int
	WinnerTeamID uuid.UUID
}

type GetGamesResponse struct {
	Data        []Game
	Cursor      *string
	HasNextPage bool
}

type Repository interface {
	GetGame(ctx context.Context, eventID uuid.UUID, gameID uuid.UUID) (Game, error)
	GetGamesForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (GetGamesResponse, error)
	CreateGame(ctx context.Context, game Game) error
	UpdateGame(ctx context.Context, game Game) error
	DeleteGame(ctx context.Context, eventID uuid.UUID, gameID uuid.UUID) error
	// RecordResult saves the game result and both team standings atomically
	RecordResult(ctx context.Context, game Game, team1Standing Standing, team2Standing Standing) error
	// GetStandingsForEvent retrieves all standings for an event
	GetStandingsForEvent(ctx context.Context, eventID uuid.UUID) (GetStandingsResponse, error)
}

// Standing represents a team's standing
type Standing struct {
	EventID       uuid.UUID
	TeamID        uuid.UUID
	TeamName      string
	Wins          int
	Losses        int
	PointsFor     int
	PointsAgainst int
	GamesPlayed   int
	WinPercentage float64
}

// GetStandingsResponse is the response for GetStandingsForEvent
type GetStandingsResponse struct {
	Data []Standing
}

type GameResult struct {
	Team1Score   int
	Team2Score   int
	WinnerID     uuid.UUID
	RoundResults []RoundResult
}

func CreateGame(ctx context.Context, repo Repository, game Game) (Game, error) {
	game.ID = uuid.New()
	game.Version = 1
	game.Status = STATUS_SCHEDULED

	err := repo.CreateGame(ctx, game)
	if err != nil {
		return Game{}, err
	}

	return game, nil
}

func UpdateGame(ctx context.Context, repo Repository, eventID uuid.UUID, gameID uuid.UUID, updates Game) (Game, error) {
	existingGame, err := repo.GetGame(ctx, eventID, gameID)
	if err != nil {
		return Game{}, err
	}

	if existingGame.Status == STATUS_COMPLETED {
		return Game{}, NewCannotModifyCompletedGameError("Cannot modify a game that has results recorded")
	}

	updatedGame := Game{
		ID:            existingGame.ID,
		Version:       existingGame.Version + 1,
		EventID:       existingGame.EventID,
		Team1ID:       updates.Team1ID,
		Team2ID:       updates.Team2ID,
		ScheduledTime: updates.ScheduledTime,
		Location:      updates.Location,
		Status:        existingGame.Status,
	}

	err = repo.UpdateGame(ctx, updatedGame)
	if err != nil {
		return Game{}, err
	}

	return updatedGame, nil
}

func RecordGameResult(ctx context.Context, repo Repository, eventID uuid.UUID, gameID uuid.UUID, result GameResult, recordedBy string) (Game, error) {
	game, err := repo.GetGame(ctx, eventID, gameID)
	if err != nil {
		return Game{}, err
	}

	if game.Status == STATUS_COMPLETED {
		return Game{}, NewGameAlreadyHasResultError("Game already has results recorded")
	}

	game.Version++
	game.Status = STATUS_COMPLETED
	game.Team1Score = &result.Team1Score
	game.Team2Score = &result.Team2Score
	game.WinnerID = &result.WinnerID
	game.RoundResults = result.RoundResults
	now := time.Now().UTC()
	game.RecordedAt = &now
	game.RecordedBy = &recordedBy

	// Calculate standings updates
	team1Standing, team2Standing := calculateStandingsFromResult(game, result)

	err = repo.RecordResult(ctx, game, team1Standing, team2Standing)
	if err != nil {
		return Game{}, err
	}

	return game, nil
}

func calculateStandingsFromResult(game Game, result GameResult) (Standing, Standing) {
	// This is a simplified version - in reality you'd fetch existing standings and update them
	// For now, we'll create new standings based on this game result

	team1Standing := Standing{
		EventID:       game.EventID,
		TeamID:        game.Team1ID,
		GamesPlayed:   1,
		PointsFor:     result.Team1Score,
		PointsAgainst: result.Team2Score,
	}

	team2Standing := Standing{
		EventID:       game.EventID,
		TeamID:        game.Team2ID,
		GamesPlayed:   1,
		PointsFor:     result.Team2Score,
		PointsAgainst: result.Team1Score,
	}

	if result.WinnerID == game.Team1ID {
		team1Standing.Wins = 1
		team2Standing.Losses = 1
	} else {
		team2Standing.Wins = 1
		team1Standing.Losses = 1
	}

	return team1Standing, team2Standing
}

func DeleteGame(ctx context.Context, repo Repository, eventID uuid.UUID, gameID uuid.UUID) error {
	game, err := repo.GetGame(ctx, eventID, gameID)
	if err != nil {
		return err
	}

	if game.Status == STATUS_COMPLETED {
		return NewCannotDeleteCompletedGameError("Cannot delete a game that has results recorded")
	}

	return repo.DeleteGame(ctx, eventID, gameID)
}
