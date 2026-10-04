package wire

import (
	"context"
	"encoding/json"
	"errors"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type workerRecordingDirectoryProbe struct {
	root  string
	err   error
	calls int
}

func (probe *workerRecordingDirectoryProbe) Getwd() (string, error) {
	probe.calls++
	return probe.root, probe.err
}

func TestWorkerRecordingDefaultDurableRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directory := &workerRecordingDirectoryProbe{root: root}
	writer, err := provideWorkerRecordingWriter(serviceedges.Edges{FactorySessionsWorkingDirectory: directory})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("construction changed project root: %v, %v", entries, err)
	}
	record := recordings.WorkerRecordingRecord{
		RecordingID: "wire-durable", WorkerSessionID: "wire-worker",
		Record: events.Record{
			ID:         events.RecordID{Topic: "worker-session/wire-worker/events", Position: 1},
			SourceType: "worker_session_lifecycle", SourceID: "wire-worker", SourceSequence: 1,
			SourceEventID: "started", SchemaID: "workers.draft.v1",
			Payload: json.RawMessage(`{"kind":"SESSION","phase":"STARTED","provenance":{"delivery":"SYNTHESIZED","fidelity":"LIFECYCLE_ONLY","nativeEventType":"worker_session_lifecycle","representation":"NOTIFICATION"},"payload":{"status":"STARTING","workerSessionId":"wire-worker"}}`),
		},
	}
	if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if directory.calls != 1 {
		t.Fatalf("Getwd calls = %d, want one", directory.calls)
	}
	reopened, err := recordingswire.NewWorkerRecordingFileWriter(platformreplay.NewLocal(runtime.GOOS), filepath.Join(root, ".you-agent-factory", "worker-recordings"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.(recordings.WorkerRecordingReader).LoadWorkerRecording(t.Context(), record.RecordingID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sessions) != 1 || len(snapshot.Sessions[0].Records) != 1 || !reflect.DeepEqual(snapshot.Sessions[0].Records[0], record.Record) {
		t.Fatalf("reopened durable history = %#v", snapshot)
	}
}

func TestWorkerRecordingRootFailureAndOverride(t *testing.T) {
	t.Parallel()
	fault := errors.New("private directory resolution fault")
	directory := &workerRecordingDirectoryProbe{err: fault}
	_, err := provideWorkerRecordingWriter(serviceedges.Edges{FactorySessionsWorkingDirectory: directory})
	if !errors.Is(err, fault) {
		t.Fatalf("root resolution error = %v", err)
	}
	override, err := recordingswire.NewWorkerRecordingFileWriter(platformreplay.NewLocal(runtime.GOOS), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory.calls = 0
	got, err := provideWorkerRecordingWriter(serviceedges.Edges{FactorySessionsWorkingDirectory: directory, WorkerRecordingWriter: override})
	if err != nil || got != override || directory.calls != 0 {
		t.Fatalf("override = %v, error %v, Getwd calls %d", got, err, directory.calls)
	}
}

func TestWorkerRecordingRejectsInvalidDefaultRoot(t *testing.T) {
	t.Parallel()
	for _, root := range []string{"", "relative-project"} {
		t.Run(root, func(t *testing.T) {
			t.Parallel()
			directory := &workerRecordingDirectoryProbe{root: root}
			writer, err := provideWorkerRecordingWriter(serviceedges.Edges{FactorySessionsWorkingDirectory: directory})
			if err == nil || writer != nil || !strings.Contains(err.Error(), "expected a non-empty absolute directory") {
				t.Fatalf("invalid root %q: writer = %v, error = %v", root, writer, err)
			}
			override, err := recordingswire.NewWorkerRecordingFileWriter(platformreplay.NewLocal(runtime.GOOS), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			directory.calls = 0
			got, err := provideWorkerRecordingWriter(serviceedges.Edges{FactorySessionsWorkingDirectory: directory, WorkerRecordingWriter: override})
			if err != nil || got != override || directory.calls != 0 {
				t.Fatalf("override for root %q: writer = %v, error = %v, Getwd calls = %d", root, got, err, directory.calls)
			}
		})
	}
}

func TestProvideWorkerRecordingReaderPreservesReader(t *testing.T) {
	t.Parallel()

	reader := workerRecordingReaderCompositionProbe{}
	got, err := provideWorkerRecordingReader(reader)
	if err != nil {
		t.Fatalf("provideWorkerRecordingReader() error = %v", err)
	}
	payload, err := got.LoadWorkerRecording(t.Context(), "wire-reader")
	if err != nil {
		t.Fatalf("LoadWorkerRecording() error = %v", err)
	}
	var snapshot recordings.WorkerRecordingSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		t.Fatalf("decode Worker recording snapshot: %v", err)
	}
	if snapshot.RecordingID != "" || len(snapshot.Sessions) != 0 {
		t.Fatalf("snapshot = %#v, want empty snapshot", snapshot)
	}
}

type workerRecordingReaderCompositionProbe struct{}

func (workerRecordingReaderCompositionProbe) PersistWorkerRecord(context.Context, recordings.WorkerRecordingRecord) error {
	return nil
}

func (workerRecordingReaderCompositionProbe) LoadWorkerRecording(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
	return recordings.WorkerRecordingSnapshot{}, nil
}

// This fixture observes provider forwarding, not the implementation of the
// supplied owner. Native owner tests and public functional journeys own policy.
type replayForwardingOwner struct {
	request  any
	response any
	err      error
	ctx      context.Context
	calls    int
}

func (owner *replayForwardingOwner) LoadReplayRecording(request recordings.LoadReplayRecordingRequest) (recordings.LoadReplayRecordingResult, error) {
	owner.request = request
	owner.calls++
	if owner.err != nil {
		return recordings.LoadReplayRecordingResult{}, owner.err
	}
	return owner.response.(recordings.LoadReplayRecordingResult), nil
}
func (owner *replayForwardingOwner) LoadReplayRecordingForResume(request recordings.LoadReplayRecordingForResumeRequest) (recordings.LoadReplayRecordingForResumeResult, error) {
	owner.request = request
	owner.calls++
	if owner.err != nil {
		return recordings.LoadReplayRecordingForResumeResult{}, owner.err
	}
	return owner.response.(recordings.LoadReplayRecordingForResumeResult), nil
}
func (owner *replayForwardingOwner) CreateReplayPlan(request recordings.CreateReplayPlanRequest) (recordings.CreateReplayPlanResult, error) {
	owner.request = request
	owner.calls++
	if owner.err != nil {
		return recordings.CreateReplayPlanResult{}, owner.err
	}
	return owner.response.(recordings.CreateReplayPlanResult), nil
}
func (owner *replayForwardingOwner) ObserveReplay(request recordings.ObserveReplayRequest) (recordings.ObserveReplayResult, error) {
	owner.request = request
	owner.calls++
	if owner.err != nil {
		return recordings.ObserveReplayResult{}, owner.err
	}
	return owner.response.(recordings.ObserveReplayResult), nil
}
func TestProvideRecordingsRootForwardsCompletedReplayOwner(t *testing.T) {
	t.Parallel()
	recording := recordings.ReplayRecordingFacts{RecordingID: "selected-recording", Events: []recordings.CanonicalEvent{{ID: "selected-event", Payload: "{}"}}}
	cases := []struct {
		name        string
		failure     error
		request     any
		response    any
		withContext bool
		call        func(recordings.Service, context.Context) (any, error)
	}{
		{name: "LoadReplayRecording", failure: recordings.ErrReplayRecordingNotFound, request: recordings.LoadReplayRecordingRequest{RecordingID: "selected-recording"}, response: recordings.LoadReplayRecordingResult{Recording: recording}, withContext: false,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.LoadReplayRecording(recordings.LoadReplayRecordingRequest{RecordingID: "selected-recording"})
			}},
		{name: "LoadReplayRecordingForResume", failure: recordings.ErrReplayRecordingNotFinalized, request: recordings.LoadReplayRecordingForResumeRequest{RecordingID: "selected-recording"}, response: recordings.LoadReplayRecordingForResumeResult{Recording: recording, RecoveredEventCount: 1}, withContext: false,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.LoadReplayRecordingForResume(recordings.LoadReplayRecordingForResumeRequest{RecordingID: "selected-recording"})
			}},
		{name: "CreateReplayPlan", failure: recordings.ErrCorruptReplayInput, request: recordings.CreateReplayPlanRequest{SchemaVersion: recordings.ReplayPlanSchemaV1, Timing: recordings.ReplayTimingOrderOnly, Recording: recording, SelectedTick: 7}, response: recordings.CreateReplayPlanResult{Plan: recordings.ReplayPlanFacts{Handle: "selected-plan"}}, withContext: false,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.CreateReplayPlan(recordings.CreateReplayPlanRequest{SchemaVersion: recordings.ReplayPlanSchemaV1, Timing: recordings.ReplayTimingOrderOnly, Recording: recording, SelectedTick: 7})
			}},
		{name: "ObserveReplay", failure: recordings.ErrReplayPlanNotFound, request: recordings.ObserveReplayRequest{Plan: "selected-plan"}, response: recordings.ObserveReplayResult{Observation: recordings.ReplayObservation{Kind: recordings.ReplayCompleted, ProcessedEvents: 1}}, withContext: false,
			call: func(service recordings.Service, ctx context.Context) (any, error) {
				return service.ObserveReplay(recordings.ObserveReplayRequest{Plan: "selected-plan"})
			}},
	}
	for _, cell := range cases {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			for _, failure := range []error{nil, cell.failure, errors.New("selected-owner-dependency-failure")} {
				owner := &replayForwardingOwner{response: cell.response, err: failure}
				root := testRecordingsRoot(serviceedges.Edges{}, inertArtifactsOwner{}, owner)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				result, err := cell.call(root, ctx)
				if err != failure || owner.calls != 1 || !reflect.DeepEqual(owner.request, cell.request) {
					t.Fatalf("forward = %v, calls %d, request %#v; want %v and %#v", err, owner.calls, owner.request, failure, cell.request)
				}
				if failure == nil && !reflect.DeepEqual(result, cell.response) {
					t.Fatalf("result = %#v, want %#v", result, cell.response)
				}
				if cell.withContext && owner.ctx != ctx {
					t.Fatal("owner did not receive caller context")
				}
			}
		})
	}
}
