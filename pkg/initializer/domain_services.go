package initializer

import (
	"github.com/portpowered/infinite-you/pkg/composebridge"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
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
func StartupWorkerConfigFromCore(core *Core, name string) (*workerconfig.Config, bool) {
	return composebridge.StartupWorkerConfigFromCore(core, name)
}
