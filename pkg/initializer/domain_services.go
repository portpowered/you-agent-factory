package initializer

import (
	"github.com/portpowered/infinite-you/pkg/composebridge"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// NewModelServiceFromCore constructs a ModelService from a composed Core without
// building the root FactoryService compatibility facade.
func NewModelServiceFromCore(core *Core) ModelService {
	return composebridge.NewModelServiceFromCore(core)
}

// NewFactoryDefinitionServiceFromCore constructs a FactoryDefinitionService from
// a composed Core without building the root FactoryService facade.
func NewFactoryDefinitionServiceFromCore(core *Core) FactoryDefinitionService {
	return composebridge.NewFactoryDefinitionServiceFromCore(core)
}

// StartupWorkerConfigFromCore returns the named worker from the composed startup runtime.
func StartupWorkerConfigFromCore(core *Core, name string) (*interfaces.WorkerConfig, bool) {
	return composebridge.StartupWorkerConfigFromCore(core, name)
}
