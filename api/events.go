package api

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/google/uuid"
)

func (a *API) GetEvents(ctx context.Context, request GetEventsRequestObject) (GetEventsResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// guaranteed to be non-nil from openapi doc
	limit := int32(*request.Params.Limit)

	result, err := a.db.GetEvents(ctx, limit, request.Params.Cursor)
	if err != nil {
		a.logger.Error("Failed to get events from the DB", "error", err)

		var eventErr *events.Error
		if errors.As(err, &eventErr) {
			switch eventErr.Reason {
			case events.REASON_INVALID_CURSOR:
				return GetEvents400JSONResponse{
					Code:    InvalidCursor,
					Message: "Passed in cursor is invalid",
				}, nil
			}
		}
		return GetEvents500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get events",
		}, nil
	}

	respEvents := []Event{}
	for _, v := range result.Data {
		convEvent, err := eventToApiEvent(v)
		if err != nil {
			a.logger.Error("Failed to convert event to api event", "error", err)

			return GetEvents500JSONResponse{
				Code:    InternalError,
				Message: "Failed to get events",
			}, nil
		}
		respEvents = append(respEvents, convEvent)
	}

	return GetEvents200JSONResponse{
		Data:        respEvents,
		Cursor:      result.Cursor,
		HasNextPage: result.HasNextPage,
	}, nil
}

func (a *API) PostEvents(ctx context.Context, request PostEventsRequestObject) (PostEventsResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	id := uuid.New()
	request.Body.Id = &id
	request.Body.Version = ptr.Int(1)
	request.Body.SignUpStats = &SignUpStats{
		NumTeams:           0,
		NumRosteredPlayers: 0,
		NumTotalPlayers:    0,
	}
	// request.Body is guaranteed to be non-nil from openapi doc
	event, err := apiEventToEvent(*request.Body)
	if err != nil {
		a.logger.Error("Failed to convert event into core type", "error", err)

		return PostEvents400JSONResponse{
			Code:    InvalidBody,
			Message: "Failed to create the event",
		}, nil
	}

	err = a.db.CreateEvent(ctx, event)
	if err != nil {
		a.logger.Error("Failed to create an event", "error", err)

		return PostEvents500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create the event",
		}, nil
	}

	return PostEvents200JSONResponse(*request.Body), nil
}

func (a *API) GetEventsId(ctx context.Context, request GetEventsIdRequestObject) (GetEventsIdResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	event, err := a.db.GetEvent(ctx, request.Id)
	if err != nil {
		a.logger.Error("Failed to fetch an event", "error", err)

		var eventErr *events.Error
		if errors.As(err, &eventErr) {
			switch eventErr.Reason {
			case events.REASON_EVENT_DOES_NOT_EXIST:
				return GetEventsId404JSONResponse{
					Code:    NotFound,
					Message: "Event does not exist",
				}, nil
			}
		}

		return GetEventsId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get event",
		}, nil
	}

	respEvent, err := eventToApiEvent(event)
	if err != nil {
		a.logger.Error("Failed to convert event into core type", "error", err)

		return GetEventsId500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get event",
		}, nil
	}
	return GetEventsId200JSONResponse{Event: respEvent}, nil
}

func eventToApiEvent(event events.Event) (Event, error) {
	regTypes := []RegistrationType{}
	for _, t := range event.RegistrationTypes {
		convT, err := registrationTypeToApiRegistrationType(t)
		if err != nil {
			return Event{}, err
		}
		regTypes = append(regTypes, convT)
	}

	return Event{
		Id:                    &event.ID,
		Version:               &event.Version,
		Name:                  event.Name,
		Location:              locationToApiLocation(event.EventLocation),
		StartTime:             event.StartTime,
		EndTime:               event.EndTime,
		RegistrationCloseTime: event.RegistrationCloseTime,
		RegistrationTypes:     regTypes,
		AllowedTeamSizeRange: Range{
			Min: event.AllowedTeamSizeRange.Min,
			Max: event.AllowedTeamSizeRange.Max,
		},
		SignUpStats: &SignUpStats{
			NumTeams:           event.NumTeams,
			NumRosteredPlayers: event.NumRosteredPlayers,
			NumTotalPlayers:    event.NumTotalPlayers,
		},
		RulesDocLink: event.RulesDocLink,
	}, nil
}

func apiEventToEvent(event Event) (events.Event, error) {
	regTypes := []events.RegistrationType{}
	for _, t := range event.RegistrationTypes {
		convT, err := apiRegistrationTypeToRegistrationType(t)
		if err != nil {
			return events.Event{}, err
		}
		regTypes = append(regTypes, convT)
	}

	return events.Event{
		ID:                    *event.Id,
		Version:               *event.Version,
		Name:                  event.Name,
		EventLocation:         apiLocationToLocation(event.Location),
		StartTime:             event.StartTime,
		EndTime:               event.EndTime,
		RegistrationCloseTime: event.RegistrationCloseTime,
		RegistrationTypes:     regTypes,
		NumTotalPlayers:       event.SignUpStats.NumTotalPlayers,
		NumRosteredPlayers:    event.SignUpStats.NumRosteredPlayers,
		NumTeams:              event.SignUpStats.NumTeams,
		AllowedTeamSizeRange: events.Range{
			Min: event.AllowedTeamSizeRange.Min,
			Max: event.AllowedTeamSizeRange.Max,
		},
		RulesDocLink: event.RulesDocLink,
	}, nil
}

func locationToApiLocation(location events.Location) Location {
	return Location{
		Name:    location.Name,
		Address: addressToApiAddress(location.LocAddress),
	}
}

func apiLocationToLocation(location Location) events.Location {
	return events.Location{
		Name:       location.Name,
		LocAddress: apiAddressToAddress(location.Address),
	}
}

func addressToApiAddress(address events.Address) Address {
	return Address{
		City:       address.City,
		Country:    address.Country,
		PostalCode: address.PostalCode,
		State:      address.State,
		Street:     address.Street,
	}
}

func apiAddressToAddress(address Address) events.Address {
	return events.Address{
		City:       address.City,
		Country:    address.Country,
		PostalCode: address.PostalCode,
		State:      address.State,
		Street:     address.Street,
	}
}

func registrationTypeToApiRegistrationType(t events.RegistrationType) (RegistrationType, error) {
	switch t {
	case events.BY_INDIVIDUAL:
		return ByIndividual, nil
	case events.BY_TEAM:
		return ByTeam, nil
	default:
		return RegistrationType(""), fmt.Errorf("unknown registration type: %s", t)
	}
}

func apiRegistrationTypeToRegistrationType(t RegistrationType) (events.RegistrationType, error) {
	switch t {
	case ByIndividual:
		return events.BY_INDIVIDUAL, nil
	case ByTeam:
		return events.BY_TEAM, nil
	default:
		return events.RegistrationType(0), fmt.Errorf("unknown registration type: %s", t)
	}
}
