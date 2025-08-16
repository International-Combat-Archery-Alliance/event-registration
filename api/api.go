//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"log/slog"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
)

type DB interface {
	events.EventRepository
}

type API struct {
	db     DB
	logger *slog.Logger
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(db DB, logger *slog.Logger) *API {
	return &API{
		db:     db,
		logger: logger,
	}
}
