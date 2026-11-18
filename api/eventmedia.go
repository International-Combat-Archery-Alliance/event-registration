package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
)

const (
	maxEventMediaItems             = 50
	maxEventMediaListBytes         = 200 * 1024
	maxEventMediaSourceLength      = 1000
	maxEventMediaTitleLength       = 255
	maxEventMediaDescriptionLength = 1000
)

func eventMediaToApiMedia(media []events.EventMediaItem) *[]EventMediaItem {
	if len(media) == 0 {
		return nil
	}

	apiMedia := slices.Map(media, eventMediaItemToApiMediaItem)
	return &apiMedia
}

func apiMediaToEventMedia(media *[]EventMediaItem) []events.EventMediaItem {
	if media == nil {
		return nil
	}
	if *media == nil {
		return make([]events.EventMediaItem, 0)
	}

	return slices.Map(*media, apiMediaItemToEventMediaItem)
}

func eventMediaItemToApiMediaItem(item events.EventMediaItem) EventMediaItem {
	apiItem := EventMediaItem{
		Id:          item.ID,
		Type:        EventMediaType(item.Type),
		Source:      item.Source,
		Title:       item.Title,
		Description: item.Description,
	}
	if item.DisplayMode != nil {
		apiItem.DisplayMode = ptr.To(EventMediaDisplayMode(*item.DisplayMode))
	}
	if item.Icon != nil {
		apiItem.Icon = ptr.To(EventMediaIcon(*item.Icon))
	}
	return apiItem
}

func apiMediaItemToEventMediaItem(item EventMediaItem) events.EventMediaItem {
	eventItem := events.EventMediaItem{
		ID:          item.Id,
		Type:        events.EventMediaType(item.Type),
		Source:      item.Source,
		Title:       item.Title,
		Description: item.Description,
	}
	if item.DisplayMode != nil {
		eventItem.DisplayMode = ptr.To(events.EventMediaDisplayMode(*item.DisplayMode))
	}
	if item.Icon != nil {
		eventItem.Icon = ptr.To(events.EventMediaIcon(*item.Icon))
	}
	return eventItem
}

func (a *API) validateMedia(media *[]EventMediaItem) error {
	if media == nil {
		return nil
	}

	items := *media
	if len(items) > maxEventMediaItems {
		return fmt.Errorf("media cannot contain more than %d items", maxEventMediaItems)
	}

	serialized, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to serialize media: %w", err)
	}
	if len(serialized) > maxEventMediaListBytes {
		return fmt.Errorf("media cannot exceed %d bytes when serialized", maxEventMediaListBytes)
	}

	ids := make(map[uuid.UUID]struct{}, len(items))
	for i, item := range items {
		if item.Id == uuid.Nil {
			return fmt.Errorf("media item %d must have an id", i)
		}
		if _, exists := ids[item.Id]; exists {
			return fmt.Errorf("media item %d repeats id %q", i, item.Id)
		}
		ids[item.Id] = struct{}{}

		if err := a.validateMediaItem(item); err != nil {
			return fmt.Errorf("media item %d: %w", i, err)
		}
	}

	return nil
}

func (a *API) validateMediaItem(item EventMediaItem) error {
	if item.Source == "" {
		return errors.New("source is required")
	}
	if utf8.RuneCountInString(item.Source) > maxEventMediaSourceLength {
		return fmt.Errorf("source cannot exceed %d characters", maxEventMediaSourceLength)
	}

	switch item.Type {
	case EventMediaTypeAssetFolder:
		if err := validateAssetFolderSource(item.Source); err != nil {
			return err
		}
	case EventMediaTypeAssetFile, EventMediaTypeVideo, EventMediaTypeExternalLink:
		if err := a.validateMediaSourceURL(item.Source); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown media type %q", item.Type)
	}

	if item.DisplayMode != nil && !isKnownMediaDisplayMode(*item.DisplayMode) {
		return fmt.Errorf("unknown display mode %q", *item.DisplayMode)
	}
	if item.Icon != nil && !isKnownMediaIcon(*item.Icon) {
		return fmt.Errorf("unknown icon %q", *item.Icon)
	}
	if item.Title != nil && utf8.RuneCountInString(*item.Title) > maxEventMediaTitleLength {
		return fmt.Errorf("title cannot exceed %d characters", maxEventMediaTitleLength)
	}
	if item.Description != nil && utf8.RuneCountInString(*item.Description) > maxEventMediaDescriptionLength {
		return fmt.Errorf("description cannot exceed %d characters", maxEventMediaDescriptionLength)
	}

	return nil
}

func validateAssetFolderSource(source string) error {
	if !strings.HasPrefix(source, "/") {
		return errors.New("folder source must start with /")
	}
	if source != "/" && strings.HasSuffix(source, "/") {
		return errors.New("folder source must not end with /")
	}
	if strings.ContainsAny(source, `\?#`) {
		return errors.New("folder source must not contain backslashes, queries, or fragments")
	}
	if containsControlChar(source) {
		return errors.New("folder source must not contain control characters")
	}

	decoded, err := url.PathUnescape(source)
	if err != nil {
		return fmt.Errorf("folder source must be a canonical unencoded path: %w", err)
	}
	if decoded != source {
		return errors.New("folder source must not contain percent-encoded characters")
	}

	if source == "/" {
		return nil
	}

	for _, segment := range strings.Split(strings.TrimPrefix(source, "/"), "/") {
		switch segment {
		case "":
			return errors.New("folder source must not contain empty segments")
		case ".", "..":
			return errors.New("folder source must not contain relative segments")
		}
	}

	return nil
}

func (a *API) validateMediaSourceURL(source string) error {
	if strings.Contains(source, `\`) {
		return errors.New("source must not contain backslashes")
	}
	if containsSpaceOrControlChar(source) {
		return errors.New("source must not contain whitespace or control characters")
	}

	parsed, err := url.Parse(source)
	if err != nil {
		return fmt.Errorf("source is not a valid URL: %w", err)
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return errors.New("source must be an absolute URL with a host")
	}
	if parsed.User != nil {
		return errors.New("source must not contain credentials")
	}

	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if a.env == LOCAL && isLoopbackHost(parsed.Hostname()) {
			return nil
		}
		return errors.New("source must use https")
	default:
		return fmt.Errorf("source must use http or https, got %q", parsed.Scheme)
	}
}

func containsControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func containsSpaceOrControlChar(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func isKnownMediaDisplayMode(mode EventMediaDisplayMode) bool {
	switch mode {
	case Auto, Carousel, Gallery, FileList, Embed, Link:
		return true
	default:
		return false
	}
}

func isKnownMediaIcon(icon EventMediaIcon) bool {
	switch icon {
	case EventMediaIconYoutube, EventMediaIconTwitch, EventMediaIconGoogleDrive, EventMediaIconInstagram,
		EventMediaIconPdf, EventMediaIconPhotoGallery, EventMediaIconFile, EventMediaIconVideo, EventMediaIconExternalLink:
		return true
	default:
		return false
	}
}
