package api

import (
	"context"
	"errors"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
	"github.com/google/uuid"
)

func (a *API) GetEvents(ctx context.Context, request GetEventsRequestObject) (GetEventsResponseObject, error) {
	limit := 10

	if request.Params.Limit != nil {
		userLimit := *request.Params.Limit
		if userLimit < 1 || userLimit > 50 {
			return GetEvents400JSONResponse{
				Code:    LimitOutOfBounds,
				Message: "Limit must be between 1 and 50",
			}, nil
		}
	}

	result, err := a.db.GetEvents(ctx, int32(limit), request.Params.Cursor)
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
			Message: "Internal server error",
		}, nil
	}

	return GetEvents200JSONResponse{
		Data: slices.Map(result.Data, func(v events.Event) Event {
			return eventToApiEvent(v)
		}),
		Cursor:      result.Cursor,
		HasNextPage: result.HasNextPage,
	}, nil
}

func (a *API) PostEvents(ctx context.Context, request PostEventsRequestObject) (PostEventsResponseObject, error) {
	if request.Body == nil {
		return PostEvents400JSONResponse{
			Code:    EmptyBody,
			Message: "Must specify a JSON body in the request",
		}, nil
	}

	id := uuid.New()
	request.Body.Id = &id
	request.Body.SignUpStats = SignUpStats{
		NumTeams:           ptr.Int(0),
		NumRosteredPlayers: ptr.Int(0),
		NumTotalPlayers:    ptr.Int(0),
	}
	event := apiEventToEvent(*request.Body)

	err := a.db.CreateEvent(ctx, event)
	if err != nil {
		a.logger.Error("Failed to create an event", "error", err)

		return PostEvents500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create the event",
		}, nil
	}

	return PostEvents200JSONResponse(eventToApiEvent(event)), nil
}

func (a *API) GetEventsId(ctx context.Context, request GetEventsIdRequestObject) (GetEventsIdResponseObject, error) {
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

	return GetEventsId200JSONResponse(eventToApiEvent(event)), nil
}

func eventToApiEvent(event events.Event) Event {
	return Event{
		Id:                    &event.ID,
		Name:                  event.Name,
		Location:              locationToApiLocation(event.EventLocation),
		StartTime:             event.StartTime,
		EndTime:               event.EndTime,
		RegistrationCloseTime: event.RegistrationCloseTime,
		RegistrationTypes:     slices.Map(event.RegistrationTypes, func(t events.RegistrationType) RegistrationType { return registrationTypeToApiRegistrationType(t) }),
		AllowedTeamSizeRange: Range{
			Min: event.AllowedTeamSizeRange.Min,
			Max: event.AllowedTeamSizeRange.Max,
		},
		SignUpStats: SignUpStats{
			NumTeams:           &event.NumTeams,
			NumRosteredPlayers: &event.NumRosteredPlayers,
			NumTotalPlayers:    &event.NumTotalPlayers,
		},
	}
}

func apiEventToEvent(event Event) events.Event {
	return events.Event{
		ID:                    *event.Id,
		Name:                  event.Name,
		EventLocation:         apiLocationToLocation(event.Location),
		StartTime:             event.StartTime,
		EndTime:               event.EndTime,
		RegistrationCloseTime: event.RegistrationCloseTime,
		RegistrationTypes:     slices.Map(event.RegistrationTypes, func(t RegistrationType) events.RegistrationType { return apiRegistrationTypeToRegistrationType(t) }),
		NumTotalPlayers:       *event.SignUpStats.NumTotalPlayers,
		NumRosteredPlayers:    *event.SignUpStats.NumRosteredPlayers,
		NumTeams:              *event.SignUpStats.NumTeams,
		AllowedTeamSizeRange: events.Range{
			Min: event.AllowedTeamSizeRange.Min,
			Max: event.AllowedTeamSizeRange.Max,
		},
	}
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

func registrationTypeToApiRegistrationType(t events.RegistrationType) RegistrationType {
	switch t {
	case events.BY_INDIVIDUAL:
		return ByIndividual
	case events.BY_TEAM:
		return ByTeam
	default:
		panic("unknown registration type")
	}
}

func apiRegistrationTypeToRegistrationType(t RegistrationType) events.RegistrationType {
	switch t {
	case ByIndividual:
		return events.BY_INDIVIDUAL
	case ByTeam:
		return events.BY_TEAM
	default:
		panic("unknown registration type")
	}
}
