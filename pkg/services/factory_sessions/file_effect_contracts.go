package factorysessions

import "github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"

// These aliases publish the Factory Sessions-owned external read effects at
// the service root. Implementations depend on the leaf package to avoid a
// root/import cycle; external consumers and the process edge aggregator use
// these canonical root contracts.
type ContractFixtureReader = fileeffects.ContractFixtureReader
type InvocationInputReader = fileeffects.InvocationInputReader
type ReplayRecordingReader = fileeffects.ReplayRecordingReader
type InitialWorkReader = fileeffects.InitialWorkReader
