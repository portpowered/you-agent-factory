package runtime

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNew_RequiresNet(t *testing.T) {
	_, err := newTestFactory()
	if err == nil {
		t.Fatal("expected error when Net is not provided")
	}
}

func TestNew_RequiresClock(t *testing.T) {
	_, err := newTestFactory(withNet(buildSimpleNet()), withClock(nil))
	if err == nil || !strings.Contains(err.Error(), "Factory Runtime clock is required") {
		t.Fatalf("New() error = %v, want required clock error", err)
	}
}

func TestNew_WithoutRestoredStateUsesFreshResourceMarking(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	net := buildSimpleNet()
	resource := &state.ResourceDef{ID: "gpu-slot", Name: "GPU slot", Capacity: 2}
	place, expectedTokens := state.GenerateResourcePlaces(resource, base)
	net.Resources = map[string]*state.ResourceDef{resource.ID: resource}
	net.Places[place.ID] = place

	f, err := newTestFactory(
		withNet(net),
		withClock(platformclock.NewDeterministic(base, time.Second)),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.Marking.WorkflowID != net.ID {
		t.Fatalf("marking workflow ID = %q, want %q", snapshot.Marking.WorkflowID, net.ID)
	}
	if got := len(snapshot.Marking.Tokens); got != len(expectedTokens) {
		t.Fatalf("initial token count = %d, want %d resource tokens", got, len(expectedTokens))
	}
	if got := len(snapshot.Marking.PlaceTokens[place.ID]); got != len(expectedTokens) {
		t.Fatalf("initial tokens at %q = %d, want %d", place.ID, got, len(expectedTokens))
	}
	if got := len(snapshot.Marking.PlaceTokens["task:init"]); got != 0 {
		t.Fatalf("initial Work tokens at task:init = %d, want none", got)
	}

	for _, expected := range expectedTokens {
		actual, ok := snapshot.Marking.Tokens[expected.ID]
		if !ok {
			t.Fatalf("initial marking is missing resource token %q", expected.ID)
		}
		if actual.PlaceID != expected.PlaceID {
			t.Errorf("resource token %q place = %q, want %q", actual.ID, actual.PlaceID, expected.PlaceID)
		}
		if actual.Color.WorkID != expected.Color.WorkID || actual.Color.WorkTypeID != expected.Color.WorkTypeID {
			t.Errorf("resource token %q identity = (%q, %q), want (%q, %q)", actual.ID, actual.Color.WorkID, actual.Color.WorkTypeID, expected.Color.WorkID, expected.Color.WorkTypeID)
		}
		if actual.Color.DataType != expected.Color.DataType {
			t.Errorf("resource token %q data type = %q, want %q", actual.ID, actual.Color.DataType, expected.Color.DataType)
		}
		if !actual.CreatedAt.Equal(base) || !actual.EnteredAt.Equal(base) {
			t.Errorf("resource token %q timestamps = (%s, %s), want (%s, %s)", actual.ID, actual.CreatedAt, actual.EnteredAt, base, base)
		}
	}
}

func TestBuildRuntimeMarkingReportsOnlyRestoredWorkActuallySeeded(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	cfg := &runtimeConfig{
		net:                buildSimpleNet(),
		clock:              platformclock.NewDeterministic(base, time.Second),
		restoredWorldState: restoredWorldStateFixture(base, ""),
	}

	marking, seededWorkIDs, _, err := buildRuntimeMarking(cfg)
	if err != nil {
		t.Fatalf("buildRuntimeMarking: %v", err)
	}
	if len(marking.TokensInPlace("task:done")) != 1 {
		t.Fatalf("restored marking task:done tokens = %d, want one", len(marking.TokensInPlace("task:done")))
	}
	if len(seededWorkIDs) != 1 {
		t.Fatalf("seeded restored Work IDs = %#v, want one actually seeded Work", seededWorkIDs)
	}
	if _, ok := seededWorkIDs["work-restored"]; !ok {
		t.Fatalf("seeded restored Work IDs = %#v, want work-restored", seededWorkIDs)
	}
	if _, ok := seededWorkIDs["work-not-on-board"]; ok {
		t.Fatalf("seeded restored Work IDs = %#v, must exclude historical Work absent from occupancy", seededWorkIDs)
	}
}

func TestRestoredWorkIDsWithRecordedDispatchIncludesReplayDispatchFacts(t *testing.T) {
	restored := &interfaces.FactoryWorldState{
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"active": {WorkItemIDs: []string{"work-active"}},
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{
			{WorkItemIDs: []string{"work-completed"}},
		},
		FailedDispatches: []interfaces.FactoryWorldDispatchCompletion{
			{WorkItemIDs: []string{"work-failed"}},
		},
		PendingHumanApprovalsByID: map[string]interfaces.FactoryWorldHumanApproval{
			"approval": {WorkItemIDs: []string{"work-pending"}},
		},
	}

	got := restoredWorkIDsWithRecordedDispatch(restored)
	for _, workID := range []string{"work-active", "work-completed", "work-failed", "work-pending"} {
		if _, ok := got[workID]; !ok {
			t.Fatalf("recorded-dispatch Work IDs = %#v, missing %q", got, workID)
		}
	}
	if len(got) != 4 {
		t.Fatalf("recorded-dispatch Work IDs = %#v, want four identities", got)
	}
}

func TestNew_WithExplicitEmptyRestoredStateUsesFreshResourceMarking(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	newFactory := func(restored *interfaces.FactoryWorldState) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
		net := buildSimpleNet()
		resource := &state.ResourceDef{ID: "gpu-slot", Name: "GPU slot", Capacity: 2}
		place, _ := state.GenerateResourcePlaces(resource, base)
		net.Resources = map[string]*state.ResourceDef{resource.ID: resource}
		net.Places[place.ID] = place
		f, err := newTestFactory(
			withNet(net),
			withClock(platformclock.NewDeterministic(base, time.Second)),
			withRestoredWorldState(restored),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		snapshot, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		return snapshot
	}

	fresh := newFactory(nil)
	empty := newFactory(&interfaces.FactoryWorldState{})
	if !reflect.DeepEqual(fresh.Marking, empty.Marking) {
		t.Fatalf("empty restored marking = %#v, fresh marking = %#v; want identical markings", empty.Marking, fresh.Marking)
	}
}

func TestNew_WithRestoredWorldStateSeedsWorkAndKeepsCurrentResourcesAuthoritative(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	net := buildSimpleNet()
	resource := &state.ResourceDef{ID: "gpu-slot", Name: "GPU slot", Capacity: 2}
	resourcePlace, expectedResources := state.GenerateResourcePlaces(resource, base)
	net.Resources = map[string]*state.ResourceDef{resource.ID: resource}
	net.Places[resourcePlace.ID] = resourcePlace
	restored := restoredWorldStateFixture(base, resourcePlace.ID)

	f, err := newTestFactory(
		withNet(net),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withRestoredWorldState(restored),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if got := len(snapshot.Marking.Tokens); got != len(expectedResources)+1 {
		t.Fatalf("initial token count = %d, want %d current resources plus restored Work", got, len(expectedResources)+1)
	}
	assertCurrentRestoredResourceTokens(t, snapshot, resourcePlace.ID, expectedResources)
	token := restoredWorkTokenFromSnapshot(t, snapshot)
	assertRestoredWorkToken(t, token, restored.WorkItemsByID["work-restored"])
}

func TestNew_WithRestoredActiveDispatchUsesRecordedWorkPlacement(t *testing.T) {
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-active": {
				ID: "work-active", WorkTypeID: "task", State: "done",
			},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-active": {
				DispatchID:  "dispatch-active",
				WorkItemIDs: []string{"work-active"},
				Inputs: []interfaces.WorkstationInput{{
					TokenID: "work-active", PlaceID: "task:done",
					WorkItem: &work.FactoryWorkItem{ID: "work-active"},
				}},
			},
		},
		// An in-flight dispatch has consumed the Work token, so the
		// reconstructed occupancy is intentionally empty.
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
	}

	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withClock(platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second)),
		withRestoredWorldState(restored),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	tokens := snapshot.Marking.PlaceTokens["task:done"]
	if len(tokens) != 1 {
		t.Fatalf("restored active Work token IDs = %#v, want one token at task:done", tokens)
	}
	token := snapshot.Marking.Tokens[tokens[0]]
	if token == nil || token.Color.WorkID != "work-active" {
		t.Fatalf("restored active Work token = %#v, want work-active", token)
	}
}

func TestNew_WithRestoredHumanApprovalDispatchDoesNotRestoreWorkPlacement(t *testing.T) {
	restored := &interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-approval": {
				ID: "work-approval", WorkTypeID: "task", State: "done",
			},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-approval": {
				DispatchID: "dispatch-approval", WorkItemIDs: []string{"work-approval"},
			},
		},
		PendingHumanApprovalsByID: map[string]interfaces.FactoryWorldHumanApproval{
			"approval-1": {ApprovalID: "approval-1", DispatchID: "dispatch-approval"},
		},
		// A pending approval retains the Work claim in its durable approval
		// projection; it must not be re-materialized as an ordinary Work token.
		PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
	}

	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withClock(platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Second)),
		withRestoredWorldState(restored),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if tokens := snapshot.Marking.PlaceTokens["task:done"]; len(tokens) != 0 {
		t.Fatalf("restored pending approval Work token IDs = %#v, want no ordinary Work token", tokens)
	}
}

func TestNew_ReconcilesRestoredActiveDispatchExactlyOnceAndLeavesWorkEligible(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	restored := &interfaces.FactoryWorldState{
		Tick: 12,
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-processing": {
				ID: "work-processing", WorkTypeID: "task", State: "init",
			},
		},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{
			"work-processing": {
				ID: "work-processing", WorkTypeID: "task", State: "init",
			},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-processing": {
				DispatchID:  "dispatch-processing",
				StartedTick: 11,
				WorkItemIDs: []string{"work-processing"},
				Inputs: []interfaces.WorkstationInput{{
					TokenID:  "work-processing",
					PlaceID:  "task:init",
					WorkItem: &work.FactoryWorkItem{ID: "work-processing", WorkTypeID: "task", State: "init"},
				}},
			},
		},
	}
	ledger := &recordingfixtures.ScriptedRuntimeLedger{}
	newRuntime := func() factoryhost.Engine {
		runtime, err := newTestFactory(
			withNet(buildSimpleNet()),
			withClock(platformclock.NewDeterministic(base, time.Second)),
			withRestoredWorldState(restored),
			withFactoryEventHistory(ledger),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return runtime
	}

	first := newRuntime()
	snapshot, err := first.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snapshot.Dispatches) != 0 {
		t.Fatalf("restored runtime dispatches = %#v, want no pre-restart live dispatch", snapshot.Dispatches)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, "work-processing", "task:init") {
		t.Fatalf("restored Work is not re-armed at task:init: %#v", snapshot.Marking.PlaceTokens)
	}

	interrupted := make([]interfaces.FactoryEvent, 0, 1)
	for _, event := range ledger.CanonicalEvents() {
		if event.Type == interfaces.FactoryEventTypeDispatchInterrupted {
			interrupted = append(interrupted, event)
		}
	}
	if len(interrupted) != 1 {
		t.Fatalf("interruption events after first restore = %d, want one", len(interrupted))
	}
	if got := stringPointerValue(interrupted[0].Context.DispatchID); got != "dispatch-processing" {
		t.Fatalf("interrupted dispatch ID = %q, want dispatch-processing", got)
	}
	var payload interfaces.DispatchInterruptedEventPayload
	if err := interrupted[0].DecodePayload(&payload); err != nil {
		t.Fatalf("decode interruption payload: %v", err)
	}
	if payload.Reason != daemonRestartDispatchInterruptionReason || !payload.RetryPlanned {
		t.Fatalf("interruption payload = %#v, want restart reason and retry planned", payload)
	}

	_ = newRuntime()
	interrupted = interrupted[:0]
	for _, event := range ledger.CanonicalEvents() {
		if event.Type == interfaces.FactoryEventTypeDispatchInterrupted {
			interrupted = append(interrupted, event)
		}
	}
	if len(interrupted) != 1 {
		t.Fatalf("interruption events after repeated restore = %d, want one", len(interrupted))
	}
}

func TestNew_ReconcilesRestoredDispatchLeavesNoAttemptTerminalAndBlockedWorkUnchanged(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	t.Run("no attempt", func(t *testing.T) {
		testRestoredNoAttempt(t, base)
	})

	t.Run("terminal work", func(t *testing.T) {
		testRestoredTerminalWork(t, base)
	})

	t.Run("dependency blocked", func(t *testing.T) {
		testRestoredDependencyBlocked(t, base)
	})
}

func TestNew_WithIncompatibleRestoredWorldStateFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		restored *interfaces.FactoryWorldState
		want     string
	}{
		{
			name: "unknown occupied place",
			restored: &interfaces.FactoryWorldState{
				WorkItemsByID: map[string]work.FactoryWorkItem{
					"work-1": {ID: "work-1", WorkTypeID: "task", State: "missing"},
				},
				PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{
					"task:missing": {PlaceID: "task:missing", WorkItemIDs: []string{"work-1"}},
				},
			},
			want: "not present in the current Factory topology",
		},
		{
			name: "active Work without occupancy",
			restored: &interfaces.FactoryWorldState{
				WorkItemsByID: map[string]work.FactoryWorkItem{
					"work-1": {ID: "work-1", WorkTypeID: "task", State: "init"},
				},
				ActiveWorkItemsByID: map[string]work.FactoryWorkItem{
					"work-1": {ID: "work-1", WorkTypeID: "task", State: "init"},
				},
				PlaceOccupancyByID: map[string]interfaces.FactoryPlaceOccupancy{},
			},
			want: "has no current place occupancy",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newTestFactory(
				withNet(buildSimpleNet()),
				withRestoredWorldState(test.restored),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func assertCurrentRestoredResourceTokens(
	t *testing.T,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	placeID string,
	expectedResources []*factoryruntime.RuntimeToken,
) {
	t.Helper()
	wantIDs := []string{expectedResources[0].ID, expectedResources[1].ID}
	if got := snapshot.Marking.PlaceTokens[placeID]; !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("resource token IDs = %#v, want current generated IDs %#v", got, wantIDs)
	}
}

func restoredWorkTokenFromSnapshot(
	t *testing.T,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) *factoryruntime.RuntimeToken {
	t.Helper()
	workTokenIDs := snapshot.Marking.PlaceTokens["task:done"]
	if len(workTokenIDs) != 1 {
		t.Fatalf("restored Work token IDs = %#v, want one token", workTokenIDs)
	}
	token := snapshot.Marking.Tokens[workTokenIDs[0]]
	if token == nil {
		t.Fatalf("restored Work token %q is missing from marking", workTokenIDs[0])
	}
	return token
}

func assertRestoredWorkToken(
	t *testing.T,
	token *factoryruntime.RuntimeToken,
	item work.FactoryWorkItem,
) {
	t.Helper()
	if token.ID != "work-restored" || token.Color.WorkID != "work-restored" || token.Color.WorkTypeID != "task" {
		t.Fatalf("restored Work identity = (%q, %q, %q), want work-restored/task", token.ID, token.Color.WorkID, token.Color.WorkTypeID)
	}
	if token.Color.RequestID != "request-restored" || token.Color.TraceID != "trace-restored" || token.Color.ParentID != "parent-restored" {
		t.Fatalf("restored Work lineage = %#v, want recorded request/trace/parent", token.Color)
	}
	if token.Color.Name != "Restored work" || !reflect.DeepEqual(token.Color.Tags, map[string]string{"priority": "high"}) {
		t.Fatalf("restored Work presentation = %#v, want recorded name and tags", token.Color)
	}
	assertRestoredWorkPayload(t, token, item)
	if len(token.Color.Relations) != 1 || token.Color.Relations[0].TargetWorkID != "dependency-restored" {
		t.Fatalf("restored Work relations = %#v, want dependency-restored", token.Color.Relations)
	}
}

func assertRestoredWorkPayload(t *testing.T, token *factoryruntime.RuntimeToken, item work.FactoryWorkItem) {
	t.Helper()
	if !reflect.DeepEqual(token.Color.Content, item.Content) ||
		!reflect.DeepEqual(token.Color.StructuredResult, item.StructuredResult) {
		t.Fatalf("restored Work payload = %#v, want recorded content and structured result", token.Color)
	}
}

func restoredWorldStateFixture(base time.Time, resourcePlaceID string) *interfaces.FactoryWorldState {
	occupancy := map[string]interfaces.FactoryPlaceOccupancy{
		"task:done": {PlaceID: "task:done", WorkItemIDs: []string{"work-restored"}, ResourceTokenIDs: []string{"old-gpu-token-1", "old-gpu-token-2", "old-gpu-token-3"}},
	}
	if resourcePlaceID != "" {
		occupancy[resourcePlaceID] = interfaces.FactoryPlaceOccupancy{
			PlaceID: resourcePlaceID, ResourceTokenIDs: []string{"recorded-resource-token"},
		}
	}
	return &interfaces.FactoryWorldState{
		EventTime: base.Add(-time.Minute),
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-restored": {
				ID: "work-restored", WorkTypeID: "task", State: "done", DisplayName: "Restored work",
				ChainingTraceDepth: 2, CurrentChainingTraceID: "chain-current",
				PreviousChainingTraceIDs: []string{"chain-previous"}, TraceID: "trace-restored",
				ParentID: "parent-restored",
				Content:  []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "recorded content"}},
				Tags:     map[string]string{"priority": "high"}, StructuredResult: map[string]any{"answer": "recorded"},
			},
			// This item is retained in the historical Work index but is not
			// present in current occupancy; it must not become a live token.
			"work-not-on-board": {ID: "work-not-on-board", WorkTypeID: "task", State: "init"},
		},
		WorkRequestsByID: map[string]interfaces.WorkRequestPayload{
			"request-restored": {RequestID: "request-restored", WorkItems: []work.FactoryWorkItem{{ID: "work-restored"}}},
		},
		RelationsByWorkID: map[string][]work.FactoryRelation{
			"work-restored": {{Type: string(work.WorkRelationDependsOn), TargetWorkID: "dependency-restored", RequiredState: "done"}},
		},
		PlaceOccupancyByID: occupancy,
	}
}

type resourceCapacityRuntimeHarness struct {
	capacity  factoryruntime.ResourceCapacityService
	admitted  factoryruntime.AdmittedResourceCapacityService
	admission factoryruntime.ResourceCapacityAdmission
	leases    factoryruntime.ResourceCapacityLeaseAdmission
	revision  factoryruntime.ResourceCapacityRevisionService
}

func newResourceCapacityRuntimeHarness(t *testing.T) resourceCapacityRuntimeHarness {
	t.Helper()
	net := buildSimpleNet()
	net.Resources = map[string]*state.ResourceDef{
		"gpu-slot": {ID: "gpu-slot", Name: "GPU slot", Capacity: 2},
		"orphan":   {ID: "orphan", Name: "Orphan pool", Capacity: 1},
	}
	for _, resource := range net.Resources {
		place, _ := state.GenerateResourcePlaces(resource, time.Unix(0, 0))
		net.Places[place.ID] = place
	}

	f, err := newTestFactory(
		withNet(net),
		withRuntimeConfig(runtimefixtures.RuntimeDefinitionLookupFixture{
			Factory: &interfaces.FactoryConfig{
				Name: "capacity-factory",
				Resources: []interfaces.ResourceConfig{{
					ID:       "gpu-slot",
					Capacity: 2,
				}},
			},
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	harness := resourceCapacityRuntimeHarness{}
	var ok bool
	harness.capacity, ok = f.(factoryruntime.ResourceCapacityService)
	if !ok {
		t.Fatal("Factory Runtime does not expose resource capacity service")
	}
	harness.admitted, ok = f.(factoryruntime.AdmittedResourceCapacityService)
	if !ok {
		t.Fatal("Factory Runtime does not expose admitted resource capacity service")
	}
	harness.admission, ok = f.(factoryruntime.ResourceCapacityAdmission)
	if !ok {
		t.Fatal("Factory Runtime does not expose resource capacity admission")
	}
	harness.leases, ok = f.(factoryruntime.ResourceCapacityLeaseAdmission)
	if !ok {
		t.Fatal("Factory Runtime does not expose resource capacity leases")
	}
	harness.revision, ok = f.(factoryruntime.ResourceCapacityRevisionService)
	if !ok {
		t.Fatal("Factory Runtime does not expose resource capacity revision")
	}
	return harness
}

func TestResourceCapacityRuntimePreviewAttachesEffectiveFactory(t *testing.T) {
	harness := newResourceCapacityRuntimeHarness(t)
	harness.revision.SetFactoryRevision(4)
	harness.revision.SetFactoryRevision(2)
	if got := harness.revision.CurrentFactoryRevision(); got != 4 {
		t.Fatalf("Factory Runtime revision = %d, want monotonic revision 4", got)
	}

	ctx := context.Background()
	preview, err := harness.capacity.PreviewResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 2,
	})
	if err != nil {
		t.Fatalf("PreviewResourceCapacity: %v", err)
	}
	assertResourceCapacityFactorySnapshot(t, preview, 2, 1)
	previewConfig := decodeResourceCapacityFactorySnapshot(t, preview)
	if previewConfig.Resources[0].Name != "GPU slot" {
		t.Fatalf("preview resource name = %q, want runtime resource name", previewConfig.Resources[0].Name)
	}
}

func TestResourceCapacityRuntimeAdmittedMutationAndNoOp(t *testing.T) {
	harness := newResourceCapacityRuntimeHarness(t)
	ctx := context.Background()
	releaseAdmission, err := harness.admission.AcquireResourceCapacityAdmission(ctx)
	if err != nil {
		t.Fatalf("AcquireResourceCapacityAdmission: %v", err)
	}
	admittedPreview, err := harness.admitted.PreviewResourceCapacityAdmitted(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 3,
	})
	if err != nil {
		releaseAdmission()
		t.Fatalf("PreviewResourceCapacityAdmitted: %v", err)
	}
	if admittedPreview.Outcome != factoryruntime.ResourceCapacityOutcomeApplied {
		releaseAdmission()
		t.Fatalf("admitted preview outcome = %q, want applied", admittedPreview.Outcome)
	}
	updated, err := harness.admitted.SetResourceCapacityAdmitted(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 3,
	})
	releaseAdmission()
	if err != nil {
		t.Fatalf("SetResourceCapacityAdmitted: %v", err)
	}
	assertResourceCapacityFactorySnapshot(t, updated, 3, 1)

	noOp, err := harness.capacity.SetResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 3,
	})
	if err != nil || noOp.Outcome != factoryruntime.ResourceCapacityOutcomeNoOp {
		t.Fatalf("SetResourceCapacity no-op = (%#v, %v), want NO_OP", noOp, err)
	}
}

func TestResourceCapacityRuntimeOrphanMutationAndLease(t *testing.T) {
	harness := newResourceCapacityRuntimeHarness(t)
	harness.revision.SetFactoryRevision(4)
	ctx := context.Background()
	orphan, err := harness.capacity.SetResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "orphan", RequestedCapacity: 2,
	})
	if err != nil {
		t.Fatalf("SetResourceCapacity append: %v", err)
	}
	assertResourceCapacityFactorySnapshot(t, orphan, 2, 2)

	lease, err := harness.leases.AcquireResourceCapacityLease(ctx, factoryruntime.ResourceCapacityLeaseRequest{ResourceID: "orphan"})
	if err != nil {
		t.Fatalf("AcquireResourceCapacityLease: %v", err)
	}
	if lease.FactoryRevision != 4 {
		t.Fatalf("resource lease revision = %d, want 4", lease.FactoryRevision)
	}
	lease.Release()
	lease.Release()
}

func TestResourceCapacityRuntimeSnapshotsRetainEarlierCapacityChanges(t *testing.T) {
	harness := newResourceCapacityRuntimeHarness(t)
	ctx := context.Background()
	if _, err := harness.capacity.SetResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 4,
	}); err != nil {
		t.Fatalf("SetResourceCapacity gpu-slot: %v", err)
	}
	second, err := harness.capacity.SetResourceCapacity(ctx, factoryruntime.ResourceCapacityRequest{
		ResourceID: "orphan", RequestedCapacity: 2,
	})
	if err != nil {
		t.Fatalf("SetResourceCapacity orphan: %v", err)
	}
	config := decodeResourceCapacityFactorySnapshot(t, second)
	capacities := make(map[string]int, len(config.Resources))
	for _, resource := range config.Resources {
		capacities[resource.ID] = resource.Capacity
	}
	if capacities["gpu-slot"] != 4 || capacities["orphan"] != 2 {
		t.Fatalf("effective capacities = %#v, want gpu-slot=4 and orphan=2", capacities)
	}
}

func assertResourceCapacityFactorySnapshot(t *testing.T, result factoryruntime.ResourceCapacityResult, capacity, resourceCount int) {
	t.Helper()
	if result.Outcome == "" {
		t.Fatalf("resource capacity result has no outcome: %#v", result)
	}
	if result.Factory == nil {
		t.Fatal("resource capacity result has no effective Factory snapshot")
	}
	config := decodeResourceCapacityFactorySnapshot(t, result)
	if len(config.Resources) != resourceCount {
		t.Fatalf("effective Factory resources = %d, want %d", len(config.Resources), resourceCount)
	}
	for _, resource := range config.Resources {
		if resource.ID == result.ResourceID && resource.Capacity != capacity {
			t.Fatalf("effective %s capacity = %d, want %d", result.ResourceID, resource.Capacity, capacity)
		}
	}
}

func decodeResourceCapacityFactorySnapshot(t *testing.T, result factoryruntime.ResourceCapacityResult) interfaces.FactoryConfig {
	t.Helper()
	var config interfaces.FactoryConfig
	if err := result.Factory.Decode(&config); err != nil {
		t.Fatalf("decode effective Factory snapshot: %v", err)
	}
	return config
}

func TestNew_ConfiguresProvidedRuntimeAwareScheduler(t *testing.T) {
	net := buildSimpleNet()
	customScheduler := &runtimeAwareScheduler{}
	runtimeCfg := runtimeSchedulerConfig(&runtimefixtures.RuntimeDefinitionLookupFixture{})

	_, err := newTestFactory(
		withNet(net),
		withScheduler(customScheduler),
		withRuntimeConfig(runtimeCfg),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if customScheduler.configured != runtimeCfg {
		t.Fatal("expected New to inject runtime config into provided scheduler")
	}

	var _ scheduler.Scheduler = customScheduler
}

func TestNew_InlineDispatchWithNoopExecutorCompletesWorkflow(t *testing.T) {
	n := buildSimpleNet()
	f, err := newTestFactory(
		withNet(n),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = submitWorkRequests(ctx, f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-1"}})
	}()

	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
}

func TestNew_InlineDispatchExecutorPanicRoutesFailedWork(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &panicExecutor{message: "simulated catastrophic panic"}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     "work-panic",
		WorkTypeID: "task",
		TraceID:    "trace-panic",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, "work-panic", "task:failed") {
		t.Fatalf("expected work-panic to reach task:failed, marking=%#v", snapshot.Marking.PlaceTokens)
	}
	if markingContainsWorkAtPlace(&snapshot.Marking, "work-panic", "task:done") {
		t.Fatal("expected work-panic to avoid task:done after executor panic")
	}
	if len(snapshot.DispatchHistory) != 1 {
		t.Fatalf("dispatch history count = %d, want 1", len(snapshot.DispatchHistory))
	}
	completed := snapshot.DispatchHistory[0]
	if completed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("dispatch outcome = %q, want %q", completed.Outcome, workerexecution.OutcomeFailed)
	}
	if !strings.Contains(completed.Reason, "executor panic:") || !strings.Contains(completed.Reason, "simulated catastrophic panic") {
		t.Fatalf("dispatch reason = %q, want panic-derived failure message", completed.Reason)
	}
}

func TestNew_InlineDispatchWithoutRegisteredExecutorRecordsMissingExecutorFailure(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tickable := tickableFactory(t, f)

	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     "work-missing-executor",
		WorkTypeID: "task",
		TraceID:    "trace-missing-executor",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("dispatch history count = %d, want 1", len(snap.DispatchHistory))
	}
	completed := snap.DispatchHistory[0]
	if completed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("dispatch outcome = %q, want %q", completed.Outcome, workerexecution.OutcomeFailed)
	}
	if !strings.Contains(completed.Reason, `no executor registered for worker type "mock"`) {
		t.Fatalf("dispatch reason = %q, want missing executor error", completed.Reason)
	}
}

func TestNew_CompletesWorkflowThroughActiveSubsystems(t *testing.T) {
	f, ledger := newPassingInlineRuntimeWithLedger(t)
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     "work-active-path",
		WorkTypeID: "task",
		TraceID:    "trace-active-path",
	}}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, "work-active-path", "task:done") {
		t.Fatalf("expected work-active-path to reach task:done, marking=%#v", snapshot.Marking.PlaceTokens)
	}

	if ledger.CallCount("RecordWorkstationRequest") != 1 {
		t.Fatalf("RecordWorkstationRequest calls = %d, want 1", ledger.CallCount("RecordWorkstationRequest"))
	}
	if ledger.CallCount("RecordWorkstationResponse") != 1 {
		t.Fatalf("RecordWorkstationResponse calls = %d, want 1", ledger.CallCount("RecordWorkstationResponse"))
	}
}

func TestNew_InitialStructureIncludesRuntimeConfigWorkerMetadata(t *testing.T) {
	_, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withRuntimeConfig(runtimeProjectionConfig{
			Workers: map[string]*interfaces.FactoryWorkerConfig{
				"mock": {
					Type:             interfaces.WorkerTypeModel,
					ExecutorProvider: "codex-cli",
					ModelProvider:    "openai",
					Model:            "gpt-5.4",
				},
			},
		}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if ledger.CallCount("RecordInitialStructure") != 1 {
		t.Fatalf("RecordInitialStructure calls = %d, want 1", ledger.CallCount("RecordInitialStructure"))
	}
	// Recordings owns the serialized worker metadata assertion in
	// TestFactoryEventHistory_RecordInitialStructure_UsesRuntimeConfigProjection.
}

func TestNew_WithMockExecutor(t *testing.T) {
	if _, err := newTestFactory(withNet(buildSimpleNet()), withWorkerExecutor("mock", &passExecutor{})); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestSubmit_AssignsTraceIDWhenMissing(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tickable := tickableFactory(t, f)
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.Marking.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(snap.Marking.Tokens))
	}
	for _, tok := range snap.Marking.Tokens {
		if tok.Color.TraceID == "" {
			t.Fatal("expected submitted token to have an assigned trace ID")
		}
	}
}

func TestNew_WithClockStampsDispatchesDeterministically(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	clock := platformclock.NewDeterministic(base, time.Second)
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
		withClock(clock),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tickable := tickableFactory(t, f)
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-clock"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("expected 1 completed dispatch, got %d", len(snap.DispatchHistory))
	}
	want := base.Add(time.Second)
	completed := snap.DispatchHistory[0]
	if !completed.StartTime.Equal(want) {
		t.Fatalf("dispatch start = %s, want %s", completed.StartTime, want)
	}
	if !completed.EndTime.Equal(want) {
		t.Fatalf("dispatch end = %s, want %s", completed.EndTime, want)
	}
}
