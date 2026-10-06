package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestOwnerRecoveryPersistsOneFencedLossWithoutWorkerCallback(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	root := t.TempDir()
	prior := platformprocess.Incarnation{Host: "host", PID: 123, Start: "prior-start"}
	original := ownerRecoveryWriter(t, local, root, "prior-runtime", &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: prior}, live: prior})
	intent := controlIntent(t, original, "recording", "worker-one", "request")
	if _, _, err := original.BeginWorkerControlOperation(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	if err := original.PersistWorkerRecord(t.Context(), journalRecord(t, "recording", "worker-two")); err != nil {
		t.Fatal(err)
	}
	probe := &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: platformprocess.Incarnation{Host: "host", PID: 456, Start: "new-start"}}, lookupErr: platformprocess.ErrProcessGone}
	recovered := ownerRecoveryWriter(t, local, root, "new-runtime", probe)
	before, err := recovered.LoadWorkerRecording(t.Context(), "recording")
	if err != nil || before.Sessions[0].ExecutionTerminal != nil || probe.lookups != 0 {
		t.Fatalf("ordinary read acquired authority: %+v %v", before, err)
	}
	for range 2 {
		if err := recovered.RecoverWorkerOwners(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if probe.lookups != 1 {
		t.Fatalf("queries=%d, want one per prior incarnation", probe.lookups)
	}
	assertOwnerLossPrefix(t, recovered, before)
	data, err := local.ReadFile(recovered.path("recording") + "l")
	if err != nil || bytes.Count(data, []byte(`"kind":"owner-loss"`)) != 2 {
		t.Fatalf("loss facts: %s %v", data, err)
	}
	assertOwnerLossReplayAndFences(t, local, root, probe, recovered, intent, before, data)
}

func assertOwnerLossReplayAndFences(t *testing.T, local platformreplay.Local, root string, probe *ownerLivenessProbe, recovered *FileWriter, intent recordings.WorkerControlOperationRecord, before recordings.WorkerRecordingSnapshot, data []byte) {
	t.Helper()
	if _, created, err := recovered.BeginWorkerControlOperation(t.Context(), intent); err != nil || created {
		t.Fatalf("committed key replay: %v %v", created, err)
	}
	intent.Operation.RequestID = "new-request"
	if _, _, err := recovered.BeginWorkerControlOperation(t.Context(), intent); !errors.Is(err, recordings.ErrWorkerControlConflict) {
		t.Fatalf("new authority after loss: %v", err)
	}
	late := journalRecord(t, "recording", "worker-one")
	late.Record = mustRecord(t, terminalAppend(late.Record.ID.Topic, "worker-one"), 2)
	if err := recovered.PersistWorkerRecord(t.Context(), late); !errors.Is(err, recordings.ErrWorkerRecordingTerminal) {
		t.Fatalf("late callback: %v", err)
	}
	next := ownerRecoveryWriter(t, local, root, "third-runtime", probe)
	if err := next.RecoverWorkerOwners(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertOwnerLossPrefix(t, next, before)
	after, err := local.ReadFile(next.path("recording") + "l")
	if err != nil || !bytes.Equal(data, after) || probe.lookups != 1 {
		t.Fatal("later boot duplicated loss or queried a terminal owner")
	}
}

func ownerRecoveryWriter(t *testing.T, local platformreplay.Local, root, epoch string, probe *ownerLivenessProbe) *FileWriter {
	t.Helper()
	store, err := NewFileWriter(local, local, local, &captureTimeProbe{}, root, epoch, probe)
	if err != nil {
		t.Fatal(err)
	}
	return store.(*FileWriter)
}

func assertOwnerLossPrefix(t *testing.T, writer *FileWriter, before recordings.WorkerRecordingSnapshot) {
	t.Helper()
	after, err := writer.LoadWorkerRecording(t.Context(), before.RecordingID)
	if err != nil || len(after.Sessions) != len(before.Sessions) {
		t.Fatalf("recovery: %+v %v", after, err)
	}
	for index, session := range after.Sessions {
		if session.Status != recordings.WorkerRecordingStatusIncomplete || session.Failure != "OWNER_LOST" || session.ExecutionTerminal == nil ||
			session.ExecutionTerminal.Phase != workers.PhaseFailed || session.ExecutionTerminal.Status != "FAILED" || session.ExecutionTerminal.Position != 0 ||
			!reflect.DeepEqual(session.Records, before.Sessions[index].Records) || session.LastPosition != before.Sessions[index].LastPosition {
			t.Fatalf("owner loss altered worker history: %+v", session)
		}
	}
}

func TestOwnerRecoveryUnknownOrAliveOwnersDoNotProduceLoss(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		host     string
		legacy   bool
		queryErr error
	}{
		{name: "alive", host: "host"}, {name: "remote", host: "remote"},
		{name: "query-failed", host: "host", queryErr: errors.New("denied")}, {name: "legacy", host: "host", legacy: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			local := platformreplay.NewLocal(runtime.GOOS)
			root := t.TempDir()
			prior := platformprocess.Incarnation{Host: test.host, PID: 123, Start: "prior-start"}
			if test.legacy {
				prior = platformprocess.Incarnation{}
			}
			original := ownerRecoveryWriter(t, local, root, "prior-runtime", &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: prior}})
			if err := original.PersistWorkerRecord(t.Context(), journalRecord(t, "recording", "worker")); err != nil {
				t.Fatal(err)
			}
			before, err := local.ReadFile(original.path("recording") + "l")
			if err != nil {
				t.Fatal(err)
			}
			probe := &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: platformprocess.Incarnation{Host: "host", PID: 456, Start: "new-start"}}, live: prior, lookupErr: test.queryErr}
			reopened := ownerRecoveryWriter(t, local, root, "new-runtime", probe)
			if err := reopened.RecoverWorkerOwners(t.Context()); err != nil {
				t.Fatal(err)
			}
			snapshot, err := reopened.LoadWorkerRecording(t.Context(), "recording")
			if err != nil || snapshot.Sessions[0].ExecutionTerminal != nil {
				t.Fatalf("unknown owner classified: %+v %v", snapshot, err)
			}
			after, err := local.ReadFile(original.path("recording") + "l")
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("unknown ownership mutated journal")
			}
		})
	}
}

func TestOwnerRecoveryUnreadableJournalKeepsReadsRecoverableWithoutAuthority(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	root := t.TempDir()
	prior := platformprocess.Incarnation{Host: "host", PID: 123, Start: "prior-start"}
	original := ownerRecoveryWriter(t, local, root, "prior-runtime", &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: prior}})
	if err := original.PersistWorkerRecord(t.Context(), journalRecord(t, "recording", "worker")); err != nil {
		t.Fatal(err)
	}
	before, err := local.ReadFile(original.path("recording") + "l")
	if err != nil {
		t.Fatal(err)
	}
	unavailable := errors.New("unavailable")
	storage := &catalogReadProbe{Local: local, fault: unavailable}
	probe := &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: platformprocess.Incarnation{Host: "host", PID: 456, Start: "new-start"}}, lookupErr: platformprocess.ErrProcessGone}
	store, err := NewFileWriter(storage, local, local, &captureTimeProbe{}, root, "new-runtime", probe)
	if err != nil {
		t.Fatal(err)
	}
	recovered := store.(*FileWriter)
	if err := recovered.RecoverWorkerOwners(t.Context()); err != nil || probe.lookups != 0 {
		t.Fatalf("unavailable journal blocked activation or acquired authority: %v lookups=%d", err, probe.lookups)
	}
	if _, err := recovered.LoadWorkerRecording(t.Context(), "recording"); !errors.Is(err, unavailable) {
		t.Fatalf("unavailable read = %v", err)
	}
	storage.fault = nil
	snapshot, err := recovered.LoadWorkerRecording(t.Context(), "recording")
	if err != nil || len(snapshot.Sessions) != 1 || snapshot.Sessions[0].ExecutionTerminal != nil {
		t.Fatalf("restored prefix = %+v, %v", snapshot, err)
	}
	after, err := local.ReadFile(original.path("recording") + "l")
	if err != nil || !bytes.Equal(before, after) || probe.lookups != 0 {
		t.Fatal("unavailable recovery or ordinary read manufactured a loss fact")
	}
}

func TestOwnerRecoveryReconcilesUncertainSyncedLoss(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	root := t.TempDir()
	prior := platformprocess.Incarnation{Host: "host", PID: 123, Start: "prior-start"}
	original := ownerRecoveryWriter(t, local, root, "prior-runtime", &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: prior}})
	if err := original.PersistWorkerRecord(t.Context(), journalRecord(t, "recording", "worker")); err != nil {
		t.Fatal(err)
	}
	probe := &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: platformprocess.Incarnation{Host: "host", PID: 456, Start: "new-start"}}, lookupErr: platformprocess.ErrProcessGone}
	writer := ownerRecoveryWriter(t, local, root, "new-runtime", probe)
	writer.appender = &journalProbe{Local: local, failAfterSync: true}
	if err := writer.RecoverWorkerOwners(t.Context()); err == nil || writer.ownerRecoveryDone {
		t.Fatal("uncertain sync activated controls")
	}
	if err := writer.RecoverWorkerOwners(t.Context()); err != nil {
		t.Fatal(err)
	}
	data, err := local.ReadFile(writer.path("recording") + "l")
	if err != nil || bytes.Count(data, []byte(`"kind":"owner-loss"`)) != 1 || probe.lookups != 1 {
		t.Fatalf("uncertain retry duplicated loss: %s %v", data, err)
	}
}

type ownerLivenessProbe struct {
	ownerIdentityProbe
	live      platformprocess.Incarnation
	lookupErr error
	lookups   int
}

func (probe *ownerLivenessProbe) LookupProcess(int) (platformprocess.Incarnation, error) {
	probe.lookups++
	return probe.live, probe.lookupErr
}

func TestOwnerDeathWitnessRequiresAffirmativeLocalEvidence(t *testing.T) {
	t.Parallel()
	prior := platformprocess.Incarnation{Host: "host", PID: 123, Start: "prior-start"}
	encoded, err := json.Marshal(captureOwnerIncarnation{Version: 1, RuntimeInstanceID: "prior-runtime", Process: prior})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		local   platformprocess.Incarnation
		live    platformprocess.Incarnation
		err     error
		epoch   string
		dead    bool
		queries int
	}{
		{name: "alive", local: prior, live: prior, queries: 1},
		{name: "absent", local: prior, err: platformprocess.ErrProcessGone, dead: true, queries: 1},
		{name: "reused-pid", local: prior, live: platformprocess.Incarnation{Host: "host", PID: 123, Start: "new-start"}, dead: true, queries: 1},
		{name: "query-failure", local: prior, err: errors.New("denied"), queries: 1},
		{name: "remote", local: platformprocess.Incarnation{Host: "other-host"}},
		{name: "legacy", local: prior, epoch: "opaque-runtime"},
		{name: "invalid-version", local: prior, epoch: `{"version":2,"runtimeInstanceId":"r","process":{"host":"host","pid":123,"start":"s"}}`},
		{name: "duplicate-pid", local: prior, epoch: `{"version":1,"runtimeInstanceId":"r","process":{"host":"host","pid":456,"pid":123,"start":"s"}}`},
		{name: "pid-alias", local: prior, epoch: `{"version":1,"runtimeInstanceId":"r","process":{"host":"host","PID":123,"start":"s"}}`},
		{name: "incomplete-local-identity"},
		{name: "incomplete-query-result", local: prior, live: platformprocess.Incarnation{Host: "host", PID: 123}, queries: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			probe := &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: test.local}, live: test.live, lookupErr: test.err}
			writer := &FileWriter{ownerProbe: probe}
			epoch := test.epoch
			if epoch == "" {
				epoch = string(encoded)
			}
			if got := writer.ownerDeathWitness(epoch); got != test.dead || probe.lookups != test.queries {
				t.Fatalf("dead=%v queries=%d, want %v/%d", got, probe.lookups, test.dead, test.queries)
			}
		})
	}
}

func TestControlIntentForDeadOwnerRefusesBeforeJournalMutation(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	prior := platformprocess.Incarnation{Host: "host", PID: 123, Start: "prior-start"}
	store, err := NewFileWriter(local, local, local, &captureTimeProbe{}, t.TempDir(), "prior-runtime", &ownerIdentityProbe{identity: prior})
	if err != nil {
		t.Fatal(err)
	}
	writer := store.(*FileWriter)
	intent := controlIntent(t, writer, "recording", "worker", "request")
	probe := &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: platformprocess.Incarnation{Host: "host", PID: 456, Start: "new-start"}}, lookupErr: platformprocess.ErrProcessGone}
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "new-runtime", probe)
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := reopened.BeginWorkerControlOperation(t.Context(), intent); !errors.Is(err, recordings.ErrWorkerControlConflict) || created {
		t.Fatalf("dead-owner intent created=%v error=%v", created, err)
	}
	rows, err := reopened.ListWorkerControlOperations(t.Context(), intent.Target)
	if err != nil || len(rows) != 0 {
		t.Fatalf("refused intent left journal facts: %v %v", rows, err)
	}
	if probe.lookups != 1 {
		t.Fatalf("death queries=%d", probe.lookups)
	}
	// A previously committed key remains historical evidence even when its
	// owner is dead; replay does not mint a new intent or acquire authority.
	if _, created, err := writer.BeginWorkerControlOperation(t.Context(), intent); err != nil || !created {
		t.Fatalf("original owner acceptance created=%v error=%v", created, err)
	}
	replayed, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "third-runtime", probe)
	if err != nil {
		t.Fatal(err)
	}
	if result, created, err := replayed.BeginWorkerControlOperation(t.Context(), intent); err != nil || created || !result.SameIntent(intent) {
		t.Fatalf("historical replay created=%v error=%v result=%+v", created, err, result)
	}
	if probe.lookups != 1 {
		t.Fatal("historical key replay acquired a new death witness")
	}
}

type ownerIdentityProbe struct {
	identity platformprocess.Incarnation
	err      error
	calls    int
}

func (probe *ownerIdentityProbe) CurrentProcess() (platformprocess.Incarnation, error) {
	probe.calls++
	return probe.identity, probe.err
}

func TestFileWriterPersistsExactOwnerIncarnationAcrossReconstruction(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	root := t.TempDir()
	identity := platformprocess.Incarnation{Host: "local-host", PID: 123, Start: "creation-token"}
	probe := &ownerIdentityProbe{identity: identity}
	store, err := NewFileWriter(local, local, local, &captureTimeProbe{}, root, "runtime-one", probe)
	if err != nil {
		t.Fatal(err)
	}
	if probe.calls != 0 {
		t.Fatal("inert construction queried the OS")
	}
	for _, worker := range []string{"worker-one", "worker-two"} {
		if err := store.PersistWorkerRecord(t.Context(), journalRecord(t, "capture", worker)); err != nil {
			t.Fatal(err)
		}
	}
	if probe.calls != 1 {
		t.Fatalf("OS identity queries = %d, want one per writer", probe.calls)
	}
	reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, root, "runtime-two", &ownerIdentityProbe{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.LoadWorkerRecording(t.Context(), "capture")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Sessions) != 2 {
		t.Fatalf("sessions = %d", len(snapshot.Sessions))
	}
	for _, session := range snapshot.Sessions {
		var owner captureOwnerIncarnation
		if err := json.Unmarshal([]byte(session.OwnerEpoch), &owner); err != nil {
			t.Fatal(err)
		}
		if owner.Version != 1 || owner.RuntimeInstanceID != "runtime-one" || owner.Process != identity {
			t.Fatalf("recovered owner differs from admitted owner: %+v", owner)
		}
		if session.ExecutionTerminal != nil {
			t.Fatal("incarnation identity alone fabricated a terminal")
		}
	}
}

func TestFileWriterUnknownOwnerIdentityRetainsOpaqueEpoch(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		identity platformprocess.Incarnation
		err      error
	}{
		{name: "query-failure", err: errors.New("OS access denied")},
		{name: "missing-host", identity: platformprocess.Incarnation{PID: 123, Start: "start"}},
		{name: "missing-pid", identity: platformprocess.Incarnation{Host: "host", Start: "start"}},
		{name: "missing-start", identity: platformprocess.Incarnation{Host: "host", PID: 123}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			local := platformreplay.NewLocal(runtime.GOOS)
			probe := &ownerIdentityProbe{identity: test.identity, err: test.err}
			store, err := NewFileWriter(local, local, local, &captureTimeProbe{}, t.TempDir(), "unknown-runtime", probe)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PersistWorkerRecord(t.Context(), journalRecord(t, "capture", "worker")); err != nil {
				t.Fatal(err)
			}
			snapshot, err := store.LoadWorkerRecording(t.Context(), "capture")
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Sessions[0].OwnerEpoch != "unknown-runtime" || snapshot.Sessions[0].ExecutionTerminal != nil {
				t.Fatal("unknown process identity acquired loss authority")
			}
		})
	}
}
