package contracts

import (
	"context"

	workerrecording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture"
)

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
