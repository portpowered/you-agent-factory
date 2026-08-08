package batch

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestAllChildrenCompleteWaitsForLastTerminalChild(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, allChildrenCompleteFactoryConfig())
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig("codex", "test-model"))
	support.WriteAgentConfig(t, dir, "completer", support.BuildModelWorkerConfig("codex", "test-model"))
	support.WriteAgentConfig(t, dir, "reopener", support.BuildModelWorkerConfig("codex", "test-model"))
	support.WriteAgentConfig(t, dir, "holder", "---\ntype: MODEL_WORKER\nmodel: test-model\nmodelProvider: codex\n---\nHold the parent while the late child is registered.\n")
	support.WriteWorkstationConfig(t, dir, "process-child", "---\ntype: MODEL_WORKSTATION\n---\nProcess the child.\n")
	support.WriteWorkstationConfig(t, dir, "complete-parent", "---\ntype: MODEL_WORKSTATION\n---\nComplete the parent.\n")
	support.WriteWorkstationConfig(t, dir, "reopen-parent", "---\ntype: MODEL_WORKSTATION\n---\nReopen the parent.\n")
	support.WriteWorkstationConfig(t, dir, "a-hold-fan-in", "---\ntype: MODEL_WORKSTATION\n---\nHold the fan-in resource.\n")
	testutil.WriteSeedBatchFile(t, dir, work.WorkRequest{
		RequestID: "all-children-complete-regression",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "parent", WorkID: "parent-1", WorkTypeID: "parent", State: "waiting"},
			{Name: "early-child", WorkID: "child-1", WorkTypeID: "child", State: "complete"},
		},
		Relations: []work.WorkRelation{
			{Type: work.WorkRelationParentChild, SourceWorkName: "early-child", TargetWorkName: "parent"},
		},
	})

	runner := newFanInGateCommandRunner()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	baseURL := server.URL()
	session := support.GetDefaultSession(t, baseURL)

	// The channel is the deterministic synchronization point; the timeout only
	// turns a missing dispatch into a bounded test failure instead of a hang.
	select {
	case <-runner.holdStarted:
	case <-time.After(10 * time.Second):
		runner.releaseHold()
		t.Fatalf("timed out waiting for the resource-holding dispatch: calls=%#v", runner.callsSnapshot())
	}
	preLate := support.ListDefaultSessionWork(t, baseURL)
	assertGuardSessionPlaces(t, preLate, map[string]int{
		"child:complete": 1,
	})
	if got := runner.callCount("completer"); got != 0 {
		t.Errorf("fan-in dispatched before the late child was registered: completer calls=%d", got)
	}
	// Releasing the parent dispatch makes its worker-emitted request enter the
	// canonical runtime stream. That request registers child-2 after child-1
	// was already terminal, then the child must reach terminal before the join.
	runner.releaseHold()
	support.WaitForSessionTerminalStatus(t, baseURL, session.Id, 10*time.Second)

	listed := support.ListDefaultSessionWork(t, baseURL)
	assertGuardSessionPlaces(t, listed, map[string]int{
		"parent:complete":  1,
		"parent:waiting":   0,
		"child:complete":   2,
		"child:processing": 0,
	})
	if got := runner.callCount("completer"); got != 1 {
		t.Fatalf("completer calls = %d, want exactly one", got)
	}

	observations := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	processorIndex := -1
	completerIndex := -1
	for index, observation := range observations {
		if observation.Request.TransitionId == "process-child" {
			processorIndex = index
		}
		if observation.Request.TransitionId == "complete-parent" {
			completerIndex = index
		}
	}
	if processorIndex < 0 || completerIndex < 0 {
		t.Fatalf("public dispatch events missing late child or parent fan-in: %#v", observations)
	}
	if completerIndex <= processorIndex {
		t.Fatalf("public fan-in dispatch index = %d, slow child dispatch index = %d; fan-in fired too early", completerIndex, processorIndex)
	}
	if session.Id == "" {
		t.Fatal("expected a public Factory Session outcome")
	}
}

func allChildrenCompleteFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{Name: "parent", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "waiting", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			}},
			{Name: "child", States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "processing", Type: interfaces.StateTypeProcessing},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			}},
		},
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "processor"},
			{Name: "completer"},
			{Name: "holder"},
			{Name: "reopener"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "a-hold-fan-in",
				WorkerTypeName: "holder",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "waiting"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "init"}},
			},
			{
				Name:           "reopen-parent",
				WorkerTypeName: "reopener",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "init"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "waiting"}},
			},
			{
				Name:           "process-child",
				WorkerTypeName: "processor",
				Inputs:         []interfaces.IOConfig{{WorkTypeName: "child", StateName: "processing"}},
				Outputs:        []interfaces.IOConfig{{WorkTypeName: "child", StateName: "complete"}},
			},
			{
				Name: "complete-parent", WorkerTypeName: "completer",
				Inputs: []interfaces.IOConfig{
					{WorkTypeName: "parent", StateName: "waiting"},
					{
						WorkTypeName: "child", StateName: "complete",
						Guard: &interfaces.InputGuardConfig{
							Type:        interfaces.GuardTypeAllChildrenComplete,
							ParentInput: "parent",
						},
					},
				},
				Outputs: []interfaces.IOConfig{{WorkTypeName: "parent", StateName: "complete"}},
			},
		},
	}
}

// fanInGateCommandRunner exercises the production ProviderCommandRunner edge.
// It records the low-level provider requests and gates only the resource-holder
// workstation request, allowing the test to observe an attempted parent
// dispatch without replacing the provider service with an in-process fake.
type fanInGateCommandRunner struct {
	holdStarted chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once

	mu    sync.Mutex
	calls map[string]int
}

func newFanInGateCommandRunner() *fanInGateCommandRunner {
	return &fanInGateCommandRunner{
		holdStarted: make(chan struct{}),
		release:     make(chan struct{}),
		calls:       make(map[string]int),
	}
}

func (r *fanInGateCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	workerType := commandWorkerType(request)
	r.mu.Lock()
	r.calls[workerType]++
	r.mu.Unlock()

	if workerType == "holder" {
		r.startOnce.Do(func() { close(r.holdStarted) })
		select {
		case <-r.release:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(`{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"late-child","workId":"child-2","workTypeName":"child","state":"processing"}]}}`)}, nil
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("COMPLETE")}, nil
}

func commandWorkerType(request platformprocess.CommandRequest) string {
	if strings.Contains(string(request.Stdin), "Process the child.") {
		return "processor"
	}
	if strings.Contains(string(request.Stdin), "Complete the parent.") {
		return "completer"
	}
	if strings.Contains(string(request.Stdin), "Reopen the parent.") {
		return "reopener"
	}
	if strings.Contains(string(request.Stdin), "Hold the fan-in resource.") {
		return "holder"
	}
	return "unknown"
}

func assertGuardSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func (r *fanInGateCommandRunner) releaseHold() {
	r.releaseOnce.Do(func() { close(r.release) })
}

func (r *fanInGateCommandRunner) callCount(workerType string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[workerType]
}

func (r *fanInGateCommandRunner) callsSnapshot() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := make(map[string]int, len(r.calls))
	for workerType, count := range r.calls {
		copy[workerType] = count
	}
	return copy
}

var _ platformprocess.CommandRunner = (*fanInGateCommandRunner)(nil)
