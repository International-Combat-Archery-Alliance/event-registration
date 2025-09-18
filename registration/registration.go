package registration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/google/uuid"
)

type Repository interface {
	CreateRegistration(ctx context.Context, registration Registration, event events.Event) error
	GetRegistration(ctx context.Context, eventId uuid.UUID, email string) (Registration, error)
	GetRegistrationIntent(ctx context.Context, eventId uuid.UUID, email string) (RegistrationIntent, error)
	GetAllRegistrationsForEvent(ctx context.Context, eventId uuid.UUID, limit int32, cursor *string) (GetAllRegistrationsResponse, error)
	CreateRegistrationWithPayment(ctx context.Context, registration Registration, intent RegistrationIntent, event events.Event) error
	UpdateRegistrationToPaid(ctx context.Context, registration Registration) error
	DeleteExpiredRegistration(ctx context.Context, registration Registration, intent RegistrationIntent, event events.Event) error
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
	SetToPaid()
	BumpVersion()
}

var _ Registration = &IndividualRegistration{}

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

func (r *IndividualRegistration) SetToPaid() {
	r.Paid = true
}

func (r *IndividualRegistration) BumpVersion() {
	r.Version++
}

var _ Registration = &TeamRegistration{}

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

func (r *TeamRegistration) SetToPaid() {
	r.Paid = true
}

func (r *TeamRegistration) BumpVersion() {
	r.Version++
}

const (
	emailKey   = "EMAIL"
	eventIdKey = "EVENT_ID"
)

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
		err = registerIndividualAsFreeAgent(&event, registrationRequest.(*IndividualRegistration))
		if err != nil {
			return nil, events.Event{}, err
		}
	case events.BY_TEAM:
		err = registerTeam(&event, registrationRequest.(*TeamRegistration))
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

func RegisterWithPayment(ctx context.Context, registrationRequest Registration, eventRepo events.Repository, registrationRepo Repository, checkoutManager payments.CheckoutManager, paymentReturnURL string) (Registration, string, events.Event, error) {
	eventId := registrationRequest.GetEventID()

	event, err := eventRepo.GetEvent(ctx, eventId)
	if err != nil {
		var eventErr *events.Error
		if errors.As(err, &eventErr) {
			switch eventErr.Reason {
			case events.REASON_EVENT_DOES_NOT_EXIST:
				return nil, "", events.Event{}, NewAssociatedEventDoesNotExistError(fmt.Sprintf("Event does not exist with ID %q", eventId), err)
			}
		}

		return nil, "", events.Event{}, NewFailedToFetchError(fmt.Sprintf("Failed to fetch event with ID %q", eventId), err)
	}

	var paymentItem payments.Item
	switch registrationRequest.Type() {
	case events.BY_INDIVIDUAL:
		regReq := registrationRequest.(*IndividualRegistration)
		err = registerIndividualAsFreeAgent(&event, regReq)
		if err != nil {
			return nil, "", events.Event{}, err
		}
		paymentItem = payments.Item{
			Name:     fmt.Sprintf("%s Free Agent Sign Up", event.Name),
			Quantity: 1,
			Price:    event.RegistrationOptions[slices.IndexFunc(event.RegistrationOptions, func(v events.EventRegistrationOption) bool { return v.RegType == events.BY_INDIVIDUAL })].Price,
		}
	case events.BY_TEAM:
		regReq := registrationRequest.(*TeamRegistration)
		err = registerTeam(&event, regReq)
		if err != nil {
			return nil, "", events.Event{}, err
		}
		paymentItem = payments.Item{
			Name:     fmt.Sprintf("%s Team Sign Up", event.Name),
			Quantity: 1,
			Price:    event.RegistrationOptions[slices.IndexFunc(event.RegistrationOptions, func(v events.EventRegistrationOption) bool { return v.RegType == events.BY_TEAM })].Price,
		}
	default:
		return nil, "", events.Event{}, NewUnknownRegistrationTypeError(fmt.Sprintf("Unknown registration type: %d", registrationRequest.Type()))
	}

	checkoutInfo, err := checkoutManager.CreateCheckout(ctx, payments.CheckoutParams{
		SessionAliveDuration: ptr.Duration(30 * time.Minute),
		ReturnURL:            paymentReturnURL,
		Items: []payments.Item{
			paymentItem,
		},
		Metadata: map[string]string{
			emailKey:   registrationRequest.GetEmail(),
			eventIdKey: event.ID.String(),
		},
		AllowAdaptivePricing: true,
		CustomerEmail:        ptr.String(registrationRequest.GetEmail()),
	})
	if err != nil {
		return nil, "", events.Event{}, NewFailedToCreateCheckoutError("Failed to create checkout", err)
	}

	event.Version++
	err = registrationRepo.CreateRegistrationWithPayment(ctx, registrationRequest, RegistrationIntent{
		EventId:          eventId,
		Version:          1,
		PaymentSessionId: checkoutInfo.SessionId,
		Email:            registrationRequest.GetEmail(),
	}, event)
	if err != nil {
		return nil, "", events.Event{}, err
	}
	return registrationRequest, checkoutInfo.ClientSecret, event, nil
}

func ConfirmRegistrationPayment(ctx context.Context, payload []byte, signature string, registrationRepo Repository, eventRepo events.Repository, checkoutManager payments.CheckoutManager) (Registration, error) {
	metadata, checkoutErr := checkoutManager.ConfirmCheckout(ctx, payload, signature)
	isExpired := checkoutIsExpired(checkoutErr)
	if checkoutErr != nil && !isExpired {
		return nil, checkoutErr
	}

	email, ok := metadata[emailKey]
	if !ok {
		return nil, NewPaymentMissingMetadataError(emailKey)
	}
	eventIdStr, ok := metadata[eventIdKey]
	if !ok {
		return nil, NewPaymentMissingMetadataError(eventIdKey)
	}

	eventId, err := uuid.Parse(eventIdStr)
	if err != nil {
		return nil, NewInvalidPaymentMetadata("Event ID is not a valid UUID", err)
	}

	if !isExpired {
		return setRegistrationToPaid(ctx, registrationRepo, eventId, email)
	} else {
		reg, err := deleteExpiredRegistration(ctx, registrationRepo, eventRepo, eventId, email)
		if err != nil {
			return nil, err
		}
		return reg, NewRegistrationExpiredError("Registration expired", checkoutErr)
	}
}

func setRegistrationToPaid(ctx context.Context, registrationRepo Repository, eventId uuid.UUID, email string) (Registration, error) {
	reg, err := registrationRepo.GetRegistration(ctx, eventId, email)
	if err != nil {
		return nil, err
	}
	reg.BumpVersion()
	reg.SetToPaid()

	err = registrationRepo.UpdateRegistrationToPaid(ctx, reg)
	return reg, err
}

func deleteExpiredRegistration(ctx context.Context, registrationRepo Repository, eventRepo events.Repository, eventId uuid.UUID, email string) (Registration, error) {
	reg, getRegErr := registrationRepo.GetRegistration(ctx, eventId, email)
	regIntent, getRegIntentErr := registrationRepo.GetRegistrationIntent(ctx, eventId, email)
	if getRegErr != nil && getRegIntentErr != nil {
		var regError *Error
		var regIntentError *Error

		// if both of them do not exist, just return nil since that means that they are already deleted
		if errors.As(getRegErr, &regError) && errors.As(getRegIntentErr, &regIntentError) {
			if regError.Reason == REASON_REGISTRATION_DOES_NOT_EXIST && regIntentError.Reason == REASON_REGISTRATION_DOES_NOT_EXIST {
				return nil, nil
			}
		}

		return nil, getRegErr
	} else if getRegErr != nil {
		return nil, getRegErr
	} else if getRegIntentErr != nil {
		return nil, getRegIntentErr
	}

	event, err := eventRepo.GetEvent(ctx, eventId)
	if err != nil {
		return nil, err
	}

	switch reg.Type() {
	case events.BY_INDIVIDUAL:
		unregisterIndividualFromEvent(&event)
	case events.BY_TEAM:
		unregisterTeamFromEvent(&event, reg.(*TeamRegistration))
	}

	event.Version++
	err = registrationRepo.DeleteExpiredRegistration(ctx, reg, regIntent, event)
	if err != nil {
		return nil, err
	}

	return reg, nil
}

func registerIndividualAsFreeAgent(event *events.Event, reg *IndividualRegistration) error {
	if !slices.ContainsFunc(event.RegistrationOptions, func(v events.EventRegistrationOption) bool { return v.RegType == events.BY_INDIVIDUAL }) {
		return NewNotAllowedToSignUpAsTypeError(events.BY_INDIVIDUAL)
	}

	if reg.RegisteredAt.After(event.RegistrationCloseTime) {
		return NewRegistrationIsClosedError(event.RegistrationCloseTime)
	}

	event.NumTotalPlayers++

	return nil
}

func unregisterIndividualFromEvent(event *events.Event) {
	event.NumTotalPlayers--
}

func registerTeam(event *events.Event, reg *TeamRegistration) error {
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

func unregisterTeamFromEvent(event *events.Event, reg *TeamRegistration) {
	teamSize := len(reg.Players)

	event.NumTeams--
	event.NumTotalPlayers -= teamSize
	event.NumRosteredPlayers -= teamSize
}

func checkoutIsExpired(err error) bool {
	var paymentError *payments.Error
	return err != nil && errors.As(err, &paymentError) && paymentError.Reason == payments.ErrorReasonCheckoutExpired
}
