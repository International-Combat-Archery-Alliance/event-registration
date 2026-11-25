package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func (a *API) GetEventsV1EventIdGames(ctx context.Context, request GetEventsV1EventIdGamesRequestObject) (GetEventsV1EventIdGamesResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "GetEventsV1EventIdGames")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if _, found, err := a.loadEvent(ctx, request.EventId); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch event for games", "error", err)

		return GetEventsV1EventIdGames500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get games",
		}, nil
	} else if !found {
		return GetEventsV1EventIdGames404JSONResponse{
			Code:    NotFound,
			Message: "Event not found",
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

	names := a.teamNames(ctx, teamIDsInGames(stored))

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

	event, found, err := a.loadEvent(ctx, request.EventId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch event for standings", "error", err)

		return GetEventsV1EventIdStandings500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get standings",
		}, nil
	}
	if !found {
		return GetEventsV1EventIdStandings404JSONResponse{
			Code:    NotFound,
			Message: "Event not found",
		}, nil
	}
	order, err := tiebreakOrderForEvent(event)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Event carries an invalid tiebreak order", "error", err)

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

	// Every listed entry is fed to the derivation with its real status:
	// non-CONFIRMED teams must appear as DNS rather than vanish or (worse)
	// count as participants (RFC-0002 §5, D5).
	standingTeams := make([]games.StandingTeam, 0, len(participations))
	participantIDs := make([]uuid.UUID, 0, len(participations))
	for _, p := range participations {
		standingTeams = append(standingTeams, games.StandingTeam{TeamID: p.TeamID, Status: p.Status})
		participantIDs = append(participantIDs, p.TeamID)
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

	names := a.teamNames(ctx, participantIDs)
	derived := games.ComputeStandings(stored, standingTeams, names, order)
	resp := make([]Standing, 0, len(derived))
	for _, s := range derived {
		resp = append(resp, Standing{
			TeamId:         s.TeamID,
			TeamName:       displayStandingName(s),
			Gp:             s.GP,
			Wins:           s.Wins,
			Draws:          s.Draws,
			Losses:         s.Losses,
			Pf:             s.PF,
			Pa:             s.PA,
			Net:            s.Net,
			QualifyingRank: s.QualifyingRank,
			Dns:            s.DNS,
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

	event, found, err := a.loadEvent(ctx, request.EventId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to fetch event for generate", "error", err)

		return PostEventsV1EventIdGamesGenerate500JSONResponse{
			Code:    InternalError,
			Message: "Failed to generate schedule",
		}, nil
	}
	if !found {
		return PostEventsV1EventIdGamesGenerate404JSONResponse{
			Code:    NotFound,
			Message: "Event not found",
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

	order, err := tiebreakOrderForEvent(event)
	if err != nil {
		return a.generateErrorResponse(span, logger, err), nil
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

	// Build (and therefore validate) the whole replacement schedule before
	// touching storage: a generator failure can never leave a half-deleted
	// schedule behind.
	schedule, err := a.buildSchedule(ctx, event.ID, *request.Body, existing, confirmed, nextGameNo(event.LastGameNo, existing), order)
	if err != nil {
		return a.generateErrorResponse(span, logger, err), nil
	}

	// Validation needs no exclusivity, so the epoch grant comes after every
	// cheap 409 and after buildSchedule: a refused generate does not bump the
	// event Version or ScheduleGeneration. The grant still serializes the
	// writes against the event snapshot this handler validated, because any
	// concurrent schedule writer bumps that Version and makes the grant fail.
	epoch, err := a.db.BeginScheduleWrite(ctx, event.ID, event.Version)
	if err != nil {
		return a.generateErrorResponse(span, logger, err), nil
	}

	if err := a.applySchedule(ctx, event.ID, epoch, schedule); err != nil {
		return a.generateErrorResponse(span, logger, err), nil
	}

	// The response is the event's schedule after the write, not just the rows
	// this request wrote: preserved results are still on the schedule order.
	all := make([]games.Game, 0, len(schedule.games)+len(schedule.preserved))
	all = append(all, schedule.games...)
	all = append(all, schedule.preserved...)
	all = scheduleOrder(all)

	names := a.teamNames(ctx, teamIDsInGames(all))

	resp := make([]Game, 0, len(all))
	for _, g := range all {
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

// generatedSchedule is the fully validated result of a generate request: the
// games to write, the recorded results the regeneration kept out of the write
// set, and the unplayed game numbers it replaces. preserved rows are response
// material only; they are never deleted or rewritten.
type generatedSchedule struct {
	games     []games.Game
	preserved []games.Game
	delete    []int
}

// scheduleOrder returns games in schedule order: QUALIFYING before PLAYOFF,
// then round, then seq.
func scheduleOrder(in []games.Game) []games.Game {
	out := append([]games.Game(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Phase != out[j].Phase {
			return out[i].Phase == games.GamePhaseQualifying
		}
		if out[i].Round != out[j].Round {
			return out[i].Round < out[j].Round
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}

func (a *API) buildSchedule(ctx context.Context, eventID uuid.UUID, body GenerateRequest, existing []games.Game, confirmed []uuid.UUID, next int, order games.TiebreakOrder) (generatedSchedule, error) {
	switch body.Format {
	case ROUNDROBIN:
		if body.Round != nil {
			return generatedSchedule{}, errRoundAppliesToSwissOnly
		}
		return buildRoundRobin(eventID, body, existing, confirmed, next)
	case SWISS:
		if body.Mirror != nil && *body.Mirror {
			return generatedSchedule{}, errMirrorAppliesToRoundRobinOnly
		}
		names := a.teamNames(ctx, confirmed)
		return buildSwiss(ctx, eventID, body, existing, confirmed, names, next, order)
	default:
		return generatedSchedule{}, errUnknownFormat
	}
}

var (
	errRoundAppliesToSwissOnly       = &conflictError{msg: "Round applies to Swiss only", badRequest: true}
	errMirrorAppliesToRoundRobinOnly = &conflictError{msg: "Mirror applies to round robin only", badRequest: true}
	errUnknownFormat                 = &conflictError{msg: "Unknown format", badRequest: true}
)

// buildRoundRobin regenerates the whole qualifying schedule. Recorded
// results are never deleted: the delete set holds only unplayed rows, and a
// generated game that would land on a recorded result's slot is dropped when
// it is the same pairing. A different pairing at that slot means the
// regenerated field disagrees with what was played, so the request is
// refused (the admin can retract the result first).
func buildRoundRobin(eventID uuid.UUID, body GenerateRequest, existing []games.Game, confirmed []uuid.UUID, next int) (generatedSchedule, error) {
	if len(existing) > 0 && !replaceRequested(body) {
		return generatedSchedule{}, &conflictError{msg: "Games already exist; pass replace to regenerate"}
	}

	mirror := body.Mirror != nil && *body.Mirror
	generated, err := games.GenerateRoundRobin(eventID, confirmed, mirror)
	if err != nil {
		return generatedSchedule{}, err
	}
	generated, preserved, err := preserveRecordedResults(generated, existing)
	if err != nil {
		return generatedSchedule{}, err
	}
	minted, err := mintGameNos(generated, next)
	if err != nil {
		return generatedSchedule{}, err
	}

	return generatedSchedule{
		games:     minted,
		preserved: preserved,
		delete:    deletableGameNumbers(existing),
	}, nil
}

func buildSwiss(ctx context.Context, eventID uuid.UUID, body GenerateRequest, existing []games.Game, confirmed []uuid.UUID, names map[string]string, next int, order games.TiebreakOrder) (generatedSchedule, error) {
	max := maxRound(existing)
	round := max + 1
	if body.Round != nil {
		round = *body.Round
	}

	prior := existing
	var deleteNumbers []int
	if replaceRequested(body) {
		deleteNumbers = roundDeletableGameNumbers(existing, round)
		prior = withoutQualifyingRound(existing, round)
	} else if body.Round != nil && round != max+1 {
		if round <= max {
			return generatedSchedule{}, &conflictError{msg: "Swiss round already generated"}
		}
		return generatedSchedule{}, &conflictError{msg: fmt.Sprintf("Swiss rounds must be generated in order; next round is %d", max+1)}
	}

	generated, err := games.GenerateSwissRound(ctx, eventID, confirmed, names, prior, round, order)
	if err != nil {
		return generatedSchedule{}, err
	}
	// Recorded results in the regenerated round are part of the round, not
	// input to its pairing (only prior rounds feed standings): keep a result
	// whose slot still holds the same pairing, and refuse a mismatch.
	generated, preserved, err := preserveRecordedResults(generated, existing)
	if err != nil {
		return generatedSchedule{}, err
	}
	minted, err := mintGameNos(generated, next)
	if err != nil {
		return generatedSchedule{}, err
	}

	return generatedSchedule{
		games:     minted,
		preserved: preserved,
		delete:    deleteNumbers,
	}, nil
}

// applySchedule runs the generate protocol after the caller's epoch grant:
// reserve the minted game numbers on the event item first, then delete the
// swept unplayed rows, then write the new rows in chunked transactions that
// check the event epoch. Reserving first means a failure in the delete or
// write phase burns numbers rather than making a retry re-mint numbers that
// were briefly visible. The reservation makes minting exact even if another
// schedule writer starts between steps. What is not atomic: chunks are
// individually atomic, so a failure after some chunks have committed leaves a
// partial schedule; results are never part of the delete set and result slots
// block puts, so no result is destroyed. Retrying the replace sweeps the
// leftovers and preserves recorded results.
func (a *API) applySchedule(ctx context.Context, eventID uuid.UUID, epoch int, schedule generatedSchedule) error {
	if len(schedule.games) > 0 {
		last := schedule.games[len(schedule.games)-1].GameNo
		reservedEpoch, err := a.db.ReserveGameNumbers(ctx, eventID, epoch, last)
		if err != nil {
			return err
		}
		epoch = reservedEpoch
	}
	if len(schedule.delete) > 0 {
		if err := a.db.DeleteGames(ctx, eventID, epoch, schedule.delete); err != nil {
			return err
		}
	}
	if len(schedule.games) == 0 {
		return nil
	}
	return a.db.CreateGames(ctx, eventID, epoch, schedule.games)
}

func (a *API) generateErrorResponse(span trace.Span, logger *slog.Logger, err error) PostEventsV1EventIdGamesGenerateResponseObject {
	var conflict *conflictError
	if errors.As(err, &conflict) {
		if conflict.badRequest {
			return PostEventsV1EventIdGamesGenerate400JSONResponse{
				Code:    InvalidBody,
				Message: conflict.msg,
			}
		}
		return PostEventsV1EventIdGamesGenerate409JSONResponse{
			Code:    Conflict,
			Message: conflict.msg,
		}
	}

	var gameErr *games.Error
	if errors.As(err, &gameErr) {
		switch gameErr.Reason {
		case games.REASON_NO_VALID_PAIRING:
			return PostEventsV1EventIdGamesGenerate409JSONResponse{
				Code:    Conflict,
				Message: "No valid pairing without rematch",
			}
		case games.REASON_PAIRING_BUDGET_EXCEEDED:
			return PostEventsV1EventIdGamesGenerate409JSONResponse{
				Code:    Conflict,
				Message: "Pairing is too complex to resolve safely; correct scores or regenerate",
			}
		case games.REASON_EVENT_FINALIZED:
			return PostEventsV1EventIdGamesGenerate409JSONResponse{
				Code:    Conflict,
				Message: "Event is FINALIZED; schedule locked",
			}
		case games.REASON_VERSION_CONFLICT:
			return PostEventsV1EventIdGamesGenerate409JSONResponse{
				Code:    Conflict,
				Message: "Concurrent schedule change; refetch and retry",
			}
		case games.REASON_SLOT_OCCUPIED:
			return PostEventsV1EventIdGamesGenerate409JSONResponse{
				Code:    Conflict,
				Message: "A game slot is held by a recorded result or a concurrent game; retry the replace",
			}
		case games.REASON_INVALID_TIEBREAKER, games.REASON_TIMEOUT, games.REASON_FAILED_TO_WRITE,
			games.REASON_FAILED_TO_FETCH, games.REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL:
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			logger.Error("Failed to generate schedule", "error", err)

			return PostEventsV1EventIdGamesGenerate500JSONResponse{
				Code:    InternalError,
				Message: "Failed to generate schedule",
			}
		default:
			// Domain validation errors already carry the precise reason
			// (round gaps, field size, duplicate teams, ...).
			return PostEventsV1EventIdGamesGenerate400JSONResponse{
				Code:    InvalidBody,
				Message: gameErr.Message,
			}
		}
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	logger.Error("Failed to generate schedule", "error", err)

	return PostEventsV1EventIdGamesGenerate500JSONResponse{
		Code:    InternalError,
		Message: "Failed to generate schedule",
	}
}

type conflictError struct {
	msg        string
	badRequest bool
}

func (e *conflictError) Error() string {
	return e.msg
}

func replaceRequested(body GenerateRequest) bool {
	return body.Replace != nil && *body.Replace
}

// nextGameNo is the next game number to mint: one past both the event's
// LastGameNo high-water mark and any existing row (older rows predate the
// counter, so the max protects a legacy event). A deleted number is never
// reused because LastGameNo never moves backwards.
func nextGameNo(lastGameNo int, existing []games.Game) int {
	next := lastGameNo + 1
	for _, g := range existing {
		if g.GameNo >= next {
			next = g.GameNo + 1
		}
	}
	return next
}

func mintGameNos(generated []games.Game, next int) ([]games.Game, error) {
	out := make([]games.Game, 0, len(generated))
	for _, g := range generated {
		if next > 9999 {
			return nil, games.NewInvalidScheduleError("no game numbers remain (max 9999)", nil)
		}
		g.GameNo = next
		next++
		out = append(out, g)
	}
	return out, nil
}

// deletableGameNumbers returns the QUALIFYING rows a replace may delete:
// SCHEDULED, BYE, and CANCELLED rows carry no result. Recorded results and
// every PLAYOFF row are left alone.
func deletableGameNumbers(existing []games.Game) []int {
	var out []int
	for _, g := range existing {
		if g.Phase != games.GamePhaseQualifying || !g.Status.Deletable() {
			continue
		}
		out = append(out, g.GameNo)
	}
	return out
}

func roundDeletableGameNumbers(existing []games.Game, round int) []int {
	var out []int
	for _, g := range existing {
		if g.Phase != games.GamePhaseQualifying || g.Round != round || !g.Status.Deletable() {
			continue
		}
		out = append(out, g.GameNo)
	}
	return out
}

// preserveRecordedResults splits a generated schedule around recorded results:
// a generated game whose (round, seq) slot holds a result with the same
// pairing is dropped from the write set and returned in preserved (the result
// already settles that game). A different pairing at the same slot means the
// regenerated schedule disagrees with what was actually played, so the request
// is refused; nothing is written or deleted before this check.
func preserveRecordedResults(generated, existing []games.Game) (remaining, preserved []games.Game, err error) {
	type slot struct{ round, seq int }
	results := map[slot]games.Game{}
	for _, g := range existing {
		if g.Phase == games.GamePhaseQualifying && isResultStatus(g.Status) {
			results[slot{g.Round, g.Seq}] = g
		}
	}
	if len(results) == 0 {
		return generated, nil, nil
	}

	out := make([]games.Game, 0, len(generated))
	for _, g := range generated {
		existingResult, ok := results[slot{g.Round, g.Seq}]
		if !ok {
			out = append(out, g)
			continue
		}
		if samePairing(g, existingResult) {
			preserved = append(preserved, existingResult)
			continue
		}
		return nil, nil, &conflictError{msg: fmt.Sprintf(
			"A recorded result occupies round %d position %d with a different pairing; retract that result via the result endpoint before regenerating",
			g.Round, g.Seq,
		)}
	}
	return out, preserved, nil
}

func samePairing(a, b games.Game) bool {
	if !a.SideA.Real() || !a.SideB.Real() || !b.SideA.Real() || !b.SideB.Real() {
		return false
	}
	return unorderedPairKey(*a.SideA.TeamID, *a.SideB.TeamID) == unorderedPairKey(*b.SideA.TeamID, *b.SideB.TeamID)
}

func unorderedPairKey(a, b uuid.UUID) string {
	if a.String() < b.String() {
		return a.String() + "|" + b.String()
	}
	return b.String() + "|" + a.String()
}

func withoutQualifyingRound(existing []games.Game, round int) []games.Game {
	out := make([]games.Game, 0, len(existing))
	for _, g := range existing {
		if g.Phase == games.GamePhaseQualifying && g.Round == round {
			continue
		}
		out = append(out, g)
	}
	return out
}

func isResultStatus(status games.GameStatus) bool {
	switch status {
	case games.GameStatusCompleted, games.GameStatusForfeit, games.GameStatusDoubleForfeit:
		return true
	default:
		return false
	}
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

func (a *API) PostEventsV1EventIdGames(ctx context.Context, request PostEventsV1EventIdGamesRequestObject) (PostEventsV1EventIdGamesResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostEventsV1EventIdGames")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	event, found, err := a.loadEvent(ctx, request.EventId)
	if err != nil {
		return a.createGameErrorResponse(span, logger, err), nil
	}
	if !found {
		return PostEventsV1EventIdGames404JSONResponse{
			Code:    NotFound,
			Message: "Event not found",
		}, nil
	}
	if event.Status.NormalizeDefault() == events.EventStatusFinalized {
		return PostEventsV1EventIdGames409JSONResponse{
			Code:    Conflict,
			Message: "Event is FINALIZED; schedule locked",
		}, nil
	}

	existing, err := a.db.ListGamesForEvent(ctx, request.EventId)
	if err != nil {
		return a.createGameErrorResponse(span, logger, err), nil
	}

	game := games.Game{
		ID:        uuid.New(),
		EventID:   event.ID,
		GameNo:    nextGameNo(event.LastGameNo, existing),
		Phase:     games.GamePhaseQualifying,
		Round:     request.Body.Round,
		Seq:       request.Body.Seq,
		Status:    games.GameStatusScheduled,
		SideA:     sideFromInput(request.Body.SideA),
		SideB:     sideFromInput(request.Body.SideB),
		Notes:     request.Body.Notes,
		StartTime: request.Body.StartTime,
		Version:   1,
	}
	if err := game.Validate(); err != nil {
		return a.createGameErrorResponse(span, logger, err), nil
	}
	// The slot check is a fast refusal; the transaction below is the
	// authority. CreateScheduledGame folds the schedule guard (event Version
	// CAS + FINALIZED barrier + LastGameNo mint) and the conditional put into
	// one transaction, so a writer that read the same version cannot also
	// mint this gameNo.
	if positionTaken(existing, game.Phase, game.Round, game.Seq) {
		return PostEventsV1EventIdGames409JSONResponse{
			Code:    Conflict,
			Message: "A game already occupies that round and position",
		}, nil
	}

	if err := a.db.CreateScheduledGame(ctx, event.ID, event.Version, game); err != nil {
		return a.createGameErrorResponse(span, logger, err), nil
	}

	respGame, err := gameToApiGame(game, a.teamNames(ctx, teamIDsInGames([]games.Game{game})))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to convert game", "error", err)

		return PostEventsV1EventIdGames500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create game",
		}, nil
	}

	return PostEventsV1EventIdGames200JSONResponse{Game: respGame}, nil
}

func (a *API) createGameErrorResponse(span trace.Span, logger *slog.Logger, err error) PostEventsV1EventIdGamesResponseObject {
	status, message := gameErrorStatus(err)
	switch status {
	case 400:
		return PostEventsV1EventIdGames400JSONResponse{Code: InvalidBody, Message: message}
	case 404:
		return PostEventsV1EventIdGames404JSONResponse{Code: NotFound, Message: "Event or game not found"}
	case 409:
		return PostEventsV1EventIdGames409JSONResponse{Code: Conflict, Message: message}
	default:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to create game", "error", err)

		return PostEventsV1EventIdGames500JSONResponse{Code: InternalError, Message: "Failed to create game"}
	}
}

func (a *API) PutEventsV1EventIdGamesGameNo(ctx context.Context, request PutEventsV1EventIdGamesGameNoRequestObject) (PutEventsV1EventIdGamesGameNoResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PutEventsV1EventIdGamesGameNo")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	event, found, err := a.loadEvent(ctx, request.EventId)
	if err != nil {
		return a.scheduleEditErrorResponse(span, logger, err), nil
	}
	if !found {
		return PutEventsV1EventIdGamesGameNo404JSONResponse{
			Code:    NotFound,
			Message: "Event not found",
		}, nil
	}
	if event.Status.NormalizeDefault() == events.EventStatusFinalized {
		return PutEventsV1EventIdGamesGameNo409JSONResponse{
			Code:    Conflict,
			Message: "Event is FINALIZED; schedule locked",
		}, nil
	}

	stored, err := a.db.GetGameByNumber(ctx, request.EventId, request.GameNo)
	if err != nil {
		return a.scheduleEditErrorResponse(span, logger, err), nil
	}
	if stored.Status != games.GameStatusScheduled {
		return PutEventsV1EventIdGamesGameNo409JSONResponse{
			Code:    Conflict,
			Message: "Only SCHEDULED games can be edited",
		}, nil
	}
	if request.Body.Version != stored.Version {
		return PutEventsV1EventIdGamesGameNo409JSONResponse{
			Code:    Conflict,
			Message: "Version conflict; refetch and retry",
		}, nil
	}

	updated, err := applyGameSchedule(stored, *request.Body)
	if err != nil {
		span.RecordError(err)
		logger.Error("Invalid schedule edit", slog.String("error", err.Error()))

		return PutEventsV1EventIdGamesGameNo400JSONResponse{
			Code:    InvalidBody,
			Message: domainErrorMessage(err),
		}, nil
	}

	// The edit folds the schedule guard and the row rewrite (including a key
	// move) into one transaction, so an edit cannot land on a version another
	// writer already replaced.
	if err := a.db.UpdateGameScheduleChecked(ctx, event.ID, event.Version, stored, updated); err != nil {
		return a.scheduleEditErrorResponse(span, logger, err), nil
	}

	respGame, err := gameToApiGame(updated, a.teamNames(ctx, teamIDsInGames([]games.Game{updated})))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to convert game", "error", err)

		return PutEventsV1EventIdGamesGameNo500JSONResponse{
			Code:    InternalError,
			Message: "Failed to edit game",
		}, nil
	}

	return PutEventsV1EventIdGamesGameNo200JSONResponse{Game: respGame}, nil
}

func (a *API) scheduleEditErrorResponse(span trace.Span, logger *slog.Logger, err error) PutEventsV1EventIdGamesGameNoResponseObject {
	var gameErr *games.Error
	if errors.As(err, &gameErr) && gameErr.Reason == games.REASON_SLOT_OCCUPIED {
		return PutEventsV1EventIdGamesGameNo409JSONResponse{
			Code:    Conflict,
			Message: "slot occupied; pick another round/seq",
		}
	}

	status, message := gameErrorStatus(err)
	switch status {
	case 400:
		return PutEventsV1EventIdGamesGameNo400JSONResponse{Code: InvalidBody, Message: message}
	case 404:
		return PutEventsV1EventIdGamesGameNo404JSONResponse{Code: NotFound, Message: "Event or game not found"}
	case 409:
		return PutEventsV1EventIdGamesGameNo409JSONResponse{Code: Conflict, Message: message}
	default:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to edit game", "error", err)

		return PutEventsV1EventIdGamesGameNo500JSONResponse{Code: InternalError, Message: "Failed to edit game"}
	}
}

func (a *API) DeleteEventsV1EventIdGamesGameNo(ctx context.Context, request DeleteEventsV1EventIdGamesGameNoRequestObject) (DeleteEventsV1EventIdGamesGameNoResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "DeleteEventsV1EventIdGamesGameNo")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	event, found, err := a.loadEvent(ctx, request.EventId)
	if err != nil {
		return a.deleteGameErrorResponse(span, logger, err), nil
	}
	if !found {
		return DeleteEventsV1EventIdGamesGameNo404JSONResponse{
			Code:    NotFound,
			Message: "Event not found",
		}, nil
	}
	if event.Status.NormalizeDefault() == events.EventStatusFinalized {
		return DeleteEventsV1EventIdGamesGameNo409JSONResponse{
			Code:    Conflict,
			Message: "Event is FINALIZED; schedule locked",
		}, nil
	}

	stored, err := a.db.GetGameByNumber(ctx, request.EventId, request.GameNo)
	if err != nil {
		return a.deleteGameErrorResponse(span, logger, err), nil
	}
	if !stored.Status.Deletable() {
		return DeleteEventsV1EventIdGamesGameNo409JSONResponse{
			Code:    Conflict,
			Message: "Only unplayed games (SCHEDULED, BYE, CANCELLED) can be deleted",
		}, nil
	}

	if err := a.db.DeleteScheduledGame(ctx, event.ID, event.Version, stored); err != nil {
		return a.deleteGameErrorResponse(span, logger, err), nil
	}

	return DeleteEventsV1EventIdGamesGameNo204Response{}, nil
}

func (a *API) deleteGameErrorResponse(span trace.Span, logger *slog.Logger, err error) DeleteEventsV1EventIdGamesGameNoResponseObject {
	status, message := gameErrorStatus(err)
	switch status {
	case 400:
		return DeleteEventsV1EventIdGamesGameNo400JSONResponse{Code: InvalidBody, Message: message}
	case 404:
		return DeleteEventsV1EventIdGamesGameNo404JSONResponse{Code: NotFound, Message: "Event or game not found"}
	case 409:
		return DeleteEventsV1EventIdGamesGameNo409JSONResponse{Code: Conflict, Message: message}
	default:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to delete game", "error", err)

		return DeleteEventsV1EventIdGamesGameNo500JSONResponse{Code: InternalError, Message: "Failed to delete game"}
	}
}

func (a *API) PutEventsV1EventIdGamesGameNoResult(ctx context.Context, request PutEventsV1EventIdGamesGameNoResultRequestObject) (PutEventsV1EventIdGamesGameNoResultResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PutEventsV1EventIdGamesGameNoResult")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	stored, err := a.db.GetGameByNumber(ctx, request.EventId, request.GameNo)
	if err != nil {
		return a.resultErrorResponse(span, logger, err), nil
	}
	if request.Body.Version != stored.Version {
		return PutEventsV1EventIdGamesGameNoResult409JSONResponse{
			Code:    Conflict,
			Message: "Version conflict; refetch and retry",
		}, nil
	}

	updated, err := applyGameResult(stored, *request.Body)
	if err != nil {
		span.RecordError(err)
		logger.Error("Invalid game result", slog.String("error", err.Error()))

		return PutEventsV1EventIdGamesGameNoResult400JSONResponse{
			Code:    InvalidBody,
			Message: domainErrorMessage(err),
		}, nil
	}

	if err := a.db.UpdateGameChecked(ctx, request.EventId, updated); err != nil {
		return a.resultErrorResponse(span, logger, err), nil
	}

	respGame, err := gameToApiGame(updated, a.teamNames(ctx, teamIDsInGames([]games.Game{updated})))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to convert game", "error", err)

		return PutEventsV1EventIdGamesGameNoResult500JSONResponse{
			Code:    InternalError,
			Message: "Failed to record result",
		}, nil
	}

	return PutEventsV1EventIdGamesGameNoResult200JSONResponse{Game: respGame}, nil
}

func (a *API) resultErrorResponse(span trace.Span, logger *slog.Logger, err error) PutEventsV1EventIdGamesGameNoResultResponseObject {
	status, message := gameErrorStatus(err)
	switch status {
	case 400:
		return PutEventsV1EventIdGamesGameNoResult400JSONResponse{Code: InvalidBody, Message: message}
	case 404:
		return PutEventsV1EventIdGamesGameNoResult404JSONResponse{Code: NotFound, Message: "Event or game not found"}
	case 409:
		return PutEventsV1EventIdGamesGameNoResult409JSONResponse{Code: Conflict, Message: message}
	default:
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to record result", "error", err)

		return PutEventsV1EventIdGamesGameNoResult500JSONResponse{Code: InternalError, Message: "Failed to record result"}
	}
}

// gameErrorStatus maps a games-port error to an HTTP status and a message
// safe to return to the client.
func gameErrorStatus(err error) (int, string) {
	var conflict *conflictError
	if errors.As(err, &conflict) {
		if conflict.badRequest {
			return 400, conflict.msg
		}
		return 409, conflict.msg
	}

	var gameErr *games.Error
	if errors.As(err, &gameErr) {
		switch gameErr.Reason {
		case games.REASON_GAME_DOES_NOT_EXIST:
			return 404, "Game not found"
		case games.REASON_EVENT_FINALIZED:
			return 409, "Event is FINALIZED; schedule locked"
		case games.REASON_VERSION_CONFLICT:
			return 409, "Concurrent schedule change; refetch and retry"
		case games.REASON_NO_VALID_PAIRING:
			return 409, "No valid pairing without rematch"
		case games.REASON_PAIRING_BUDGET_EXCEEDED:
			return 409, "Pairing is too complex to resolve safely; correct scores or regenerate"
		case games.REASON_SLOT_OCCUPIED:
			return 409, "Game slot is occupied; refetch and retry"
		case games.REASON_INVALID_TIEBREAKER, games.REASON_TIMEOUT, games.REASON_FAILED_TO_WRITE,
			games.REASON_FAILED_TO_FETCH, games.REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL:
			return 500, "Internal error"
		default:
			return 400, gameErr.Message
		}
	}

	return 500, "Internal error"
}

// domainErrorMessage unwraps the human-readable part of a domain error so a
// 400 explains which rule failed instead of hiding it behind a generic text.
func domainErrorMessage(err error) string {
	var gameErr *games.Error
	if errors.As(err, &gameErr) {
		return gameErr.Message
	}
	return err.Error()
}

func applyGameResult(stored games.Game, body GameResultRequest) (games.Game, error) {
	// The result endpoint always replaces the full result shape: an omitted
	// score or forfeit side clears it, so SCHEDULED retracts a mis-entered
	// result and there is no partial-result ambiguity.
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

func applyGameSchedule(stored games.Game, body GameScheduleRequest) (games.Game, error) {
	// Rebuild from the stored row so identity, phase, status, and every
	// result/playoff link survive untouched.
	updated := stored
	updated.Round = body.Round
	updated.Seq = body.Seq
	updated.SideA = sideFromInput(body.SideA)
	updated.SideB = sideFromInput(body.SideB)
	updated.StartTime = body.StartTime
	updated.Notes = body.Notes
	updated.Version = stored.Version + 1

	if err := updated.Validate(); err != nil {
		return games.Game{}, err
	}
	return updated, nil
}

func sideFromInput(in GameSideInput) games.Side {
	return games.Side{TeamID: in.TeamId}
}

func positionTaken(existing []games.Game, phase games.GamePhase, round, seq int) bool {
	for _, g := range existing {
		if g.Phase == phase && g.Round == round && g.Seq == seq {
			return true
		}
	}
	return false
}

// tiebreakOrderForEvent returns the event's single comparator order and
// validates the stored enum. An absent or empty list means the D34 default
// (net-first), so old events behave exactly as before D34.
func tiebreakOrderForEvent(event events.Event) (games.TiebreakOrder, error) {
	if len(event.Tiebreakers) == 0 {
		return games.DefaultTiebreakOrder(), nil
	}
	if err := event.Tiebreakers.Validate(); err != nil {
		return nil, err
	}
	return event.Tiebreakers, nil
}

func (a *API) loadEvent(ctx context.Context, eventID uuid.UUID) (events.Event, bool, error) {
	event, err := a.db.GetEvent(ctx, eventID)
	if err != nil {
		var eventErr *events.Error
		if errors.As(err, &eventErr) && eventErr.Reason == events.REASON_EVENT_DOES_NOT_EXIST {
			return events.Event{}, false, nil
		}
		return events.Event{}, false, err
	}
	return event, true, nil
}

// teamNames joins display names for the given teams. A missing or
// unreadable name is cosmetic: the row is returned with a null name rather
// than failing the whole schedule or standings read.
func (a *API) teamNames(ctx context.Context, ids []uuid.UUID) map[string]string {
	logger := a.getLoggerOrBaseLogger(ctx)
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
			logger.Warn("Failed to join team name", slog.String("team-id", id.String()), slog.String("error", err.Error()))
			continue
		}
		names[id.String()] = team.Name
	}
	return names
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

	eventID := g.EventID
	gameNo := g.GameNo

	return Game{
		EventId:     &eventID,
		GameNo:      &gameNo,
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
		Notes:       g.Notes,
		StartTime:   g.StartTime,
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
