// Package service is the parent-private implementation of the Chat Sessions
// FactoryTargetCatalogService detached root contract. It is composed only
// through pkg/services/chat_sessions/wire and consumed by peers exclusively
// through the chatsessions.FactoryTargetCatalogService interface.
package service

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Service implements chatsessions.FactoryTargetCatalogService by combining
// the singular Operator Settings public service root and Factory
// Definitions' narrow, read-only catalog/path capability, injected directly
// and exactly once.
type Service struct {
	operatorSettings   operatorsettings.Service
	factoryDefinitions factorydefinitions.CatalogPathsService
	logger             logging.Logger
}

var _ chatsessions.FactoryTargetCatalogService = (*Service)(nil)

// New stores concrete collaborator roots and the selected operation logger.
// The owning Wire provider validates required peers and selects nil compatibility
// before calling this inert constructor.
func New(
	operatorSettings operatorsettings.Service,
	factoryDefinitions factorydefinitions.CatalogPathsService,
	logger logging.Logger,
) *Service {
	return &Service{
		operatorSettings:   operatorSettings,
		factoryDefinitions: factoryDefinitions,
		logger:             logger,
	}
}
