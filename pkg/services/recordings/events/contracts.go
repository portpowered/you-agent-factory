package events

import (
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
)

// RuntimeLedgerFactory constructs session-scoped canonical ledgers.
type RuntimeLedgerFactory func(
	recordings.InitialStructureSource,
	func() time.Time,
	string,
	interfaces.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger

// NewRuntimeLedger exposes the concrete event ledger through its public port.
func NewRuntimeLedger(
	topology recordings.InitialStructureSource,
	now func() time.Time,
	streamGenerationID string,
	definitions interfaces.RuntimeDefinitionLookup,
) recordings.RuntimeEventLedger {
	return NewFactoryEventHistory(topology, now, streamGenerationID, definitions)
}

var _ recordings.RuntimeEventLedger = (*FactoryEventHistory)(nil)
var _ recordings.WorkerEventRecorder = (*FactoryEventHistory)(nil)
