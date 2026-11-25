package events

import (
	"context"
	"time"

	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/International-Combat-Archery-Alliance/event-registration/games"
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
	Status                EventStatus
	// ScheduleGeneration counts accepted schedule writes. A generate takes its
	// epoch only after the request validated and the schedule built, but the
	// grant itself counts: a later storage failure can leave it advanced. It
	// is an internal audit counter carried on the event item, not an API
	// field.
	ScheduleGeneration int
	// LastGameNo is the per-event high-water mark for game numbers. It never
	// decreases, so a number is never recycled after a delete, and it makes
	// minting exact under the schedule-write guard.
	LastGameNo int
	// Tiebreakers is the per-event ordered comparator for standings and Swiss
	// pairing (D34). Empty means games.DefaultTiebreakOrder().
	Tiebreakers games.TiebreakOrder
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

	existingStatus := existingEvent.Status.NormalizeDefault()
	if !existingStatus.Valid() {
		err := NewInvalidEventStatusError("invalid existing event status", nil)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Event{}, err
	}
	nextStatus := event.Status
	if nextStatus == "" {
		// Status is optional in the API (old clients / pre-status rows):
		// preserve the existing status.
		nextStatus = existingStatus
	} else {
		if !nextStatus.Valid() {
			err := NewInvalidEventStatusError("invalid event status", nil)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return Event{}, err
		}
		if !AllowedTransition(existingStatus, nextStatus) {
			err := NewInvalidStatusTransitionError("invalid event status transition", nil)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return Event{}, err
		}
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
		Media:                 event.Media,
		MailingListGroupID:    existingEvent.MailingListGroupID,
		Status:                nextStatus,
		// Schedule counters are schedule state, not event-body fields: a PATCH
		// must carry them forward or the next schedule write would restart
		// minting from zero. Tiebreakers are body fields: the PATCH body is a
		// full replacement, and an absent list means the default order.
		ScheduleGeneration: existingEvent.ScheduleGeneration,
		LastGameNo:         existingEvent.LastGameNo,
		Tiebreakers:        event.Tiebreakers,
	}

	err = repo.UpdateEvent(ctx, updatedEvent)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Event{}, err
	}

	return updatedEvent, nil
}
