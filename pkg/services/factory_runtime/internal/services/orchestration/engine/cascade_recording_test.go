package engine

import (
	"context"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestCascadeRecordsOnlyAppliedMove(t *testing.T) {
	t.Parallel()
	for _, destination := range []string{"task:complete", "task:missing"} {
		t.Run(destination, func(t *testing.T) {
			t.Parallel()
			var records []work.WorkStateChangeRecord
			mover := &mockSubsystem{group: subsystems.CascadingFailure, execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
				if snap.TickCount > 1 {
					return nil, nil
				}
				return &interfaces.TickResult{
					Mutations:        []interfaces.MarkingMutation{{Type: interfaces.MutationMove, TokenID: "tok-task-1", FromPlace: "task:init", ToPlace: destination}},
					WorkStateChanges: []work.WorkStateChangeRecord{{WorkID: "child", FromState: "init", ToState: "complete", TriggerWorkID: "parent", Source: work.WorkStateChangeSourceCascadingFailure}},
				}, nil
			}}
			e := newTestFactoryEngine(buildTestNet(), petri.NewMarking("cascade"), []subsystems.Subsystem{mover})
			e.recordWorkStateChange = func(tick int, change work.WorkStateChangeRecord) {
				if tick != 1 || e.runtimeState.Marking.Tokens["tok-task-1"].PlaceID != destination {
					t.Fatal("recording preceded applied mutation")
				}
				records = append(records, change)
			}
			if _, err := submitWorkRequests(t.Context(), e, []work.SubmitRequest{{WorkID: "child", WorkTypeID: "task"}}); err != nil {
				t.Fatal(err)
			}
			err := e.Tick(t.Context())
			if destination == "task:missing" {
				if err == nil || len(records) != 0 {
					t.Fatalf("rejected move err=%v records=%v", err, records)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Tick(t.Context()); err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 || records[0].TriggerWorkID != "parent" {
				t.Fatalf("records = %#v, want one correlated move", records)
			}
		})
	}
}
