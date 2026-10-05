package internal

import (
	"context"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
	instancehostwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/wire"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type stubAssemblyWorkerSessions struct {
	workersessions.Service
	factoryruntime.WorkerAttemptOpener
}

type stubWorkersService struct{ workers.Service }

type assemblyWorldStateOpening struct {
	recordings.RuntimeScopeService
	state   interfaces.FactoryWorldState
	tick    int
	events  []interfaces.FactoryEvent
	request recordings.RuntimeScopeRequest
}

func (opening *assemblyWorldStateOpening) OpenRuntime(
	_ context.Context,
	request recordings.RuntimeScopeRequest,
) (recordings.RuntimeScopeResult, error) {
	opening.request = request
	return recordings.RuntimeScopeResult{}, nil
}

func (opening *assemblyWorldStateOpening) ReconstructCanonicalFactoryWorldState(
	events []interfaces.FactoryEvent,
	selectedTick int,
) (interfaces.FactoryWorldState, error) {
	opening.events = events
	opening.tick = selectedTick
	return opening.state, nil
}

func TestRuntimeOpeningWithFlushOnlySeedsResumeCanonicalEvents(t *testing.T) {
	resumeEvents := []interfaces.FactoryEvent{{Id: "resume-event"}}
	for name, events := range map[string][]interfaces.FactoryEvent{
		"ordinary replay": nil,
		"process resume":  resumeEvents,
	} {
		t.Run(name, func(t *testing.T) {
			opening := &assemblyWorldStateOpening{}
			wrapped := runtimeScopeWithFlush{
				RuntimeScopeService:   opening,
				flushInterval:         time.Second,
				resumeCanonicalEvents: events,
			}
			if _, err := wrapped.OpenRuntime(context.Background(), recordings.RuntimeScopeRequest{}); err != nil {
				t.Fatalf("OpenRuntime: %v", err)
			}
			if opening.request.FlushInterval != time.Second {
				t.Fatalf("flush interval = %v, want 1s", opening.request.FlushInterval)
			}
			if len(opening.request.ReplayEvents) != len(events) {
				t.Fatalf("seeded replay events = %d, want %d", len(opening.request.ReplayEvents), len(events))
			}
			if len(events) > 0 && opening.request.ReplayEvents[0].Id != events[0].Id {
				t.Fatalf("seeded event ID = %q, want %q", opening.request.ReplayEvents[0].Id, events[0].Id)
			}
		})
	}
}

func TestReconstructRestoredWorldStateUsesLatestReplayTick(t *testing.T) {
	events := []interfaces.FactoryEvent{
		{Context: interfaces.FactoryEventContext{Tick: 2}},
		{Context: interfaces.FactoryEventContext{Tick: 7}},
		{Context: interfaces.FactoryEventContext{Tick: 4}},
	}
	opening := &assemblyWorldStateOpening{state: interfaces.FactoryWorldState{Tick: 7}}

	state, err := reconstructRestoredWorldState(opening, events)
	if err != nil {
		t.Fatalf("reconstructRestoredWorldState: %v", err)
	}
	if state == nil || state.Tick != 7 {
		t.Fatalf("restored state = %#v, want tick 7", state)
	}
	if opening.tick != 7 {
		t.Fatalf("selected reconstruction tick = %d, want latest replay tick 7", opening.tick)
	}
	if len(opening.events) != len(events) {
		t.Fatalf("reconstruction events = %d, want %d", len(opening.events), len(events))
	}
}

func TestReconstructRestoredWorldStateUsesSuccessorTickAfterDispatchInterruption(t *testing.T) {
	restartSource := "daemon-restart"
	events := []interfaces.FactoryEvent{
		{Context: interfaces.FactoryEventContext{Tick: 5}},
		{Type: interfaces.FactoryEventTypeDispatchInterrupted, Context: interfaces.FactoryEventContext{Tick: 5, Source: &restartSource}},
		{Type: interfaces.FactoryEventTypeDispatchRequest, Context: interfaces.FactoryEventContext{Tick: 1}},
		{Type: interfaces.FactoryEventTypeRunResponse, Context: interfaces.FactoryEventContext{Tick: 4}},
	}
	opening := &assemblyWorldStateOpening{state: interfaces.FactoryWorldState{Tick: 9}}

	state, err := reconstructRestoredWorldStateForResume(opening, events)
	if err != nil {
		t.Fatalf("reconstructRestoredWorldState: %v", err)
	}
	if state == nil || state.Tick != 9 {
		t.Fatalf("restored state = %#v, want normalized successor tick 9", state)
	}
	if opening.tick != 9 {
		t.Fatalf("selected reconstruction tick = %d, want normalized successor tick 9", opening.tick)
	}
	wantTicks := []int{5, 5, 6, 9}
	for index, want := range wantTicks {
		if opening.events[index].Context.Tick != want {
			t.Fatalf("reconstruction event %d tick = %d, want %d", index, opening.events[index].Context.Tick, want)
		}
		if events[index].Context.Tick != []int{5, 5, 1, 4}[index] {
			t.Fatalf("source event %d was mutated to tick %d", index, events[index].Context.Tick)
		}
	}
}

func TestReconstructRestoredWorldStateNormalizesReplaySuccessorGeneration(t *testing.T) {
	restartSource := "daemon-restart"
	events := []interfaces.FactoryEvent{
		{Id: "predecessor", Context: interfaces.FactoryEventContext{Tick: 10, Sequence: 1}},
		{Id: "restart", Type: interfaces.FactoryEventTypeDispatchInterrupted, Context: interfaces.FactoryEventContext{Tick: 12, Sequence: 2, Source: &restartSource}},
		{Id: "successor", Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 3}},
		{Id: "successor-result", Context: interfaces.FactoryEventContext{Tick: 4, Sequence: 4}},
	}
	opening := &assemblyWorldStateOpening{state: interfaces.FactoryWorldState{Tick: 16}}

	if _, err := reconstructRestoredWorldState(opening, events); err != nil {
		t.Fatalf("reconstructRestoredWorldState: %v", err)
	}
	wantTicks := []int{10, 12, 13, 16}
	if opening.tick != 16 {
		t.Fatalf("selected replay reconstruction tick = %d, want 16", opening.tick)
	}
	for index, want := range wantTicks {
		if opening.events[index].Context.Tick != want {
			t.Fatalf("replay event %d tick = %d, want %d", index, opening.events[index].Context.Tick, want)
		}
	}
	if events[2].Context.Tick != 1 {
		t.Fatalf("replay source event was mutated to tick %d", events[2].Context.Tick)
	}
}

func TestNormalizeRestoredEventTicksPreservesMultipleCanonicalRestartGenerations(t *testing.T) {
	restartSource := "daemon-restart"
	events := []interfaces.FactoryEvent{
		{Id: "first", Context: interfaces.FactoryEventContext{Tick: 10, Sequence: 1}},
		{Id: "first-stop", Type: interfaces.FactoryEventTypeDispatchInterrupted, Context: interfaces.FactoryEventContext{Tick: 12, Sequence: 2, Source: &restartSource}},
		{Id: "second", Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 3}},
		{Id: "second-stop", Type: interfaces.FactoryEventTypeDispatchInterrupted, Context: interfaces.FactoryEventContext{Tick: 4, Sequence: 4, Source: &restartSource}},
		{Id: "third", Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 5}},
	}
	normalized := normalizeRestoredEventTicks(events)
	wantTicks := []int{10, 12, 13, 16, 17}
	for index, want := range wantTicks {
		if normalized[index].Context.Tick != want || normalized[index].Id != events[index].Id ||
			normalized[index].Context.Sequence != events[index].Context.Sequence {
			t.Fatalf("normalized event %d = %#v, want tick %d with identity and sequence preserved", index, normalized[index], want)
		}
	}
	if events[2].Context.Tick != 1 || events[4].Context.Tick != 1 {
		t.Fatalf("source events were mutated: %#v", events)
	}
}

func TestNormalizeRestoredEventTicksDoesNotTreatOrdinaryInterruptionAsRestartBoundary(t *testing.T) {
	operatorSource := "operator"
	events := []interfaces.FactoryEvent{
		{Id: "running", Context: interfaces.FactoryEventContext{Tick: 12}},
		{Id: "stopped", Type: interfaces.FactoryEventTypeDispatchInterrupted, Context: interfaces.FactoryEventContext{Tick: 12, Source: &operatorSource}},
		{Id: "late-response", Context: interfaces.FactoryEventContext{Tick: 4}},
	}
	normalized := normalizeRestoredEventTicks(events)
	if &normalized[0] != &events[0] || normalized[2].Context.Tick != 4 {
		t.Fatalf("ordinary interruption changed historical tick ordering: %#v", normalized)
	}
}

func TestNormalizeRestoredEventTicksPreservesSuccessorAppendOrderAcrossAsyncResponses(t *testing.T) {
	restartSource := "daemon-restart"
	events := []interfaces.FactoryEvent{
		{Id: "predecessor", Context: interfaces.FactoryEventContext{Tick: 68, Sequence: 1}},
		{Id: "restart", Type: interfaces.FactoryEventTypeDispatchInterrupted, Context: interfaces.FactoryEventContext{Tick: 68, Sequence: 2, Source: &restartSource}},
		{Id: "successor-dispatch", Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 3}},
		{Id: "later-dispatch", Context: interfaces.FactoryEventContext{Tick: 8, Sequence: 4}},
		{Id: "async-response", Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 5}},
		{Id: "current-result", Context: interfaces.FactoryEventContext{Tick: 19, Sequence: 6}},
	}
	normalized := normalizeRestoredEventTicks(events)
	wantTicks := []int{68, 68, 69, 76, 76, 87}
	for index, want := range wantTicks {
		if normalized[index].Context.Tick != want {
			t.Fatalf("normalized event %d tick = %d, want %d", index, normalized[index].Context.Tick, want)
		}
		if index > 0 && normalized[index].Context.Tick < normalized[index-1].Context.Tick {
			t.Fatalf("normalized successor ticks decrease at %d: %#v", index, normalized)
		}
	}
}

func TestNormalizeRestoredEventTicksRecognizesQuiescentSessionResumeBoundary(t *testing.T) {
	events := []interfaces.FactoryEvent{
		{Id: "paused", Type: interfaces.FactoryEventTypeSessionPaused, Context: interfaces.FactoryEventContext{Tick: 68, Sequence: 1}},
		{Id: "resumed", Type: interfaces.FactoryEventTypeSessionResumed, Context: interfaces.FactoryEventContext{Tick: 68, Sequence: 2}},
		{Id: "successor-dispatch", Context: interfaces.FactoryEventContext{Tick: 1, Sequence: 3}},
		{Id: "async-response", Context: interfaces.FactoryEventContext{Tick: 0, Sequence: 4}},
	}
	normalized := normalizeRestoredEventTicks(events)
	wantTicks := []int{68, 68, 69, 69}
	for index, want := range wantTicks {
		if normalized[index].Context.Tick != want {
			t.Fatalf("normalized event %d tick = %d, want %d", index, normalized[index].Context.Tick, want)
		}
	}
}

func TestNormalizeRestoredEventTicksLeavesOrdinarySelectedTickHistoryUnchanged(t *testing.T) {
	events := []interfaces.FactoryEvent{
		{Id: "later", Context: interfaces.FactoryEventContext{Tick: 5}},
		{Id: "earlier", Context: interfaces.FactoryEventContext{Tick: 2}},
	}
	if normalized := normalizeRestoredEventTicks(events); &normalized[0] != &events[0] {
		t.Fatal("ordinary historical projection was detached or normalized without a proven restart boundary")
	}
}

func TestNormalizeRestoredEventTicksLeavesEmptyHistoryUnchanged(t *testing.T) {
	if normalized := normalizeRestoredEventTicks(nil); normalized != nil {
		t.Fatalf("normalized empty history = %#v, want nil", normalized)
	}
	if successorRecordingRestartsLogicalClock(nil) {
		t.Fatal("empty history cannot prove a successor logical-clock restart")
	}
}

func TestResumeInputSelectsRecordedEventsForRestoredWorldState(t *testing.T) {
	resumeEvents := []interfaces.FactoryEvent{
		{Id: "resume-event", Context: interfaces.FactoryEventContext{Tick: 9}},
	}
	resumeInput := &recordings.LoadResumeInputResult{
		Input: recordings.LoadReplayInputResult{
			Legacy: &recordings.ReplayArtifact{Events: resumeEvents},
		},
	}

	selected, err := restoredEventsForOpening(nil, resumeInput)
	if err != nil {
		t.Fatalf("restoredEventsForOpening(resume) error = %v", err)
	}
	opening := &assemblyWorldStateOpening{state: interfaces.FactoryWorldState{Tick: 9}}
	state, err := reconstructRestoredWorldStateForResume(opening, selected)
	if err != nil {
		t.Fatalf("reconstructRestoredWorldState(resume) error = %v", err)
	}
	if state == nil || state.Tick != 9 {
		t.Fatalf("resumed state = %#v, want selected tick 9", state)
	}
	if len(opening.events) != 1 || opening.events[0].Id != "resume-event" {
		t.Fatalf("reconstructed resume events = %#v, want selected recording event", opening.events)
	}
}

func TestResumeInputRejectsPortableOrEmptyHistory(t *testing.T) {
	for name, input := range map[string]*recordings.LoadResumeInputResult{
		"portable":     {Input: recordings.LoadReplayInputResult{Portable: &recordings.PortableRecording{}}},
		"empty legacy": {Input: recordings.LoadReplayInputResult{Legacy: &recordings.ReplayArtifact{}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := restoredEventsForOpening(nil, input); err == nil {
				t.Fatal("restoredEventsForOpening() error = nil, want invalid resume history")
			}
		})
	}
}

func TestNewBundleOpeningRequiresWireConstructedRuntimeFactory(t *testing.T) {
	opening, err := NewBundleOpening(nil, nil, &stubAssemblyWorkerSessions{}, &stubAssemblyWorkerSessions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime factory is required") {
		t.Fatalf("NewBundleOpening(nil) error = %v, want required dependency", err)
	}
	if opening != nil {
		t.Fatalf("NewBundleOpening(nil) = %#v, want nil opening", opening)
	}
}

func TestNewBundleOpeningRequiresWorkerSessionsService(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	opening, err := NewBundleOpening(runtimeFactory, stubWorkersService{}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "worker sessions service is required") {
		t.Fatalf("NewBundleOpening(nil worker sessions) error = %v, want required dependency", err)
	}
	if opening != nil {
		t.Fatalf("NewBundleOpening(nil worker sessions) = %#v, want nil opening", opening)
	}
}

func TestNewBundleOpeningRequiresWorkersService(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	opening, err := NewBundleOpening(runtimeFactory, nil, &stubAssemblyWorkerSessions{}, &stubAssemblyWorkerSessions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "Workers service is required") {
		t.Fatalf("NewBundleOpening(nil Workers service) error = %v, want required dependency", err)
	}
	if opening != nil {
		t.Fatalf("NewBundleOpening(nil Workers service) = %#v, want nil opening", opening)
	}
}

func TestNewBundleOpeningBindsRuntimeFactory(t *testing.T) {
	runtimeFactory := &RuntimeFactory{}
	workerService := stubWorkersService{}
	opening, err := NewBundleOpening(runtimeFactory, workerService, &stubAssemblyWorkerSessions{}, &stubAssemblyWorkerSessions{}, nil)
	if err != nil {
		t.Fatalf("NewBundleOpening() error = %v", err)
	}
	if opening == nil || opening.runtimeFactory != runtimeFactory {
		t.Fatalf("NewBundleOpening() = %#v, want supplied Runtime Factory", opening)
	}
	if opening.workerService != workerService {
		t.Fatalf("NewBundleOpening() worker service = %#v, want supplied service", opening.workerService)
	}
}

func TestRuntimeCompositionComposesInertInstanceHost(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	lifecycle, err := newAssemblyTestHost(clock, platformclock.Real{})
	if err != nil {
		t.Fatalf("instancehostwire.New() error = %v", err)
	}
	var _ factoryruntime.RuntimeLifecycle = lifecycle
	if _, ok := lifecycle.(instancehost.Service); !ok {
		t.Fatalf("composed lifecycle type = %T, want instance_host.Service", lifecycle)
	}
}

func TestBoundRuntimeServiceUsesPublishedEngineForWideOperations(t *testing.T) {
	t.Parallel()

	root, err := newCompletedRoot(
		func() string { return "runtime-test-id" },
		nil,
		nil,
		clockwork.NewFakeClock(),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		nil,
		platformclock.Real{},
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	engine := &wideOperationRuntimeFake{}
	root.active["runtime-1"] = &runtimeActivationState{service: engine, ingress: engine}

	binding := &boundRuntimeService{root: root, runtimeID: "runtime-1"}
	if _, err := binding.SubmitWorkRequest(context.Background(), work.WorkRequest{}); err != nil {
		t.Fatalf("SubmitWorkRequest() error = %v", err)
	}
	if _, err := binding.SubscribeFactoryEvents(context.Background(), nil, interfaces.FactoryEventReconnectScope{}); err != nil {
		t.Fatalf("SubscribeFactoryEvents() error = %v", err)
	}
	if engine.submitCalls != 1 || engine.eventCalls != 1 {
		t.Fatalf("engine wide-operation calls = (%d, %d), want (1, 1)", engine.submitCalls, engine.eventCalls)
	}
}

func TestBoundRuntimeServiceUsesPublishedEngineForLegacyWorkSnapshot(t *testing.T) {
	t.Parallel()

	root, err := newCompletedRoot(
		func() string { return "runtime-test-id" },
		nil,
		nil,
		clockwork.NewFakeClock(),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		nil,
		platformclock.Real{},
	)
	if err != nil {
		t.Fatalf("NewRoot() error = %v", err)
	}
	snapshot := &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet]{}
	engine := &legacySnapshotRuntimeFake{snapshot: snapshot}
	root.active["runtime-1"] = &runtimeActivationState{
		service: engine,
		ingress: engine,
	}

	binding := &boundRuntimeService{root: root, runtimeID: "runtime-1"}
	legacyObservation, ok := factoryruntime.Service(binding).(interface {
		GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet], error)
	})
	if !ok {
		t.Fatal("bound Runtime service does not expose the legacy Work snapshot capability")
	}
	got, err := legacyObservation.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot() error = %v", err)
	}
	if got != snapshot {
		t.Fatalf("GetEngineStateSnapshot() = %p, want concrete delegate snapshot %p", got, snapshot)
	}
}

func TestBoundControlsAttributeResultsAndErrorsToActivatedRuntime(t *testing.T) {
	t.Parallel()
	for _, failure := range []bool{false, true} {
		name := "success"
		if failure {
			name = "controlled errors"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := newBoundControlRoot(t)
			delegates := [2]*boundControlRuntimeFake{}
			bindings := [2]factoryruntime.RuntimeBinding{}
			for i, identity := range []string{"first", "second"} {
				delegates[i] = newBoundControlRuntimeFake(identity, failure)
				bindings[i] = activateBoundControlRuntime(t, root, identity, delegates[i])
			}
			for i, binding := range bindings {
				selected, peer := delegates[i], delegates[1-i]
				for _, control := range selected.controlCases() {
					ctx, cancel := context.WithCancel(t.Context())
					before, peerBefore := len(selected.calls), len(peer.calls)
					got, err := control.invoke(ctx, binding.Service())
					cancel()
					if !reflect.DeepEqual(got, control.result) || err != control.err {
						t.Fatalf("binding %d %s = %#v, %v; want %#v, %v", i, control.method, got, err, control.result, control.err)
					}
					if len(selected.calls) != before+1 || len(peer.calls) != peerBefore {
						t.Fatalf("binding %d %s call counts = %d/%d; want %d/%d", i, control.method, len(selected.calls), len(peer.calls), before+1, peerBefore)
					}
					want := boundControlCall{method: control.method, ctx: ctx, request: control.request}
					call := selected.calls[before]
					if call.method != want.method || call.ctx != ctx || !reflect.DeepEqual(call.request, want.request) {
						t.Fatalf("binding %d forwarded call = %#v; want %#v with identical context", i, call, want)
					}
					if selected.closeCalls != 0 || peer.closeCalls != 0 {
						t.Fatal("control delegation invoked activation cleanup")
					}
				}
			}
		})
	}
}

func TestBoundControlsPreservePeerAfterSelectedDeactivation(t *testing.T) {
	t.Parallel()
	for _, selectedIndex := range []int{0, 1} {
		t.Run(fmt.Sprintf("selected %d", selectedIndex), func(t *testing.T) {
			t.Parallel()
			root := newBoundControlRoot(t)
			delegates := [2]*boundControlRuntimeFake{}
			bindings := [2]factoryruntime.RuntimeBinding{}
			for i, identity := range []string{"first", "second"} {
				delegates[i] = newBoundControlRuntimeFake(identity, false)
				bindings[i] = activateBoundControlRuntime(t, root, identity, delegates[i])
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			assertBoundRuntimeNotActive(t, root, ctx, "unknown-runtime", factoryruntime.RuntimeDeactivationRequest{RuntimeID: "unknown-runtime"})
			assertBoundRuntimeEffects(t, delegates, [2]int{}, [2]int{})
			for i := range delegates {
				assertBoundRuntimeControls(t, ctx, bindings[i], delegates[i], delegates[1-i])
			}
			selected, peer := delegates[selectedIndex], delegates[1-selectedIndex]
			closeCounts := [2]int{}
			closeCounts[selectedIndex] = 1
			assertBoundRuntimeStopped(t, root, ctx, bindings[selectedIndex], selected)
			assertBoundRuntimeEffects(t, delegates, [2]int{3, 3}, closeCounts)
			for _, control := range selected.controlCases() {
				_, err := control.invoke(ctx, bindings[selectedIndex].Service())
				if !errors.Is(err, factoryruntime.ErrNotRunning) {
					t.Fatalf("stale %s error = %v; want ErrNotRunning", control.method, err)
				}
			}
			assertBoundRuntimeNotActive(t, root, ctx, "runtime-"+selected.identity, factoryruntime.RuntimeDeactivationRequest{Binding: bindings[selectedIndex]})
			assertBoundRuntimeEffects(t, delegates, [2]int{3, 3}, closeCounts)
			assertBoundRuntimeControls(t, ctx, bindings[1-selectedIndex], peer, selected)
			callCounts := [2]int{6, 6}
			callCounts[selectedIndex] = 3
			assertBoundRuntimeEffects(t, delegates, callCounts, closeCounts)
			assertBoundRuntimeStopped(t, root, ctx, bindings[1-selectedIndex], peer)
			assertBoundRuntimeEffects(t, delegates, callCounts, [2]int{1, 1})
		})
	}
}

func assertBoundRuntimeEffects(t *testing.T, delegates [2]*boundControlRuntimeFake, calls, closes [2]int) {
	t.Helper()
	for i, delegate := range delegates {
		if len(delegate.calls) != calls[i] || delegate.closeCalls != closes[i] {
			t.Fatalf("%s effects: controls/Close = %d/%d; want %d/%d", delegate.identity, len(delegate.calls), delegate.closeCalls, calls[i], closes[i])
		}
	}
}

func assertBoundRuntimeControls(t *testing.T, ctx context.Context, binding factoryruntime.RuntimeBinding, selected, peer *boundControlRuntimeFake) {
	t.Helper()
	for _, control := range selected.controlCases() {
		before, peerBefore := len(selected.calls), len(peer.calls)
		got, err := control.invoke(ctx, binding.Service())
		if !reflect.DeepEqual(got, control.result) || err != control.err {
			t.Fatalf("%s %s = %#v, %v; want %#v, %v", selected.identity, control.method, got, err, control.result, control.err)
		}
		if len(selected.calls) != before+1 || len(peer.calls) != peerBefore {
			t.Fatalf("%s %s did not call only its selected delegate", selected.identity, control.method)
		}
		call := selected.calls[before]
		if call.method != control.method || call.ctx != ctx || !reflect.DeepEqual(call.request, control.request) {
			t.Fatalf("%s %s forwarded call = %#v; want identical context and %#v", selected.identity, control.method, call, control.request)
		}
	}
}

func assertBoundRuntimeStopped(t *testing.T, root *Root, ctx context.Context, binding factoryruntime.RuntimeBinding, delegate *boundControlRuntimeFake) {
	t.Helper()
	got, err := root.Deactivate(ctx, factoryruntime.RuntimeDeactivationRequest{Binding: binding})
	want := factoryruntime.RuntimeDeactivationResult{RuntimeID: "runtime-" + delegate.identity, State: factoryruntime.RuntimeLifecycleStateStopped}
	if err != nil || got != want || delegate.closeCalls != 1 || delegate.closeContext != ctx {
		t.Fatalf("%s Deactivate = %#v, %v; cleanup %d, context %v; want %#v and one Close with identical context", delegate.identity, got, err, delegate.closeCalls, delegate.closeContext, want)
	}
}

func assertBoundRuntimeNotActive(t *testing.T, root *Root, ctx context.Context, runtimeID string, request factoryruntime.RuntimeDeactivationRequest) {
	t.Helper()
	_, err := root.Deactivate(ctx, request)
	var activationErr *factoryruntime.RuntimeActivationError
	if !errors.Is(err, factoryruntime.ErrRuntimeNotActive) || !errors.As(err, &activationErr) ||
		activationErr.Kind != factoryruntime.RuntimeActivationErrorNotActive || activationErr.RuntimeID != runtimeID {
		t.Fatalf("Deactivate(%s) error = %#v; want typed NOT_ACTIVE for requested identity", runtimeID, err)
	}
}

func newBoundControlRoot(t *testing.T) *Root {
	t.Helper()
	root, err := newCompletedRoot(
		func() string { return "bound-control-id" }, nil, nil,
		clockwork.NewFakeClockAt(time.Date(2026, time.October, 3, 0, 0, 0, 0, time.UTC)),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
			return workers.WorkstationDispatchCancelResult{}, nil
		},
		platformclock.Real{},
	)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	return root
}

func activateBoundControlRuntime(t *testing.T, root *Root, identity string, delegate *boundControlRuntimeFake) factoryruntime.RuntimeBinding {
	t.Helper()
	request := factoryruntime.RuntimeActivationRequest{
		RuntimeID: "runtime-" + identity, FactorySessionID: "session-" + identity,
		Snapshot: interfaces.RuntimeSnapshot{
			FactoryDir: "/factories/" + identity, RuntimeBaseDir: "/runtime/" + identity,
			DefinitionVersion: &interfaces.FactoryVersion{Logical: 1},
			EffectiveFactory:  interfaces.FactoryConfig{Name: identity},
		},
	}
	want, err := request.Normalize()
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	starts := 0
	result, err := root.Activate(ctx, request, func(gotCtx context.Context, got factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
		starts++
		if gotCtx != ctx || !reflect.DeepEqual(got, want) {
			t.Fatalf("start received %v, %#v; want identical context and %#v", gotCtx, got, want)
		}
		return &factoryruntime.RuntimeActivation{Service: delegate, Close: func(closeCtx context.Context) error {
			delegate.closeCalls++
			delegate.closeContext = closeCtx
			return nil
		}}, nil
	})
	if err != nil || starts != 1 || result.RuntimeID != request.RuntimeID ||
		result.Runtime.FactorySessionID != request.FactorySessionID || result.State != factoryruntime.RuntimeLifecycleStateActive || result.Binding.IsZero() {
		t.Fatalf("Activate(%s) = %#v, %v; starts %d; want matching ACTIVE binding and one start", identity, result, err, starts)
	}
	return result.Binding
}

type boundControlCall struct {
	method  string
	ctx     context.Context
	request any
}

type boundControlRuntimeFake struct {
	factoryruntime.Service
	identity                          string
	calls                             []boundControlCall
	closeCalls                        int
	closeContext                      context.Context
	pause                             factoryruntime.PauseResult
	resume                            factoryruntime.ResumeResult
	terminate                         factoryruntime.TerminateResult
	pauseErr, resumeErr, terminateErr error
}

func newBoundControlRuntimeFake(identity string, failure bool) *boundControlRuntimeFake {
	outcome := factoryruntime.ControlOutcomeAccepted
	if identity == "second" {
		outcome = factoryruntime.ControlOutcomeNoOp
	}
	evidence := func(action factoryruntime.WorkerSessionControlAction) factoryruntime.WorkerSessionControlResult {
		return factoryruntime.WorkerSessionControlResult{
			TurnID: "turn-" + identity, Action: action,
			Outcome: factoryruntime.WorkerSessionControlAggregateOutcomePartial,
			Children: []factoryruntime.WorkerSessionControlChildResult{{
				WorkerSessionID: "worker-" + identity, DispatchID: "dispatch-" + identity,
				Outcome: factoryruntime.WorkerSessionControlChildOutcomeUnsupported,
			}},
		}
	}
	fake := &boundControlRuntimeFake{
		identity:  identity,
		pause:     factoryruntime.PauseResult{Outcome: outcome, WorkerSessionControl: evidence(factoryruntime.WorkerSessionControlActionPause)},
		resume:    factoryruntime.ResumeResult{Outcome: outcome, WorkerSessionControl: evidence(factoryruntime.WorkerSessionControlActionResume)},
		terminate: factoryruntime.TerminateResult{Outcome: outcome, WorkerSessionControl: evidence(factoryruntime.WorkerSessionControlActionCancel)},
	}
	if failure {
		fake.pauseErr = errors.New(identity + " pause failure")
		fake.resumeErr = errors.New(identity + " resume failure")
		fake.terminateErr = errors.New(identity + " terminate failure")
	}
	return fake
}

func (f *boundControlRuntimeFake) ControlPause(ctx context.Context, request factoryruntime.PauseRequest) (factoryruntime.PauseResult, error) {
	f.calls = append(f.calls, boundControlCall{"pause", ctx, request})
	return f.pause, f.pauseErr
}

func (f *boundControlRuntimeFake) ControlResume(ctx context.Context, request factoryruntime.ResumeRequest) (factoryruntime.ResumeResult, error) {
	f.calls = append(f.calls, boundControlCall{"resume", ctx, request})
	return f.resume, f.resumeErr
}

func (f *boundControlRuntimeFake) ControlTerminate(ctx context.Context, request factoryruntime.TerminateRequest) (factoryruntime.TerminateResult, error) {
	f.calls = append(f.calls, boundControlCall{"terminate", ctx, request})
	return f.terminate, f.terminateErr
}

type boundControlCase struct {
	method          string
	request, result any
	err             error
	invoke          func(context.Context, factoryruntime.Service) (any, error)
}

func (f *boundControlRuntimeFake) controlCases() []boundControlCase {
	pause := factoryruntime.PauseRequest{TurnID: "turn-" + f.identity, ControlID: "pause-" + f.identity}
	resume := factoryruntime.ResumeRequest{TurnID: "turn-" + f.identity, ControlID: "resume-" + f.identity}
	terminate := factoryruntime.TerminateRequest{
		Reason: "stop " + f.identity, TurnID: "turn-" + f.identity, ControlID: "terminate-" + f.identity,
		WorkerSessionAction: factoryruntime.WorkerSessionControlActionCancel,
	}
	return []boundControlCase{
		{"pause", pause, f.pause, f.pauseErr, func(ctx context.Context, service factoryruntime.Service) (any, error) {
			return service.ControlPause(ctx, pause)
		}},
		{"resume", resume, f.resume, f.resumeErr, func(ctx context.Context, service factoryruntime.Service) (any, error) {
			return service.ControlResume(ctx, resume)
		}},
		{"terminate", terminate, f.terminate, f.terminateErr, func(ctx context.Context, service factoryruntime.Service) (any, error) {
			return service.ControlTerminate(ctx, terminate)
		}},
	}
}

type wideOperationRuntimeFake struct {
	factoryruntime.Service
	submitCalls int
	eventCalls  int
}

type legacySnapshotRuntimeFake struct {
	wideOperationRuntimeFake
	snapshot *interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet]
}

func (service *legacySnapshotRuntimeFake) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet], error) {
	return service.snapshot, nil
}

func (service *wideOperationRuntimeFake) SubmitWorkRequest(context.Context, work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	service.submitCalls++
	return work.WorkRequestSubmitResult{}, nil
}

func (service *wideOperationRuntimeFake) SubscribeFactoryEvents(
	context.Context,
	*interfaces.FactoryEventReconnectCursor,
	interfaces.FactoryEventReconnectScope,
) (*interfaces.FactoryEventStream, error) {
	service.eventCalls++
	return nil, nil
}

func newAssemblyTestHost(clock factoryruntime.Clock, scheduler platformclock.TimerSource) (instancehost.Service, error) {
	lifecycle, err := factoryhost.NewLifecycleService(clock, scheduler)
	if err != nil {
		return nil, err
	}
	return instancehostwire.New(clock, scheduler, lifecycle)
}

// This component cell associates Root's published binding with the engine in
// an actual hosted handle. Provider execution and public session isolation are
// separate functional properties; no application graph is constructed here.
func TestBoundRuntimeControlsReachActivatedPhysicalHandle(t *testing.T) {
	t.Parallel()
	root := newBoundControlRoot(t)
	ctx := t.Context()
	var bindings [2]factoryruntime.RuntimeBinding
	var handles [2]factoryruntime.RuntimeRun
	var engines [2]*boundHostedEngine
	for i, identity := range []string{"first", "second"} {
		engine := &boundHostedEngine{boundControlRuntimeFake: newBoundControlRuntimeFake(identity, false), started: make(chan struct{})}
		engines[i] = engine
		request := factoryruntime.RuntimeActivationRequest{
			RuntimeID: "runtime-" + identity, FactorySessionID: "session-" + identity,
			Snapshot: interfaces.RuntimeSnapshot{
				FactoryDir: "/factories/" + identity, RuntimeBaseDir: "/runtime/" + identity,
				DefinitionVersion: &interfaces.FactoryVersion{Logical: 1}, EffectiveFactory: interfaces.FactoryConfig{Name: identity},
			},
		}
		result, err := root.Activate(ctx, request, func(ctx context.Context, request factoryruntime.RuntimeActivationRequest) (*factoryruntime.RuntimeActivation, error) {
			scope := root.instanceHost.Scope(clockwork.NewFakeClock())
			bundle := &factoryhost.Bundle{RuntimeInstanceID: request.RuntimeID, FactorySessionID: request.FactorySessionID, Factory: engine}
			run, err := scope.Start(ctx, bundle)
			if err != nil {
				return nil, err
			}
			handles[i] = run
			t.Cleanup(func() { _ = scope.Stop(run) })
			// Run admission is a signal, not a readiness poll or wall-time delay.
			select {
			case <-engine.started:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &factoryruntime.RuntimeActivation{Service: bundle.RuntimeService(), Close: func(context.Context) error { return scope.Stop(run) }}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		bindings[i] = result.Binding
	}
	assertBoundRuntimeNotActive(t, root, ctx, "unknown-runtime", factoryruntime.RuntimeDeactivationRequest{RuntimeID: "unknown-runtime"})
	for i := range engines {
		assertBoundRuntimeControls(t, ctx, bindings[i], engines[i].boundControlRuntimeFake, engines[1-i].boundControlRuntimeFake)
	}
	// The process Host also controls the same engine registered by the scope.
	before := len(engines[0].calls)
	if _, err := root.instanceHost.Pause(ctx, handles[0]); err != nil || len(engines[0].calls) != before+1 {
		t.Fatalf("process Host Pause = %v; calls %d, want %d", err, len(engines[0].calls), before+1)
	}
	assertSelectedPhysicalHandleStopped(t, ctx, root, bindings, handles, engines)
}

func assertSelectedPhysicalHandleStopped(t *testing.T, ctx context.Context, root *Root, bindings [2]factoryruntime.RuntimeBinding, handles [2]factoryruntime.RuntimeRun, engines [2]*boundHostedEngine) {
	t.Helper()
	if _, err := bindings[0].Deactivate(ctx); err != nil {
		t.Fatal(err)
	}
	if !handles[0].Completed() || !errors.Is(handles[0].Result(), context.Canceled) || handles[1].Completed() {
		t.Fatalf("selected/peer runs = %v/%v; selected result %v", handles[0].Completed(), handles[1].Completed(), handles[0].Result())
	}
	for _, control := range engines[0].controlCases() {
		if _, err := control.invoke(ctx, bindings[0].Service()); !errors.Is(err, factoryruntime.ErrNotRunning) {
			t.Fatalf("stale %s = %v, want ErrNotRunning", control.method, err)
		}
	}
	if _, err := root.instanceHost.Resume(ctx, handles[0]); !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("stale physical Resume = %v, want ErrNotRunning", err)
	}
	assertBoundRuntimeControls(t, ctx, bindings[1], engines[1].boundControlRuntimeFake, engines[0].boundControlRuntimeFake)
	if _, err := bindings[1].Deactivate(ctx); err != nil || !handles[1].Completed() {
		t.Fatalf("peer Deactivate = %v; completed %v", err, handles[1].Completed())
	}
}

type boundHostedEngine struct {
	factoryhost.Engine // Unused engine operations are outside this component cell.
	*boundControlRuntimeFake
	started chan struct{}
}

func (engine *boundHostedEngine) Run(ctx context.Context) error {
	close(engine.started)
	<-ctx.Done()
	return ctx.Err()
}

func (*boundHostedEngine) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet], error) {
	return &interfaces.EngineStateSnapshot[factoryruntime.PetriMarkingSnapshot, *factoryruntime.RuntimeNet]{
		RuntimeStatus: interfaces.RuntimeStatusActive, FactoryState: string(interfaces.FactoryStateRunning),
	}, nil
}
func newCompletedRoot(newID factoryruntime.IDGenerator, workflows factoryruntime.JavaScriptWorkflowDefinitions, runtime factoryruntime.JavaScriptWorkflowRuntime, clock factoryruntime.Clock, publisher dispatchplanning.WorkersPublisher, canceler dispatchplanning.WorkersCanceler, scheduler platformclock.TimerSource) (*Root, error) {
	host, err := newAssemblyTestHost(clock, scheduler)
	if err != nil {
		return nil, err
	}
	mapper, err := definitionmapping.New(newID)
	if err != nil {
		return nil, err
	}
	return NewRoot(orchestrationwire.New(mapper, workflows, runtime), host, dispatchplanningwire.New(publisher, canceler))
}

func TestNewAssemblyRetainsSelectedBundleOpening(t *testing.T) {
	opening := &BundleOpening{}
	sidecars := NewSidecarOpening(nil, platformclock.Real{})
	assembly, err := NewAssembly(opening, sidecars, nil, nil)
	if err != nil || assembly.bundleOpening != opening {
		t.Fatalf("NewAssembly = %#v, %v; want selected opening", assembly, err)
	}
	if assembly, err := NewAssembly(nil, nil, nil, nil); err == nil || assembly != nil {
		t.Fatalf("NewAssembly without opening = %#v, %v; want required dependency failure", assembly, err)
	}
}
