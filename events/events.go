package events

import (
	"context"
	"time"

	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
)

type Event struct {
	ID                    uuid.UUID
	Version               int
	Name                  string
	EventLocation         Location
	StartTime             time.Time
	EndTime               time.Time
	RegistrationCloseTime time.Time
	RegistrationOptions   []EventRegistrationOption
	AllowedTeamSizeRange  Range
	NumTeams              int
	NumRosteredPlayers    int
	NumTotalPlayers       int
	RulesDocLink          *string
}

type EventRegistrationOption struct {
	RegType RegistrationType
	Price   money.Money
}

type Range struct {
	Min int
	Max int
}

type GetEventsResponse struct {
	Data        []Event
	Cursor      *string
	HasNextPage bool
}

type Repository interface {
	GetEvent(ctx context.Context, id uuid.UUID) (Event, error)
	GetEvents(ctx context.Context, limit int32, cursor *string) (GetEventsResponse, error)
	CreateEvent(ctx context.Context, event Event) error
	UpdateEvent(ctx context.Context, event Event) error
}
