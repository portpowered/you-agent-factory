package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestSelectedSummaryControlTargetOrderingAndDetachment(t *testing.T) {
	t.Parallel()
	entry := &recordingEntry{}
	target := recordings.WorkerControlTarget{RecordingID: "recording", WorkerSessionID: "worker", FactorySessionID: "factory", RecordingGenerationID: "generation", OwnerEpoch: "epoch", ExpectedAttemptID: "terminal-attempt"}
	want := []recordings.WorkerControlOperationRecord{
		{Target: target, Revision: 2, Operation: recordings.WorkerControlOperation{RequestID: "a", Phase: "SOURCE_STOPPED"}, Result: json.RawMessage(`{"stopped":true}`)},
		{Target: target, Revision: 3, Operation: recordings.WorkerControlOperation{RequestID: "a", Phase: "FAILED"}},
		{Target: target, Revision: 1, Operation: recordings.WorkerControlOperation{RequestID: "b", Phase: "INTENT"}},
	}
	for _, index := range []int{2, 0, 1} {
		entry.acceptControl(want[index])
	}
	for _, field := range []string{"worker", "recording", "factory", "generation", "epoch", "attempt"} {
		other := want[0]
		switch field {
		case "worker":
			other.Target.WorkerSessionID = "foreign"
		case "recording":
			other.Target.RecordingID = "foreign"
		case "factory":
			other.Target.FactorySessionID = "foreign"
		case "generation":
			other.Target.RecordingGenerationID = "foreign"
		case "epoch":
			other.Target.OwnerEpoch = "foreign"
		case "attempt":
			other.Target.ExpectedAttemptID = "foreign"
		}
		entry.acceptControl(other)
	}
	payload, _ := json.Marshal(workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseCanceled, DispatchID: target.ExpectedAttemptID})
	item := recordings.WorkerCapturedCatalogItem{
		Catalog:         recordings.WorkerSessionCatalogEntry{RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID, FactorySessionID: target.FactorySessionID, RecordingGenerationID: target.RecordingGenerationID, OwnerEpoch: target.OwnerEpoch},
		Terminal:        &recordings.WorkerRecordingTerminal{Position: 2},
		MetadataRecords: []events.Record{{ID: events.RecordID{Position: 2}, Payload: payload}},
	}
	for range 2 {
		got := entry.summaryControls(item)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("selected control facts=%+v want=%+v", got, want)
		}
		got[0].Result[0] = '!'
	}
	item.Terminal.Position = 0
	if got := entry.summaryControls(item); len(got) != 0 {
		t.Fatalf("owner-loss terminal selected physical control: %+v", got)
	}
}

func TestSelectedSummaryControlsCommitAndStartup(t *testing.T) {
	t.Parallel()
	probe := &catalogAppendProbe{Local: platformreplay.NewLocal(runtime.GOOS)}
	writer := journalWriter(t, probe)
	intent := controlIntent(t, writer, "recording", "worker", "stop")
	if _, _, err := writer.BeginWorkerControlOperation(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	terminal := journalRecord(t, "recording", "worker")
	request := terminalAppend(terminal.Record.ID.Topic, "worker")
	var draft workers.Draft
	_ = json.Unmarshal(request.Payload, &draft)
	draft.DispatchID = intent.Target.ExpectedAttemptID
	request.Payload, _ = json.Marshal(draft)
	terminal.Record = mustRecord(t, request, 2)
	if err := writer.PersistWorkerRecord(t.Context(), terminal); err != nil {
		t.Fatal(err)
	}
	next := intent
	next.Revision, next.Operation.Phase = 2, "COMPLETED"
	next.Result = json.RawMessage(`{"outcome":"APPLIED"}`)
	assertSelectedControls(t, writer, []recordings.WorkerControlOperationRecord{intent})
	probe.fault = errors.New("append rejected")
	if _, err := writer.AdvanceWorkerControlOperation(t.Context(), next, 1); !errors.Is(err, probe.fault) {
		t.Fatalf("rejected control append=%v", err)
	}
	if got, err := writer.LookupWorkerSessionSummary(t.Context(), "worker"); !errors.Is(err, recordings.ErrWorkerRecordingReplay) || len(got.ControlOperations) != 0 {
		t.Fatalf("uncertain journal advertised controls=%+v err=%v", got.ControlOperations, err)
	}
	probe.fault = nil
	if _, err := writer.AdvanceWorkerControlOperation(t.Context(), next, 1); err != nil {
		t.Fatal(err)
	}
	assertSelectedControls(t, writer, []recordings.WorkerControlOperationRecord{intent, next})
	reopened, err := newTestFileWriter(probe, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	reader := reopened.(*FileWriter)
	if err := reader.RecoverWorkerOwners(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertSelectedControls(t, reader, []recordings.WorkerControlOperationRecord{intent, next})
}

func assertSelectedControls(t *testing.T, reader *FileWriter, want []recordings.WorkerControlOperationRecord) {
	t.Helper()
	got, err := reader.LookupWorkerSessionSummary(t.Context(), "worker")
	if err != nil || !reflect.DeepEqual(got.ControlOperations, want) {
		t.Fatalf("committed selected controls=%+v err=%v want=%+v", got.ControlOperations, err, want)
	}
}
