package automations

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// API-owned session opening activates each source without selecting ~default.
// Both cells share the root-built host but own their command route, directory,
// session and cleanup. The next command proves the previous cycle completed;
// no internal cursor file or service pointer is used as a public observer.
func TestScriptPollerSessionRecoveryAndEmptyCycle(t *testing.T) {
	t.Parallel()
	recoveryDir := support.ScaffoldFactory(t, scriptPollerFactoryConfig())
	emptyDir := support.ScaffoldFactory(t, scriptPollerFactoryConfig())
	support.ClearSeedInputs(t, recoveryDir)
	support.ClearSeedInputs(t, emptyDir)
	cursor, checkpoint := "cursor-雪-\\opaque", "checkpoint-λ-\"quoted\""
	output, err := json.Marshal(map[string]any{
		"request": json.RawMessage(scriptPollerExternalWorkRequestJSON(t)),
		"cursor":  cursor, "checkpoint": checkpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	recovery := newScriptCycleRoute(output)
	empty := newScriptCycleRoute(nil)
	router := scriptCycleRouter{routes: map[string]*scriptCycleRoute{
		filepath.Clean(recoveryDir): recovery,
		filepath.Clean(emptyDir):    empty,
	}}
	hostDir := support.ScaffoldFactory(t, map[string]any{
		"name": "idle-automation-host", "workTypes": []map[string]any{{
			"name": "idle", "states": []map[string]string{
				{"name": "init", "type": "INITIAL"}, {"name": "done", "type": "TERMINAL"},
			},
		}},
	})
	support.ClearSeedInputs(t, hostDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: hostDir, Edges: serviceedges.Edges{ScriptCommandRunner: router},
		BeforeStart: func(tb testing.TB, process support.Process, input root.Input) {
			support.InitializeCustomerHomeWithProcess(tb, process, input.Env, hostDir)
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	for _, cell := range []struct {
		name  string
		dir   string
		route *scriptCycleRoute
	}{
		{"committed_opaque_facts_resume_with_public_Work", recoveryDir, recovery},
		{"completed_empty_cycle_admits_no_Work", emptyDir, empty},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			sessionID := support.OpenFactorySessionAt(t, server.URL(), cell.dir).Session.Id
			t.Cleanup(func() { support.CloseFactorySessionAt(t, server.URL(), sessionID) })
			if sessionID == "" || sessionID == "~default" {
				t.Fatalf("source session = %q, want explicit identity", sessionID)
			}
			first := awaitScriptCycleCommand(t, cell.route)
			if slices.Contains(first.Env, "INFINITE_YOU_SCRIPT_POLLER_CURSOR="+cursor) {
				t.Fatalf("first command unexpectedly resumed: %#v", first.Env)
			}
			// Release exactly one output, then hold its successor command. Reaching
			// that successor acknowledges parsing, admission and commit/empty handling.
			close(cell.route.release)
			resumed := awaitScriptCycleCommand(t, cell.route)
			listed := support.GetJSON[factoryapi.ListWorkResponse](t,
				support.SessionWorkURL(server.URL(), sessionID, "/work"))
			if cell.route == empty {
				if len(listed.Results) != 0 || len(resumed.Env) != 0 {
					t.Fatalf("empty cycle Work=%#v env=%#v, want no admission/advancement", listed.Results, resumed.Env)
				}
				return
			}
			assertScriptResumeEnvironment(t, resumed, cursor, checkpoint)
			assertScriptIngressWork(t, listed)
		})
	}
}

func assertScriptResumeEnvironment(t *testing.T, request platformprocess.CommandRequest, cursor, checkpoint string) {
	t.Helper()
	for _, expected := range []string{
		"INFINITE_YOU_SCRIPT_POLLER_CURSOR=" + cursor,
		"INFINITE_YOU_SCRIPT_POLLER_CHECKPOINT=" + checkpoint,
	} {
		if !slices.Contains(request.Env, expected) {
			t.Fatalf("resumed source command missing exact %q", expected)
		}
	}
}

func assertScriptIngressWork(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	location := support.WorkCustomerLocation(scriptPollerWorkTypeName, scriptPollerOutputStateName)
	if len(listed.Results) != 1 || !support.HasWorkAtCustomerState(listed, scriptPollerExternalWorkID, location) {
		t.Fatalf("session Work=%#v, want one %q at %q", listed.Results, scriptPollerExternalWorkID, location)
	}
	payload, ok := listed.Results[0].Payload.(map[string]any)
	if !ok || payload["id"] != "ISSUE-101" || payload["title"] != "External ingress item" {
		t.Fatalf("admitted payload=%#v, want preserved external item", listed.Results[0].Payload)
	}
}

type scriptCycleRouter struct{ routes map[string]*scriptCycleRoute }

func (r scriptCycleRouter) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	// A POLLER workstation also executes its SCRIPT_WORKER for admitted Work.
	// Worker attempts have an execution scope and receive Work on stdin; source
	// polling has neither. Keep the real worker path separate from source gates.
	if request.ExecutionScopeID != "" {
		return platformprocess.CommandResult{Stdout: request.Stdin}, nil
	}
	route := r.routes[filepath.Clean(request.WorkDir)]
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("unowned script command directory %q", request.WorkDir)
	}
	return route.Run(ctx, request)
}

type scriptCycleRoute struct {
	mu      sync.Mutex
	calls   int
	output  []byte
	entered chan platformprocess.CommandRequest
	release chan struct{}
}

func newScriptCycleRoute(output []byte) *scriptCycleRoute {
	return &scriptCycleRoute{output: output, entered: make(chan platformprocess.CommandRequest, 4), release: make(chan struct{})}
}

func (r *scriptCycleRoute) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	ordinal := r.calls
	r.mu.Unlock()
	select {
	case r.entered <- request:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	if ordinal == 1 {
		select {
		case <-r.release:
			return platformprocess.CommandResult{Stdout: r.output}, nil
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

func awaitScriptCycleCommand(t *testing.T, route *scriptCycleRoute) platformprocess.CommandRequest {
	t.Helper()
	select {
	case request := <-route.entered:
		return request
	case <-time.After(10 * time.Second):
		t.Fatal("session source did not reach its owned script command edge")
		return platformprocess.CommandRequest{}
	}
}

const (
	scriptPollerWorkTypeName      = "story"
	scriptPollerOutputStateName   = "queued"
	scriptPollerExternalWorkID    = "external-issue-101"
	scriptPollerExternalRequestID = "poller-external-batch-1"
	scriptPollerScriptCommand     = "factory/scripts/poller.sh"
	scriptPollerWorkstationName   = "poll-tasks"
	scriptPollerWorkerName        = "script-poller"
)

// TestBuildProcessRemainsScriptPollerInertBeforeRuntimeLifecycle proves BuildProcess
// does not invoke script poller commands before the runtime lifecycle starts.
func TestBuildProcessRemainsScriptPollerInertBeforeRuntimeLifecycle(t *testing.T) {
	t.Parallel()

	runner := newScriptPollerIngressCommandRunner(t, nil)
	_ = support.BuildProcess(t, serviceedges.Edges{
		ScriptCommandRunner: runner,
	})
	if runner.callCount() != 0 {
		t.Fatalf(
			"BuildProcess() invoked script command runner %d times, want zero before runtime lifecycle",
			runner.callCount(),
		)
	}
}

// TestAutomationsScriptPollerAdmitsWorkThroughRuntimeLifecycle proves script poller
// workstations admit Work through the runtime lifecycle after BuildProcess composition.
func TestAutomationsScriptPollerAdmitsWorkThroughRuntimeLifecycle(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, scriptPollerFactoryConfig())
	support.ClearSeedInputs(t, dir)

	runner := newScriptPollerIngressCommandRunner(t, scriptPollerExternalWorkRequestJSON(t))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	outputLocation := support.WorkCustomerLocation(scriptPollerWorkTypeName, scriptPollerOutputStateName)
	listed := waitForScriptPollerListedWorkAtCustomerState(
		t,
		server.URL(),
		outputLocation,
		1,
		10*time.Second,
	)
	if got := support.CountWorkAtCustomerState(listed, outputLocation); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(%q) = %d, want 1; listed=%#v", outputLocation, got, listed)
	}
	if !support.HasWorkAtCustomerState(listed, scriptPollerExternalWorkID, outputLocation) {
		t.Fatalf(
			"listed work = %#v, want work %q at %q",
			listed.Results,
			scriptPollerExternalWorkID,
			outputLocation,
		)
	}
	if runner.callCount() < 1 {
		t.Fatalf("script poller command calls = %d, want at least one external poll invocation", runner.callCount())
	}
}

func scriptPollerFactoryConfig() map[string]any {
	return map[string]any{
		"name": "poller-external-items",
		"workTypes": []map[string]any{{
			"name": scriptPollerWorkTypeName,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": scriptPollerOutputStateName, "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{
			"name":    scriptPollerWorkerName,
			"type":    "SCRIPT_WORKER",
			"command": scriptPollerScriptCommand,
		}},
		"workstations": []map[string]any{{
			"name":      scriptPollerWorkstationName,
			"behavior":  "POLLER",
			"worker":    scriptPollerWorkerName,
			"inputs":    []map[string]string{{"workType": scriptPollerWorkTypeName, "state": "init"}},
			"outputs":   []map[string]string{{"workType": scriptPollerWorkTypeName, "state": scriptPollerOutputStateName}},
			"onFailure": []map[string]string{{"workType": scriptPollerWorkTypeName, "state": "failed"}},
		}},
	}
}

func scriptPollerExternalWorkRequestJSON(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"requestId": scriptPollerExternalRequestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name":         "external-issue-101",
			"workId":       scriptPollerExternalWorkID,
			"workTypeName": scriptPollerWorkTypeName,
			"payload": map[string]string{
				"id":    "ISSUE-101",
				"title": "External ingress item",
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal script poller work request: %v", err)
	}
	return payload
}

func waitForScriptPollerListedWorkAtCustomerState(
	t *testing.T,
	baseURL string,
	location string,
	wantCount int,
	timeout time.Duration,
) factoryapi.ListWorkResponse {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	var last factoryapi.ListWorkResponse
	for {
		last = support.ListDefaultSessionWork(t, baseURL)
		if support.CountWorkAtCustomerState(last, location) >= wantCount {
			return last
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf(
				"timed out waiting for %d work at %s; listed=%#v",
				wantCount,
				location,
				last.Results,
			)
		}
	}
}

type scriptPollerIngressCommandRunner struct {
	mu       sync.Mutex
	stdout   []byte
	calls    int
	pollDone chan struct{}
}

func newScriptPollerIngressCommandRunner(t *testing.T, stdout []byte) *scriptPollerIngressCommandRunner {
	t.Helper()
	return &scriptPollerIngressCommandRunner{
		stdout:   append([]byte(nil), stdout...),
		pollDone: make(chan struct{}, 1),
	}
}

func (r *scriptPollerIngressCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	callNumber := r.calls
	stdout := append([]byte(nil), r.stdout...)
	r.mu.Unlock()

	r.signalPollCycle()

	if callNumber == 1 {
		return platformprocess.CommandResult{Stdout: stdout}, nil
	}
	return platformprocess.CommandResult{}, nil
}

func (r *scriptPollerIngressCommandRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *scriptPollerIngressCommandRunner) signalPollCycle() {
	select {
	case r.pollDone <- struct{}{}:
	default:
	}
}

var _ platformprocess.CommandRunner = (*scriptPollerIngressCommandRunner)(nil)
