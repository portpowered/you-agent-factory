package runtime

import (
	"reflect"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// This wholly authored history characterizes the pre-fix failure: the cron
// input was consumed, a distinct task was emitted, and no dispatch still owns it.
func syntheticFailedCronRestoreState() *interfaces.FactoryWorldState {
	cron := work.FactoryWorkItem{
		ID: "synthetic-cron-input", WorkTypeID: interfaces.SystemTimeWorkTypeID,
		State: interfaces.SystemTimePendingState,
		Tags: map[string]string{
			interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
			interfaces.TimeWorkTagKeyCronWorkstation: "cron-refresh",
		},
	}
	output := work.FactoryWorkItem{ID: "synthetic-downstream-task", WorkTypeID: "task", State: "failed"}
	completion := interfaces.FactoryWorldDispatchCompletion{
		DispatchID: "synthetic-failed-dispatch", TransitionID: "cron-refresh",
		WorkItemIDs: []string{cron.ID, output.ID},
		ConsumedInputs: []interfaces.WorkstationInput{{
			TokenID: cron.ID, PlaceID: interfaces.SystemTimePendingPlaceID, WorkItem: &cron,
		}},
		InputWorkItems: []work.FactoryWorkItem{cron}, OutputWorkItems: []work.FactoryWorkItem{output},
		Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeFailed), Error: "synthetic failure"},
	}
	return &interfaces.FactoryWorldState{
		WorkItemsByID:       map[string]work.FactoryWorkItem{cron.ID: cron, output.ID: output},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{cron.ID: cron},
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
			"task:failed": {PlaceID: "task:failed", WorkItemIDs: []string{output.ID}},
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{completion},
		FailedDispatches:    []interfaces.FactoryWorldDispatchCompletion{completion},
	}
}

func TestNew_CharacterizesConsumedFailedCronMissingPlacement(t *testing.T) {
	t.Parallel()
	restored := syntheticFailedCronRestoreState()
	beforeInput := restored.WorkItemsByID["synthetic-cron-input"]
	beforeOutput := restored.PlaceOccupancyByID["task:failed"]
	_, err := newTestFactory(withNet(buildCronRestoreNet()), withRestoredWorldState(restored))
	want := `restore Factory Runtime Work board: restore Work board: active Work "synthetic-cron-input" has no current place occupancy`
	if err == nil || err.Error() != want {
		t.Fatalf("New error = %v, want %q", err, want)
	}
	if !reflect.DeepEqual(restored.WorkItemsByID[beforeInput.ID], beforeInput) ||
		!reflect.DeepEqual(restored.PlaceOccupancyByID["task:failed"], beforeOutput) {
		t.Fatal("failed restore changed historical input or downstream placement")
	}
	if len(restored.CompletedDispatches) != 1 || len(restored.FailedDispatches) != 1 || len(restored.ActiveDispatches) != 0 {
		t.Fatal("failed restore changed completed, failed, or active dispatch history")
	}
}
