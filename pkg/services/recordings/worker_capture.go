package recordings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/events"
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

// WorkerRecordingSnapshot is the durable sidecar shape used by the default
// local writer. The schema is deliberately source-native: later replay work
// can load the exact Events records without reconstructing them from a
// provider-specific projection.
type WorkerRecordingSnapshot struct {
	RecordingID string                           `json:"recordingId"`
	Sessions    []WorkerSessionRecordingSnapshot `json:"sessions"`
}

// WorkerSessionRecordingSnapshot contains one detached Worker topic history
// in aggregate order.
type WorkerSessionRecordingSnapshot struct {
	WorkerSessionID string          `json:"workerSessionId"`
	Records         []events.Record `json:"records"`
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
	ErrInvalidWorkerRecordingRequest = errors.New("recordings: invalid Worker recording request")
	ErrMissingWorkerRecordingWriter  = errors.New("recordings: Worker recording writer is required")
	ErrWorkerRecordingSubscribe      = errors.New("recordings: Worker recording subscription failed")
	ErrWorkerRecordingOpening        = errors.New("recordings: Worker recording opening is invalid")
	ErrWorkerRecordingPersistence    = errors.New("recordings: Worker recording persistence failed")
	ErrWorkerRecordingDelivery       = errors.New("recordings: Worker recording delivery failed")
	ErrWorkerRecordingGap            = errors.New("recordings: Worker recording retention gap")
	ErrWorkerRecordingClosed         = errors.New("recordings: Worker recording source closed")
	ErrWorkerRecordingCanceled       = errors.New("recordings: Worker recording subscription canceled")
	ErrWorkerRecordingBackpressure   = errors.New("recordings: Worker recording backpressure")
	ErrWorkerRecordingOrder          = errors.New("recordings: Worker recording order is invalid")
	ErrWorkerRecordingDuplicate      = errors.New("recordings: Worker recording duplicate conflicts")
)
