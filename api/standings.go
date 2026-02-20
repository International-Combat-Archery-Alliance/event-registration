package api

import (
	"context"
	"errors"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/standings"
)

func (a *API) GetEventsV1EventIdStandings(ctx context.Context, request GetEventsV1EventIdStandingsRequestObject) (GetEventsV1EventIdStandingsResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Check if schedule is public
	event, err := a.db.GetEvent(ctx, request.EventId)
	if err != nil {
		logger.Error("Failed to get event", "error", err)
		return GetEventsV1EventIdStandings500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get standings",
		}, nil
	}

	if !event.SchedulePublic {
		return GetEventsV1EventIdStandings403JSONResponse{
			Code:    AuthError,
			Message: "Schedule is not public",
		}, nil
	}

	result, err := a.db.GetStandingsForEvent(ctx, request.EventId)
	if err != nil {
		logger.Error("Failed to get standings from the DB", "error", err)

		var standingsErr *standings.Error
		if errors.As(err, &standingsErr) {
			return GetEventsV1EventIdStandings500JSONResponse{
				Code:    InternalError,
				Message: "Failed to get standings",
			}, nil
		}
		return GetEventsV1EventIdStandings500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get standings",
		}, nil
	}

	respStandings := []Standing{}
	for _, v := range result.Data {
		respStandings = append(respStandings, standingToApiStanding(v))
	}

	return GetEventsV1EventIdStandings200JSONResponse{
		Data: respStandings,
	}, nil
}

func standingToApiStanding(standing standings.Standing) Standing {
	return Standing{
		EventId:       standing.EventID,
		TeamId:        standing.TeamID,
		TeamName:      standing.TeamName,
		Wins:          standing.Wins,
		Losses:        standing.Losses,
		PointsFor:     standing.PointsFor,
		PointsAgainst: standing.PointsAgainst,
		GamesPlayed:   standing.GamesPlayed,
		WinPercentage: float32(standing.WinPercentage),
	}
}
