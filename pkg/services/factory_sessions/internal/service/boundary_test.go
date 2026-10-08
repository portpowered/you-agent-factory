package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"

	"go.uber.org/goleak"
)

func TestOperatorConfigPathRequiresExplicitProcessHome(t *testing.T) {
	t.Parallel()

	_, err := operatorConfigPath("", "")
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

func TestOrderlyRecordingPublishesCurrentBoardOnlyAfterFlush(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"fresh explicit", "refresh", "flush failure", "publication failure", "invalid reference", "canceled", "batch", "peer", "replay", "no record", "no server"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			artifact := filepath.Join(directory, "actual-writer.json")
			owner := &boardReferenceOwner{}
			opening := &sessionRuntimeOpening{
				sessionID: "~default",
				sessionSelection: &factorysessions.SessionRuntimeSelection{
					Mode: factorysessions.SessionRuntimeModeService,
					Host: factorysessions.RuntimeHostRequest{Port: 1234},
				},
				configured:       preparedRuntime{Recordings: recordings.RuntimeSelection{RecordPath: artifact}},
				load:             RuntimeLoad{LoadedFactoryCfg: boardReferenceSource{directory: directory}},
				durableExecution: DurableExecution{Service: owner},
			}
			want := errors.New("controlled failure")
			wantSave, wantError := configureOrderlyBoardPublicationCase(name, directory, opening, owner, want)
			flushed := false
			operation := opening.orderlyCurrentBoardStop(func(ctx context.Context) error {
				if owner.saves != 0 || owner.loads != 0 {
					t.Fatal("publication preceded flush")
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if name == "flush failure" {
					return want
				}
				flushed = true
				return nil
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if name == "canceled" {
				cancel()
			}
			err := operation(ctx)
			if (err != nil) != wantError || (owner.saves == 1) != wantSave {
				t.Fatalf("stop error=%v saves=%d, want error=%v save=%v", err, owner.saves, wantError, wantSave)
			}
			if owner.saves == 1 && (!flushed || owner.savedArtifact != artifact || owner.savedFactory != directory) {
				t.Fatal("reference does not name the flushed writer")
			}
		})
	}
}

func configureOrderlyBoardPublicationCase(name, directory string, opening *sessionRuntimeOpening, owner *boardReferenceOwner, failure error) (wantSave, wantError bool) {
	wantSave = true
	switch name {
	case "refresh":
		owner.path = filepath.Join(directory, "old-writer.json")
	case "flush failure", "canceled":
		wantSave, wantError = false, true
	case "publication failure":
		owner.saveFailure, wantError = failure, true
	case "invalid reference":
		owner.failure = failure
		wantSave, wantError = false, true
	case "batch":
		opening.sessionSelection.Mode = factorysessions.SessionRuntimeModeBatch
		wantSave = false
	case "peer":
		opening.sessionID, wantSave = "peer", false
	case "replay":
		opening.configured.Recordings.ReplayPath, wantSave = "replay.json", false
	case "no record":
		opening.configured.Recordings.RecordPath, wantSave = "", false
	case "no server":
		opening.sessionSelection.Host.Port, wantSave = 0, false
	}
	return wantSave, wantError
}

// TestMain fails the package when a test leaves goroutines running, which
// otherwise surfaces as teardown hangs and cross-test interference.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
