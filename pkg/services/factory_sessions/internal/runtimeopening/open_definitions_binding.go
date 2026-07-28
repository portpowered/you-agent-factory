package runtimeopening

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type factoryDefinitionServiceInstaller interface {
	AttachFactoryDefinitionService(factorydefinitions.Service) factorydefinitions.Service
}

func attachFactoryDefinitionServiceToRuntime(
	sessionRuntime any,
	factoryDefinitionOwner factorydefinitions.Service,
) error {
	if factoryDefinitionOwner == nil {
		return fmt.Errorf("construct runtime scope: Factory Definitions factory returned nil service")
	}
	installer, ok := sessionRuntime.(factoryDefinitionServiceInstaller)
	if !ok {
		return fmt.Errorf("construct runtime scope: session runtime does not accept Factory Definitions binding")
	}
	installer.AttachFactoryDefinitionService(factoryDefinitionOwner)
	return nil
}
