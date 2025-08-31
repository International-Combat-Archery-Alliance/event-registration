package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"
)

func (a *API) PostEventsV1EventIdRegister(ctx context.Context, request PostEventsV1EventIdRegisterRequestObject) (PostEventsV1EventIdRegisterResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	validatedData, err := a.captchaValidator.Validate(ctx, request.Params.CfTurnstileResponse, "")
	if err != nil {
		logger.Warn("Invalid captcha", slog.String("error", err.Error()))

		return PostEventsV1EventIdRegister400JSONResponse{
			Code:    CaptchaInvalid,
			Message: "Invalid captcha",
		}, nil
	}
	if a.env == PROD && validatedData.Hostname() != "icaa.world" {
		logger.Warn("Invalid captcha hostname", slog.String("givenHostname", validatedData.Hostname()))

		return PostEventsV1EventIdRegister400JSONResponse{
			Code:    CaptchaInvalid,
			Message: "Invalid hostname, must come from icaa.world",
		}, nil
	}

	// request.Body is guaranteed to be non-nil from openapi doc
	reg, err := apiRegistrationToRegistration(*request.Body, request.EventId)
	if err != nil {
		logger.Warn("Invalid body for registration", "error", err)

		return PostEventsV1EventIdRegister400JSONResponse{
			Code:    InvalidBody,
			Message: "Invalid body",
		}, nil
	}
	signedUpReg, event, err := registration.AttemptRegistration(ctx, reg, a.db, a.db)
	if err != nil {
		logger.Error("Error trying to register", "error", err)

		var registrationErr *registration.Error

		if errors.As(err, &registrationErr) {
			switch registrationErr.Reason {
			case registration.REASON_ASSOCIATED_EVENT_DOES_NOT_EXIST:
				return PostEventsV1EventIdRegister404JSONResponse{
					Code:    NotFound,
					Message: "Event to register with was not found",
				}, nil
			case registration.REASON_REGISTRATION_IS_CLOSED:
				return PostEventsV1EventIdRegister403JSONResponse{
					Code:    RegistrationClosed,
					Message: "Registration has closed for this event",
				}, nil
			case registration.REASON_REGISTRATION_ALREADY_EXISTS:
				return PostEventsV1EventIdRegister409JSONResponse{
					Code:    AlreadyExists,
					Message: "Registration already exists for this email",
				}, nil
			}
		}

		return PostEventsV1EventIdRegister500JSONResponse{
			Code:    InternalError,
			Message: "Failed to register",
		}, nil
	}

	respReg, err := registrationToApiRegistration(signedUpReg)
	if err != nil {
		logger.Error("Failed to convert registration to api registration", "error", err)

		return PostEventsV1EventIdRegister500JSONResponse{
			Code:    InternalError,
			Message: "Failed to register",
		}, nil
	}

	err = registration.SendRegistrationConfirmationEmail(ctx, a.emailSender, "info@icaa.world", signedUpReg, event)
	if err != nil {
		logger.Error("failed to send email to signed up player", slog.String("error", err.Error()), slog.String("email", reg.GetEmail()))

		// TODO: Is there other error handling we should do here?
		// I don't want to send a failed status code to the user
		// because they did actually sign up succesfully still...
	}

	return PostEventsV1EventIdRegister200JSONResponse{Registration: respReg}, nil
}

func (a *API) GetEventsV1EventIdRegistrations(ctx context.Context, request GetEventsV1EventIdRegistrationsRequestObject) (GetEventsV1EventIdRegistrationsResponseObject, error) {
	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// limit is guaranteed to be non-nil from openapi doc
	limit := *request.Params.Limit

	result, err := a.db.GetAllRegistrationsForEvent(ctx, request.EventId, int32(limit), request.Params.Cursor)
	if err != nil {
		logger.Error("Failed to get registrations for event", "error", err, "eventId", request.EventId)

		var registrationErr *registration.Error
		if errors.As(err, &registrationErr) {
			switch registrationErr.Reason {
			case registration.REASON_INVALID_CURSOR:
				return GetEventsV1EventIdRegistrations400JSONResponse{
					Code:    InvalidCursor,
					Message: "Cursor is invalid",
				}, nil
			}
		}
		return GetEventsV1EventIdRegistrations500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get registrations",
		}, nil
	}

	respRegs := []Registration{}
	for _, v := range result.Data {
		convReg, err := registrationToApiRegistration(v)
		if err != nil {
			logger.Error("Failed to convert registration to api registration", "error", err)

			return GetEventsV1EventIdRegistrations500JSONResponse{
				Code:    InternalError,
				Message: "Failed to get registrations",
			}, nil
		}
		respRegs = append(respRegs, convReg)
	}

	return GetEventsV1EventIdRegistrations200JSONResponse{
		Data:        respRegs,
		Cursor:      result.Cursor,
		HasNextPage: result.HasNextPage,
	}, nil
}

func apiRegistrationToRegistration(apiReg Registration, eventId uuid.UUID) (registration.Registration, error) {
	discrim, err := apiReg.Discriminator()
	if err != nil {
		return nil, fmt.Errorf("Failed to get discriminator: %w", err)
	}

	// TODO: this doesn't work for updates, but that I can figure out later
	id := uuid.New()
	version := 1
	registeredAt := time.Now()
	paid := false

	switch discrim {
	case string(ByIndividual):
		apiIndivReg, err := apiReg.AsIndividualRegistration()
		if err != nil {
			return nil, fmt.Errorf("Failed to convert to indiv registration: %w", err)
		}

		experience, err := apiExperienceToExperience(apiIndivReg.Experience)
		if err != nil {
			return nil, err
		}

		return registration.IndividualRegistration{
			ID:           id,
			EventID:      eventId,
			Version:      version,
			RegisteredAt: registeredAt,
			HomeCity:     apiIndivReg.HomeCity,
			Paid:         paid,
			Email:        string(apiIndivReg.Email),
			PlayerInfo:   apiPlayerInfoToPlayerInfo(apiIndivReg.PlayerInfo),
			Experience:   experience,
		}, nil
	case string(ByTeam):
		apiTeamReg, err := apiReg.AsTeamRegistration()
		if err != nil {
			return nil, fmt.Errorf("Failed to convert to team registration")
		}

		return registration.TeamRegistration{
			ID:           id,
			EventID:      eventId,
			Version:      version,
			RegisteredAt: registeredAt,
			HomeCity:     apiTeamReg.HomeCity,
			TeamName:     apiTeamReg.TeamName,
			Paid:         paid,
			CaptainEmail: string(apiTeamReg.CaptainEmail),
			Players: slices.Map(apiTeamReg.Players, func(v PlayerInfo) registration.PlayerInfo {
				return apiPlayerInfoToPlayerInfo(v)
			}),
		}, nil
	default:
		return nil, fmt.Errorf("Unknown discriminator: %s", discrim)
	}
}

func registrationToApiRegistration(reg registration.Registration) (Registration, error) {
	switch reg.Type() {
	case events.BY_INDIVIDUAL:
		indivReg := reg.(registration.IndividualRegistration)

		experience, err := experienceToApiExperience(indivReg.Experience)
		if err != nil {
			return Registration{}, err
		}

		apiIndivReg := IndividualRegistration{
			Id:           &indivReg.ID,
			EventId:      &indivReg.EventID,
			Version:      &indivReg.Version,
			Email:        types.Email(indivReg.Email),
			Paid:         &indivReg.Paid,
			RegisteredAt: &indivReg.RegisteredAt,
			HomeCity:     indivReg.HomeCity,
			Experience:   experience,
			PlayerInfo:   playerInfoToApiPlayerInfo(indivReg.PlayerInfo),
		}

		apiReg := &Registration{}
		err = apiReg.FromIndividualRegistration(apiIndivReg)
		if err != nil {
			return Registration{}, fmt.Errorf("Failed to convert individual registration to api type: %w", err)
		}

		return *apiReg, nil
	case events.BY_TEAM:
		teamReg := reg.(registration.TeamRegistration)

		apiTeamReg := TeamRegistration{
			Id:           &teamReg.ID,
			EventId:      &teamReg.EventID,
			Version:      &teamReg.Version,
			CaptainEmail: types.Email(teamReg.CaptainEmail),
			HomeCity:     teamReg.HomeCity,
			Paid:         &teamReg.Paid,
			TeamName:     teamReg.TeamName,
			RegisteredAt: &teamReg.RegisteredAt,
			Players: slices.Map(teamReg.Players, func(v registration.PlayerInfo) PlayerInfo {
				return playerInfoToApiPlayerInfo(v)
			}),
		}

		apiReg := &Registration{}
		err := apiReg.FromTeamRegistration(apiTeamReg)
		if err != nil {
			return Registration{}, fmt.Errorf("Failed to convert team registration to api type: %w", err)
		}

		return *apiReg, nil
	default:
		return Registration{}, fmt.Errorf("Unknown registration type: %s", reg.Type())
	}
}

func apiPlayerInfoToPlayerInfo(playerInfo PlayerInfo) registration.PlayerInfo {
	return registration.PlayerInfo{
		FirstName: playerInfo.FirstName,
		LastName:  playerInfo.LastName,
	}
}

func playerInfoToApiPlayerInfo(playerInfo registration.PlayerInfo) PlayerInfo {
	return PlayerInfo{
		FirstName: playerInfo.FirstName,
		LastName:  playerInfo.LastName,
	}
}

func apiExperienceToExperience(exp ExperienceLevel) (registration.ExperienceLevel, error) {
	switch exp {
	case Novice:
		return registration.NOVICE, nil
	case Intermediate:
		return registration.INTERMEDIATE, nil
	case Advanced:
		return registration.ADVANCED, nil
	default:
		return registration.ExperienceLevel(0), fmt.Errorf("Unknown experience level: %s", exp)
	}
}

func experienceToApiExperience(exp registration.ExperienceLevel) (ExperienceLevel, error) {
	switch exp {
	case registration.NOVICE:
		return Novice, nil
	case registration.INTERMEDIATE:
		return Intermediate, nil
	case registration.ADVANCED:
		return Advanced, nil
	default:
		return ExperienceLevel(""), fmt.Errorf("Unknown experience level: %s", exp)
	}
}
