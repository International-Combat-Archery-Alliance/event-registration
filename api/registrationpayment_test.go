package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestStripeRegistrationPaymentWebhookMiddleware(t *testing.T) {
	t.Run("successful payment confirmation webhook", func(t *testing.T) {
		eventID := uuid.New()
		email := "webhook@example.com"

		reg := &registration.IndividualRegistration{
			ID:      uuid.New(),
			EventID: eventID,
			Email:   email,
			Version: 1,
			Paid:    false,
		}

		mockDB := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				return reg, nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, registration registration.Registration) error {
				return nil
			},
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{
					ID:   eventID,
					Name: "Test Event",
				}, nil
			},
		}

		mockCheckout := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				return map[string]string{
					"EMAIL":    email,
					"EVENT_ID": eventID.String(),
				}, nil
			},
		}

		api := NewAPI(mockDB, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, mockCheckout)

		// Create a test server with the middleware
		middleware := api.stripeRegistrationPaymentWebhookMiddleware("/test/webhook")
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound) // Should not reach here
		}))

		req := httptest.NewRequest("POST", "/test/webhook", strings.NewReader("test_payload"))
		req.Header.Set("Stripe-Signature", "test_signature")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("webhook with invalid signature", func(t *testing.T) {
		mockDB := &mockDB{}
		mockCheckout := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				return nil, errors.New("invalid signature")
			},
		}

		api := NewAPI(mockDB, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, mockCheckout)

		middleware := api.stripeRegistrationPaymentWebhookMiddleware("/test/webhook")
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

		req := httptest.NewRequest("POST", "/test/webhook", strings.NewReader("test_payload"))
		req.Header.Set("Stripe-Signature", "invalid_signature")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("webhook with payment error that should be ignored", func(t *testing.T) {
		mockDB := &mockDB{}
		mockCheckout := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				return nil, &payments.Error{Reason: payments.ErrorReasonNotCheckoutConfirmedEvent}
			},
		}

		api := NewAPI(mockDB, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, mockCheckout)

		middleware := api.stripeRegistrationPaymentWebhookMiddleware("/test/webhook")
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

		req := httptest.NewRequest("POST", "/test/webhook", strings.NewReader("test_payload"))
		req.Header.Set("Stripe-Signature", "test_signature")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code) // Should be OK since we ignore this error type
	})

	t.Run("non-matching path should pass through", func(t *testing.T) {
		mockDB := &mockDB{}
		mockCheckout := &mockCheckoutManager{}

		api := NewAPI(mockDB, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, mockCheckout)

		middleware := api.stripeRegistrationPaymentWebhookMiddleware("/test/webhook")
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot) // Should reach this handler
		}))

		req := httptest.NewRequest("POST", "/other/path", strings.NewReader("test_payload"))
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTeapot, w.Code) // Should pass through to next handler
	})

	t.Run("webhook with request body too large", func(t *testing.T) {
		mockDB := &mockDB{}
		mockCheckout := &mockCheckoutManager{}

		api := NewAPI(mockDB, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, mockCheckout)

		middleware := api.stripeRegistrationPaymentWebhookMiddleware("/test/webhook")
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

		// Create a large payload (larger than 65536 bytes)
		largePayload := strings.Repeat("x", 70000)
		req := httptest.NewRequest("POST", "/test/webhook", strings.NewReader(largePayload))
		req.Header.Set("Stripe-Signature", "test_signature")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}
