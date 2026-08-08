package guards_batch

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestAllChildrenCompleteWaitsForLastTerminalChild(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, allChildrenCompleteFactoryConfig())
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig("codex", "test-model"))
	support.WriteAgentConfig(t, dir, "completer", support.BuildModelWorkerConfig("codex", "test-model"))
	support.WriteWorkstationConfig(t, dir, "process-child", "---\ntype: MODEL_WORKSTATION\n---\nProcess the child.\n")
	support.WriteWorkstationConfig(t, dir, "complete-parent", "---\ntype: MODEL_WORKSTATION\n---\nComplete the parent.\n")
	testutil.WriteSeedBatchFile(t, dir, work.WorkRequest{
		RequestID: "all-children-complete-regression",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "parent", WorkID: "parent-1", WorkTypeID: "parent", State: "waiting"},
			{Name: "early-child", WorkID: "child-1", WorkTypeID: "child", State: "complete"},
			{Name: "slow-child", WorkID: "child-2", WorkTypeID: "child", State: "processing"},
		},
		Relations: []work.WorkRelation{
			{Type: work.WorkRelationParentChild, SourceWorkName: "early-child", TargetWorkName: "parent"},
			{Type: work.WorkRelationParentChild, SourceWorkName: "slow-child", TargetWorkName: "parent"},
		},
	})

	provider := newFanInGateProvider()
	type runResult struct {
		session factoryapi.FactorySession
		work    factoryapi.ListWorkResponse
		events  []factoryapi.FactoryEvent
	}
	done := make(chan runResult, 1)
	go func() {
		session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
			t,
			dir,
			serviceedges.Edges{ProviderOverride: provider},
			10*time.Second,
		)
		done <- runResult{session: session, work: listed, events: events}
	}()

	// The channel is the deterministic synchronization point; the timeout only
	// turns a missing dispatch into a bounded test failure instead of a hang.
	select {
	case <-provider.slowChildStarted:
	case <-time.After(10 * time.Second):
		provider.releaseSlowChild()
		result := <-done
		t.Fatalf("timed out waiting for controlled child dispatch: calls=%#v session=%#v work=%#v events=%d", provider.callsSnapshot(), result.session, result.work, len(result.events))
	}
	if got := provider.callCount("completer"); got != 0 {
		t.Errorf("fan-in dispatched before the slow child was released: completer calls=%d", got)
	}
	provider.releaseSlowChild()

	result := <-done
	assertGuardSessionPlaces(t, result.work, map[string]int{
		"parent:complete":  1,
		"parent:waiting":   0,
		"child:complete":   2,
		"child:processing": 0,
	})
	if got := provider.callCount("completer"); got != 1 {
		t.Fatalf("completer calls = %d, want exactly one", got)
	}

	observations := support.ObserveDispatchEvents(t, result.events)
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
		t.Fatalf("public dispatch events missing slow child or parent fan-in: %#v", observations)
	}
	if completerIndex <= processorIndex {
		t.Fatalf("public fan-in dispatch index = %d, slow child dispatch index = %d; fan-in fired too early", completerIndex, processorIndex)
	}
	if result.session.Id == "" {
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
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
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

type fanInGateProvider struct {
	slowChildStarted chan struct{}
	release          chan struct{}
	startOnce        sync.Once
	releaseOnce      sync.Once

	mu    sync.Mutex
	calls map[string]int
}

func newFanInGateProvider() *fanInGateProvider {
	return &fanInGateProvider{
		slowChildStarted: make(chan struct{}),
		release:          make(chan struct{}),
		calls:            make(map[string]int),
	}
}

func (p *fanInGateProvider) Infer(ctx context.Context, request workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	workerType := request.WorkerType
	if workerType == "" {
		workerType = request.Dispatch.WorkerType
	}
	p.mu.Lock()
	p.calls[workerType]++
	p.mu.Unlock()

	if workerType == "processor" {
		p.startOnce.Do(func() { close(p.slowChildStarted) })
		select {
		case <-p.release:
		case <-ctx.Done():
			return workerexecution.InferenceResponse{}, ctx.Err()
		}
	}
	return workerexecution.InferenceResponse{Content: "COMPLETE"}, nil
}

func (p *fanInGateProvider) releaseSlowChild() {
	p.releaseOnce.Do(func() { close(p.release) })
}

func (p *fanInGateProvider) callCount(workerType string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[workerType]
}

func (p *fanInGateProvider) callsSnapshot() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	copy := make(map[string]int, len(p.calls))
	for workerType, count := range p.calls {
		copy[workerType] = count
	}
	return copy
}

var _ workerexecution.Provider = (*fanInGateProvider)(nil)
