package api

import (
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
)

func eventMediaToApiMedia(media []events.EventMediaItem) *[]EventMediaItem {
	if len(media) == 0 {
		return nil
	}

	apiMedia := slices.Map(media, eventMediaItemToApiMediaItem)
	return &apiMedia
}

func apiMediaToEventMedia(media *[]EventMediaItem) []events.EventMediaItem {
	if media == nil || len(*media) == 0 {
		return nil
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
