package contracts

import (
	"context"
	"io"
	"time"

	workerrecording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture"
)

// WorkerCaptureClock supplies host commit-operation time.
type WorkerCaptureClock interface{ Now() time.Time }

type WorkerCapturedActivityReader interface {
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
	WorkerRecordingFailureWriter
	WorkerCapturedActivityReader
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
