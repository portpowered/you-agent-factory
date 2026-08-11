package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	runner := newLiveCapacityBarrierRunner()
	dir := scaffoldLiveCapacityFactory(t, 1)
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

	close(runner.releaseFirst)
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
	mu           sync.Mutex
	calls        int
	started      chan int
	releaseFirst chan struct{}
}

func newLiveCapacityBarrierRunner() *liveCapacityBarrierRunner {
	return &liveCapacityBarrierRunner{
		started:      make(chan int, 16),
		releaseFirst: make(chan struct{}),
	}
}

func (r *liveCapacityBarrierRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	r.started <- call
	if call == 1 {
		select {
		case <-r.releaseFirst:
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
