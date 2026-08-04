package wire

import (
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionswire "github.com/portpowered/infinite-you/pkg/services/chat_sessions/wire"
	events "github.com/portpowered/infinite-you/pkg/services/events"
)

// provideChatSessionsService constructs the singular canonical
// chat_sessions.Service instance through its focused wire provider,
// threading the same canonical events.Service instance in as both its
// Sequence operation's EventsAppender dependency and its
// AcknowledgeAttachment operation's EventsReader dependency. It is the one
// construction path to a Service value in production code: no alternate
// constructor, dependency bag, or secondary injector exists.
func provideChatSessionsService(eventsService events.Service, logger logging.Logger) (chatsessions.Service, error) {
	return chatsessionswire.NewService(uuid.NewString, time.Now, eventsService, eventsService, logger)
}
