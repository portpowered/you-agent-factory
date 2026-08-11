package recordings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workerrecording "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/worker_capture"
)

// WorkerSessionRecordingService is the narrow Recordings capability used by
// Worker Sessions before provider handoff. StartWorkerSessionRecording must
// subscribe to the complete Worker topic before it returns; the returned
// handle does not release the caller until the authoritative opening has been
// durably accepted.
type WorkerSessionRecordingService interface {
	StartWorkerSessionRecording(
		context.Context,
		WorkerSessionRecordingRequest,
	) (WorkerSessionRecording, error)
}

// WorkerSessionRecording is the per-Worker barrier and capture lifecycle.
// AwaitOpening is the only operation that may release provider handoff.
type WorkerSessionRecording interface {
	AwaitOpening(context.Context) error
	Close(context.Context) error
}

// WorkerSessionRecordingRequest identifies the exact source stream Recordings
// must capture. Topic is explicit so a caller cannot accidentally subscribe to
// a sibling or provider-owned stream.
type WorkerSessionRecordingRequest struct {
	RecordingID     string
	WorkerSessionID string
	Topic           events.Topic
}

// Validate reports whether the request names one concrete Worker topic.
func (request WorkerSessionRecordingRequest) Validate() error {
	if strings.TrimSpace(request.WorkerSessionID) == "" {
		return fmt.Errorf("%w: Worker Session ID is required", ErrInvalidWorkerRecordingRequest)
	}
	if err := request.Topic.Validate(); err != nil {
		return fmt.Errorf("%w: topic: %w", ErrInvalidWorkerRecordingRequest, err)
	}
	expectedTopic := events.Topic("worker-session/" + strings.TrimSpace(request.WorkerSessionID) + "/events")
	if request.Topic != expectedTopic {
		return fmt.Errorf("%w: topic %q is not the canonical Worker Session topic %q", ErrInvalidWorkerRecordingRequest, request.Topic, expectedTopic)
	}
	return nil
}

// WorkerRecordingRecord is the detached value passed to the durable writer.
// The writer receives the Events record unchanged, including its aggregate
// position and complete source idempotency identity.
type WorkerRecordingRecord struct {
	RecordingID     string
	WorkerSessionID string
	Record          events.Record
}

// Worker recording values and the shared pure reducer live in the focused
// Recordings-owned internal Worker capture package and are re-exported here
// as the customer-facing service vocabulary.
type (
	WorkerRecordingSnapshot        = workerrecording.WorkerRecordingSnapshot
	WorkerSessionRecordingSnapshot = workerrecording.WorkerSessionRecordingSnapshot
	WorkerRecordingStatus          = workerrecording.WorkerRecordingStatus
	WorkerRecordingHistory         = workerrecording.WorkerRecordingHistory
	WorkerRecordingTerminal        = workerrecording.WorkerRecordingTerminal
	WorkerRecordingProjection      = workerrecording.WorkerRecordingProjection
	WorkerRecordingReplayRequest   = workerrecording.WorkerRecordingReplayRequest
	WorkerRecordingReplayResult    = workerrecording.WorkerRecordingReplayResult
)

const (
	WorkerRecordingStatusActive    = workerrecording.WorkerRecordingStatusActive
	WorkerRecordingStatusCompleted = workerrecording.WorkerRecordingStatusCompleted
	WorkerRecordingStatusFailed    = workerrecording.WorkerRecordingStatusFailed
)

// ReduceWorkerRecording applies the one deterministic Worker history reducer.
func ReduceWorkerRecording(history WorkerRecordingHistory) (WorkerRecordingProjection, error) {
	return workerrecording.ReduceWorkerRecording(history)
}

// ReplayWorkerRecording reduces one durable Worker snapshot and rejects an
// active prefix as incomplete replay.
func ReplayWorkerRecording(request WorkerRecordingReplayRequest) (WorkerRecordingReplayResult, error) {
	return workerrecording.ReplayWorkerRecording(request)
}

// WorkerRecordingReader is the optional read side of the durable Worker
// recording store. The default local FileWriter implements it; keeping it
// separate from WorkerRecordingWriter lets tests and alternate stores expose
// only the effect they own.
type WorkerRecordingReader interface {
	LoadWorkerRecording(context.Context, string) (WorkerRecordingSnapshot, error)
}

// WorkerRecordingProjectionReader is an optional observation seam for a live
// capture. It is separate from WorkerSessionRecording so the opening barrier
// remains the only handoff gate and existing callers need not implement a
// replay-shaped method.
type WorkerRecordingProjectionReader interface {
	WorkerRecordingProjection() (WorkerRecordingProjection, error)
}

// WorkerRecordingFailure is the safe durable classification written when
// capture cannot reach a legal terminal. It intentionally carries no raw
// provider payload or implementation error text.
type WorkerRecordingFailure struct {
	RecordingID     string
	WorkerSessionID string
	Topic           events.Topic
	Code            string
}

// WorkerRecordingFailureWriter is an optional failure side of the durable
// Worker recording store. The capture writer remains the only Recordings
// acceptance port; Worker Sessions never receives this capability.
type WorkerRecordingFailureWriter interface {
	PersistWorkerRecordingFailure(context.Context, WorkerRecordingFailure) error
}

// WorkerRecordingWriter is the exact durable acceptance port owned by
// Recordings. It is intentionally narrower than the Recordings root service;
// Worker Sessions never receives this writer and never writes recording data.
type WorkerRecordingWriter interface {
	PersistWorkerRecord(context.Context, WorkerRecordingRecord) error
}

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
	ErrInvalidWorkerRecordingRequest = workerrecording.ErrInvalidWorkerRecordingRequest
	ErrMissingWorkerRecordingWriter  = errors.New("recordings: Worker recording writer is required")
	ErrWorkerRecordingSubscribe      = errors.New("recordings: Worker recording subscription failed")
	ErrWorkerRecordingOpening        = workerrecording.ErrWorkerRecordingOpening
	ErrWorkerRecordingPersistence    = errors.New("recordings: Worker recording persistence failed")
	ErrWorkerRecordingDelivery       = workerrecording.ErrWorkerRecordingDelivery
	ErrWorkerRecordingGap            = errors.New("recordings: Worker recording retention gap")
	ErrWorkerRecordingClosed         = errors.New("recordings: Worker recording source closed")
	ErrWorkerRecordingCanceled       = errors.New("recordings: Worker recording subscription canceled")
	ErrWorkerRecordingBackpressure   = errors.New("recordings: Worker recording backpressure")
	ErrWorkerRecordingOrder          = workerrecording.ErrWorkerRecordingOrder
	ErrWorkerRecordingDuplicate      = workerrecording.ErrWorkerRecordingDuplicate
	ErrWorkerRecordingTerminal       = workerrecording.ErrWorkerRecordingTerminal
	ErrWorkerRecordingIncomplete     = workerrecording.ErrWorkerRecordingIncomplete
	ErrWorkerRecordingReplay         = workerrecording.ErrWorkerRecordingReplay
)
