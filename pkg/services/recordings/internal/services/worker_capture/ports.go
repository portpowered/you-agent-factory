package worker_capture

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// Service is the pure Worker recording capability. It keeps reduction,
// portable validation, and portable replay behind one focused subservice
// contract while the enclosing Recordings service owns lifecycle and storage
// effects.
type Service interface {
	ReduceWorkerRecording(WorkerRecordingHistory) (WorkerRecordingProjection, error)
	ReplayWorkerRecording(WorkerRecordingReplayRequest) (WorkerRecordingReplayResult, error)
	BuildWorkerPortableRecording(WorkerRecordingSnapshot, ...string) (WorkerPortableRecording, error)
	ExportWorkerPortableRecording(WorkerRecordingSnapshot, ...string) (WorkerPortableRecording, error)
	ValidateWorkerPortableRecording(WorkerPortableRecording) error
	EncodeWorkerPortableRecording(WorkerPortableRecording) ([]byte, error)
	DecodeWorkerPortableRecording([]byte) (WorkerPortableRecording, error)
	ReplayWorkerPortableRecording(WorkerPortableRecording) (WorkerRecordingReplayResult, error)
}

var _ Service = WorkerRecordingCodec{}

// WorkerSessionRecordingRequest identifies the exact source stream Recordings
// must capture. Topic is explicit so a caller cannot accidentally subscribe to
// a sibling or provider-owned stream.
type WorkerSessionRecordingRequest struct {
	RecordingID      string
	FactorySessionID string
	WorkerSessionID  string
	Topic            events.Topic
	WorkIDs          []string
	AttemptID        string
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
	RecordingID      string
	FactorySessionID string
	WorkerSessionID  string
	WorkIDs          []string
	AttemptID        string
	Record           events.Record
}

// WorkerRecordingFailure is the safe durable classification written when
// capture cannot reach a legal terminal. It intentionally carries no raw
// provider payload or implementation error text.
type WorkerRecordingFailure struct {
	RecordingID       string
	FactorySessionID  string
	WorkerSessionID   string
	WorkIDs           []string
	AttemptID         string
	Topic             events.Topic
	Code              string
	ExecutionTerminal *WorkerRecordingTerminal
}
