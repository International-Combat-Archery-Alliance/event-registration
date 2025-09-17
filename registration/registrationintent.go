package registration

import "github.com/google/uuid"

type RegistrationIntent struct {
	Version          int
	EventId          uuid.UUID
	PaymentSessionId string
	Email            string
}
