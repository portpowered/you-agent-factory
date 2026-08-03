package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionswire "github.com/portpowered/infinite-you/pkg/services/chat_sessions/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// provideChatSessionsFactoryTargetCatalogService constructs the Chat
// Sessions Factory target-catalog root through the owning Chat Sessions Wire
// package's focused constructor, from the same singular Operator Settings
// and Factory Definitions public service roots the rest of this graph
// composes. It performs no dependency-bag or service-locator composition:
// every collaborator is a direct constructor parameter.
func provideChatSessionsFactoryTargetCatalogService(
	operatorSettings operatorsettings.Service,
	factoryDefinitions factorydefinitions.Service,
	logger logging.Logger,
) (chatsessions.FactoryTargetCatalogService, error) {
	return chatsessionswire.NewFactoryTargetCatalogService(operatorSettings, factoryDefinitions, logger)
}
