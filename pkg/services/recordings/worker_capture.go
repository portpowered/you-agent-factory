package recordings

import (
	"context"
	"errors"

	recordingcontracts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/contracts"
	workerrecording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture"
)

// Worker recording values and the shared pure reducer live in the focused
// Recordings-owned internal Worker capture package and are re-exported here
// as the customer-facing service vocabulary.
type (
	WorkerSessionRecordingService            = recordingcontracts.WorkerSessionRecordingService
	WorkerSessionRecording                   = recordingcontracts.WorkerSessionRecording
	WorkerSessionRecordingFinalizer          = recordingcontracts.WorkerSessionRecordingFinalizer
	WorkerSessionRecordingRequest            = workerrecording.WorkerSessionRecordingRequest
	WorkerRecordingRecord                    = workerrecording.WorkerRecordingRecord
	WorkerRecordingReader                    = recordingcontracts.WorkerRecordingReader
	WorkerRecordingHistoryReader             = recordingcontracts.WorkerRecordingHistoryReader
	WorkerRecordingProjectionReader          = recordingcontracts.WorkerRecordingProjectionReader
	WorkerRecordingFailure                   = workerrecording.WorkerRecordingFailure
	WorkerRecordingFailureWriter             = recordingcontracts.WorkerRecordingFailureWriter
	WorkerRecordingWriter                    = recordingcontracts.WorkerRecordingWriter
	WorkerRecordingSnapshot                  = workerrecording.WorkerRecordingSnapshot
	WorkerSessionRecordingSnapshot           = workerrecording.WorkerSessionRecordingSnapshot
	WorkerRecordingStatus                    = workerrecording.WorkerRecordingStatus
	WorkerRecordingHistory                   = workerrecording.WorkerRecordingHistory
	WorkerRecordingTerminal                  = workerrecording.WorkerRecordingTerminal
	WorkerRecordingProjection                = workerrecording.WorkerRecordingProjection
	WorkerRecordingReplayRequest             = workerrecording.WorkerRecordingReplayRequest
	WorkerRecordingReplayResult              = workerrecording.WorkerRecordingReplayResult
	WorkerRecordingListRequest               = workerrecording.WorkerRecordingListRequest
	WorkerRecordingListResult                = workerrecording.WorkerRecordingListResult
	WorkerRecordingCatalogDiagnostic         = workerrecording.WorkerRecordingCatalogDiagnostic
	WorkerRecordingCatalogDiagnosticCode     = workerrecording.WorkerRecordingCatalogDiagnosticCode
	WorkerPortableRecording                  = workerrecording.WorkerPortableRecording
	WorkerPortableRecordingIdentity          = workerrecording.WorkerPortableRecordingIdentity
	WorkerPortableRecordingLifecycle         = workerrecording.WorkerPortableRecordingLifecycle
	WorkerPortableRecordingCorrelation       = workerrecording.WorkerPortableRecordingCorrelation
	WorkerPortableProviderAttribution        = workerrecording.WorkerPortableProviderAttribution
	WorkerPortableTerminal                   = workerrecording.WorkerPortableTerminal
	WorkerPortableRecord                     = workerrecording.WorkerPortableRecord
	WorkerPortableRecordingIntegrity         = workerrecording.WorkerPortableRecordingIntegrity
	WorkerPortableRecordingDiagnostic        = workerrecording.WorkerPortableRecordingDiagnostic
	WorkerPortableRecordingDiagnosticCode    = workerrecording.WorkerPortableRecordingDiagnosticCode
	WorkerPortableRecordingDecodeDiagnostics = workerrecording.WorkerPortableRecordingDecodeDiagnostics
	WorkerRecordingCodec                     = workerrecording.WorkerRecordingCodec
	WorkerRecordingService                   = workerrecording.Service
)

const (
	WorkerRecordingStatusComplete             = workerrecording.WorkerRecordingStatusComplete
	WorkerRecordingStatusDegraded             = workerrecording.WorkerRecordingStatusDegraded
	WorkerRecordingStatusIncomplete           = workerrecording.WorkerRecordingStatusIncomplete
	WorkerRecordingInterruptionProcessStopped = workerrecording.WorkerRecordingInterruptionProcessStopped
	// Legacy capture-state values are retained for explicit sidecar
	// compatibility. New recordings expose only the three health statuses
	// above.
	WorkerRecordingStatusActive            = workerrecording.WorkerRecordingStatusActive
	WorkerRecordingStatusCompleted         = workerrecording.WorkerRecordingStatusCompleted
	WorkerRecordingStatusFailed            = workerrecording.WorkerRecordingStatusFailed
	WorkerPortableRecordingKind            = workerrecording.WorkerPortableRecordingKind
	WorkerPortableRecordingSchemaV1        = workerrecording.WorkerPortableRecordingSchemaV1
	WorkerPortableRecordingReplayCompatV1  = workerrecording.WorkerPortableRecordingReplayCompatV1
	WorkerPortableRecordingIntegritySHA256 = workerrecording.WorkerPortableRecordingIntegritySHA256
	WorkerPortableCodeMalformedContract    = workerrecording.WorkerPortableCodeMalformedContract
	WorkerPortableCodeUnsupportedVersion   = workerrecording.WorkerPortableCodeUnsupportedVersion
	WorkerPortableCodeInvalidIdentity      = workerrecording.WorkerPortableCodeInvalidIdentity
	WorkerPortableCodeInvalidLifecycle     = workerrecording.WorkerPortableCodeInvalidLifecycle
	WorkerPortableCodeInvalidCorrelation   = workerrecording.WorkerPortableCodeInvalidCorrelation
	WorkerPortableCodeInvalidProvenance    = workerrecording.WorkerPortableCodeInvalidProvenance
	WorkerPortableCodeInvalidFidelity      = workerrecording.WorkerPortableCodeInvalidFidelity
	WorkerPortableCodeInvalidOrder         = workerrecording.WorkerPortableCodeInvalidOrder
	WorkerPortableCodeInvalidTerminal      = workerrecording.WorkerPortableCodeInvalidTerminal
	WorkerPortableCodeInvalidIntegrity     = workerrecording.WorkerPortableCodeInvalidIntegrity
	WorkerRecordingCatalogMalformedTail    = workerrecording.WorkerRecordingCatalogMalformedTail
	WorkerRecordingCatalogUnsupported      = workerrecording.WorkerRecordingCatalogUnsupported
	WorkerRecordingCatalogUnreadable       = workerrecording.WorkerRecordingCatalogUnreadable
	WorkerRecordingCatalogRetention        = workerrecording.WorkerRecordingCatalogRetention
	WorkerRecordingCatalogInvalidIdentity  = workerrecording.WorkerRecordingCatalogInvalidIdentity
)

// WorkerRecordingWriterFunc adapts a function to WorkerRecordingWriter.
type WorkerRecordingWriterFunc func(context.Context, WorkerRecordingRecord) error

// PersistWorkerRecord implements WorkerRecordingWriter.
func (writer WorkerRecordingWriterFunc) PersistWorkerRecord(
	ctx context.Context,
	record WorkerRecordingRecord,
) error {
	if writer == nil {
		return ErrMissingWorkerRecordingWriter
	}
	return writer(ctx, record)
}

var (
	ErrInvalidWorkerRecordingRequest        = workerrecording.ErrInvalidWorkerRecordingRequest
	ErrMissingWorkerRecordingWriter         = errors.New("recordings: Worker recording writer is required")
	ErrWorkerRecordingSubscribe             = errors.New("recordings: Worker recording subscription failed")
	ErrWorkerRecordingOpening               = workerrecording.ErrWorkerRecordingOpening
	ErrWorkerRecordingPersistence           = errors.New("recordings: Worker recording persistence failed")
	ErrWorkerRecordingDelivery              = workerrecording.ErrWorkerRecordingDelivery
	ErrWorkerRecordingGap                   = errors.New("recordings: Worker recording retention gap")
	ErrWorkerRecordingClosed                = errors.New("recordings: Worker recording source closed")
	ErrWorkerRecordingCanceled              = errors.New("recordings: Worker recording subscription canceled")
	ErrWorkerRecordingBackpressure          = errors.New("recordings: Worker recording backpressure")
	ErrWorkerRecordingOrder                 = workerrecording.ErrWorkerRecordingOrder
	ErrWorkerRecordingDuplicate             = workerrecording.ErrWorkerRecordingDuplicate
	ErrWorkerRecordingTerminal              = workerrecording.ErrWorkerRecordingTerminal
	ErrWorkerRecordingIncomplete            = workerrecording.ErrWorkerRecordingIncomplete
	ErrWorkerRecordingCompatibility         = workerrecording.ErrWorkerRecordingCompatibility
	ErrWorkerRecordingReplay                = workerrecording.ErrWorkerRecordingReplay
	ErrWorkerRecordingCorruptTail           = workerrecording.ErrWorkerRecordingCorruptTail
	ErrWorkerRecordingAppend                = workerrecording.ErrWorkerRecordingAppend
	ErrWorkerRecordingRetention             = workerrecording.ErrWorkerRecordingRetention
	ErrWorkerRecordingCursor                = workerrecording.ErrWorkerRecordingCursor
	ErrMissingWorkerRecordingReader         = errors.New("recordings: Worker recording reader is required")
	ErrWorkerPortableRecording              = workerrecording.ErrWorkerPortableRecording
	ErrWorkerPortableRecordingCompatibility = workerrecording.ErrWorkerPortableRecordingCompatibility
	ErrWorkerPortableRecordingIdentity      = workerrecording.ErrWorkerPortableRecordingIdentity
	ErrWorkerPortableRecordingLifecycle     = workerrecording.ErrWorkerPortableRecordingLifecycle
	ErrWorkerPortableRecordingCorrelation   = workerrecording.ErrWorkerPortableRecordingCorrelation
	ErrWorkerPortableRecordingProvenance    = workerrecording.ErrWorkerPortableRecordingProvenance
	ErrWorkerPortableRecordingFidelity      = workerrecording.ErrWorkerPortableRecordingFidelity
	ErrWorkerPortableRecordingOrder         = workerrecording.ErrWorkerPortableRecordingOrder
	ErrWorkerPortableRecordingTerminal      = workerrecording.ErrWorkerPortableRecordingTerminal
	ErrWorkerPortableRecordingIntegrity     = workerrecording.ErrWorkerPortableRecordingIntegrity
)
