package factorydefinitions

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

const ReplayV1SourceFormat = "agent-factory.replay.v1"

// FactorySnapshotSource is the narrow effective-definition projection required
// by snapshot capture.
type FactorySnapshotSource = contracts.FactorySnapshotSource

// LoadedFactorySource is the complete effective definition view retained by a
// live Factory Session.
type LoadedFactorySource = contracts.LoadedFactorySource
