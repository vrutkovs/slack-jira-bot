package router

import (
	"github.com/slack-go/slack"

	"github.com/vrutkovs/slack-jira-bot/pkg/slack/events"
	"github.com/vrutkovs/slack-jira-bot/pkg/slack/events/mention"
)

// ForEvents returns a Handler that appropriately routes
// event callbacks for the handlers we know about
func ForEvents(client *slack.Client) events.Handler {
	return events.MultiHandler(
		mention.Handler(client),
	)
}
