package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const (
	liveCapacityResourceID       = "reviewers"
	liveCapacityResourceName     = "Reviewers"
	liveCapacityWorkType         = "review-task"
	liveCapacityWorker           = "capacity-worker"
	liveCapacityWorkstation      = "review"
	liveCapacityInitialWorkName  = "held-review"
	liveCapacityQueuedWorkName   = "queued-review"
	liveCapacitySecondQueuedName = "second-queued-review"
	liveCapacityRaiseRequestID   = "capacity-raise-functional"
	liveCapacityBarrierCommand   = "functional-capacity-barrier"
	liveCapacityBarrierOutput    = "capacity barrier completed"
	liveCapacityTestTimeout      = 20 * time.Second
)

// TestLiveResourceCapacityIncreaseAdmitsWaitingMockDispatch proves the public
// live-capacity operation changes an already-running mock-worker Factory
// Session. One admitted dispatch stays active at the injected command edge;
// queued Work remains pending at capacity one, then a CLI capacity increase
// wakes another dispatch without replacing the session or interrupting the
// first one.
func TestLiveResourceCapacityIncreaseAdmitsWaitingMockDispatch(t *testing.T) {
	runner := newLiveCapacityBarrierRunner(1)
	dir := scaffoldLiveCapacityFactory(t, 1)
	server := startLiveCapacityServer(t, dir, runner)

	first := submitLiveCapacityWork(t, server.URL(), liveCapacityInitialWorkName)
	if first.WorkId == nil || *first.WorkId == "" {
		t.Fatalf("first submit response = %#v, want work id", first)
	}
	runner.waitForCall(t, 1)

	before := support.GetDefaultSession(t, server.URL())
	if before.Id == "" {
		t.Fatal("default Factory Session has no durable identity")
	}
	submitLiveCapacityWork(t, server.URL(), liveCapacityQueuedWorkName)
	submitLiveCapacityWork(t, server.URL(), liveCapacitySecondQueuedName)

	capacity := runLiveCapacityCLI(t, dir, server.URL(), liveCapacityResourceID, 2, 0, liveCapacityRaiseRequestID)
	if capacity.ResourceId != liveCapacityResourceID || capacity.EffectiveCapacity != 2 ||
		capacity.PreviousCapacity != 1 || capacity.RequestedCapacity != 2 ||
		capacity.InUseCount != 1 || capacity.AvailableCount != 1 ||
		capacity.MinimumCapacity != 1 || capacity.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("APPLIED") ||
		capacity.Revision != 1 || capacity.SessionId != before.Id {
		t.Fatalf("capacity response = %#v, want applied reviewers 1->2 at revision 1", capacity)
	}

	// The second invocation is the observable wake-up edge. It must begin while
	// the first command is still held, proving the live mutation reached the
	// shared admission gate instead of restarting or draining the session.
	runner.waitForCall(t, 2)
	afterRaise := support.GetDefaultSession(t, server.URL())
	if afterRaise.Id != before.Id {
		t.Fatalf("Factory Session id changed from %q to %q after live capacity raise", before.Id, afterRaise.Id)
	}

	close(runner.releaseBlocked)
	support.WaitForTerminalStatus(t, server.URL(), liveCapacityTestTimeout)

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	if len(dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want one admitted dispatch plus two queued dispatches; dispatches=%#v", len(dispatches), dispatches)
	}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("dispatch %q response = %#v, want accepted terminal response", dispatch.DispatchID, dispatch.Response)
		}
	}
	for _, event := range server.GetFactoryEvents(t) {
		if event.Type == factoryapi.FactoryEventTypeDispatchInterrupted {
			t.Fatalf("live capacity raise interrupted a dispatch: %#v", event)
		}
	}

	functionalevidence.Covers(t, "cli/you.session.resource.set")
}

// TestLiveResourceCapacityReductionPreservesActiveWork proves a safe live
// reduction updates the durable resource projection while an admitted mock
// dispatch remains in flight. The effective capacity may equal in-use work,
// but the active dispatch is neither interrupted nor restarted.
func TestLiveResourceCapacityReductionPreservesActiveWork(t *testing.T) {
	runner := newLiveCapacityBarrierRunner(1)
	dir := scaffoldLiveCapacityFactory(t, 3)
	server := startLiveCapacityServer(t, dir, runner)

	submitLiveCapacityWork(t, server.URL(), liveCapacityInitialWorkName)
	runner.waitForCall(t, 1)
	before := support.GetDefaultSession(t, server.URL())

	capacity := setLiveCapacityREST(t, server.URL(), "~default", liveCapacityResourceID, 1, 0, "capacity-lower-safe")
	if capacity.ResourceId != liveCapacityResourceID || capacity.PreviousCapacity != 3 ||
		capacity.RequestedCapacity != 1 || capacity.EffectiveCapacity != 1 ||
		capacity.InUseCount != 1 || capacity.AvailableCount != 0 || capacity.MinimumCapacity != 1 ||
		capacity.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("APPLIED") ||
		capacity.Revision != 1 || capacity.SessionId != before.Id {
		t.Fatalf("safe reduction response = %#v, want applied reviewers 3->1 at revision 1", capacity)
	}

	close(runner.releaseBlocked)
	support.WaitForTerminalStatus(t, server.URL(), liveCapacityTestTimeout)
	after := support.GetDefaultSession(t, server.URL())
	if after.Id != before.Id {
		t.Fatalf("Factory Session id changed from %q to %q after safe live capacity reduction", before.Id, after.Id)
	}
	assertLiveCapacityUsage(t, after, liveCapacityResourceID, 1, 1)
	assertNoLiveCapacityInterruptions(t, server.GetFactoryEvents(t))
}

// TestLiveResourceCapacityRejectsReductionBelowActiveUse proves an unsafe
// reduction is rejected before admission. The rejection emits no live-change
// events, leaves the revision and usage unchanged, and allows the already
// admitted mock dispatches to complete normally.
func TestLiveResourceCapacityRejectsReductionBelowActiveUse(t *testing.T) {
	runner := newLiveCapacityBarrierRunner(2)
	dir := scaffoldLiveCapacityFactory(t, 2)
	server := startLiveCapacityServer(t, dir, runner)

	submitLiveCapacityWork(t, server.URL(), liveCapacityInitialWorkName)
	submitLiveCapacityWork(t, server.URL(), liveCapacityQueuedWorkName)
	runner.waitForCall(t, 2)

	beforeEvents := server.GetFactoryEvents(t)
	before := support.GetDefaultSession(t, server.URL())
	if before.Runtime.Usage.Resources == nil {
		t.Fatal("active session has no resource usage projection")
	}

	errResponse := rejectLiveCapacityREST(t, server.URL(), "~default", liveCapacityResourceID, 1, 0, "capacity-lower-rejected")
	if errResponse.Code != factoryapi.ErrorResponseCodeRESOURCECAPACITYINUSE || errResponse.ResourceCapacity == nil {
		t.Fatalf("reduction rejection = %#v, want RESOURCE_CAPACITY_IN_USE details", errResponse)
	}
	details := errResponse.ResourceCapacity
	if details.ResourceId != liveCapacityResourceID || details.CurrentCapacity != 2 ||
		details.RequestedCapacity != 1 || details.InUseCount != 2 || details.AvailableCount != 0 ||
		details.MinimumCapacity != 2 {
		t.Fatalf("reduction rejection details = %#v, want current/requested/in-use/available/minimum 2/1/2/0/2", details)
	}

	afterRejectEvents := server.GetFactoryEvents(t)
	if len(afterRejectEvents) != len(beforeEvents) {
		t.Fatalf("event count changed from %d to %d for pre-admission rejection", len(beforeEvents), len(afterRejectEvents))
	}
	for index := range beforeEvents {
		if beforeEvents[index].Id != afterRejectEvents[index].Id {
			t.Fatalf("event %d changed across pre-admission rejection: before=%q after=%q", index, beforeEvents[index].Id, afterRejectEvents[index].Id)
		}
	}
	after := support.GetDefaultSession(t, server.URL())
	if after.Id != before.Id {
		t.Fatalf("Factory Session id changed from %q to %q after rejected live capacity reduction", before.Id, after.Id)
	}
	assertLiveCapacityUsage(t, after, liveCapacityResourceID, 2, 0)

	close(runner.releaseBlocked)
	support.WaitForTerminalStatus(t, server.URL(), liveCapacityTestTimeout)
	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want two admitted dispatches", len(dispatches))
	}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("dispatch %q response = %#v, want accepted terminal response", dispatch.DispatchID, dispatch.Response)
		}
	}
	assertNoLiveCapacityInterruptions(t, server.GetFactoryEvents(t))
}

func startLiveCapacityServer(t *testing.T, dir string, runner *liveCapacityBarrierRunner) *support.FunctionalAPIServer {
	t.Helper()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		MockWorkersConfig: &workers.MockWorkersConfig{
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      liveCapacityWorker,
				WorkstationName: liveCapacityWorkstation,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: liveCapacityBarrierCommand,
				},
			}},
		},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: support.NewStaticSuccessCommandRunner(liveCapacityBarrierOutput),
			ScriptCommandRunner:   runner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })
	return server
}

func setLiveCapacityREST(
	t *testing.T,
	serverURL, sessionID, resourceID string,
	capacity, expectedRevision int,
	requestID string,
) factoryapi.FactorySessionResourceCapacityResponse {
	t.Helper()
	status, body := postLiveCapacityREST(t, serverURL, sessionID, resourceID, capacity, expectedRevision, requestID)
	if status != http.StatusOK {
		t.Fatalf("set resource capacity status = %d, want 200\n%s", status, body)
	}
	var response factoryapi.FactorySessionResourceCapacityResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode set resource capacity response: %v\n%s", err, body)
	}
	return response
}

func rejectLiveCapacityREST(
	t *testing.T,
	serverURL, sessionID, resourceID string,
	capacity, expectedRevision int,
	requestID string,
) factoryapi.ErrorResponse {
	t.Helper()
	status, body := postLiveCapacityREST(t, serverURL, sessionID, resourceID, capacity, expectedRevision, requestID)
	if status != http.StatusConflict {
		t.Fatalf("rejected resource capacity status = %d, want 409\n%s", status, body)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode rejected resource capacity response: %v\n%s", err, body)
	}
	return response
}

func postLiveCapacityREST(
	t *testing.T,
	serverURL, sessionID, resourceID string,
	capacity, expectedRevision int,
	requestID string,
) (int, []byte) {
	t.Helper()
	reason := "functional live resource capacity test"
	payload, err := json.Marshal(factoryapi.FactorySessionResourceCapacityRequest{
		Capacity:         capacity,
		ExpectedRevision: expectedRevision,
		Reason:           &reason,
		RequestId:        requestID,
	})
	if err != nil {
		t.Fatalf("marshal resource capacity request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + sessionID + "/resources/" + resourceID + "/capacity"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build resource capacity request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST resource capacity: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read resource capacity response: %v", err)
	}
	return response.StatusCode, body
}

func assertLiveCapacityUsage(t *testing.T, session factoryapi.FactorySession, name string, total, available int) {
	t.Helper()
	for _, usage := range session.Runtime.Usage.Resources {
		if usage.Name == name {
			if usage.Total != total || usage.Available != available {
				t.Fatalf("resource %q usage = %#v, want total=%d available=%d", name, usage, total, available)
			}
			return
		}
	}
	t.Fatalf("session resource usage missing %q: %#v", name, session.Runtime.Usage.Resources)
}

func assertNoLiveCapacityInterruptions(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchInterrupted {
			t.Fatalf("live capacity change interrupted a dispatch: %#v", event)
		}
	}
}

func submitLiveCapacityWork(t *testing.T, serverURL, name string) factoryapi.SubmitWorkResponse {
	t.Helper()
	return support.SubmitDefaultSessionWork(t, serverURL, factoryapi.SubmitWorkRequest{
		Name:         stringPointer(name),
		WorkTypeName: liveCapacityWorkType,
		Payload:      map[string]any{"name": name},
	})
}

func runLiveCapacityCLI(
	t *testing.T,
	factoryDir, serverURL, resourceID string,
	capacity, expectedRevision int,
	requestID string,
) factoryapi.FactorySessionResourceCapacityResponse {
	t.Helper()
	process := support.BuildProcess(t, serviceedges.Edges{})
	if closer, ok := process.(interface{ Close(context.Context) error }); ok {
		t.Cleanup(func() {
			if err := closer.Close(context.Background()); err != nil {
				t.Errorf("close capacity CLI process: %v", err)
			}
		})
	}
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--json",
		"--server", serverURL,
		"session", "resource", "set",
		resourceID, fmt.Sprintf("%d", capacity), "~default",
		"--request-id", requestID,
		"--expected-revision", fmt.Sprintf("%d", expectedRevision),
		"--reason", "raise functional throughput",
	})
	inputs.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("you session resource set: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var response factoryapi.FactorySessionResourceCapacityResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(inputs.Stdout())), &response); err != nil {
		t.Fatalf("decode you session resource set JSON: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	return response
}

func scaffoldLiveCapacityFactory(t *testing.T, capacity int) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "live-capacity-functional",
		"resources": []map[string]any{{
			"id":       liveCapacityResourceID,
			"name":     liveCapacityResourceName,
			"capacity": capacity,
		}},
		"workTypes": []map[string]any{{
			"name": liveCapacityWorkType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": liveCapacityWorker,
		}},
		"workstations": []map[string]any{{
			"name":      liveCapacityWorkstation,
			"type":      "MODEL_WORKSTATION",
			"worker":    liveCapacityWorker,
			"inputs":    []map[string]string{{"workType": liveCapacityWorkType, "state": "init"}},
			"outputs":   []map[string]string{{"workType": liveCapacityWorkType, "state": "complete"}},
			"onFailure": []map[string]string{{"workType": liveCapacityWorkType, "state": "failed"}},
			"resources": []map[string]any{{"name": liveCapacityResourceName, "capacity": 1}},
		}},
	})
	support.WriteAgentConfig(t, dir, liveCapacityWorker, "---\n"+
		"type: SCRIPT_WORKER\n"+
		"command: authored-capacity-command\n"+
		"---\n"+
		"Run the capacity test work.\n")
	return dir
}

type liveCapacityBarrierRunner struct {
	mu             sync.Mutex
	calls          int
	blockedCalls   int
	started        chan int
	releaseBlocked chan struct{}
}

func newLiveCapacityBarrierRunner(blockedCalls int) *liveCapacityBarrierRunner {
	return &liveCapacityBarrierRunner{
		blockedCalls:   blockedCalls,
		started:        make(chan int, 16),
		releaseBlocked: make(chan struct{}),
	}
}

func (r *liveCapacityBarrierRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	r.started <- call
	if call <= r.blockedCalls {
		select {
		case <-r.releaseBlocked:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return platformprocess.CommandResult{Stdout: []byte(liveCapacityBarrierOutput)}, nil
}

func (r *liveCapacityBarrierRunner) waitForCall(t *testing.T, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), liveCapacityTestTimeout)
	defer cancel()
	for {
		select {
		case call := <-r.started:
			if call >= want {
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for command barrier call %d", want)
		}
	}
}

func stringPointer(value string) *string { return &value }

var _ platformprocess.CommandRunner = (*liveCapacityBarrierRunner)(nil)
