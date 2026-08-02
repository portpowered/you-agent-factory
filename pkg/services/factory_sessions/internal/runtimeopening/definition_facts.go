package runtimeopening

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// ReplayRuntimeConfig is the detached lookup required to rebuild a loaded
// runtime source from a replay snapshot. It is owned by runtime opening; no
// Definitions implementation or Wire package crosses this boundary.
type ReplayRuntimeConfig interface {
	factorydefinitions.RuntimeDefinitionLookup
	FactoryConfig() *factorydefinitions.FactoryConfig
	FactoryDir() string
	RuntimeBaseDir() string
	Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool)
	WorkstationByID(string) (*factorydefinitions.FactoryWorkstationConfig, bool)
}

// ReplayRuntimeConfigDecoder reconstructs the detached replay lookup consumed
// by runtime opening.
type ReplayRuntimeConfigDecoder func(*factorydefinitions.FactorySnapshot) (ReplayRuntimeConfig, error)
