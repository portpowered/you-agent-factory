package service

import (
	"encoding/json"
	"errors"
	"runtime"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
)

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
