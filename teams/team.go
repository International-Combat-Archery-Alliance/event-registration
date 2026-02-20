//go:generate go tool stringer -type=TeamSourceType
//go:generate go tool stringer -type=PlayerSourceType

package teams

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Team represents a global team entity that exists outside of events
// This allows tracking team history across multiple events
type Team struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

// TeamSourceType indicates how an event team was created
type TeamSourceType int

const (
	SOURCE_TEAM_REGISTRATION TeamSourceType = iota
	SOURCE_ADMIN_CREATED
	SOURCE_MIXED
)

// PlayerSourceType indicates the source of a player on an event team
type PlayerSourceType int

const (
	PLAYER_SOURCE_TEAM_REGISTRATION PlayerSourceType = iota
	PLAYER_SOURCE_INDIVIDUAL_REGISTRATION
)

// TeamRepository interface for global team operations
type TeamRepository interface {
	GetTeam(ctx context.Context, teamID uuid.UUID) (Team, error)
	GetTeams(ctx context.Context, limit int32, cursor *string) (GetTeamsResponse, error)
	CreateTeam(ctx context.Context, team Team) error
	UpdateTeam(ctx context.Context, team Team) error
	DeleteTeam(ctx context.Context, teamID uuid.UUID) error
}

type GetTeamsResponse struct {
	Data        []Team
	Cursor      *string
	HasNextPage bool
}

func CreateTeam(ctx context.Context, repo TeamRepository, team Team) (Team, error) {
	team.ID = uuid.New()
	team.CreatedAt = time.Now().UTC()

	err := repo.CreateTeam(ctx, team)
	if err != nil {
		return Team{}, err
	}

	return team, nil
}

func UpdateTeam(ctx context.Context, repo TeamRepository, teamID uuid.UUID, updates Team) (Team, error) {
	existingTeam, err := repo.GetTeam(ctx, teamID)
	if err != nil {
		return Team{}, err
	}

	updatedTeam := Team{
		ID:        existingTeam.ID,
		Name:      updates.Name,
		CreatedAt: existingTeam.CreatedAt,
	}

	err = repo.UpdateTeam(ctx, updatedTeam)
	if err != nil {
		return Team{}, err
	}

	return updatedTeam, nil
}
