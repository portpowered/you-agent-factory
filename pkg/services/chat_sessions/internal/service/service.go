// Package service is the parent-private implementation of the Chat Sessions
// FactoryTargetCatalogService detached root contract. It is composed only
// through pkg/services/chat_sessions/wire and consumed by peers exclusively
// through the chatsessions.FactoryTargetCatalogService interface.
package service

import (
	"fmt"

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

// New constructs a Service from its required collaborator roots. logger is
// the direct, required operation-logging abstraction; callers with no
// operation logging pass logging.NoopLogger{}.
func New(
	operatorSettings operatorsettings.Service,
	factoryDefinitions factorydefinitions.CatalogPathsService,
	logger logging.Logger,
) (*Service, error) {
	if operatorSettings == nil {
		return nil, fmt.Errorf("construct chat sessions factory target catalog: operator settings root is required")
	}
	if factoryDefinitions == nil {
		return nil, fmt.Errorf("construct chat sessions factory target catalog: factory definitions catalog/path capability is required")
	}
	if logger == nil {
		logger = logging.NoopLogger{}
	}
	return &Service{
		operatorSettings:   operatorSettings,
		factoryDefinitions: factoryDefinitions,
		logger:             logger,
	}, nil
}
