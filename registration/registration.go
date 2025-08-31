package registration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/google/uuid"
)

type Repository interface {
	CreateRegistration(ctx context.Context, registration Registration, event events.Event) error
	GetAllRegistrationsForEvent(ctx context.Context, eventId uuid.UUID, limit int32, cursor *string) (GetAllRegistrationsResponse, error)
}

type GetAllRegistrationsResponse struct {
	Data        []Registration
	Cursor      *string
	HasNextPage bool
}

type Registration interface {
	GetEventID() uuid.UUID
	GetEmail() string
	Type() events.RegistrationType
}

var _ Registration = IndividualRegistration{}

type IndividualRegistration struct {
	ID           uuid.UUID
	Version      int
	EventID      uuid.UUID
	RegisteredAt time.Time
	HomeCity     string
	Paid         bool
	Email        string
	PlayerInfo   PlayerInfo
	Experience   ExperienceLevel
}

func (r IndividualRegistration) GetEventID() uuid.UUID {
	return r.EventID
}

func (r IndividualRegistration) GetEmail() string {
	return r.Email
}

func (r IndividualRegistration) Type() events.RegistrationType {
	return events.BY_INDIVIDUAL
}

var _ Registration = TeamRegistration{}

type TeamRegistration struct {
	ID           uuid.UUID
	Version      int
	EventID      uuid.UUID
	RegisteredAt time.Time
	HomeCity     string
	Paid         bool
	TeamName     string
	CaptainEmail string
	Players      []PlayerInfo
}

func (r TeamRegistration) GetEventID() uuid.UUID {
	return r.EventID
}

func (r TeamRegistration) GetEmail() string {
	return r.CaptainEmail
}

func (r TeamRegistration) Type() events.RegistrationType {
	return events.BY_TEAM
}

func AttemptRegistration(ctx context.Context, registrationRequest Registration, eventRepo events.Repository, registrationRepo Repository) (Registration, events.Event, error) {
	eventId := registrationRequest.GetEventID()

	event, err := eventRepo.GetEvent(ctx, eventId)
	if err != nil {
		var eventErr *events.Error
		if errors.As(err, &eventErr) {
			switch eventErr.Reason {
			case events.REASON_EVENT_DOES_NOT_EXIST:
				return nil, events.Event{}, NewAssociatedEventDoesNotExistError(fmt.Sprintf("Event does not exist with ID %q", eventId), err)
			}
		}

		return nil, events.Event{}, NewFailedToFetchError(fmt.Sprintf("Failed to fetch event with ID %q", eventId), err)
	}

	switch registrationRequest.Type() {
	case events.BY_INDIVIDUAL:
		err = registerIndividualAsFreeAgent(&event, registrationRequest.(IndividualRegistration))
		if err != nil {
			return nil, events.Event{}, err
		}
	case events.BY_TEAM:
		err = registerTeam(&event, registrationRequest.(TeamRegistration))
		if err != nil {
			return nil, events.Event{}, err
		}
	default:
		return nil, events.Event{}, NewUnknownRegistrationTypeError(fmt.Sprintf("Unknown registration type: %d", registrationRequest.Type()))
	}

	event.Version++
	err = registrationRepo.CreateRegistration(ctx, registrationRequest, event)
	if err != nil {
		return nil, events.Event{}, err
	}
	return registrationRequest, event, nil
}

func registerIndividualAsFreeAgent(event *events.Event, reg IndividualRegistration) error {
	if !slices.ContainsFunc(event.RegistrationOptions, func(v events.EventRegistrationOption) bool { return v.RegType == events.BY_INDIVIDUAL }) {
		return NewNotAllowedToSignUpAsTypeError(events.BY_INDIVIDUAL)
	}

	if reg.RegisteredAt.After(event.RegistrationCloseTime) {
		return NewRegistrationIsClosedError(event.RegistrationCloseTime)
	}

	event.NumTotalPlayers++

	return nil
}

func registerTeam(event *events.Event, reg TeamRegistration) error {
	if !slices.ContainsFunc(event.RegistrationOptions, func(v events.EventRegistrationOption) bool { return v.RegType == events.BY_TEAM }) {
		return NewNotAllowedToSignUpAsTypeError(events.BY_TEAM)
	}

	if reg.RegisteredAt.After(event.RegistrationCloseTime) {
		return NewRegistrationIsClosedError(event.RegistrationCloseTime)
	}

	teamSize := len(reg.Players)

	if teamSize < event.AllowedTeamSizeRange.Min || teamSize > event.AllowedTeamSizeRange.Max {
		return NewTeamSizeNotAllowedError(teamSize, event.AllowedTeamSizeRange.Min, event.AllowedTeamSizeRange.Max)
	}

	event.NumTeams++
	event.NumTotalPlayers += teamSize
	event.NumRosteredPlayers += teamSize

	return nil
}
