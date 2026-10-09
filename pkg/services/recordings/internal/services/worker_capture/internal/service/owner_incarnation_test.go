package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"runtime"
	"sort"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestOwnerRecoveryCatalogReusesCommittedPrefixes(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"empty", "ended", "owner-lost", "alive"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			local := platformreplay.NewLocal(runtime.GOOS)
			root := t.TempDir()
			prior := platformprocess.Incarnation{Host: "host", PID: 123, Start: "prior-start"}
			original := ownerRecoveryWriter(t, local, root, "prior-runtime", &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: prior}})
			if state != "empty" {
				opening := journalRecord(t, "recording", "worker")
				if err := original.PersistWorkerRecord(t.Context(), opening); err != nil {
					t.Fatal(err)
				}
				if state == "ended" {
					opening.Record = mustRecord(t, terminalAppend(opening.Record.ID.Topic, "worker"), 2)
					if err := original.PersistWorkerRecord(t.Context(), opening); err != nil {
						t.Fatal(err)
					}
				}
			}
			storage := &catalogReadProbe{Local: local}
			scan := &catalogScanProbe{local: local}
			probe := &ownerLivenessProbe{ownerIdentityProbe: ownerIdentityProbe{identity: platformprocess.Incarnation{Host: "host", PID: 456, Start: "new-start"}}, live: prior}
			if state == "owner-lost" {
				probe.lookupErr = platformprocess.ErrProcessGone
			}
			reader, err := NewFileWriter(storage, local, scan, &captureTimeProbe{}, root, "new-runtime", probe)
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.(*FileWriter).RecoverWorkerOwners(t.Context()); err != nil {
				t.Fatal(err)
			}
			// A history read after readiness must need neither another directory
			// scan nor journal IO; failed edges make accidental rebuilds visible.
			storage.fault, scan.fault = errors.New("unexpected journal read"), errors.New("unexpected scan")
			want := 1
			if state == "empty" {
				want = 0
			}
			for range 2 {
				page, err := reader.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
				if err != nil || scan.calls != 1 || len(page.Items) != want {
					t.Fatalf("ready catalog: %+v err=%v scans=%d", page, err, scan.calls)
				}
				if state != "empty" {
					assertRecoveredCatalogState(t, page.Items[0], state)
					page.Items[0].Opening.Payload[0] = '!'
				}
			}
		})
	}
}

// The file owner proves startup IO with a counting storage edge and asserts
// recovered public facts. No application graph or capacity fixture is needed.
func TestOwnerRecoveryReadsEachCaptureSourceOnce(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, format string
		journalFirst bool
	}{
		{name: "journal", format: "journal"},
		{name: "snapshot", format: "snapshot"},
		{name: "snapshot-first", format: "snapshot-and-journal"},
		{name: "journal-first", format: "snapshot-and-journal", journalFirst: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			local := platformreplay.NewLocal(runtime.GOOS)
			original := seedRecoverySource(t, local, fixture.format)
			want, err := original.LoadWorkerRecording(t.Context(), "recording")
			if err != nil {
				t.Fatal(err)
			}
			storage := &recoverySourceReadProbe{Local: local, reads: make(map[string]int), journalFirst: fixture.journalFirst}
			store, err := NewFileWriter(storage, local, storage, &captureTimeProbe{}, original.root, "reopened", nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.(*FileWriter).RecoverWorkerOwners(t.Context()); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{original.path("recording"), original.path("recording") + "l"} {
				_, err := local.ReadFile(path)
				count := 1
				if errors.Is(err, os.ErrNotExist) {
					count = 0
				} else if err != nil {
					t.Fatal(err)
				}
				if storage.reads[path] != count {
					t.Fatalf("successful source reads = %d, want %d for %s", storage.reads[path], count, fixture.name)
				}
			}
			storage.fault = errors.New("read after readiness")
			got, err := store.LoadWorkerRecording(t.Context(), "recording")
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("recovery changed committed history: got=%+v want=%+v err=%v", got, want, err)
			}
			page, err := store.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
			if err != nil || len(page.Items) != 1 || page.Items[0].Terminal == nil || page.Items[0].Terminal.Status != "COMPLETED" {
				t.Fatalf("ready catalog: %+v err=%v", page, err)
			}
		})
	}
}

func seedRecoverySource(t *testing.T, local platformreplay.Local, format string) *FileWriter {
	t.Helper()
	original := journalWriter(t, local)
	opening := journalRecord(t, "recording", "worker")
	terminal := opening
	terminal.Record = mustRecord(t, terminalAppend(opening.Record.ID.Topic, "worker"), 2)
	if format == "journal" {
		persistWorkerRecoveryPrefix(t, original, "recording", "worker", opening.Record, terminal.Record)
		return original
	}
	records := []events.Record{opening.Record}
	if format == "snapshot" {
		records = append(records, terminal.Record)
	}
	data, err := json.Marshal(recordings.WorkerRecordingSnapshot{
		RecordingID: "recording", Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: "worker", Topic: opening.Record.ID.Topic, Records: records,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := local.WriteFile(original.path("recording"), data); err != nil {
		t.Fatal(err)
	}
	if format == "snapshot-and-journal" {
		if err := original.PersistWorkerRecord(t.Context(), terminal); err != nil {
			t.Fatal(err)
		}
	}
	return original
}

type recoverySourceReadProbe struct {
	platformreplay.Local
	reads        map[string]int
	fault        error
	journalFirst bool
}

func (probe *recoverySourceReadProbe) ScanDirectory(path string, size int, visit func([]os.DirEntry) error) error {
	return probe.Local.ScanDirectory(path, size, func(files []os.DirEntry) error {
		sort.Slice(files, func(i, j int) bool {
			if probe.journalFirst {
				return files[i].Name() > files[j].Name()
			}
			return files[i].Name() < files[j].Name()
		})
		return visit(files)
	})
}

func (probe *recoverySourceReadProbe) ReadFile(path string) ([]byte, error) {
	if probe.fault != nil {
		return nil, probe.fault
	}
	data, err := probe.Local.ReadFile(path)
	if err == nil {
		probe.reads[path]++
	}
	return data, err
}

func assertRecoveredCatalogState(t *testing.T, item recordings.WorkerCapturedCatalogItem, state string) {
	t.Helper()
	if item.Catalog.WorkerSessionID != "worker" || item.Opening.Payload[0] != '{' {
		t.Fatalf("identity or detached opening changed: %+v", item)
	}
	switch state {
	case "ended":
		if item.Terminal == nil || item.Terminal.Status != "COMPLETED" || item.Health != recordings.WorkerRecordingStatusComplete {
			t.Fatalf("ended capture: %+v", item)
		}
	case "owner-lost":
		if item.Terminal == nil || item.Terminal.Status != "FAILED" || item.Terminal.Position != 0 || item.HealthReason != "OWNER_LOST" {
			t.Fatalf("fenced capture: %+v", item)
		}
	case "alive":
		if item.Terminal != nil || item.HealthReason == "OWNER_LOST" {
			t.Fatalf("live prior owner acquired fabricated terminal: %+v", item)
		}
	}
}

func TestOwnerRecoveryCanceledScanLeavesCatalogRetryable(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	original := journalWriter(t, local)
	if err := original.PersistWorkerRecord(t.Context(), journalRecord(t, "recording", "worker")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	scan := &catalogScanProbe{local: local, afterScan: cancel}
	reader, err := NewFileWriter(local, local, scan, &captureTimeProbe{}, original.root, "reopened", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.(*FileWriter).RecoverWorkerOwners(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled activation: %v", err)
	}
	scan.afterScan = nil
	if err := reader.(*FileWriter).RecoverWorkerOwners(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := reader.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
	if err != nil || len(page.Items) != 1 || scan.calls != 2 {
		t.Fatalf("fresh activation/list after cancellation: %+v err=%v scans=%d", page, err, scan.calls)
	}
}

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
	if _, err := recovered.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true}); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		t.Fatalf("unavailable recovery cached empty catalog: %v", err)
	}
	if _, err := recovered.LoadWorkerRecording(t.Context(), "recording"); !errors.Is(err, unavailable) {
		t.Fatalf("unavailable read = %v", err)
	}
	storage.fault = nil
	page, err := recovered.ListWorkerSessionCaptures(t.Context(), recordings.WorkerCapturedCatalogRequest{RequireCompleteMembership: true})
	if err != nil || len(page.Items) != 1 || page.Items[0].Terminal != nil {
		t.Fatalf("restored catalog: %+v %v", page, err)
	}
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
