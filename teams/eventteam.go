package teams

import (
	"context"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/google/uuid"
)

// EventTeam represents a team's participation in a specific event
// It links a global Team to an Event and contains event-specific data like players
type EventTeam struct {
	ID             uuid.UUID
	Version        int
	EventID        uuid.UUID
	TeamID         uuid.UUID // Reference to the global Team
	Name           string    // Event-specific name (can differ from global team name)
	SourceType     TeamSourceType
	RegistrationID *uuid.UUID
	Players        []TeamPlayer
	CreatedAt      time.Time
}

type TeamPlayer struct {
	PlayerInfo     registration.PlayerInfo
	SourceType     PlayerSourceType
	RegistrationID uuid.UUID
	AssignedAt     time.Time
}

type GetEventTeamsResponse struct {
	Data        []EventTeam
	Cursor      *string
	HasNextPage bool
}

// EventTeamRepository interface for event-specific team operations
type EventTeamRepository interface {
	GetEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) (EventTeam, error)
	GetEventTeamsForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (GetEventTeamsResponse, error)
	GetEventTeamsByTeam(ctx context.Context, teamID uuid.UUID, limit int32, cursor *string) (GetEventTeamsResponse, error) // For GSI queries
	CreateEventTeam(ctx context.Context, eventTeam EventTeam) error
	UpdateEventTeam(ctx context.Context, eventTeam EventTeam) error
	DeleteEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) error
	AddPlayerToEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID, player TeamPlayer) error
	RemovePlayerFromEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID, registrationID uuid.UUID) error
	HasGames(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) (bool, error)
	IsIndividualAssigned(ctx context.Context, eventID uuid.UUID, registrationID uuid.UUID) (bool, error)
}

func CreateEventTeam(ctx context.Context, repo EventTeamRepository, eventTeam EventTeam) (EventTeam, error) {
	eventTeam.ID = uuid.New()
	eventTeam.Version = 1
	eventTeam.CreatedAt = time.Now().UTC()

	if len(eventTeam.Players) > 0 {
		now := time.Now().UTC()
		for i := range eventTeam.Players {
			eventTeam.Players[i].AssignedAt = now
		}
	}

	err := repo.CreateEventTeam(ctx, eventTeam)
	if err != nil {
		return EventTeam{}, err
	}

	return eventTeam, nil
}

func UpdateEventTeam(ctx context.Context, repo EventTeamRepository, eventID uuid.UUID, eventTeamID uuid.UUID, updates EventTeam) (EventTeam, error) {
	existingEventTeam, err := repo.GetEventTeam(ctx, eventID, eventTeamID)
	if err != nil {
		return EventTeam{}, err
	}

	updatedEventTeam := EventTeam{
		ID:             existingEventTeam.ID,
		Version:        existingEventTeam.Version + 1,
		EventID:        existingEventTeam.EventID,
		TeamID:         existingEventTeam.TeamID,
		Name:           updates.Name,
		SourceType:     existingEventTeam.SourceType,
		RegistrationID: existingEventTeam.RegistrationID,
		Players:        existingEventTeam.Players,
		CreatedAt:      existingEventTeam.CreatedAt,
	}

	err = repo.UpdateEventTeam(ctx, updatedEventTeam)
	if err != nil {
		return EventTeam{}, err
	}

	return updatedEventTeam, nil
}

func AddPlayerToEventTeam(ctx context.Context, repo EventTeamRepository, eventID uuid.UUID, eventTeamID uuid.UUID, player TeamPlayer) error {
	isAssigned, err := repo.IsIndividualAssigned(ctx, eventID, player.RegistrationID)
	if err != nil {
		return err
	}
	if isAssigned {
		return NewPlayerAlreadyAssignedError("Player is already assigned to a team")
	}

	player.AssignedAt = time.Now().UTC()

	err = repo.AddPlayerToEventTeam(ctx, eventID, eventTeamID, player)
	if err != nil {
		return err
	}

	return nil
}

func RemovePlayerFromEventTeam(ctx context.Context, repo EventTeamRepository, eventID uuid.UUID, eventTeamID uuid.UUID, registrationID uuid.UUID) error {
	hasGames, err := repo.HasGames(ctx, eventID, eventTeamID)
	if err != nil {
		return err
	}
	if hasGames {
		return NewCannotModifyTeamWithGamesError("Cannot remove players from a team that has games")
	}

	err = repo.RemovePlayerFromEventTeam(ctx, eventID, eventTeamID, registrationID)
	if err != nil {
		return err
	}

	return nil
}

func DeleteEventTeam(ctx context.Context, repo EventTeamRepository, eventID uuid.UUID, eventTeamID uuid.UUID) error {
	hasGames, err := repo.HasGames(ctx, eventID, eventTeamID)
	if err != nil {
		return err
	}
	if hasGames {
		return NewCannotDeleteTeamWithGamesError("Cannot delete a team that has games")
	}

	return repo.DeleteEventTeam(ctx, eventID, eventTeamID)
}
