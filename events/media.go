package events

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxMediaItems             = 50
	maxMediaSourceLength      = 1000
	maxMediaTitleLength       = 255
	maxMediaDescriptionLength = 1000
)

// ValidateMedia checks a media list against the event media rules. allowLoopbackHTTP
// permits plain http loopback URLs, which are useful during local development.
func ValidateMedia(media []EventMediaItem, allowLoopbackHTTP bool) error {
	if len(media) > maxMediaItems {
		return fmt.Errorf("media cannot contain more than %d items", maxMediaItems)
	}

	ids := make(map[uuid.UUID]struct{}, len(media))
	for i, item := range media {
		if item.ID == uuid.Nil {
			return fmt.Errorf("media item %d must have an id", i)
		}
		if _, exists := ids[item.ID]; exists {
			return fmt.Errorf("media item %d repeats id %q", i, item.ID)
		}
		ids[item.ID] = struct{}{}

		if err := validateMediaItem(item, allowLoopbackHTTP); err != nil {
			return fmt.Errorf("media item %d: %w", i, err)
		}
	}

	return nil
}

func validateMediaItem(item EventMediaItem, allowLoopbackHTTP bool) error {
	if item.Source == "" {
		return errors.New("source is required")
	}
	if utf8.RuneCountInString(item.Source) > maxMediaSourceLength {
		return fmt.Errorf("source cannot exceed %d characters", maxMediaSourceLength)
	}

	switch item.Type {
	case MediaTypeAssetFolder:
		if err := validateAssetFolderSource(item.Source); err != nil {
			return err
		}
	case MediaTypeAssetFile, MediaTypeVideo, MediaTypeExternalLink:
		if err := validateMediaSourceURL(item.Source, allowLoopbackHTTP); err != nil {
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
	if item.Title != nil && utf8.RuneCountInString(*item.Title) > maxMediaTitleLength {
		return fmt.Errorf("title cannot exceed %d characters", maxMediaTitleLength)
	}
	if item.Description != nil && utf8.RuneCountInString(*item.Description) > maxMediaDescriptionLength {
		return fmt.Errorf("description cannot exceed %d characters", maxMediaDescriptionLength)
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

func validateMediaSourceURL(source string, allowLoopbackHTTP bool) error {
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
		if allowLoopbackHTTP && isLoopbackHost(parsed.Hostname()) {
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
	case MediaDisplayModeAuto, MediaDisplayModeCarousel, MediaDisplayModeGallery,
		MediaDisplayModeFileList, MediaDisplayModeEmbed, MediaDisplayModeLink:
		return true
	default:
		return false
	}
}

func isKnownMediaIcon(icon EventMediaIcon) bool {
	switch icon {
	case MediaIconYouTube, MediaIconTwitch, MediaIconGoogleDrive, MediaIconInstagram,
		MediaIconPDF, MediaIconPhotoGallery, MediaIconFile, MediaIconVideo, MediaIconExternalLink:
		return true
	default:
		return false
	}
}
