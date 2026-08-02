package replay

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// SnapshotDecoder decodes the durable Factory snapshot representation at the
// Recordings replay boundary. The decoder is supplied by composition; it is
// not a Definitions root effect or public policy port.
type SnapshotDecoder func([]byte) (*factorydefinitions.FactorySnapshot, error)

// RuntimeConfigDecoder reconstructs the owner-local lookup used while reducing
// recorded dispatch events.
type RuntimeConfigDecoder func(*factorydefinitions.FactorySnapshot) (ReplayRuntimeConfig, error)

// FactorySnapshotDirectoryLoader loads the authored snapshot adjacent to an
// event stream. Replay owns this narrow file-operation shape; it is not a
// Definitions effect port.
type FactorySnapshotDirectoryLoader func(string) (*factorydefinitions.FactorySnapshot, error)

// ReplayRuntimeConfig is the minimal lookup needed to replay dispatches and
// is owned by Recordings rather than Factory Definitions.
type ReplayRuntimeConfig interface {
	FactoryConfig() *factorydefinitions.FactoryConfig
	FactoryDir() string
	RuntimeBaseDir() string
	Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool)
	Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool)
	WorkstationByID(string) (*factorydefinitions.FactoryWorkstationConfig, bool)
}
