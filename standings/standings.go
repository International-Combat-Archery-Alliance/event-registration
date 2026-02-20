package standings

import (
	"context"

	"github.com/google/uuid"
)

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

type GetStandingsResponse struct {
	Data []Standing
}

type Repository interface {
	GetStandingsForEvent(ctx context.Context, eventID uuid.UUID) (GetStandingsResponse, error)
	UpdateStandings(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID, standing Standing) error
}

func CalculateStandings(games []GameInfo) GetStandingsResponse {
	standingsMap := make(map[uuid.UUID]*Standing)

	for _, game := range games {
		if game.Status != "completed" || game.WinnerID == nil {
			continue
		}

		// Initialize team standings if not exists
		if _, ok := standingsMap[game.Team1ID]; !ok {
			standingsMap[game.Team1ID] = &Standing{
				TeamID:   game.Team1ID,
				TeamName: game.Team1Name,
			}
		}
		if _, ok := standingsMap[game.Team2ID]; !ok {
			standingsMap[game.Team2ID] = &Standing{
				TeamID:   game.Team2ID,
				TeamName: game.Team2Name,
			}
		}

		team1 := standingsMap[game.Team1ID]
		team2 := standingsMap[game.Team2ID]

		// Update games played
		team1.GamesPlayed++
		team2.GamesPlayed++

		// Update wins/losses
		if *game.WinnerID == game.Team1ID {
			team1.Wins++
			team2.Losses++
		} else {
			team2.Wins++
			team1.Losses++
		}

		// Update points
		if game.Team1Score != nil {
			team1.PointsFor += *game.Team1Score
			team2.PointsAgainst += *game.Team1Score
		}
		if game.Team2Score != nil {
			team2.PointsFor += *game.Team2Score
			team1.PointsAgainst += *game.Team2Score
		}
	}

	// Calculate win percentages and convert to slice
	standings := make([]Standing, 0, len(standingsMap))
	for _, standing := range standingsMap {
		if standing.GamesPlayed > 0 {
			standing.WinPercentage = float64(standing.Wins) / float64(standing.GamesPlayed)
		}
		standings = append(standings, *standing)
	}

	return GetStandingsResponse{Data: standings}
}

type GameInfo struct {
	Team1ID    uuid.UUID
	Team1Name  string
	Team2ID    uuid.UUID
	Team2Name  string
	Status     string
	Team1Score *int
	Team2Score *int
	WinnerID   *uuid.UUID
}
