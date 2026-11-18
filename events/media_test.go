package events

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
)

func TestValidateMedia(t *testing.T) {
	folderItem := func(source string) EventMediaItem {
		return EventMediaItem{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: source}
	}
	urlItem := func(mediaType EventMediaType, source string) EventMediaItem {
		return EventMediaItem{ID: uuid.New(), Type: mediaType, Source: source}
	}
	validURL := "https://example.com/resource"

	manyItems := make([]EventMediaItem, 0, maxMediaItems+1)
	for range maxMediaItems + 1 {
		manyItems = append(manyItems, folderItem("/images/events/photo"))
	}

	duplicateID := uuid.New()
	duplicateItems := []EventMediaItem{
		{ID: duplicateID, Type: MediaTypeAssetFolder, Source: "/images/events/a"},
		{ID: duplicateID, Type: MediaTypeAssetFolder, Source: "/images/events/b"},
	}

	tests := []struct {
		name              string
		allowLoopbackHTTP bool
		media             []EventMediaItem
		wantErr           bool
	}{
		{name: "omitted media", allowLoopbackHTTP: true, media: nil},
		{name: "empty media list", allowLoopbackHTTP: true, media: []EventMediaItem{}},
		{name: "asset folder", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/events/boston-2026")}},
		{name: "asset folder root", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/")}},
		{name: "asset folder without leading slash", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("images/events")}, wantErr: true},
		{name: "asset folder with trailing slash", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/events/")}, wantErr: true},
		{name: "asset folder with empty segment", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images//events")}, wantErr: true},
		{name: "asset folder with dot segment", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/./events")}, wantErr: true},
		{name: "asset folder with parent segment", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/../events")}, wantErr: true},
		{name: "asset folder with encoded parent segment", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/%2e%2e/events")}, wantErr: true},
		{name: "asset folder with double-encoded parent segment", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/%252e%252e/events")}, wantErr: true},
		{name: "asset folder with encoded separator", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images%2Fevents")}, wantErr: true},
		{name: "asset folder with encoded backslash", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images%5Cevents")}, wantErr: true},
		{name: "asset folder with backslash", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem(`/images\events`)}, wantErr: true},
		{name: "asset folder with query", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/events?sort=asc")}, wantErr: true},
		{name: "asset folder with fragment", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/events#top")}, wantErr: true},
		{name: "asset folder with invalid percent encoding", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/%zz")}, wantErr: true},
		{name: "asset folder with percent-encoded character", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/events%20photos")}, wantErr: true},
		{name: "asset folder with literal percent", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/100%")}, wantErr: true},
		{name: "asset folder with control character", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/events\x01")}, wantErr: true},
		{name: "asset folder with unicode segments", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("/images/événements/boston-2026")}},
		{name: "empty source", allowLoopbackHTTP: true, media: []EventMediaItem{folderItem("")}, wantErr: true},
		{name: "asset file over https", media: []EventMediaItem{urlItem(MediaTypeAssetFile, validURL)}},
		{name: "asset file over loopback http in local mode", allowLoopbackHTTP: true, media: []EventMediaItem{urlItem(MediaTypeAssetFile, "http://localhost:8080/photo.jpg")}},
		{name: "asset file over loopback ipv4 in local mode", allowLoopbackHTTP: true, media: []EventMediaItem{urlItem(MediaTypeAssetFile, "http://127.0.0.1:8080/photo.jpg")}},
		{name: "asset file over loopback ipv6 in local mode", allowLoopbackHTTP: true, media: []EventMediaItem{urlItem(MediaTypeAssetFile, "http://[::1]:8080/photo.jpg")}},
		{name: "video over https", media: []EventMediaItem{urlItem(MediaTypeVideo, "https://www.twitch.tv/videos/987654321")}},
		{name: "external link over https", media: []EventMediaItem{urlItem(MediaTypeExternalLink, validURL)}},
		{name: "asset file over loopback http in prod", media: []EventMediaItem{urlItem(MediaTypeAssetFile, "http://localhost:8080/photo.jpg")}, wantErr: true},
		{name: "video over loopback http in prod", media: []EventMediaItem{urlItem(MediaTypeVideo, "http://127.0.0.1:8080/stream")}, wantErr: true},
		{name: "external link over non-loopback http in local mode", allowLoopbackHTTP: true, media: []EventMediaItem{urlItem(MediaTypeExternalLink, "http://example.com/page")}, wantErr: true},
		{name: "javascript scheme", allowLoopbackHTTP: true, media: []EventMediaItem{urlItem(MediaTypeExternalLink, "javascript:alert(1)")}, wantErr: true},
		{name: "data scheme", allowLoopbackHTTP: true, media: []EventMediaItem{urlItem(MediaTypeAssetFile, "data:text/html,<p>hi</p>")}, wantErr: true},
		{name: "protocol-relative url", allowLoopbackHTTP: true, media: []EventMediaItem{urlItem(MediaTypeExternalLink, "//example.com/page")}, wantErr: true},
		{name: "url with credentials", media: []EventMediaItem{urlItem(MediaTypeExternalLink, "https://user:password@example.com/page")}, wantErr: true},
		{name: "relative url", media: []EventMediaItem{urlItem(MediaTypeAssetFile, "/photo.jpg")}, wantErr: true},
		{name: "url with space in host", media: []EventMediaItem{urlItem(MediaTypeAssetFile, "https://exa mple.com/photo.jpg")}, wantErr: true},
		{name: "url with space in path", media: []EventMediaItem{urlItem(MediaTypeAssetFile, "https://example.com/my photo.jpg")}, wantErr: true},
		{name: "url with backslash", media: []EventMediaItem{urlItem(MediaTypeExternalLink, `https://example.com\evil.com`)}, wantErr: true},
		{name: "url with control character", media: []EventMediaItem{urlItem(MediaTypeAssetFile, "https://example.com/photo.jpg\x00")}, wantErr: true},
		{name: "unknown media type", allowLoopbackHTTP: true, media: []EventMediaItem{urlItem(EventMediaType("audio"), validURL)}, wantErr: true},
		{name: "auto display mode", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", DisplayMode: ptr.To(MediaDisplayModeAuto)}}},
		{name: "carousel display mode", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", DisplayMode: ptr.To(MediaDisplayModeCarousel)}}},
		{name: "gallery display mode", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", DisplayMode: ptr.To(MediaDisplayModeGallery)}}},
		{name: "file list display mode", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", DisplayMode: ptr.To(MediaDisplayModeFileList)}}},
		{name: "embed display mode", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeVideo, Source: validURL, DisplayMode: ptr.To(MediaDisplayModeEmbed)}}},
		{name: "link display mode", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeExternalLink, Source: validURL, DisplayMode: ptr.To(MediaDisplayModeLink)}}},
		{name: "unknown display mode", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", DisplayMode: ptr.To(EventMediaDisplayMode("slideshow"))}}, wantErr: true},
		{name: "youtube icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeVideo, Source: validURL, Icon: ptr.To(MediaIconYouTube)}}},
		{name: "twitch icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeVideo, Source: validURL, Icon: ptr.To(MediaIconTwitch)}}},
		{name: "google drive icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeExternalLink, Source: validURL, Icon: ptr.To(MediaIconGoogleDrive)}}},
		{name: "instagram icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeExternalLink, Source: validURL, Icon: ptr.To(MediaIconInstagram)}}},
		{name: "pdf icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFile, Source: validURL, Icon: ptr.To(MediaIconPDF)}}},
		{name: "photo gallery icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", Icon: ptr.To(MediaIconPhotoGallery)}}},
		{name: "file icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFile, Source: validURL, Icon: ptr.To(MediaIconFile)}}},
		{name: "video icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeVideo, Source: validURL, Icon: ptr.To(MediaIconVideo)}}},
		{name: "external link icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeExternalLink, Source: validURL, Icon: ptr.To(MediaIconExternalLink)}}},
		{name: "unknown icon", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeExternalLink, Source: validURL, Icon: ptr.To(EventMediaIcon("spotify"))}}, wantErr: true},
		{name: "title at limit", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", Title: ptr.String(strings.Repeat("a", maxMediaTitleLength))}}},
		{name: "multibyte title at rune limit", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", Title: ptr.String(strings.Repeat("😀", maxMediaTitleLength))}}},
		{name: "title over limit", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", Title: ptr.String(strings.Repeat("a", maxMediaTitleLength+1))}}, wantErr: true},
		{name: "description at limit", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", Description: ptr.String(strings.Repeat("a", maxMediaDescriptionLength))}}},
		{name: "description over limit", allowLoopbackHTTP: true, media: []EventMediaItem{{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events", Description: ptr.String(strings.Repeat("a", maxMediaDescriptionLength+1))}}, wantErr: true},
		{name: "source at character limit", media: []EventMediaItem{urlItem(MediaTypeAssetFile, "https://example.com/"+strings.Repeat("a", maxMediaSourceLength-20))}},
		{name: "source over character limit", media: []EventMediaItem{urlItem(MediaTypeAssetFile, "https://example.com/"+strings.Repeat("a", maxMediaSourceLength-19))}, wantErr: true},
		{name: "too many items", allowLoopbackHTTP: true, media: manyItems, wantErr: true},
		{name: "duplicate item ids", allowLoopbackHTTP: true, media: duplicateItems, wantErr: true},
		{name: "nil item id", allowLoopbackHTTP: true, media: []EventMediaItem{{Type: MediaTypeAssetFolder, Source: "/images/events"}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMedia(tt.media, tt.allowLoopbackHTTP)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
