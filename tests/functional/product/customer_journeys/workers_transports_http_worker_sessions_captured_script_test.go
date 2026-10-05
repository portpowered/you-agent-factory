package customer_journeys_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type capturedScriptGate struct {
	started, failNext, canceled chan struct{}
}

func (gate *capturedScriptGate) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return gate.RunStreaming(ctx, request, nil)
}

func (gate *capturedScriptGate) RunStreaming(ctx context.Context, _ platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	observe(platformprocess.OutputStreamStdout, []byte("committed script output\n"))
	close(gate.started)
	select {
	case <-gate.failNext:
		observe(platformprocess.OutputStreamStderr, []byte("uncaptured script output\n"))
	case <-ctx.Done():
		close(gate.canceled)
		return platformprocess.CommandResult{}, ctx.Err()
	}
	<-ctx.Done()
	close(gate.canceled)
	return platformprocess.CommandResult{}, ctx.Err()
}

// A real Script runner crosses the controlled command edge while a real
// journal accepts the opening and one chunk, then loses its write effect.
// Public CLI/HTTP reads and cancellation observe that same active attempt.
// This fault shape needs its own process; all mutable gates/store/home/session
// identities are local to this parallel scenario, with no executable build.
func TestWorkerSessionCapturedLogsActiveScriptWriteFailure(t *testing.T) {
	t.Parallel()
	fault := &capturedFaultStore{acceptedThrough: 2, failed: make(chan struct{})}
	gate := &capturedScriptGate{started: make(chan struct{}), failNext: make(chan struct{}), canceled: make(chan struct{})}
	dir := support.ScaffoldSingleStepFactory(t, "captured-script-failure")
	if err := os.MkdirAll(filepath.Join(dir, "workers", "processor"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workers", "processor", "AGENTS.md"), []byte("---\ntype: SCRIPT_WORKER\ncommand: captured-script\n---\nExecute the script.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true, Env: []string{"HOME=" + home, "USERPROFILE=" + home},
		Edges: serviceedges.Edges{ScriptCommandRunner: gate, WorkerRecordingWriter: fault, WorkerRecordingStoreObserver: func(store recordings.WorkerRecordingStore) { fault.WorkerRecordingStore = store }, FactorySessionsWorkingDirectory: capturedRecordingDirectory(dir)},
	})
	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	if opened.Session == nil {
		t.Fatal("missing Factory Session")
	}
	name := "captured-script-work"
	submitted := support.SubmitSessionWorkAt(t, server.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{
		Name: &name, WorkTypeName: "task", Payload: map[string]string{"title": "capture script output"},
	})
	select {
	case <-gate.started:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("script did not publish its first output")
	}
	workID := support.StringPointerValue(submitted.WorkId)
	waitForScopedProcessingWork(t, server.URL(), opened.Session.Id, workID)
	list := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, workerSessionsListURL(server.URL(), opened.Session.Id, workID))
	if len(list.Sessions) != 1 {
		t.Fatalf("active Worker Sessions = %d", len(list.Sessions))
	}
	id := list.Sessions[0].WorkerSessionId
	prefix := assertCapturedLogsCLIHTTPParity(t, server, id)
	if prefix.CommittedPosition != 2 || len(prefix.Events) != 2 {
		t.Fatalf("script chunk was not committed: %+v", prefix)
	}
	assertCapturedScriptChunk(t, prefix.Events[1])
	close(gate.failNext)
	select {
	case <-fault.failed:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("active output did not reach write fault")
	}
	assertCapturedActiveFailureAndCancel(t, server, id, prefix, gate)
}

func assertCapturedActiveFailureAndCancel(t *testing.T, server *support.FunctionalAPIServer, id string, prefix factoryapi.WorkerSessionLogPage, gate *capturedScriptGate) {
	t.Helper()
	incomplete := assertCapturedLogsCLIHTTPParity(t, server, id)
	if incomplete.Health != factoryapi.INCOMPLETE || incomplete.CommittedPosition != prefix.CommittedPosition || !reflect.DeepEqual(incomplete.Events, prefix.Events) {
		t.Fatal("failed active write changed the committed prefix")
	}
	active := support.GetJSON[factoryapi.WorkerSessionObservation](t, server.URL()+"/worker-sessions/"+id)
	if active.State != factoryapi.WorkerSessionObservationStateRunning {
		t.Fatalf("capture loss changed execution state: %s", active.State)
	}
	response := postWorkerSessionControl(t, server.URL(), id, "cancel")
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("live cancel after capture loss = %d", response.StatusCode)
	}
	select {
	case <-gate.canceled:
	case <-time.After(functionalWorkerSignalTimeout):
		t.Fatal("live cancellation did not reach the running command")
	}
	// The command's cancellation callback precedes authoritative terminal
	// publication, so observe the resulting public capture health explicitly.
	degraded, err := support.WaitForObservation(functionalWorkerSignalTimeout, func() (factoryapi.WorkerSessionLogPage, error) {
		return support.GetJSON[factoryapi.WorkerSessionLogPage](t, server.URL()+"/worker-sessions/"+id+"/logs"), nil
	}, func(page factoryapi.WorkerSessionLogPage) bool { return page.Health == factoryapi.DEGRADED })
	if err != nil {
		t.Fatal(err)
	}
	if degraded.CommittedPosition != prefix.CommittedPosition || !reflect.DeepEqual(degraded.Events, prefix.Events) || !reflect.DeepEqual(assertCapturedLogsCLIHTTPParity(t, server, id), degraded) {
		t.Fatal("terminal capture loss changed committed activity or CLI/HTTP parity")
	}
}

func assertCapturedScriptChunk(t *testing.T, frame factoryapi.WorkerSessionEvent) {
	t.Helper()
	data, err := json.Marshal(frame.Event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var draft workers.Draft
	if err := json.Unmarshal(data, &draft); err != nil {
		t.Fatal(err)
	}
	var progress workers.ProgressPayload
	if err := json.Unmarshal(draft.Payload, &progress); err != nil {
		t.Fatal(err)
	}
	if draft.Kind != workers.KindProgress || progress.Label != "stdout" || progress.Message != "committed script output\n" || frame.Event.CapturedAt == nil {
		t.Fatalf("captured script chunk lost content/time: %+v %+v", draft, progress)
	}
}
