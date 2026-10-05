package service

import (
	"encoding/json"
	"errors"
	"runtime"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

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
