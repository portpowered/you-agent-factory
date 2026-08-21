package runtime

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func recordDispatchWorkerSessionAssociation(
	ledger recordings.RuntimeLedger,
	tick int,
	dispatchID string,
	workerSessionID string,
	requestID string,
	facts recordings.DispatchWorkerSessionExecutionFacts,
	eventTime time.Time,
) {
	if ledger == nil {
		return
	}
	if recorder, ok := ledger.(recordings.DispatchWorkerSessionAssociationRecorder); ok {
		recorder.RecordDispatchWorkerSessionAssociationWithExecution(
			tick,
			dispatchID,
			workerSessionID,
			requestID,
			facts,
			eventTime,
		)
		return
	}
	ledger.RecordDispatchWorkerSessionAssociation(tick, dispatchID, workerSessionID, requestID, eventTime)
}
