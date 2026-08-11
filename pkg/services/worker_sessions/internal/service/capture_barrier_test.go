package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionservice "github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestInvokeSessionWaitsForDurableOpeningBeforeProviderHandoff(t *testing.T) {
	execution := succeedingExecution()
	recording := newControlledRecording()
	registry, err := workersessionservice.New(
		executionBoundary{execution: execution},
		newEventsAppender(),
		logging.NoopLogger{},
		platformclock.Real{},
		unavailableProviderSessionsForCapture{},
		recording,
	)
	if err != nil {
		t.Fatal(err)
	}

	resultCh := make(chan workersessions.InvokeSessionResult, 1)
	errCh := make(chan error, 1)
	request := validStartRequest("worker-capture", "dispatch-capture")
	request.Execution.Execution.RecordingID = "recording-capture"
	go func() {
		result, err := registry.InvokeSession(context.Background(), request)
		resultCh <- result
		errCh <- err
	}()
	<-recording.started
	if got := execution.callCount(); got != 0 {
		t.Fatalf("provider calls before durable opening = %d, want 0", got)
	}
	close(recording.release)
	if err := <-errCh; err != nil {
		t.Fatalf("InvokeSession() error = %v, want nil", err)
	}
	result := <-resultCh
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("InvokeSession() state = %q, want COMPLETED", result.Session.State)
	}
	if got := execution.callCount(); got != 1 {
		t.Fatalf("provider calls after durable opening = %d, want 1", got)
	}
	if !recording.closed() {
		t.Fatal("Worker recording was not closed after terminal publication")
	}
}

func TestInvokeSessionOpeningBarrierFailureMakesZeroProviderCalls(t *testing.T) {
	execution := succeedingExecution()
	openingErr := errors.New("durable opening rejected")
	recording := &failingRecordingService{err: openingErr}
	registry, err := workersessionservice.New(
		executionBoundary{execution: execution},
		newEventsAppender(),
		logging.NoopLogger{},
		platformclock.Real{},
		unavailableProviderSessionsForCapture{},
		recording,
	)
	if err != nil {
		t.Fatal(err)
	}
	request := validStartRequest("worker-failure", "dispatch-failure")
	request.Execution.Execution.RecordingID = "recording-failure"
	result, err := registry.InvokeSession(context.Background(), request)
	if err != nil {
		t.Fatalf("InvokeSession() error = %v, want nil terminal result", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("InvokeSession() state = %q, want FAILED", result.Session.State)
	}
	if got := execution.callCount(); got != 0 {
		t.Fatalf("provider calls after opening failure = %d, want 0", got)
	}
}

type controlledRecordingService struct {
	started chan struct{}
	release chan struct{}
	handle  *controlledRecording
}

func newControlledRecording() *controlledRecordingService {
	handle := &controlledRecording{release: make(chan struct{}), closedCh: make(chan struct{})}
	return &controlledRecordingService{started: make(chan struct{}), release: handle.release, handle: handle}
}

func (service *controlledRecordingService) StartWorkerSessionRecording(
	context.Context,
	recordings.WorkerSessionRecordingRequest,
) (recordings.WorkerSessionRecording, error) {
	close(service.started)
	return service.handle, nil
}

type controlledRecording struct {
	release   chan struct{}
	closedCh  chan struct{}
	closeOnce sync.Once
}

func (recording *controlledRecording) AwaitOpening(ctx context.Context) error {
	select {
	case <-recording.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (recording *controlledRecording) Close(context.Context) error {
	recording.closeOnce.Do(func() { close(recording.closedCh) })
	return nil
}

func (service *controlledRecordingService) closed() bool {
	select {
	case <-service.handle.closedCh:
		return true
	default:
		return false
	}
}

type failingRecordingService struct{ err error }

func (service *failingRecordingService) StartWorkerSessionRecording(
	context.Context,
	recordings.WorkerSessionRecordingRequest,
) (recordings.WorkerSessionRecording, error) {
	return nil, service.err
}

type unavailableProviderSessionsForCapture struct {
	providersessions.Service
}

func (unavailableProviderSessionsForCapture) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

var _ recordings.WorkerSessionRecordingService = (*controlledRecordingService)(nil)

var _ recordings.WorkerSessionRecordingService = (*failingRecordingService)(nil)

var _ workers.WorkstationPoolBoundary = executionBoundary{}

var _ platformclock.Source = platformclock.Real{}

var _ logging.Logger = logging.NoopLogger{}
