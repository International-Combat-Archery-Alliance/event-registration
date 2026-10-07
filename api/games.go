package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
)

func (a *API) GetEventsV1EventIdGames(ctx context.Context, request GetEventsV1EventIdGamesRequestObject) (GetEventsV1EventIdGamesResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "GetEventsV1EventIdGames")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if _, err := a.db.GetEvent(ctx, request.EventId); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch event for games", "error", err)

		var eventErr *events.Error
		if errors.As(err, &eventErr) && eventErr.Reason == events.REASON_EVENT_DOES_NOT_EXIST {
			return GetEventsV1EventIdGames404JSONResponse{
				Code:    NotFound,
				Message: "Event not found",
			}, nil
		}
		return GetEventsV1EventIdGames500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get games",
		}, nil
	}

	stored, err := a.db.ListGamesForEvent(ctx, request.EventId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch games", "error", err)

		return GetEventsV1EventIdGames500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get games",
		}, nil
	}

	names, err := a.teamNames(ctx, teamIDsInGames(stored))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to join team names", "error", err)

		return GetEventsV1EventIdGames500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get games",
		}, nil
	}

	resp := make([]Game, 0, len(stored))
	for _, g := range stored {
		conv, err := gameToApiGame(g, names)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error("Failed to convert game", "error", err)

			return GetEventsV1EventIdGames500JSONResponse{
				Code:    InternalError,
				Message: "Failed to get games",
			}, nil
		}
		resp = append(resp, conv)
	}

	return GetEventsV1EventIdGames200JSONResponse{Games: resp}, nil
}

func (a *API) GetEventsV1EventIdStandings(ctx context.Context, request GetEventsV1EventIdStandingsRequestObject) (GetEventsV1EventIdStandingsResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "GetEventsV1EventIdStandings")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if _, err := a.db.GetEvent(ctx, request.EventId); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch event for standings", "error", err)

		var eventErr *events.Error
		if errors.As(err, &eventErr) && eventErr.Reason == events.REASON_EVENT_DOES_NOT_EXIST {
			return GetEventsV1EventIdStandings404JSONResponse{
				Code:    NotFound,
				Message: "Event not found",
			}, nil
		}
		return GetEventsV1EventIdStandings500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get standings",
		}, nil
	}

	participations, err := a.db.ListParticipationsForEvent(ctx, request.EventId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch participations", "error", err)

		return GetEventsV1EventIdStandings500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get standings",
		}, nil
	}

	confirmed := make([]uuid.UUID, 0, len(participations))
	for _, p := range participations {
		if p.Status == teams.ParticipationStatusConfirmed {
			confirmed = append(confirmed, p.TeamID)
		}
	}

	stored, err := a.db.ListGamesForEvent(ctx, request.EventId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch games for standings", "error", err)

		return GetEventsV1EventIdStandings500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get standings",
		}, nil
	}

	names, err := a.teamNames(ctx, confirmed)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to join team names", "error", err)

		return GetEventsV1EventIdStandings500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get standings",
		}, nil
	}

	derived := games.ComputeStandings(stored, confirmed, names)
	resp := make([]Standing, 0, len(derived))
	for _, s := range derived {
		resp = append(resp, Standing{
			TeamId:   s.TeamID,
			TeamName: displayStandingName(s),
			Gp:       s.GP,
			Wins:     s.Wins,
			Losses:   s.Losses,
			Pf:       s.PF,
			Pa:       s.PA,
			Net:      s.Net,
			Rank:     s.Rank,
			Dns:      s.DNS,
		})
	}

	return GetEventsV1EventIdStandings200JSONResponse{Standings: resp}, nil
}

func displayStandingName(s games.TeamStanding) string {
	if s.TeamName != "" {
		return s.TeamName
	}
	return s.TeamID.String()
}

func (a *API) PostEventsV1EventIdGamesGenerate(ctx context.Context, request PostEventsV1EventIdGamesGenerateRequestObject) (PostEventsV1EventIdGamesGenerateResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostEventsV1EventIdGamesGenerate")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	event, err := a.db.GetEvent(ctx, request.EventId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch event for generate", "error", err)

		var eventErr *events.Error
		if errors.As(err, &eventErr) && eventErr.Reason == events.REASON_EVENT_DOES_NOT_EXIST {
			return PostEventsV1EventIdGamesGenerate404JSONResponse{
				Code:    NotFound,
				Message: "Event not found",
			}, nil
		}
		return PostEventsV1EventIdGamesGenerate500JSONResponse{
			Code:    InternalError,
			Message: "Failed to generate schedule",
		}, nil
	}

	if time.Now().Before(event.RegistrationCloseTime) {
		return PostEventsV1EventIdGamesGenerate409JSONResponse{
			Code:    Conflict,
			Message: "Registration is still open",
		}, nil
	}
	if event.Status.NormalizeDefault() != events.EventStatusInProgress {
		return PostEventsV1EventIdGamesGenerate409JSONResponse{
			Code:    Conflict,
			Message: "Event must be IN_PROGRESS to generate",
		}, nil
	}

	participations, err := a.db.ListParticipationsForEvent(ctx, request.EventId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch participations for generate", "error", err)

		return PostEventsV1EventIdGamesGenerate500JSONResponse{
			Code:    InternalError,
			Message: "Failed to generate schedule",
		}, nil
	}

	confirmed := make([]uuid.UUID, 0, len(participations))
	for _, p := range participations {
		if p.Status == teams.ParticipationStatusConfirmed {
			confirmed = append(confirmed, p.TeamID)
		}
	}
	if len(confirmed) == 0 {
		return PostEventsV1EventIdGamesGenerate409JSONResponse{
			Code:    Conflict,
			Message: "No CONFIRMED teams to schedule",
		}, nil
	}

	existing, err := a.db.ListGamesForEvent(ctx, request.EventId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch existing games", "error", err)

		return PostEventsV1EventIdGamesGenerate500JSONResponse{
			Code:    InternalError,
			Message: "Failed to generate schedule",
		}, nil
	}

	var generated []games.Game
	names := map[string]string{}
	switch request.Body.Format {
	case ROUNDROBIN:
		if request.Body.Round != nil {
			return PostEventsV1EventIdGamesGenerate400JSONResponse{
				Code:    InvalidBody,
				Message: "Round applies to Swiss only",
			}, nil
		}
		generated, err = a.generateRoundRobin(ctx, request, existing, confirmed)
	case SWISS:
		if request.Body.Mirror != nil && *request.Body.Mirror {
			return PostEventsV1EventIdGamesGenerate400JSONResponse{
				Code:    InvalidBody,
				Message: "Mirror applies to round robin only",
			}, nil
		}
		names, err = a.teamNames(ctx, confirmed)
		if err == nil {
			generated, err = a.generateSwiss(ctx, request, existing, confirmed, names)
		}
	default:
		return PostEventsV1EventIdGamesGenerate400JSONResponse{
			Code:    InvalidBody,
			Message: "Unknown format",
		}, nil
	}
	if err != nil {
		var conflict *conflictError
		if errors.As(err, &conflict) {
			return PostEventsV1EventIdGamesGenerate409JSONResponse{
				Code:    Conflict,
				Message: conflict.msg,
			}, nil
		}
		var gameErr *games.Error
		if errors.As(err, &gameErr) {
			switch gameErr.Reason {
			case games.REASON_NO_VALID_PAIRING:
				return PostEventsV1EventIdGamesGenerate409JSONResponse{
					Code:    Conflict,
					Message: "No valid pairing without rematch",
				}, nil
			case games.REASON_TIMEOUT, games.REASON_FAILED_TO_WRITE, games.REASON_FAILED_TO_FETCH:
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				logger.Error("Failed to generate schedule", "error", err)

				return PostEventsV1EventIdGamesGenerate500JSONResponse{
					Code:    InternalError,
					Message: "Failed to generate schedule",
				}, nil
			default:
				return PostEventsV1EventIdGamesGenerate400JSONResponse{
					Code:    InvalidBody,
					Message: "Invalid schedule",
				}, nil
			}
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to generate schedule", "error", err)

		return PostEventsV1EventIdGamesGenerate500JSONResponse{
			Code:    InternalError,
			Message: "Failed to generate schedule",
		}, nil
	}

	if len(names) == 0 {
		names, err = a.teamNames(ctx, confirmed)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error("Failed to join team names", "error", err)

			return PostEventsV1EventIdGamesGenerate500JSONResponse{
				Code:    InternalError,
				Message: "Failed to generate schedule",
			}, nil
		}
	}

	resp := make([]Game, 0, len(generated))
	for _, g := range generated {
		conv, err := gameToApiGame(g, names)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error("Failed to convert game", "error", err)

			return PostEventsV1EventIdGamesGenerate500JSONResponse{
				Code:    InternalError,
				Message: "Failed to generate schedule",
			}, nil
		}
		resp = append(resp, conv)
	}

	return PostEventsV1EventIdGamesGenerate200JSONResponse{Games: resp}, nil
}

type conflictError struct {
	msg string
}

func (e *conflictError) Error() string {
	return e.msg
}

func replaceRequested(body GenerateRequest) bool {
	return body.Replace != nil && *body.Replace
}

func (a *API) generateRoundRobin(ctx context.Context, request PostEventsV1EventIdGamesGenerateRequestObject, existing []games.Game, confirmed []uuid.UUID) ([]games.Game, error) {
	// Round robin owns the whole qualifying schedule: replace resets it
	// entirely (any format's unplayed rows included), never results.
	if hasResults(existing) {
		return nil, &conflictError{msg: "Results exist; replace must not destroy them"}
	}
	if len(existing) > 0 && !replaceRequested(*request.Body) {
		return nil, &conflictError{msg: "Games already exist; pass replace to regenerate"}
	}

	if len(existing) > 0 {
		// Delete-then-create is non-atomic (a 16-team schedule exceeds
		// the transaction item cap): a crash between loses the schedule
		// and the admin regenerates.
		var ids []uuid.UUID
		for _, g := range existing {
			ids = append(ids, g.ID)
		}
		if err := a.db.DeleteGames(ctx, request.EventId, ids); err != nil {
			return nil, err
		}
	}

	mirror := request.Body.Mirror != nil && *request.Body.Mirror
	generated, err := games.GenerateRoundRobin(request.EventId, confirmed, mirror)
	if err != nil {
		return nil, err
	}
	if err := a.db.CreateGames(ctx, generated); err != nil {
		return nil, err
	}
	return generated, nil
}

func (a *API) generateSwiss(ctx context.Context, request PostEventsV1EventIdGamesGenerateRequestObject, existing []games.Game, confirmed []uuid.UUID, names map[string]string) ([]games.Game, error) {
	round := nextSwissRound(existing)
	if request.Body.Round != nil {
		if *request.Body.Round <= maxRound(existing) {
			return nil, &conflictError{msg: "Swiss round already generated"}
		}
		round = *request.Body.Round
	}

	if replaceRequested(*request.Body) {
		if err := a.replaceSwissRound(ctx, request.EventId, existing, round); err != nil {
			return nil, err
		}
		existing = removeRound(existing, round)
	}

	generated, err := games.GenerateSwissRound(request.EventId, confirmed, names, existing, round)
	if err != nil {
		return nil, err
	}
	if err := a.db.CreateGames(ctx, generated); err != nil {
		return nil, err
	}
	return generated, nil
}

// replaceSwissRound deletes the target round's unplayed games so the round
// can be regenerated; any result in the round blocks the replace.
func (a *API) replaceSwissRound(ctx context.Context, eventID uuid.UUID, existing []games.Game, round int) error {
	var ids []uuid.UUID
	for _, g := range existing {
		if g.Round != round {
			continue
		}
		switch g.Status {
		case games.GameStatusCompleted, games.GameStatusForfeit, games.GameStatusDoubleForfeit:
			return &conflictError{msg: "Round holds results; replace must not destroy them"}
		default:
			ids = append(ids, g.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return a.db.DeleteGames(ctx, eventID, ids)
}

func removeRound(existing []games.Game, round int) []games.Game {
	out := existing[:0]
	for _, g := range existing {
		if g.Round != round {
			out = append(out, g)
		}
	}
	return out
}

func hasResults(existing []games.Game) bool {
	for _, g := range existing {
		switch g.Status {
		case games.GameStatusCompleted, games.GameStatusForfeit, games.GameStatusDoubleForfeit:
			return true
		}
	}
	return false
}

func maxRound(existing []games.Game) int {
	max := 0
	for _, g := range existing {
		if g.Round > max {
			max = g.Round
		}
	}
	return max
}

func nextSwissRound(existing []games.Game) int {
	return maxRound(existing) + 1
}

func (a *API) PutEventsV1EventIdGamesGameId(ctx context.Context, request PutEventsV1EventIdGamesGameIdRequestObject) (PutEventsV1EventIdGamesGameIdResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PutEventsV1EventIdGamesGameId")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	stored, err := a.db.GetGame(ctx, request.EventId, request.GameId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch game", "error", err)

		var gameErr *games.Error
		if errors.As(err, &gameErr) && gameErr.Reason == games.REASON_GAME_DOES_NOT_EXIST {
			return PutEventsV1EventIdGamesGameId404JSONResponse{
				Code:    NotFound,
				Message: "Game not found",
			}, nil
		}
		return PutEventsV1EventIdGamesGameId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to update game",
		}, nil
	}

	if stored.EventID != request.EventId {
		return PutEventsV1EventIdGamesGameId404JSONResponse{
			Code:    NotFound,
			Message: "Game not found",
		}, nil
	}

	if request.Body.Version != stored.Version {
		return PutEventsV1EventIdGamesGameId409JSONResponse{
			Code:    Conflict,
			Message: "Version conflict; refetch and retry",
		}, nil
	}

	updated, err := applyGameUpdate(stored, *request.Body)
	if err != nil {
		span.RecordError(err)
		logger.Error("Invalid game update", slog.String("error", err.Error()))

		return PutEventsV1EventIdGamesGameId400JSONResponse{
			Code:    InvalidBody,
			Message: "Invalid game update",
		}, nil
	}

	if err := a.db.UpdateGameChecked(ctx, request.EventId, updated); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to update game", "error", err)

		var gameErr *games.Error
		if errors.As(err, &gameErr) {
			switch gameErr.Reason {
			case games.REASON_EVENT_FINALIZED:
				return PutEventsV1EventIdGamesGameId409JSONResponse{
					Code:    Conflict,
					Message: "Event is FINALIZED; scoring locked",
				}, nil
			case games.REASON_GAME_DOES_NOT_EXIST:
				// The game existed at read time, so this is a concurrent
				// write (version race, or a replace that deleted under
				// us): refetch resolves either way.
				return PutEventsV1EventIdGamesGameId409JSONResponse{
					Code:    Conflict,
					Message: "Version conflict; refetch and retry",
				}, nil
			}
		}
		return PutEventsV1EventIdGamesGameId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to update game",
		}, nil
	}

	names, err := a.teamNames(ctx, teamIDsInGames([]games.Game{updated}))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to join team names", "error", err)

		return PutEventsV1EventIdGamesGameId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to update game",
		}, nil
	}

	respGame, err := gameToApiGame(updated, names)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to convert game", "error", err)

		return PutEventsV1EventIdGamesGameId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to update game",
		}, nil
	}

	return PutEventsV1EventIdGamesGameId200JSONResponse{Game: respGame}, nil
}

func applyGameUpdate(stored games.Game, body PutGameRequest) (games.Game, error) {
	// PUT replaces the whole result: omitted scores clear (nil), so an
	// admin can also retract a mis-entered result pre-finalize.
	status, err := apiGameStatusToGameStatus(body.Status)
	if err != nil {
		return games.Game{}, err
	}

	updated := stored
	updated.Status = status
	updated.ScoreA = body.ScoreA
	updated.ScoreB = body.ScoreB
	updated.ForfeitSide = nil
	if body.ForfeitSide != nil {
		slot := games.GameSlot(string(*body.ForfeitSide))
		updated.ForfeitSide = &slot
	}
	updated.Version = stored.Version + 1

	if err := updated.Validate(); err != nil {
		return games.Game{}, err
	}
	return updated, nil
}

func (a *API) teamNames(ctx context.Context, ids []uuid.UUID) (map[string]string, error) {
	// Bounded fan-out: one read per participant (fields cap at 100,
	// typically 16). Unknown teams are skipped so a removed team cannot
	// break schedule reads.
	names := make(map[string]string, len(ids))
	for _, id := range ids {
		if _, ok := names[id.String()]; ok {
			continue
		}
		team, err := a.db.GetTeam(ctx, id)
		if err != nil {
			var teamErr *teams.Error
			if errors.As(err, &teamErr) && teamErr.Reason == teams.REASON_TEAM_DOES_NOT_EXIST {
				continue
			}
			return nil, err
		}
		names[id.String()] = team.Name
	}
	return names, nil
}

func teamIDsInGames(stored []games.Game) []uuid.UUID {
	var ids []uuid.UUID
	seen := map[uuid.UUID]bool{}
	for _, g := range stored {
		for _, side := range []games.Side{g.SideA, g.SideB} {
			if side.Real() && !seen[*side.TeamID] {
				seen[*side.TeamID] = true
				ids = append(ids, *side.TeamID)
			}
		}
	}
	return ids
}

func gameToApiGame(g games.Game, names map[string]string) (Game, error) {
	phase, err := gamePhaseToApiPhase(g.Phase)
	if err != nil {
		return Game{}, err
	}
	status, err := gameStatusToApiStatus(g.Status)
	if err != nil {
		return Game{}, err
	}

	return Game{
		Id:          &g.ID,
		EventId:     &g.EventID,
		Phase:       phase,
		Round:       g.Round,
		Seq:         g.Seq,
		Status:      status,
		SideA:       sideToApiSide(g.SideA, names),
		SideB:       sideToApiSide(g.SideB, names),
		ScoreA:      g.ScoreA,
		ScoreB:      g.ScoreB,
		Version:     &g.Version,
		ForfeitSide: forfeitSlotToApi(g.ForfeitSide),
	}, nil
}

func sideToApiSide(s games.Side, names map[string]string) GameSide {
	side := GameSide{IsBye: &s.IsBye}
	if s.TeamID != nil {
		teamID := *s.TeamID
		side.TeamId = &teamID
		if name, ok := names[s.TeamID.String()]; ok {
			side.TeamName = &name
		}
	}
	return side
}

func forfeitSlotToApi(slot *games.GameSlot) *GameForfeitSide {
	if slot == nil {
		return nil
	}
	s := GameForfeitSide(string(*slot))
	return &s
}

func gamePhaseToApiPhase(p games.GamePhase) (GamePhase, error) {
	switch p {
	case games.GamePhaseQualifying:
		return QUALIFYING, nil
	case games.GamePhasePlayoff:
		return PLAYOFF, nil
	default:
		return GamePhase(""), fmt.Errorf("unknown game phase: %q", string(p))
	}
}

func gameStatusToApiStatus(s games.GameStatus) (GameStatus, error) {
	switch s {
	case games.GameStatusScheduled:
		return SCHEDULED, nil
	case games.GameStatusCompleted:
		return COMPLETED, nil
	case games.GameStatusForfeit:
		return FORFEIT, nil
	case games.GameStatusDoubleForfeit:
		return DOUBLEFORFEIT, nil
	case games.GameStatusCancelled:
		return CANCELLED, nil
	case games.GameStatusBye:
		return BYE, nil
	default:
		return GameStatus(""), fmt.Errorf("unknown game status: %q", string(s))
	}
}

func apiGameStatusToGameStatus(s GameStatus) (games.GameStatus, error) {
	switch s {
	case SCHEDULED:
		return games.GameStatusScheduled, nil
	case COMPLETED:
		return games.GameStatusCompleted, nil
	case FORFEIT:
		return games.GameStatusForfeit, nil
	case DOUBLEFORFEIT:
		return games.GameStatusDoubleForfeit, nil
	case CANCELLED:
		return games.GameStatusCancelled, nil
	case BYE:
		return games.GameStatusBye, nil
	default:
		return games.GameStatus(""), fmt.Errorf("unknown game status: %q", string(s))
	}
}
