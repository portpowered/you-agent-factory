package automations

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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
