package contracts

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerrecording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkerCaptureClock supplies host commit-operation time.
type WorkerCaptureClock interface{ Now() time.Time }

// WorkerCapturePreparationOperation prepares committed capture facts before
// runtime activation, independently of Factory Work-name attribution.
type WorkerCapturePreparationOperation func(context.Context) error

type WorkerCapturedActivityReader interface {
	// ListPreparedWorkerSessionCaptures reads only activation-prepared metadata;
	// it refuses unavailable summaries without hydrating recording history.
	ListPreparedWorkerSessionCaptures(context.Context, workerrecording.WorkerCapturedCatalogRequest) (workerrecording.WorkerCapturedCatalogPage, error)
	ListWorkerSessionCaptures(context.Context, workerrecording.WorkerCapturedCatalogRequest) (workerrecording.WorkerCapturedCatalogPage, error)
	LookupWorkerSessionCapture(context.Context, string) (workerrecording.WorkerSessionCatalogEntry, error)
	ReadWorkerCapturedActivity(context.Context, workerrecording.WorkerCapturedActivityRequest) (workerrecording.WorkerCapturedActivityPage, error)
}

// WorkerCapturedArtifactReader retrieves the exact committed payload of one
// oversized record within the selected Worker Session and profile.
type WorkerCapturedArtifactReader interface {
	ReadWorkerCapturedArtifact(context.Context, string, string) (io.ReadCloser, error)
}

type WorkerRecordingStore interface {
	WorkerRecordingWriter
	WorkerRecordingReader
	WorkerRecordingHealthReader
	WorkerRecordingFailureWriter
	WorkerCapturedActivityReader
	WorkerControlOperationStore
	WorkerRestartInputStore
}

type WorkerRestartInputStore interface {
	ValidateWorkerRestartRecipe(context.Context, string, workers.WorkstationDispatchRequest) error
	SaveWorkerRestartRecipe(context.Context, workerrecording.WorkerControlTarget, workers.WorkstationDispatchRequest) error
	ReadWorkerRestartRecipe(context.Context, workerrecording.WorkerControlTarget) (workers.WorkstationDispatchRequest, error)
	// LookupPreparedWorkerContinuationSource selects activated metadata and the
	// bounded immutable recipe. It never hydrates recording history.
	LookupPreparedWorkerContinuationSource(context.Context, workerrecording.WorkerControlTarget) (WorkerContinuationSource, error)
	ReadWorkerContinuationInput(context.Context, workerrecording.WorkerControlOperationKey) (json.RawMessage, error)
}

// WorkerContinuationSource is detached proof from committed source history.
// It contains no execution handle and grants no admission or control authority.
type WorkerContinuationSource struct {
	Execution workers.WorkstationDispatchRequest
	Reference providers.SessionRef
	Terminal  workerrecording.WorkerRecordingTerminal
	TurnID    string
}

// WorkerControlOperationStore shares the recording journal's sync boundary.
// Begin's bool is true only for a newly committed intent. Advance uses CAS.
type WorkerControlOperationStore interface {
	BeginWorkerControlOperation(context.Context, workerrecording.WorkerControlOperationRecord) (workerrecording.WorkerControlOperationRecord, bool, error)
	AdvanceWorkerControlOperation(context.Context, workerrecording.WorkerControlOperationRecord, uint64) (workerrecording.WorkerControlOperationRecord, error)
	LoadWorkerControlOperation(context.Context, workerrecording.WorkerControlOperationKey) (workerrecording.WorkerControlOperationRecord, error)
	ListWorkerControlOperations(context.Context, workerrecording.WorkerControlTarget) ([]workerrecording.WorkerControlOperationRecord, error)
	PersistWorkerControlInput(context.Context, workerrecording.WorkerControlOperationKey, json.RawMessage) (string, error)
	ReadWorkerControlInput(context.Context, workerrecording.WorkerControlOperationKey, string) (json.RawMessage, error)
}

// WorkerSessionRecordingService is the narrow capture capability used by
// Worker Sessions before provider handoff.
type WorkerSessionRecordingService interface {
	StartWorkerSessionRecording(
		context.Context,
		workerrecording.WorkerSessionRecordingRequest,
	) (WorkerSessionRecording, error)
}

// WorkerSessionRecording is the per-Worker barrier and capture lifecycle.
type WorkerSessionRecording interface {
	AwaitOpening(context.Context) error
	Abort(context.Context, error) error
	Close(context.Context) error
}

// WorkerSessionRecordingFinalizer is the optional terminal-aware extension
// implemented by the Recordings-owned capture. Worker Sessions supplies the
// authoritative terminal fact after it commits its own outcome so a capture
// that already lost durable fidelity can still be classified as DEGRADED.
// Older recording implementations may expose only WorkerSessionRecording;
// callers must retain the Close fallback for that compatibility boundary.
type WorkerSessionRecordingFinalizer interface {
	CloseWithTerminal(context.Context, workerrecording.WorkerRecordingTerminal) error
}

// WorkerRecordingReader is the durable read side of a Worker recording store.
type WorkerRecordingReader interface {
	LoadWorkerRecording(context.Context, string) (workerrecording.WorkerRecordingSnapshot, error)
}

// WorkerRecordingHealthReader selects prepared health for exact Worker IDs.
// The snapshot contains only opening records, never activity history. The
// selected recording's preparation error is returned even for an empty ID set.
type WorkerRecordingHealthReader interface {
	CurrentWorkerRecordingHealth(context.Context, string, []string) (workerrecording.WorkerRecordingSnapshot, error)
}

// WorkerRecordingProjectionReader observes one live capture projection.
type WorkerRecordingProjectionReader interface {
	WorkerRecordingProjection() (workerrecording.WorkerRecordingProjection, error)
}

// WorkerRecordingFailureWriter persists a safe failed-capture classification.
type WorkerRecordingFailureWriter interface {
	PersistWorkerRecordingFailure(context.Context, workerrecording.WorkerRecordingFailure) error
}

// WorkerRecordingWriter is the durable acceptance port for one Worker record.
type WorkerRecordingWriter interface {
	PersistWorkerRecord(context.Context, workerrecording.WorkerRecordingRecord) error
}

// WorkerCapturedSummary contains bounded committed metadata and control facts.
type WorkerCapturedSummary struct {
	Capture           workerrecording.WorkerCapturedCatalogItem
	ControlOperations []workerrecording.WorkerControlOperationRecord
}

// WorkerCapturedSummaryReader never hydrates or scans recording history.
type WorkerCapturedSummaryReader interface {
	LookupWorkerSessionSummary(context.Context, string) (WorkerCapturedSummary, error)
}
