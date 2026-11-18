package events

import (
	"context"
	"time"

	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("github.com/International-Combat-Archery-Alliance/event-registration/events")

type Event struct {
	ID                    uuid.UUID
	Version               int
	Name                  string
	EventLocation         Location
	TimeZone              *time.Location
	StartTime             time.Time
	EndTime               time.Time
	RegistrationCloseTime time.Time
	RegistrationOptions   []EventRegistrationOption
	AllowedTeamSizeRange  Range
	NumTeams              int
	NumRosteredPlayers    int
	NumTotalPlayers       int
	RulesDocLink          *string
	ImageName             *string
	Media                 []EventMediaItem
	MailingListGroupID    *string
}

type EventMediaType string

const (
	MediaTypeAssetFolder  EventMediaType = "asset_folder"
	MediaTypeAssetFile    EventMediaType = "asset_file"
	MediaTypeVideo        EventMediaType = "video"
	MediaTypeExternalLink EventMediaType = "external_link"
)

type EventMediaDisplayMode string

const (
	MediaDisplayModeAuto     EventMediaDisplayMode = "auto"
	MediaDisplayModeCarousel EventMediaDisplayMode = "carousel"
	MediaDisplayModeGallery  EventMediaDisplayMode = "gallery"
	MediaDisplayModeFileList EventMediaDisplayMode = "file_list"
	MediaDisplayModeEmbed    EventMediaDisplayMode = "embed"
	MediaDisplayModeLink     EventMediaDisplayMode = "link"
)

type EventMediaIcon string

const (
	MediaIconYouTube      EventMediaIcon = "youtube"
	MediaIconTwitch       EventMediaIcon = "twitch"
	MediaIconGoogleDrive  EventMediaIcon = "google_drive"
	MediaIconInstagram    EventMediaIcon = "instagram"
	MediaIconPDF          EventMediaIcon = "pdf"
	MediaIconPhotoGallery EventMediaIcon = "photo_gallery"
	MediaIconFile         EventMediaIcon = "file"
	MediaIconVideo        EventMediaIcon = "video"
	MediaIconExternalLink EventMediaIcon = "external_link"
)

type EventMediaItem struct {
	ID          uuid.UUID
	Type        EventMediaType
	Source      string
	Title       *string
	Description *string
	DisplayMode *EventMediaDisplayMode
	Icon        *EventMediaIcon
}

type EventRegistrationOption struct {
	RegType RegistrationType
	Price   *money.Money
}

type Range struct {
	Min int
	Max int
}

type GetEventsResponse struct {
	Data        []Event
	Cursor      *string
	HasNextPage bool
}

type Repository interface {
	GetEvent(ctx context.Context, id uuid.UUID) (Event, error)
	GetEvents(ctx context.Context, limit int32, cursor *string) (GetEventsResponse, error)
	CreateEvent(ctx context.Context, event Event) error
	UpdateEvent(ctx context.Context, event Event) error
}

func UpdateEvent(ctx context.Context, repo Repository, id uuid.UUID, event Event) (Event, error) {
	ctx, span := tracer.Start(ctx, "UpdateEvent")
	defer span.End()

	span.SetAttributes(attribute.String("event_id", id.String()))

	existingEvent, err := repo.GetEvent(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Event{}, err
	}

	media := existingEvent.Media
	if event.Media != nil {
		media = event.Media
	}

	updatedEvent := Event{
		ID:                    id,
		Version:               existingEvent.Version + 1,
		Name:                  event.Name,
		StartTime:             event.StartTime,
		EndTime:               event.EndTime,
		TimeZone:              event.TimeZone,
		EventLocation:         event.EventLocation,
		RegistrationCloseTime: event.RegistrationCloseTime,
		RegistrationOptions:   event.RegistrationOptions,
		AllowedTeamSizeRange:  event.AllowedTeamSizeRange,
		NumTeams:              existingEvent.NumTeams,
		NumRosteredPlayers:    existingEvent.NumRosteredPlayers,
		NumTotalPlayers:       existingEvent.NumTotalPlayers,
		RulesDocLink:          event.RulesDocLink,
		ImageName:             event.ImageName,
		Media:                 media,
		MailingListGroupID:    existingEvent.MailingListGroupID,
	}

	err = repo.UpdateEvent(ctx, updatedEvent)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Event{}, err
	}

	return updatedEvent, nil
}
