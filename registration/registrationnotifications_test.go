package registration

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func noopRegistrationLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

type stubEmailSender struct {
	sendErr error
	sent    []email.Email
}

func (s *stubEmailSender) SendEmail(ctx context.Context, e email.Email) error {
	s.sent = append(s.sent, e)
	return s.sendErr
}

type stubSubscriberManager struct {
	added  [][3]string
	addErr error
}

func (s *stubSubscriberManager) CreateGroup(ctx context.Context, name string) (string, error) {
	return "group-id", nil
}

func (s *stubSubscriberManager) FindOrCreateGroup(ctx context.Context, name string) (string, error) {
	return "group-id", nil
}

func (s *stubSubscriberManager) AddSubscriberToGroup(ctx context.Context, email, name, groupID string) error {
	s.added = append(s.added, [3]string{email, name, groupID})
	return s.addErr
}

func TestSendRegistrationNotifications(t *testing.T) {
	newReg := func() *IndividualRegistration {
		return &IndividualRegistration{
			ID:         uuid.New(),
			Version:    2,
			EventID:    uuid.New(),
			Email:      "test@test.com",
			Paid:       true,
			Experience: NOVICE,
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
		}
	}
	newEvent := func() events.Event {
		return events.Event{
			ID:                 uuid.New(),
			Name:               "Test Event",
			MailingListGroupID: ptr.String("group-id"),
		}
	}

	t.Run("sends email and adds to mailing list", func(t *testing.T) {
		sender := &stubEmailSender{}
		subscribers := &stubSubscriberManager{}
		reg := newReg()
		event := newEvent()

		err := SendRegistrationNotifications(context.Background(), sender, subscribers, reg, event, noopRegistrationLogger())
		assert.NoError(t, err)
		assert.Len(t, sender.sent, 1)
		assert.Equal(t, []string{"test@test.com"}, sender.sent[0].ToAddresses)
		assert.Len(t, subscribers.added, 1)
		assert.Equal(t, [3]string{"test@test.com", "first last", "group-id"}, subscribers.added[0])
	})

	t.Run("email failure still adds to mailing list and returns error", func(t *testing.T) {
		sender := &stubEmailSender{sendErr: errors.New("smtp down")}
		subscribers := &stubSubscriberManager{}

		err := SendRegistrationNotifications(context.Background(), sender, subscribers, newReg(), newEvent(), noopRegistrationLogger())
		assert.Error(t, err)
		assert.Len(t, subscribers.added, 1)
	})

	t.Run("no mailing list group skips subscription", func(t *testing.T) {
		sender := &stubEmailSender{}
		subscribers := &stubSubscriberManager{}
		event := newEvent()
		event.MailingListGroupID = nil

		err := SendRegistrationNotifications(context.Background(), sender, subscribers, newReg(), event, noopRegistrationLogger())
		assert.NoError(t, err)
		assert.Len(t, sender.sent, 1)
		assert.Empty(t, subscribers.added)
	})
}

func TestNotifyRegistration(t *testing.T) {
	newReg := func() *IndividualRegistration {
		return &IndividualRegistration{
			ID:         uuid.New(),
			Version:    2,
			EventID:    uuid.New(),
			Email:      "test@test.com",
			Paid:       true,
			Experience: NOVICE,
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
		}
	}
	newEventRepo := func(event events.Event, err error) *mockEventRepository {
		return &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, err
			},
		}
	}

	t.Run("fetches event and notifies", func(t *testing.T) {
		sender := &stubEmailSender{}
		subscribers := &stubSubscriberManager{}
		event := events.Event{ID: uuid.New(), Name: "Test Event", MailingListGroupID: ptr.String("group-id")}

		NotifyRegistration(context.Background(), newEventRepo(event, nil), sender, subscribers, newReg(), noopRegistrationLogger())
		assert.Len(t, sender.sent, 1)
		assert.Len(t, subscribers.added, 1)
	})

	t.Run("event fetch failure notifies nothing", func(t *testing.T) {
		sender := &stubEmailSender{}
		subscribers := &stubSubscriberManager{}

		NotifyRegistration(context.Background(), newEventRepo(events.Event{}, errors.New("some error")), sender, subscribers, newReg(), noopRegistrationLogger())
		assert.Empty(t, sender.sent)
		assert.Empty(t, subscribers.added)
	})

	t.Run("email failure still adds to mailing list", func(t *testing.T) {
		sender := &stubEmailSender{sendErr: errors.New("smtp down")}
		subscribers := &stubSubscriberManager{}
		event := events.Event{ID: uuid.New(), Name: "Test Event", MailingListGroupID: ptr.String("group-id")}

		NotifyRegistration(context.Background(), newEventRepo(event, nil), sender, subscribers, newReg(), noopRegistrationLogger())
		assert.Len(t, sender.sent, 1)
		assert.Len(t, subscribers.added, 1)
	})
}

func TestConfirmPaidRegistration(t *testing.T) {
	newUnpaidReg := func(eventID uuid.UUID) *IndividualRegistration {
		return &IndividualRegistration{
			ID:         uuid.New(),
			Version:    1,
			EventID:    eventID,
			Email:      "test@test.com",
			Paid:       false,
			Experience: NOVICE,
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
		}
	}
	newEventRepo := func(err error, called *bool) *mockEventRepository {
		return &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				if called != nil {
					*called = true
				}
				if err != nil {
					return events.Event{}, err
				}
				return events.Event{ID: id, Name: "Test Event", MailingListGroupID: ptr.String("group-id")}, nil
			},
		}
	}

	t.Run("unpaid registration is marked paid and notified", func(t *testing.T) {
		eventID := uuid.New()
		updateCalled := false
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				return newUnpaidReg(eventID), nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, registration Registration) error {
				updateCalled = true
				return nil
			},
		}
		sender := &stubEmailSender{}
		subscribers := &stubSubscriberManager{}

		result, alreadyPaid, err := ConfirmPaidRegistration(context.Background(), eventID, "test@test.com", registrationRepo, newEventRepo(nil, nil), sender, subscribers, noopRegistrationLogger())
		assert.NoError(t, err)
		assert.False(t, alreadyPaid)
		assert.True(t, updateCalled)
		assert.True(t, result.(*IndividualRegistration).Paid)
		assert.Len(t, sender.sent, 1)
		assert.Len(t, subscribers.added, 1)
	})

	t.Run("already paid registration writes and sends nothing", func(t *testing.T) {
		eventID := uuid.New()
		updateCalled := false
		eventFetched := false
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				reg := newUnpaidReg(eventID)
				reg.Paid = true
				reg.Version = 2
				return reg, nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, registration Registration) error {
				updateCalled = true
				return nil
			},
		}
		sender := &stubEmailSender{}
		subscribers := &stubSubscriberManager{}

		result, alreadyPaid, err := ConfirmPaidRegistration(context.Background(), eventID, "test@test.com", registrationRepo, newEventRepo(nil, &eventFetched), sender, subscribers, noopRegistrationLogger())
		assert.NoError(t, err)
		assert.True(t, alreadyPaid)
		assert.False(t, updateCalled)
		assert.False(t, eventFetched)
		assert.Empty(t, sender.sent)
		assert.Equal(t, 2, result.(*IndividualRegistration).Version)
	})

	t.Run("missing registration returns error without notifying", func(t *testing.T) {
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				return nil, NewRegistrationDoesNotExistsError("not found", nil)
			},
		}
		sender := &stubEmailSender{}
		subscribers := &stubSubscriberManager{}

		_, _, err := ConfirmPaidRegistration(context.Background(), uuid.New(), "missing@test.com", registrationRepo, newEventRepo(nil, nil), sender, subscribers, noopRegistrationLogger())
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_REGISTRATION_DOES_NOT_EXIST, registrationErr.Reason)
		assert.Empty(t, sender.sent)
	})

	t.Run("event fetch failure still succeeds without notifying", func(t *testing.T) {
		eventID := uuid.New()
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				return newUnpaidReg(eventID), nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, registration Registration) error {
				return nil
			},
		}
		sender := &stubEmailSender{}
		subscribers := &stubSubscriberManager{}

		result, alreadyPaid, err := ConfirmPaidRegistration(context.Background(), eventID, "test@test.com", registrationRepo, newEventRepo(errors.New("some error"), nil), sender, subscribers, noopRegistrationLogger())
		assert.NoError(t, err)
		assert.False(t, alreadyPaid)
		assert.True(t, result.(*IndividualRegistration).Paid)
		assert.Empty(t, sender.sent)
	})

	t.Run("email failure still succeeds", func(t *testing.T) {
		eventID := uuid.New()
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				return newUnpaidReg(eventID), nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, registration Registration) error {
				return nil
			},
		}
		sender := &stubEmailSender{sendErr: errors.New("smtp down")}
		subscribers := &stubSubscriberManager{}

		_, alreadyPaid, err := ConfirmPaidRegistration(context.Background(), eventID, "test@test.com", registrationRepo, newEventRepo(nil, nil), sender, subscribers, noopRegistrationLogger())
		assert.NoError(t, err)
		assert.False(t, alreadyPaid)
		assert.Len(t, subscribers.added, 1)
	})
}

func TestSendRegistrationNotificationsSubscriberFailure(t *testing.T) {
	t.Run("mailing list failure does not fail notifications", func(t *testing.T) {
		sender := &stubEmailSender{}
		subscribers := &stubSubscriberManager{addErr: errors.New("mailerlite down")}
		reg := &IndividualRegistration{
			ID:         uuid.New(),
			Version:    2,
			EventID:    uuid.New(),
			Email:      "test@test.com",
			Paid:       true,
			Experience: NOVICE,
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
		}
		event := events.Event{
			ID:                 reg.EventID,
			Name:               "Test Event",
			MailingListGroupID: ptr.String("group-id"),
		}

		err := SendRegistrationNotifications(context.Background(), sender, subscribers, reg, event, noopRegistrationLogger())
		assert.NoError(t, err)
		assert.Len(t, sender.sent, 1)
		assert.Len(t, subscribers.added, 1)
	})
}
