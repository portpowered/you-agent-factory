package replayhooks

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestAdaptPreservesRecordingsReplayContracts exercises the runtime-owned
// adapter at its internal boundary, where the engine snapshot representation
// is intentionally allowed to remain private.
func TestAdaptPreservesRecordingsReplayContracts(t *testing.T) {
	t.Parallel()

	stubHook := &runtimeReplayHookStub{
		name:     factoryruntime.ReplaySubmissionHookName,
		priority: -100,
	}
	replayFactory := factoryruntime.ReplayExecutionFactory(func(
		artifact *recordings.ReplayArtifact,
	) (
		workers.Runner,
		workers.CommandRunner,
		[]recordings.ReplayHook,
		recordings.CompletionDeliveryPlanner,
		error,
	) {
		if artifact == nil {
			t.Fatal("replay artifact = nil")
		}
		return nil, nil, []recordings.ReplayHook{stubHook}, &runtimeReplayPlannerStub{}, nil
	})

	_, _, hooks, planner, err := replayFactory(&recordings.ReplayArtifact{})
	if err != nil {
		t.Fatalf("ReplayExecutionFactory: %v", err)
	}
	if planner == nil {
		t.Fatal("completion planner = nil")
	}
	if len(hooks) != 1 {
		t.Fatalf("replay hooks = %d, want 1", len(hooks))
	}

	adapted := Adapt(hooks)
	if len(adapted) != 1 {
		t.Fatalf("adapted hooks = %d, want 1", len(adapted))
	}
	if adapted[0].Name() != factoryruntime.ReplaySubmissionHookName {
		t.Fatalf("adapted hook name = %q, want %q", adapted[0].Name(), factoryruntime.ReplaySubmissionHookName)
	}
	if adapted[0].Priority() != -100 {
		t.Fatalf("adapted hook priority = %d, want -100", adapted[0].Priority())
	}

	_, err = adapted[0].OnTick(
		context.Background(),
		interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
			Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
				TickCount: 4,
				Marking: petri.MarkingSnapshot{
					Tokens: map[string]*workers.Token{
						"token-1": {
							PlaceID: "place-review",
							Color: workers.Color{
								WorkID:   "work-story",
								DataType: workers.DataTypeWork,
							},
						},
					},
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("adapted hook OnTick: %v", err)
	}
	if stubHook.lastSnapshot.Tick != 4 {
		t.Fatalf("replay snapshot tick = %d, want 4", stubHook.lastSnapshot.Tick)
	}
	token, ok := stubHook.lastSnapshot.TokenByWorkID["work-story"]
	if !ok || token.PlaceID != "place-review" || token.TokenID != "token-1" {
		t.Fatalf("replay snapshot tokens = %#v, want work-story at place-review", stubHook.lastSnapshot.TokenByWorkID)
	}

	deliveryTick, keepAlive, err := planner.DeliveryTickForDispatch(work.WorkDispatch{DispatchID: "dispatch-1"})
	if err != nil {
		t.Fatalf("DeliveryTickForDispatch: %v", err)
	}
	if deliveryTick != 7 || !keepAlive {
		t.Fatalf("delivery tick = (%d, %v), want (7, true)", deliveryTick, keepAlive)
	}
}

type runtimeReplayHookStub struct {
	name         string
	priority     int
	lastSnapshot recordings.ReplaySnapshot
}

func (stub *runtimeReplayHookStub) Name() string {
	return stub.name
}

func (stub *runtimeReplayHookStub) Priority() int {
	return stub.priority
}

func (stub *runtimeReplayHookStub) OnTick(
	_ context.Context,
	snapshot recordings.ReplaySnapshot,
) (recordings.ReplayHookResult, error) {
	stub.lastSnapshot = snapshot
	return recordings.ReplayHookResult{KeepAlive: true}, nil
}

type runtimeReplayPlannerStub struct{}

func (stub *runtimeReplayPlannerStub) DeliveryTickForDispatch(
	work.WorkDispatch,
) (int, bool, error) {
	return 7, true, nil
}
