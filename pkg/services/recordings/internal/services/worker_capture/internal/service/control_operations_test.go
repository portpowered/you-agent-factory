package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func controlIntent(t *testing.T, writer *FileWriter, recordingID, workerID, requestID string) recordings.WorkerControlOperationRecord {
	t.Helper()
	if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, recordingID, workerID)); err != nil {
		t.Fatal(err)
	}
	capture, err := writer.LookupWorkerSessionCapture(t.Context(), workerID)
	if err != nil {
		t.Fatal(err)
	}
	return recordings.WorkerControlOperationRecord{
		Target:    recordings.WorkerControlTarget{RecordingID: recordingID, WorkerSessionID: workerID, RecordingGenerationID: capture.RecordingGenerationID, OwnerEpoch: capture.OwnerEpoch, ExpectedAttemptID: "attempt-1"},
		Revision:  1,
		Operation: recordings.WorkerControlOperation{Version: 1, RequestID: requestID, WorkerSessionID: workerID, ExpectedAttemptID: "attempt-1", Action: "cancel", Phase: "INTENT", InputDigest: strings.Repeat("a", 64)},
	}
}

func TestControlOperationReplayPreservesPostTerminalPhasesAndResult(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	writer := journalWriter(t, local)
	intent := controlIntent(t, writer, "recording", "worker", "request")
	intent.Operation.Action = "interrupt"
	intent.Operation.ResumeMode = "provider"
	intent.Operation.SuccessorWorkerSessionID = "reserved-successor"
	inputRef, err := writer.PersistWorkerControlInput(t.Context(), operationKey(intent), json.RawMessage(`{"message":"input"}`))
	if err != nil {
		t.Fatal(err)
	}
	intent.InputArtifactRef = inputRef
	committed, created, err := writer.BeginWorkerControlOperation(t.Context(), intent)
	if err != nil || !created || !reflect.DeepEqual(committed, intent) {
		t.Fatalf("begin: %+v created=%v error=%v", committed, created, err)
	}
	opening := journalRecord(t, "recording", "worker")
	terminal := opening
	terminal.Record = mustRecord(t, terminalAppend(opening.Record.ID.Topic, "worker"), 2)
	if err := writer.PersistWorkerRecord(t.Context(), terminal); err != nil {
		t.Fatal(err)
	}
	stopped := intent
	stopped.Revision, stopped.Operation.Phase = 2, "SOURCE_STOPPED"
	if _, err := writer.AdvanceWorkerControlOperation(t.Context(), stopped, 1); err != nil {
		t.Fatal(err)
	}
	failed := stopped
	failed.Revision, failed.Operation.Phase = 3, "FAILED"
	failed.FailureCode = "SUCCESSOR_ADMISSION"
	failed.Result = json.RawMessage(`{"outcome":"FAILED","successorWorkerSessionId":"reserved-successor"}`)
	if _, err := writer.AdvanceWorkerControlOperation(t.Context(), failed, 2); err != nil {
		t.Fatal(err)
	}
	reopened, err := newTestFileWriter(local, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []recordings.WorkerRecordingStore{writer, reopened} {
		assertControlOperationRecovery(t, store, intent, stopped, failed)
	}
}

func assertControlOperationRecovery(t *testing.T, store recordings.WorkerRecordingStore, intent, stopped, failed recordings.WorkerControlOperationRecord) {
	t.Helper()
	result, created, err := store.BeginWorkerControlOperation(t.Context(), intent)
	if err != nil || created || !reflect.DeepEqual(result, failed) {
		t.Fatalf("replay: %+v created=%v error=%v", result, created, err)
	}
	result.Result[0] = 'x'
	latest, err := store.LoadWorkerControlOperation(t.Context(), operationKey(intent))
	if err != nil || !reflect.DeepEqual(latest, failed) {
		t.Fatalf("detached load: %+v error=%v", latest, err)
	}
	history, err := store.ListWorkerControlOperations(t.Context(), intent.Target)
	if err != nil || !reflect.DeepEqual(history, []recordings.WorkerControlOperationRecord{intent, stopped, failed}) {
		t.Fatalf("phase history: %+v error=%v", history, err)
	}
	snapshot, err := store.LoadWorkerRecording(t.Context(), "recording")
	if err != nil || len(snapshot.Sessions[0].Records) != 2 || snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusComplete {
		t.Fatalf("control rows changed Worker history: %+v error=%v", snapshot, err)
	}
}

func TestControlOperationRejectsChangedIntentAndStaleTarget(t *testing.T) {
	t.Parallel()
	writer := journalWriter(t, platformreplay.NewLocal(runtime.GOOS))
	intent := controlIntent(t, writer, "recording", "worker", "request")
	if _, _, err := writer.BeginWorkerControlOperation(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*recordings.WorkerControlOperationRecord){
		"action": func(r *recordings.WorkerControlOperationRecord) { r.Operation.Action = "terminate" },
		"input":  func(r *recordings.WorkerControlOperationRecord) { r.Operation.InputDigest = strings.Repeat("b", 64) },
		"attempt": func(r *recordings.WorkerControlOperationRecord) {
			r.Target.ExpectedAttemptID = "other"
			r.Operation.ExpectedAttemptID = "other"
		},
		"generation": func(r *recordings.WorkerControlOperationRecord) { r.Target.RecordingGenerationID = "other" },
		"epoch":      func(r *recordings.WorkerControlOperationRecord) { r.Target.OwnerEpoch = "other" },
		"scope":      func(r *recordings.WorkerControlOperationRecord) { r.Target.FactorySessionID = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := intent
			change(&changed)
			if _, _, err := writer.BeginWorkerControlOperation(t.Context(), changed); !errors.Is(err, recordings.ErrWorkerControlConflict) {
				t.Fatalf("changed replay: %v", err)
			}
			changed.Operation.RequestID = "new-" + name
			if name == "generation" || name == "epoch" || name == "scope" {
				if _, _, err := writer.BeginWorkerControlOperation(t.Context(), changed); !errors.Is(err, recordings.ErrWorkerControlConflict) {
					t.Fatalf("stale target accepted: %v", err)
				}
			}
		})
	}
	other := controlIntent(t, writer, "other-recording", "other-worker", "request")
	reopened, err := newTestFileWriter(platformreplay.NewLocal(runtime.GOOS), writer.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, store := range []recordings.WorkerControlOperationStore{writer, reopened} {
		if _, _, err := store.BeginWorkerControlOperation(t.Context(), other); !errors.Is(err, recordings.ErrWorkerControlConflict) {
			t.Fatalf("cross-source key: %v", err)
		}
	}
	key := operationKey(intent)
	key.FactorySessionID = "other"
	if _, err := writer.LoadWorkerControlOperation(t.Context(), key); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("foreign scope load: %v", err)
	}
}

func TestControlOperationCASHasOneWinnerAndAbsorbingResult(t *testing.T) {
	t.Parallel()
	writer := journalWriter(t, platformreplay.NewLocal(runtime.GOOS))
	intent := controlIntent(t, writer, "recording", "worker", "request")
	if _, _, err := writer.BeginWorkerControlOperation(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	next := intent
	next.Revision, next.Operation.Phase = 2, "COMPLETED"
	next.Result = json.RawMessage(`{"outcome":"APPLIED"}`)
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := writer.AdvanceWorkerControlOperation(t.Context(), next, 1); results <- err }()
	}
	wins, conflicts := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			wins++
		} else if errors.Is(err, recordings.ErrWorkerControlConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
	next.Revision, next.Operation.Phase = 3, "FAILED"
	next.FailureCode = "FAILURE"
	if _, err := writer.AdvanceWorkerControlOperation(t.Context(), next, 2); !errors.Is(err, recordings.ErrWorkerControlConflict) {
		t.Fatalf("terminal advanced: %v", err)
	}
}

func TestControlOperationSyncBarrierAndUncertainCommit(t *testing.T) {
	t.Parallel()
	probe := &journalProbe{Local: platformreplay.NewLocal(runtime.GOOS), entered: make(chan struct{}), release: make(chan struct{})}
	writer := journalWriter(t, probe)
	intent := controlIntent(t, writer, "recording", "worker", "request")
	sibling := controlIntent(t, writer, "sibling-recording", "sibling-worker", "sibling-request")
	// Initialize the rebuildable index before holding one recording's append.
	if err := writer.rebuildControlIndex(t.Context()); err != nil {
		t.Fatal(err)
	}
	probe.gatePath = writer.path("recording") + "l"
	t.Cleanup(func() {
		select {
		case <-probe.release:
		default:
			close(probe.release)
		}
	})
	result := make(chan error, 1)
	go func() { _, _, err := writer.BeginWorkerControlOperation(t.Context(), intent); result <- err }()
	<-probe.entered
	select {
	case err := <-result:
		t.Fatalf("accepted before sync: %v", err)
	default:
	}
	if _, created, err := writer.BeginWorkerControlOperation(t.Context(), sibling); err != nil || !created {
		t.Fatalf("sibling blocked: %v", err)
	}
	close(probe.release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	probe.gatePath = ""
	probe.mu.Lock()
	probe.failAfterSync = true
	probe.mu.Unlock()
	next := intent
	next.Revision, next.Operation.Phase = 2, "COMPLETED"
	next.Result = json.RawMessage(`{"outcome":"APPLIED"}`)
	if _, err := writer.AdvanceWorkerControlOperation(t.Context(), next, 1); err == nil {
		t.Fatal("uncertain append falsely acknowledged")
	}
	latest, err := writer.LoadWorkerControlOperation(t.Context(), operationKey(intent))
	if err != nil || !reflect.DeepEqual(latest, next) {
		t.Fatalf("uncertain commit not reconciled: %+v %v", latest, err)
	}
	if _, err := writer.AdvanceWorkerControlOperation(t.Context(), next, 1); !errors.Is(err, recordings.ErrWorkerControlConflict) {
		t.Fatalf("uncertain CAS duplicated: %v", err)
	}
}

func TestControlInputDurableImmutableScopedReference(t *testing.T) {
	t.Parallel()
	probe := &journalProbe{Local: platformreplay.NewLocal(runtime.GOOS)}
	writer := journalWriter(t, probe)
	intent := controlIntent(t, writer, "recording", "worker", "request")
	key := operationKey(intent)
	input := json.RawMessage(` { "message": "preserve this input" } `)
	ref, err := writer.PersistWorkerControlInput(t.Context(), key, input)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(writer.controlInputPath(ref))
	if err != nil {
		t.Fatal(err)
	}
	if replayRef, err := writer.PersistWorkerControlInput(t.Context(), key, input); err != nil || replayRef != ref {
		t.Fatalf("input replay: %s %v", replayRef, err)
	}
	after, err := os.ReadFile(writer.controlInputPath(ref))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("input replay appended duplicate JSON")
	}
	if _, err := writer.PersistWorkerControlInput(t.Context(), key, json.RawMessage(`{"message":"changed"}`)); !errors.Is(err, recordings.ErrWorkerControlConflict) {
		t.Fatalf("input mutation: %v", err)
	}
	reopened, err := newTestFileWriter(probe.Local, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := reopened.ReadWorkerControlInput(t.Context(), key, ref)
	if err != nil || !bytes.Equal(decoded, input) {
		t.Fatalf("input recovery: %s %v", decoded, err)
	}
	for _, foreign := range []string{"../secret", ref + "x"} {
		if _, err := reopened.ReadWorkerControlInput(t.Context(), key, foreign); !errors.Is(err, recordings.ErrInvalidWorkerControlOperation) {
			t.Fatalf("foreign reference: %v", err)
		}
	}
	key.RequestID = "different-request"
	if _, err := reopened.ReadWorkerControlInput(t.Context(), key, ref); !errors.Is(err, recordings.ErrInvalidWorkerControlOperation) {
		t.Fatalf("cross-request input: %v", err)
	}
	key.RequestID = "failed-input"
	probe.mu.Lock()
	probe.failAfterSync = true
	probe.mu.Unlock()
	if _, err := writer.PersistWorkerControlInput(t.Context(), key, input); err == nil {
		t.Fatal("input sync failure falsely acknowledged")
	}
}

func TestControlInputRefusesCorruptBlobAndSymlink(t *testing.T) {
	t.Parallel()
	writer := journalWriter(t, platformreplay.NewLocal(runtime.GOOS))
	intent := controlIntent(t, writer, "recording", "worker", "request")
	key := operationKey(intent)
	ref, err := writer.PersistWorkerControlInput(t.Context(), key, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(writer.controlInputPath(ref), []byte(`{"torn":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ReadWorkerControlInput(t.Context(), key, ref); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		t.Fatalf("corrupt input: %v", err)
	}
	if _, err := writer.PersistWorkerControlInput(t.Context(), key, json.RawMessage(`{}`)); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		t.Fatalf("corrupt input overwritten: %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	} // Unprivileged Windows does not support creating this fixture.
	if err := os.Remove(writer.controlInputPath(ref)); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "foreign.json")
	if err := os.WriteFile(outside, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, writer.controlInputPath(ref)); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ReadWorkerControlInput(t.Context(), key, ref); !errors.Is(err, recordings.ErrInvalidWorkerControlOperation) {
		t.Fatalf("symlink accepted: %v", err)
	}
}
