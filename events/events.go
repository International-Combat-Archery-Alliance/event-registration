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
	TimeZone              *time.Location
	StartTime             time.Time
	EndTime               time.Time
	RegistrationCloseTime time.Time
	RegistrationOptions   []EventRegistrationOption
	AllowedTeamSizeRange  Range
	NumTeams              int
	NumRosteredPlayers    int
	NumTotalPlayers       int
	RulesDocLink          *string
	ImageName             *string
}

type EventRegistrationOption struct {
	RegType RegistrationType
	Price   *money.Money
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

func UpdateEvent(ctx context.Context, repo Repository, id uuid.UUID, event Event) (Event, error) {
	existingEvent, err := repo.GetEvent(ctx, id)
	if err != nil {
		return Event{}, err
	}

	updatedEvent := Event{
		ID:                    id,
		Version:               existingEvent.Version + 1,
		Name:                  event.Name,
		StartTime:             event.StartTime,
		EndTime:               event.EndTime,
		TimeZone:              event.TimeZone,
		EventLocation:         event.EventLocation,
		RegistrationCloseTime: event.RegistrationCloseTime,
		RegistrationOptions:   event.RegistrationOptions,
		AllowedTeamSizeRange:  event.AllowedTeamSizeRange,
		NumTeams:              existingEvent.NumTeams,
		NumRosteredPlayers:    existingEvent.NumRosteredPlayers,
		NumTotalPlayers:       existingEvent.NumTotalPlayers,
		RulesDocLink:          event.RulesDocLink,
		ImageName:             event.ImageName,
	}

	err = repo.UpdateEvent(ctx, updatedEvent)
	if err != nil {
		return Event{}, err
	}

	return updatedEvent, nil
}
