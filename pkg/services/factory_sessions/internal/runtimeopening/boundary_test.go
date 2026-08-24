package runtimeopening

import (
	"context"
	"errors"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestOperatorConfigPathRequiresExplicitProcessHome(t *testing.T) {
	t.Parallel()

	_, err := operatorConfigPath(factorysessions.SessionRuntimeOpeningRequest{})
	if err == nil || !strings.Contains(err.Error(), "operator config home is required") {
		t.Fatalf("operatorConfigPath() error = %v, want required process home", err)
	}
}

func TestNewOrderlyRecordingFlushSkipsUnboundRecording(t *testing.T) {
	service := &orderlyRecordingService{}
	tests := map[string]struct {
		service     recordings.Service
		recordingID string
		recordPath  string
	}{
		"nil service": {
			service: nil, recordingID: "recording-1", recordPath: "recording.json",
		},
		"missing recording id": {
			service: service, recordingID: "", recordPath: "recording.json",
		},
		"disabled recording": {
			service: service, recordingID: "recording-1", recordPath: "",
		},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			operation := newOrderlyRecordingFlush(test.service, test.recordingID, test.recordPath)
			if operation != nil {
				t.Fatal("newOrderlyRecordingFlush returned an operation for an unbound recording")
			}
		})
	}
	if service.calls != 0 {
		t.Fatalf("FlushRecording calls = %d, want none", service.calls)
	}
}

func TestNewOrderlyRecordingFlushDelegatesSynchronously(t *testing.T) {
	service := &orderlyRecordingService{}
	operation := newOrderlyRecordingFlush(service, "recording-1", "recording.json")
	if operation == nil {
		t.Fatal("newOrderlyRecordingFlush returned nil for a live recording")
	}
	if err := operation(context.Background()); err != nil {
		t.Fatalf("orderly recording flush: %v", err)
	}
	if service.calls != 1 || service.id != "recording-1" {
		t.Fatalf("FlushRecording call = (%d, %q), want (1, recording-1)", service.calls, service.id)
	}
}

func TestNewOrderlyRecordingFlushPreservesFailure(t *testing.T) {
	want := errors.New("recording write failed")
	service := &orderlyRecordingService{err: want}
	operation := newOrderlyRecordingFlush(service, "recording-1", "recording.json")
	err := operation(context.Background())
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "orderly shutdown") {
		t.Fatalf("orderly recording flush error = %v, want wrapped recording failure", err)
	}
}

func TestNewOrderlyRecordingFlushHonorsCanceledContext(t *testing.T) {
	service := &orderlyRecordingService{}
	operation := newOrderlyRecordingFlush(service, "recording-1", "recording.json")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := operation(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled orderly recording flush error = %v, want context.Canceled", err)
	}
	if service.calls != 0 {
		t.Fatalf("FlushRecording calls = %d, want no call after cancellation", service.calls)
	}
}

type orderlyRecordingService struct {
	recordings.Service
	calls int
	id    recordings.RecordingID
	err   error
}

func (service *orderlyRecordingService) FlushRecording(
	request recordings.FlushRecordingRequest,
) (recordings.FlushRecordingResult, error) {
	service.calls++
	service.id = request.RecordingID
	return recordings.FlushRecordingResult{}, service.err
}

var _ recordings.Service = (*orderlyRecordingService)(nil)
