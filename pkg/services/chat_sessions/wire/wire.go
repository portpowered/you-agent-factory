// Package wire is the Chat Sessions FactoryTargetCatalogService composition
// boundary.
//
// Wire performs construction only, returns the singular
// chatsessions.FactoryTargetCatalogService root interface, and starts no
// lifecycle components. It composes the implementation through direct single
// injection of the singular Operator Settings and Factory Definitions
// public service roots, with no dependency bag, service locator, or
// alternate construction path.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	chatsessionsservice "github.com/portpowered/infinite-you/pkg/services/chat_sessions/internal/service"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// NewFactoryTargetCatalogService constructs the Chat Sessions Factory
// target-catalog root from the singular Operator Settings and Factory
// Definitions public service roots. logger is the direct, required
// operation-logging abstraction; callers with no operation logging pass
// logging.NoopLogger{}.
func NewFactoryTargetCatalogService(
	operatorSettings operatorsettings.Service,
	factoryDefinitions factorydefinitions.Service,
	logger logging.Logger,
) (chatsessions.FactoryTargetCatalogService, error) {
	return chatsessionsservice.New(operatorSettings, factoryDefinitions, logger)
}
