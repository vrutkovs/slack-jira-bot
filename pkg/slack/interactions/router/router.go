package router

import (
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"

	"github.com/vrutkovs/slack-jira-bot/pkg/jira"
	"github.com/vrutkovs/slack-jira-bot/pkg/slack/interactions"
	"github.com/vrutkovs/slack-jira-bot/pkg/slack/modals"
	"github.com/vrutkovs/slack-jira-bot/pkg/slack/modals/bug"
)

// ForModals returns a Handler that appropriately routes
// interaction callbacks for the modals we know about
func ForModals(filer jira.IssueFiler, client *slack.Client) interactions.Handler {
	router := &modalRouter{
		slackClient:         client,
		viewsById:           map[modals.Identifier]slack.ModalViewRequest{},
		handlersByIdAndType: map[modals.Identifier]map[slack.InteractionType]interactions.Handler{},
	}

	toRegister := []*modals.FlowWithViewAndFollowUps{
		bug.Register(filer, client),
	}

	for _, entry := range toRegister {
		router.viewsById[entry.Identifier] = entry.View
		router.handlersByIdAndType[entry.Identifier] = entry.FollowUps
	}

	return router
}

type modalRouter struct {
	slackClient slackClient

	// viewsById maps callback IDs to modal flows, for triggering
	// modals as a response to short-cut interaction events
	viewsById map[modals.Identifier]slack.ModalViewRequest
	// handlersByIdAndType holds handlers for different types of
	// interaction payloads, further mapping to identifiers we
	// store in private metadata for routing
	handlersByIdAndType map[modals.Identifier]map[slack.InteractionType]interactions.Handler
}

// Handle routes the interaction callback to the appropriate handler
func (r *modalRouter) Handle(callback *slack.InteractionCallback, logger *logrus.Entry) (output []byte, err error) {
	switch callback.Type {
	case slack.InteractionTypeShortcut:
		return nil, r.viewForShortcut(callback, logger)
	case slack.InteractionTypeBlockActions:
		if isMessageButtonPress(callback) {
			return nil, r.viewForButton(callback, logger)
		}
		return r.delegate(callback, logger)
	default:
		return r.delegate(callback, logger)
	}
}

// isMessageButtonPress determines if an interaction callback is for a button press in a message
func isMessageButtonPress(callback *slack.InteractionCallback) bool {
	return callback.View.ID == "" && callback.Message.Text != "" && len(callback.ActionCallback.BlockActions) > 0 && callback.ActionCallback.BlockActions[0].Type == "button"
}

type slackClient interface {
	OpenView(triggerID string, view slack.ModalViewRequest) (*slack.ViewResponse, error)
}

// viewForShortcut reacts to the original shortcut action from the user
// to open the first modal view for them
func (r *modalRouter) viewForShortcut(callback *slack.InteractionCallback, logger *logrus.Entry) error {
	id := modals.Identifier(callback.CallbackID)
	return r.openModal(id, callback.TriggerID, logger)
}

// viewForButton reacts to the a user pressing a button in a bot message
// to open the a modal view for them
func (r *modalRouter) viewForButton(callback *slack.InteractionCallback, logger *logrus.Entry) error {
	id := modals.Identifier(callback.ActionCallback.BlockActions[0].Value)
	return r.openModal(id, callback.TriggerID, logger)
}

func (r *modalRouter) openModal(id modals.Identifier, triggerID string, logger *logrus.Entry) error {
	logger = logger.WithField("view_id", id)
	logger.Infof("Opening modal view %s.", id)
	view, exists := r.viewsById[id]
	if id != "" && !exists {
		logger.Debug("Unknown callback ID.")
		return nil
	}

	response, err := r.slackClient.OpenView(triggerID, view)
	if err != nil {
		logger.WithError(err).Warn("Failed to open a modal flow.")
	}
	logger.WithField("response", response).Trace("Got a modal response.")
	return err
}

// delegate routes the interaction callback to the appropriate handler
func (r *modalRouter) delegate(callback *slack.InteractionCallback, logger *logrus.Entry) (output []byte, err error) {
	id := modals.Identifier(callback.View.PrivateMetadata)
	logger = logger.WithField("view_id", id)
	handlersForId, registered := r.handlersByIdAndType[id]
	if !registered {
		logger.Debugf("Received a callback ID (%s) for which no handlers were registered.", id)
		return nil, nil
	}
	handler, exists := handlersForId[callback.Type]
	if !exists {
		logger.Debugf("Received a callback ID (%s) and type (%s) for which no handlers were registered.", callback.Type, id)
		return nil, nil
	}
	return handler.Handle(callback, logger)
}

func (r *modalRouter) Identifier() string {
	return "modal"
}
