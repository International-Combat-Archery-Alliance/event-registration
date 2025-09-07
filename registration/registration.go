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
	GetAllRegistrationsForEvent(ctx context.Context, eventId uuid.UUID, limit int32, cursor *string) (GetAllRegistrationsResponse, error)
	CreateRegistrationWithPayment(ctx context.Context, registration Registration, intent RegistrationIntent, event events.Event) error
	UpdateRegistrationToPaid(ctx context.Context, registration Registration) error
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
	})
	if err != nil {
		return nil, "", events.Event{}, NewFailedToCreateCheckoutError("Failed to create checkout", err)
	}

	event.Version++
	err = registrationRepo.CreateRegistrationWithPayment(ctx, registrationRequest, RegistrationIntent{
		Version:          1,
		PaymentSessionId: checkoutInfo.SessionId,
		Email:            registrationRequest.GetEmail(),
	}, event)
	if err != nil {
		return nil, "", events.Event{}, err
	}
	return registrationRequest, checkoutInfo.ClientSecret, event, nil
}

func ConfirmRegistrationPayment(ctx context.Context, payload []byte, signature string, registrationRepo Repository, checkoutManager payments.CheckoutManager) (Registration, error) {
	metadata, err := checkoutManager.ConfirmCheckout(ctx, payload, signature)
	if err != nil {
		return nil, err
	}

	// TODO: Need to handle payment failed/cancelled/expired

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

	reg, err := registrationRepo.GetRegistration(ctx, eventId, email)
	if err != nil {
		return nil, err
	}
	reg.BumpVersion()
	reg.SetToPaid()

	err = registrationRepo.UpdateRegistrationToPaid(ctx, reg)
	return reg, err
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
