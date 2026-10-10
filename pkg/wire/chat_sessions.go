package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionswire "github.com/portpowered/infinite-you/pkg/services/chat_sessions/wire"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// provideChatSessionsFactoryTargetCatalogService constructs the Chat
// Sessions Factory target-catalog root through the owning Chat Sessions Wire
// package's focused constructor, from the same singular Operator Settings
// public service root and Factory Definitions' narrow, read-only
// catalog/path capability the rest of this graph composes. It performs no
// dependency-bag or service-locator composition: every collaborator is a
// direct constructor parameter.
func provideChatSessionsFactoryTargetCatalogService(
	operatorSettings operatorsettings.Service,
	catalogPaths factorydefinitions.CatalogPathsService,
	logger logging.Logger,
) chatsessions.FactoryTargetCatalogService {
	return chatsessionswire.NewFactoryTargetCatalogService(operatorSettings, catalogPaths, logger)
}

// provideChatSessionsService constructs the singular canonical
// chat_sessions.Service instance through its focused wire provider,
// threading the same canonical events.Service instance in as both its
// Sequence operation's EventsAppender dependency and its
// AcknowledgeAttachment operation's EventsReader dependency. It is the one
// construction path to a Service value in production code: no alternate
// constructor, dependency bag, or secondary injector exists.
func provideChatSessionsService(eventsService events.Service, logger logging.Logger, source platformclock.Source) chatsessions.Service {
	return chatsessionswire.NewService(uuid.NewString, source.Now, eventsService, eventsService, logger)
}
