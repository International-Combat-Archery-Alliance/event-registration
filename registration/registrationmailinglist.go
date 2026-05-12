package registration

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
)

func AddToMailingList(ctx context.Context, subscriberManager email.SubscriberManager, reg Registration, groupID string, logger *slog.Logger) {
	ctx, span := tracer.Start(ctx, "AddToMailingList")
	defer span.End()

	switch reg.Type() {
	case events.BY_INDIVIDUAL:
		indiv := reg.(*IndividualRegistration)
		name := indiv.PlayerInfo.FirstName + " " + indiv.PlayerInfo.LastName
		if err := subscriberManager.AddSubscriberToGroup(ctx, indiv.Email, name, groupID); err != nil {
			logger.Warn("failed to add individual subscriber to mailerlite group", "email", indiv.Email, "error", err)
		}
	case events.BY_TEAM:
		team := reg.(*TeamRegistration)
		if err := subscriberManager.AddSubscriberToGroup(ctx, team.CaptainEmail, team.TeamName, groupID); err != nil {
			logger.Warn("failed to add team captain to mailerlite group", "email", team.CaptainEmail, "error", err)
		}
		for _, player := range team.Players {
			if player.Email == nil {
				continue
			}
			name := fmt.Sprintf("%s %s", player.FirstName, player.LastName)
			if err := subscriberManager.AddSubscriberToGroup(ctx, *player.Email, name, groupID); err != nil {
				logger.Warn("failed to add team player to mailerlite group", "email", *player.Email, "error", err)
			}
		}
	}
}
