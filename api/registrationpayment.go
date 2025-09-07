package api

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/middleware"
)

func (a *API) stripeRegistrationPaymentWebhookMiddleware(path string) middleware.MiddlewareFunc {
	server := http.NewServeMux()

	server.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		logger := a.getLoggerOrBaseLogger(ctx)

		r.Body = http.MaxBytesReader(w, r.Body, 65536)
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			logger.Error("Failed to read stripe webhook body", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		reg, err := registration.ConfirmRegistrationPayment(ctx, payload, r.Header.Get("Stripe-Signature"), a.db, a.checkoutManager)
		if err != nil {
			logger.Error("Failed to confirm registration payment", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		event, err := a.db.GetEvent(ctx, reg.GetEventID())
		if err != nil {
			logger.Error("Failed to get event ID to send email with", slog.String("error", err.Error()))

			// TODO: Probably wantbetter error handling here
			w.WriteHeader(http.StatusOK)
			return
		}

		err = registration.SendRegistrationConfirmationEmail(ctx, a.emailSender, "ICAA <info@icaa.world>", reg, event)
		if err != nil {
			logger.Error("failed to send email to signed up player", slog.String("error", err.Error()), slog.String("email", reg.GetEmail()))

			// TODO: Is there other error handling we should do here?
			// I don't want to send a failed status code to the user
			// because they did actually sign up succesfully still...
		}

		w.WriteHeader(http.StatusOK)
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler, matchedPath := server.Handler(r)

			if matchedPath == "" {
				next.ServeHTTP(w, r)
				return
			}

			handler.ServeHTTP(w, r)
		})
	}
}
