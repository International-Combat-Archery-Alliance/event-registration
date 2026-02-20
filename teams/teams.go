//go:generate go tool stringer -type=TeamSourceType
//go:generate go tool stringer -type=PlayerSourceType

package teams

import (
	"context"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/google/uuid"
)

type TeamSourceType int

const (
	SOURCE_TEAM_REGISTRATION TeamSourceType = iota
	SOURCE_ADMIN_CREATED
	SOURCE_MIXED
)

type PlayerSourceType int

const (
	PLAYER_SOURCE_TEAM_REGISTRATION PlayerSourceType = iota
	PLAYER_SOURCE_INDIVIDUAL_REGISTRATION
)

type Team struct {
	ID             uuid.UUID
	Version        int
	EventID        uuid.UUID
	Name           string
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

type GetTeamsResponse struct {
	Data        []Team
	Cursor      *string
	HasNextPage bool
}

type Repository interface {
	GetTeam(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID) (Team, error)
	GetTeamsForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (GetTeamsResponse, error)
	CreateTeam(ctx context.Context, team Team) error
	UpdateTeam(ctx context.Context, team Team) error
	DeleteTeam(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID) error
	AddPlayerToTeam(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID, player TeamPlayer) error
	RemovePlayerFromTeam(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID, registrationID uuid.UUID) error
	HasGames(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID) (bool, error)
	IsIndividualAssigned(ctx context.Context, eventID uuid.UUID, registrationID uuid.UUID) (bool, error)
}

func CreateTeam(ctx context.Context, repo Repository, team Team) (Team, error) {
	team.ID = uuid.New()
	team.Version = 1
	team.CreatedAt = time.Now().UTC()

	if len(team.Players) > 0 {
		now := time.Now().UTC()
		for i := range team.Players {
			team.Players[i].AssignedAt = now
		}
	}

	err := repo.CreateTeam(ctx, team)
	if err != nil {
		return Team{}, err
	}

	return team, nil
}

func UpdateTeam(ctx context.Context, repo Repository, eventID uuid.UUID, teamID uuid.UUID, updates Team) (Team, error) {
	existingTeam, err := repo.GetTeam(ctx, eventID, teamID)
	if err != nil {
		return Team{}, err
	}

	updatedTeam := Team{
		ID:             existingTeam.ID,
		Version:        existingTeam.Version + 1,
		EventID:        existingTeam.EventID,
		Name:           updates.Name,
		SourceType:     existingTeam.SourceType,
		RegistrationID: existingTeam.RegistrationID,
		Players:        existingTeam.Players,
		CreatedAt:      existingTeam.CreatedAt,
	}

	err = repo.UpdateTeam(ctx, updatedTeam)
	if err != nil {
		return Team{}, err
	}

	return updatedTeam, nil
}

func AddPlayerToTeam(ctx context.Context, repo Repository, eventID uuid.UUID, teamID uuid.UUID, player TeamPlayer) error {
	team, err := repo.GetTeam(ctx, eventID, teamID)
	if err != nil {
		return err
	}

	isAssigned, err := repo.IsIndividualAssigned(ctx, eventID, player.RegistrationID)
	if err != nil {
		return err
	}
	if isAssigned {
		return NewPlayerAlreadyAssignedError("Player is already assigned to a team")
	}

	player.AssignedAt = time.Now().UTC()

	team.Version++
	err = repo.AddPlayerToTeam(ctx, eventID, teamID, player)
	if err != nil {
		return err
	}

	return nil
}

func RemovePlayerFromTeam(ctx context.Context, repo Repository, eventID uuid.UUID, teamID uuid.UUID, registrationID uuid.UUID) error {
	team, err := repo.GetTeam(ctx, eventID, teamID)
	if err != nil {
		return err
	}

	hasGames, err := repo.HasGames(ctx, eventID, teamID)
	if err != nil {
		return err
	}
	if hasGames {
		return NewCannotModifyTeamWithGamesError("Cannot remove players from a team that has games")
	}

	team.Version++
	err = repo.RemovePlayerFromTeam(ctx, eventID, teamID, registrationID)
	if err != nil {
		return err
	}

	return nil
}

func DeleteTeam(ctx context.Context, repo Repository, eventID uuid.UUID, teamID uuid.UUID) error {
	hasGames, err := repo.HasGames(ctx, eventID, teamID)
	if err != nil {
		return err
	}
	if hasGames {
		return NewCannotDeleteTeamWithGamesError("Cannot delete a team that has games")
	}

	return repo.DeleteTeam(ctx, eventID, teamID)
}
