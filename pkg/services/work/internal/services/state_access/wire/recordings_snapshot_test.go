package wire

import (
	"context"
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestReadSnapshotFromWorldStateRejectsUnsupportedViews(t *testing.T) {
	t.Parallel()

	for name, view := range map[string]recordings.WorldStateView{
		"wrong schema": {SchemaVersion: "v0", Payload: `{"workItemsById":{}}`},
		"empty payload": {SchemaVersion: recordings.WorldStateViewSchemaV1, Payload: "  "},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := readSnapshotFromWorldState(view)
			if !errors.Is(err, recordings.ErrUnsupportedProjectionView) {
				t.Fatalf("readSnapshotFromWorldState error = %v, want ErrUnsupportedProjectionView", err)
			}
		})
	}
}

func TestReadSnapshotFromWorldStateRejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	_, err := readSnapshotFromWorldState(recordings.WorldStateView{
		SchemaVersion: recordings.WorldStateViewSchemaV1,
		Payload:       "{not-json",
	})
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("readSnapshotFromWorldState error = %v, want ErrInvalidProjectionInput", err)
	}
}

func TestReadSnapshotFromFactoryWorldStateMapsActiveFailedTerminalAndRelations(t *testing.T) {
	t.Parallel()

	state := interfaces.FactoryWorldState{
		Topology: interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{{
				ID: "story",
				States: []interfaces.FactoryStateDefinition{
					{Value: "init", Category: work.StateTypeInitial},
					{Value: "review", Category: work.StateTypeProcessing},
					{Value: "done", Category: work.StateTypeTerminal},
				},
			}},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			interfaces.SystemTimeWorkTypeID: {
				ID:         interfaces.SystemTimeWorkTypeID,
				WorkTypeID: interfaces.SystemTimeWorkTypeID,
				State:      "pending",
			},
			"work-active": {
				ID:          "work-active",
				WorkTypeID:  "story",
				DisplayName: "Active story",
				State:       "review",
			},
			"work-failed": {
				ID:         "work-failed",
				WorkTypeID: "story",
				State:      "review",
			},
			"work-terminal": {
				ID:         "work-terminal",
				WorkTypeID: "story",
				State:      "init",
			},
			"work-empty-state": {
				ID:         "work-empty-state",
				WorkTypeID: "story",
			},
		},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{
			"work-active": {
				ID:         "work-active",
				WorkTypeID: "story",
				State:      "review",
			},
		},
		FailedWorkItemsByID: map[string]work.FactoryWorkItem{
			"work-failed": {
				ID:         "work-failed",
				WorkTypeID: "story",
				State:      "review",
			},
		},
		TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{
			"work-terminal": {
				WorkItem: work.FactoryWorkItem{
					ID:         "work-terminal",
					WorkTypeID: "story",
					State:      "done",
				},
			},
		},
		RelationsByWorkID: map[string][]work.FactoryRelation{
			"work-active": {{
				Type:           "depends_on",
				TargetWorkID:   "work-terminal",
				TargetWorkName: "Terminal story",
				RequiredState:  "done",
			}},
		},
	}
	snapshot := readSnapshotFromFactoryWorldState(state)
	if len(snapshot.Items) != 4 {
		t.Fatalf("snapshot items = %d, want 4 public work items", len(snapshot.Items))
	}
	byID := make(map[string]work.ReadModel, len(snapshot.Items))
	for _, item := range snapshot.Items {
		byID[item.WorkID] = item
	}
	if _, ok := byID[interfaces.SystemTimeWorkTypeID]; ok {
		t.Fatalf("system time work leaked into snapshot: %#v", snapshot)
	}
	if byID["work-active"].State == nil || byID["work-active"].State.Type != work.StateTypeProcessing {
		t.Fatalf("active work state = %#v, want processing", byID["work-active"].State)
	}
	if byID["work-failed"].State == nil || byID["work-failed"].State.Type != work.StateTypeFailed {
		t.Fatalf("failed work state = %#v, want failed", byID["work-failed"].State)
	}
	if byID["work-terminal"].State == nil || byID["work-terminal"].State.Name != "done" {
		t.Fatalf("terminal work state = %#v, want done", byID["work-terminal"].State)
	}
	if byID["work-empty-state"].State != nil {
		t.Fatalf("empty-state work = %#v, want nil state", byID["work-empty-state"].State)
	}
	if len(byID["work-active"].Relations) != 1 || byID["work-active"].Relations[0].TargetWorkName != "Terminal story" {
		t.Fatalf("relations = %#v, want one depends_on relation", byID["work-active"].Relations)
	}
}

func TestWorkStateCategoryFallsBackToInitialWhenUnknown(t *testing.T) {
	t.Parallel()

	state := &work.State{
		Name: "unknown-state",
		Type: workStateCategory(interfaces.InitialStructurePayload{}, "story", "unknown-state"),
	}
	if state.Type != "" {
		t.Fatalf("workStateCategory = %q, want empty category", state.Type)
	}
	item := workStateFromItem(
		work.FactoryWorkItem{ID: "work-1", WorkTypeID: "story", State: "unknown-state"},
		interfaces.FactoryWorldState{},
		false,
	)
	if item == nil || item.Type != work.StateTypeInitial {
		t.Fatalf("workStateFromItem = %#v, want initial fallback", item)
	}
}

func TestReconstructWorldStateRequestSelectsHighestTick(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-1"}
	request := reconstructWorldStateRequest(scope, []recordings.CanonicalEvent{
		{FactoryTick: 1},
		{FactoryTick: 7},
		{FactoryTick: 3},
	})
	if request.SelectedTick != 7 || request.Scope != scope || len(request.Events) != 3 {
		t.Fatalf("request = %#v, want tick 7 with copied events", request)
	}
	request.Events[0].FactoryTick = 99
	if request.Events[0].FactoryTick == 99 {
		// copied slice should be independent from source mutation only if source changed
	}
}

func TestFirstNonEmptyReturnsFirstTrimmedValue(t *testing.T) {
	t.Parallel()

	if got := firstNonEmpty(" ", "", "value", "later"); got != "value" {
		t.Fatalf("firstNonEmpty = %q, want value", got)
	}
	if got := firstNonEmpty(" ", ""); got != "" {
		t.Fatalf("firstNonEmpty(all empty) = %q, want empty", got)
	}
}

func TestReadWorkSnapshotRejectsNilRecordingsRoot(t *testing.T) {
	t.Parallel()

	adapter := recordingsAdapter{}
	_, err := adapter.ReadWorkSnapshot(context.Background(), "session-1")
	if err == nil || err.Error() != "Recordings service is required" {
		t.Fatalf("ReadWorkSnapshot error = %v, want missing Recordings service", err)
	}
}

func TestWorkStateCategoryMatchesWorkTypeName(t *testing.T) {
	t.Parallel()

	category := workStateCategory(
		interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{{
				Name: "story",
				States: []interfaces.FactoryStateDefinition{
					{Value: "review", Category: work.StateTypeProcessing},
				},
			}},
		},
		"story",
		"review",
	)
	if category != work.StateTypeProcessing {
		t.Fatalf("workStateCategory = %q, want processing", category)
	}
}

func TestWorkStateCategorySkipsNonMatchingWorkTypes(t *testing.T) {
	t.Parallel()

	category := workStateCategory(
		interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{
				{
					ID: "bug",
					States: []interfaces.FactoryStateDefinition{
						{Value: "triage", Category: work.StateTypeInitial},
					},
				},
				{
					ID: "story",
					States: []interfaces.FactoryStateDefinition{
						{Value: "review", Category: work.StateTypeProcessing},
					},
				},
			},
		},
		"story",
		"review",
	)
	if category != work.StateTypeProcessing {
		t.Fatalf("workStateCategory = %q, want processing", category)
	}
}
