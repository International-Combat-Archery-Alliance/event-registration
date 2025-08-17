package api

import (
	"context"
	"errors"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
	"github.com/google/uuid"
)

func eventToApiEvent(event events.Event) Event {
	return Event{
		Id:                    &event.ID,
		Name:                  event.Name,
		Location:              locationToApiLocation(event.EventLocation),
		StartTime:             event.StartTime,
		EndTime:               event.EndTime,
		RegistrationCloseTime: event.RegistrationCloseTime,
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

		var eventErr *events.EventError
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
	event := events.Event{
		ID:                    uuid.New(),
		Name:                  request.Body.Name,
		EventLocation:         apiLocationToLocation(request.Body.Location),
		StartTime:             request.Body.StartTime,
		EndTime:               request.Body.EndTime,
		RegistrationCloseTime: request.Body.RegistrationCloseTime,
	}

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

		var eventErr *events.EventError
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
