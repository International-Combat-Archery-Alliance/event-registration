//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"context"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type API struct {
}

var _ StrictServerInterface = (*API)(nil)

func (a *API) GetEvents(ctx context.Context, request GetEventsRequestObject) (GetEventsResponseObject, error) {
	var id openapi_types.UUID = uuid.New()
	return GetEvents200JSONResponse{
		Event{Id: &id, Name: "fsad", EventDateTime: time.Now()},
	}, nil
}
