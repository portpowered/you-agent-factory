package runtime

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

func TestTickWhilePaused_SkipsCascadeButOperatorMoveUpdatesMarking(t *testing.T) {
	f, err := New(
		factory.WithNet(buildMoveControlNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []interfaces.SubmitRequest{
		{WorkID: "parent-work", WorkTypeID: "task", TraceID: "trace-parent"},
		{
			WorkID:     "child-work",
			WorkTypeID: "task",
			TraceID:    "trace-child",
			Relations: []interfaces.Relation{{
				Type:          interfaces.RelationDependsOn,
				TargetWorkID:  "parent-work",
				RequiredState: "complete",
			}},
		},
	}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick inject: %v", err)
	}

	if _, err := f.MoveWork(ctx, "parent-work", "failed", interfaces.WorkStateChangeSourceCLI, ""); err != nil {
		t.Fatalf("MoveWork parent to failed: %v", err)
	}
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	before, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before tick: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	afterTick, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after tick: %v", err)
	}
	if !markingContainsWorkAtPlace(&before.Marking, "child-work", "task:init") {
		t.Fatalf("pre-tick marking = %#v, want child-work in task:init", before.Marking.Tokens)
	}
	if !markingContainsWorkAtPlace(&afterTick.Marking, "child-work", "task:init") {
		t.Fatalf("post-tick marking = %#v, want child-work still in task:init (no cascade)", afterTick.Marking.Tokens)
	}

	result, err := f.MoveWork(ctx, "child-work", "complete", interfaces.WorkStateChangeSourceCLI, "")
	if err != nil {
		t.Fatalf("MoveWork while paused: %v", err)
	}
	if result.FromState != "init" || result.ToState != "complete" {
		t.Fatalf("move result = %#v, want init -> complete", result)
	}
	assertOperatorWorkStateChangeEvent(t, f, "child-work", "init", "complete", factoryapi.WorkStateChangeSourceCLI)

	afterMove, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after move: %v", err)
	}
	if !markingContainsWorkAtPlace(&afterMove.Marking, "child-work", "task:complete") {
		t.Fatalf("marking = %#v, want child-work at task:complete after operator move", afterMove.Marking.Tokens)
	}
}
