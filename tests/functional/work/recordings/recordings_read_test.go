package workrecordings_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestRecordingsBackedWorkReadsUseRecordingsRootContract proves functional
// coverage for CUT-WORK-REC: leased Work state_access reads construct Recordings
// queries through the published Recordings service root when no live session is
// available.
func TestRecordingsBackedWorkReadsUseRecordingsRootContract(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-functional"}
	events := []recordings.CanonicalEvent{{
		ID:          "event-1",
		Sequence:    0,
		FactoryTick: 3,
		Scope:       scope,
		Kind:        recordings.CanonicalEventKind(interfaces.FactoryEventTypeWorkRequest),
		Payload:     `{"requestId":"request-1"}`,
	}}
	fake := &functionalRecordingsRootFake{
		events: events,
		worldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Scope:         scope,
			SelectedTick:  3,
			Payload:       recordingsBackedWorldPayload(t, "work-story", "story", "review"),
		},
	}
	ctx := context.Background()
	svc := workwire.RecordingsStateAccessService(fake)

	list, err := svc.ListWork(
		ctx,
		"session-recordings-functional",
		work.ListOptions{},
	)
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(list.Results) != 1 || list.Results[0].WorkID != "work-story" {
		t.Fatalf("ListWork = %#v, want one story work item", list)
	}
	if len(fake.subscribeRequests) < 1 || len(fake.reconstructRequests) < 1 {
		t.Fatalf("subscribe=%d reconstruct=%d, want at least 1 each",
			len(fake.subscribeRequests), len(fake.reconstructRequests))
	}
}

func TestGetWorkFromRecordingsRootUsesRecordingsServiceRoot(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-functional"}
	events := []recordings.CanonicalEvent{
		{
			ID:          "event-1",
			Sequence:    0,
			FactoryTick: 1,
			Scope:       scope,
			Kind:        recordings.CanonicalEventKind(interfaces.FactoryEventTypeWorkRequest),
		},
		{
			ID:          "event-2",
			Sequence:    1,
			FactoryTick: 5,
			Scope:       scope,
			Kind:        recordings.CanonicalEventKind(interfaces.FactoryEventTypeWorkRequest),
		},
	}
	fake := &functionalRecordingsRootFake{
		events: events,
		worldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Scope:         scope,
			SelectedTick:  5,
			Payload:       recordingsBackedRichWorldPayload(t),
		},
	}

	svc := workwire.RecordingsStateAccessService(fake)

	got, err := svc.GetWork(
		context.Background(),
		"session-recordings-functional",
		"work-active",
	)
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if got.WorkID != "work-active" || got.State == nil || got.State.Type != work.StateTypeProcessing {
		t.Fatalf("GetWork = %#v, want active processing work item", got)
	}
	if len(fake.reconstructRequests) != 1 || fake.reconstructRequests[0].SelectedTick != 5 {
		t.Fatalf("reconstruct requests = %#v, want highest tick 5", fake.reconstructRequests)
	}
}

func TestRecordingsBackedWorkReadsMapRichWorldState(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-recordings-functional"}
	fake := &functionalRecordingsRootFake{
		events: []recordings.CanonicalEvent{{
			ID:          "event-1",
			FactoryTick: 2,
			Scope:       scope,
		}},
		worldState: recordings.WorldStateView{
			SchemaVersion: recordings.WorldStateViewSchemaV1,
			Payload:       recordingsBackedRichWorldPayload(t),
		},
	}

	svc := workwire.RecordingsStateAccessService(fake)

	list, err := svc.ListWork(
		context.Background(),
		"session-recordings-functional",
		work.ListOptions{WorkTypeName: "story", MaxResults: 2},
	)
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(list.Results) != 2 || list.NextToken == "" {
		t.Fatalf("ListWork = %#v, want paginated story items", list)
	}

	byID := make(map[string]work.ReadModel, len(list.Results))
	for _, item := range list.Results {
		byID[item.WorkID] = item
	}
	if byID["work-active"].State == nil || byID["work-active"].State.Type != work.StateTypeProcessing {
		t.Fatalf("active work state = %#v, want processing", byID["work-active"].State)
	}
	if len(byID["work-active"].Relations) != 1 || byID["work-active"].Relations[0].TargetWorkName != "Terminal story" {
		t.Fatalf("relations = %#v, want one depends_on relation", byID["work-active"].Relations)
	}

	failed, err := svc.GetWork(
		context.Background(),
		"session-recordings-functional",
		"work-failed",
	)
	if err != nil {
		t.Fatalf("GetWork(failed): %v", err)
	}
	if failed.State == nil || failed.State.Type != work.StateTypeFailed {
		t.Fatalf("failed work state = %#v, want failed", failed.State)
	}
}

func TestRecordingsBackedWorkReadsSurfaceTypedProjectionFailures(t *testing.T) {
	t.Parallel()

	svc := workwire.RecordingsStateAccessService(
		&functionalRecordingsRootFake{reconstructErr: recordings.ErrInvalidProjectionInput},
	)

	_, err := svc.ListWork(
		context.Background(),
		"session-recordings-functional",
		work.ListOptions{},
	)
	if err == nil {
		t.Fatal("ListWork error = nil, want typed projection failure")
	}
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ListWork error = %v, want ErrInvalidProjectionInput", err)
	}
}

func TestRecordingsBackedWorkReadsRejectInvalidWorldStateViews(t *testing.T) {
	t.Parallel()

	for name, view := range map[string]recordings.WorldStateView{
		"unsupported schema": {SchemaVersion: "v0", Payload: `{}`},
		"empty payload":      {SchemaVersion: recordings.WorldStateViewSchemaV1, Payload: "  "},
		"invalid payload":    {SchemaVersion: recordings.WorldStateViewSchemaV1, Payload: "{not-json"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			svc := workwire.RecordingsStateAccessService(&functionalRecordingsRootFake{worldState: view})
			_, err := svc.ListWork(context.Background(), "session-recordings-functional", work.ListOptions{})
			if err == nil || !errors.Is(err, recordings.ErrUnsupportedProjectionView) && !errors.Is(err, recordings.ErrInvalidProjectionInput) {
				t.Fatalf("ListWork error = %v, want typed invalid projection error", err)
			}
		})
	}
}

func TestRecordingsBackedWorkReadsProjectDispatchArtifactFacts(t *testing.T) {
	t.Parallel()

	scope := recordings.CanonicalEventScope{FactorySessionID: "session-artifacts-functional"}
	state := recordingsArtifactProjectionWorldState()
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	fake := &functionalRecordingsRootFake{
		events:     []recordings.CanonicalEvent{{Scope: scope, FactoryTick: 9}},
		worldState: recordings.WorldStateView{SchemaVersion: recordings.WorldStateViewSchemaV1, Scope: scope, Payload: string(payload)},
	}
	list, err := workwire.RecordingsStateAccessService(fake).ListWork(context.Background(), scope.FactorySessionID, work.ListOptions{MaxResults: 100})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	byID := make(map[string]work.ReadModel, len(list.Results))
	for _, read := range list.Results {
		byID[read.WorkID] = read
	}
	assertArtifactDispatchProjection(t, byID)
}

func recordingsArtifactProjectionWorldState() interfaces.FactoryWorldState {
	topology := recordingsArtifactProjectionTopology()
	active := recordingsArtifactProjectionItem("active", "Active story", "story", "review")
	activeInput := recordingsArtifactProjectionItem("active-input", "Active input", "story", "review")
	completed := recordingsArtifactProjectionItem("completed", "Completed story", "story", "review")
	completedOutput := recordingsArtifactProjectionItem("completed-output", "Completed output", "story", "review")
	failed := recordingsArtifactProjectionItem("failed", "Failed story", "story", "review")
	detail := recordingsArtifactProjectionItem("detail", "Detail story", "story", "review")
	noVerification := recordingsArtifactProjectionItem("no-verification", "No verification", "story", "review")
	fallback := recordingsArtifactProjectionItem("fallback", "Fallback story", "story", "review")
	plain := recordingsArtifactProjectionItem("plain", "Plain work", "plain", "review")
	emptyState := recordingsArtifactProjectionItem("empty-state", "Empty state", "story", "")
	return interfaces.FactoryWorldState{
		Topology: topology,
		WorkItemsByID: map[string]work.FactoryWorkItem{
			active.ID: active, activeInput.ID: activeInput, completed.ID: completed,
			completedOutput.ID: completedOutput, failed.ID: failed, detail.ID: detail,
			noVerification.ID: noVerification, fallback.ID: fallback, plain.ID: plain,
			emptyState.ID: emptyState,
		},
		ActiveWorkItemsByID: map[string]work.FactoryWorkItem{active.ID: active, activeInput.ID: activeInput},
		FailedWorkItemsByID: map[string]work.FactoryWorkItem{failed.ID: failed, detail.ID: detail},
		TerminalWorkByID: map[string]interfaces.FactoryTerminalWork{
			completed.ID: {WorkItem: work.FactoryWorkItem{ID: completed.ID, WorkTypeID: "story", State: "done"}},
		},
		RelationsByWorkID: map[string][]work.FactoryRelation{
			plain.ID: {{Type: "depends_on", TargetWorkID: "", RequiredState: "done"}},
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"active-direct": {
				TransitionID: "publish", Workstation: interfaces.FactoryWorkstationRef{ID: "publish", Name: "publish"},
				WorkItemIDs: []string{active.ID}, ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{Project: "active-project"},
			},
			"active-input": {
				TransitionID: "publish-name", Workstation: interfaces.FactoryWorkstationRef{Name: "publish"},
				Inputs: []interfaces.WorkstationInput{{}, {WorkItem: &work.FactoryWorkItem{ID: activeInput.ID, DisplayName: "Input source", WorkTypeID: "story"}}},
			},
			"active-skip": {WorkItemIDs: []string{"unrelated"}},
		},
		CompletedDispatches: recordingsArtifactCompletedDispatches(completed, completedOutput),
		FailedDispatches:    recordingsArtifactFailedDispatches(failed, noVerification),
		FailureDetailsByWorkID: map[string]interfaces.FactoryWorldFailureDetail{
			detail.ID: {
				TransitionID: "unknown", WorkstationName: "publish", WorkItem: detail,
				ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{Project: "detail-project"},
				ArtifactVerification: &workerexecution.ExpectedArtifactVerification{Entries: []workerexecution.ExpectedArtifactVerificationEntry{{
					DeclarationIndex: 2, Name: "manifest", Pattern: "detail-project/manifests/Detail story.json", Reason: workerexecution.ExpectedArtifactVerificationReasonMissing,
				}}},
			},
		},
	}
}

func recordingsArtifactProjectionTopology() interfaces.InitialStructurePayload {
	storyStates := []interfaces.FactoryStateDefinition{
		{Value: "review", Category: work.StateTypeProcessing}, {Value: "done", Category: work.StateTypeTerminal},
	}
	return interfaces.InitialStructurePayload{
		WorkTypes: []interfaces.FactoryWorkType{
			{ID: "story", Name: "story", States: storyStates, ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{
				Name: "report", Pattern: "reports/{{ (index .Inputs 0).Name }}/report.txt", NonEmpty: true,
			}}},
			{ID: "other", Name: "other", States: storyStates},
		},
		Workstations: []interfaces.FactoryWorkstation{
			{ID: "ignore", Name: "ignore", InputPlaceIDs: []string{"other:review"}},
			{ID: "publish", Name: "publish", InputPlaceIDs: []string{"story:review"}, ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{
				Name: "manifest", Pattern: "{{ .Context.Project }}/manifests/{{ (index .Inputs 0).Name }}.json",
			}}},
		},
	}
}

func recordingsArtifactProjectionItem(id, displayName, workType, state string) work.FactoryWorkItem {
	return work.FactoryWorkItem{ID: id, DisplayName: displayName, WorkTypeID: workType, State: state, PlaceID: workType + ":" + state, Tags: map[string]string{"project": "input-project"}}
}

func recordingsArtifactCompletedDispatches(completed, completedOutput work.FactoryWorkItem) []interfaces.FactoryWorldDispatchCompletion {
	return []interfaces.FactoryWorldDispatchCompletion{
		{TransitionID: "publish-name", Workstation: interfaces.FactoryWorkstationRef{Name: "publish"}, InputWorkItems: []work.FactoryWorkItem{completed}, ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{SessionID: "session-completed"}, Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)}},
		{TransitionID: "publish-name", Workstation: interfaces.FactoryWorkstationRef{Name: "publish"}, OutputWorkItems: []work.FactoryWorkItem{completedOutput}, Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeAccepted)}},
		{WorkItemIDs: []string{"completed-skip"}},
	}
}

func recordingsArtifactFailedDispatches(failed, noVerification work.FactoryWorkItem) []interfaces.FactoryWorldDispatchCompletion {
	return []interfaces.FactoryWorldDispatchCompletion{
		{TransitionID: "unknown", Workstation: interfaces.FactoryWorkstationRef{Name: "publish"}, WorkItemIDs: []string{failed.ID}, Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeFailed), ArtifactVerification: &workerexecution.ExpectedArtifactVerification{Entries: []workerexecution.ExpectedArtifactVerificationEntry{{
			DeclarationIndex: 2, Name: "manifest", Pattern: "default-project/manifests/Failed story.json", Reason: workerexecution.ExpectedArtifactVerificationReasonEmpty,
		}}}}},
		{TransitionID: "missing", Workstation: interfaces.FactoryWorkstationRef{Name: "missing"}, WorkItemIDs: []string{noVerification.ID}, Result: interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeFailed)}},
	}
}

func assertArtifactDispatchProjection(t *testing.T, byID map[string]work.ReadModel) {
	t.Helper()
	if got := byID["active"].ExpectedArtifacts; len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationPending {
		t.Fatalf("active artifact projection = %#v, want pending effective declarations", got)
	}
	if got := byID["active-input"].ExpectedArtifacts; len(got) != 2 || got[0].Pattern != "reports/Input source/report.txt" {
		t.Fatalf("active input artifact projection = %#v, want input-derived pattern", got)
	}
	for _, id := range []string{"completed", "completed-output"} {
		if got := byID[id].ExpectedArtifacts; len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationSatisfied || got[1].Verification != work.ExpectedArtifactVerificationSatisfied {
			t.Fatalf("completed artifact projection for %q = %#v, want satisfied declarations", id, got)
		}
	}
	if got := byID["failed"].ExpectedArtifacts; len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationSatisfied || got[1].Verification != work.ExpectedArtifactVerificationFailed {
		t.Fatalf("failed artifact projection = %#v, want mixed satisfied/failed declarations", got)
	}
	if got := byID["detail"].ExpectedArtifacts; len(got) != 2 || got[1].Verification != work.ExpectedArtifactVerificationFailed || got[1].Reason == nil || *got[1].Reason != work.ExpectedArtifactVerificationReasonMissing {
		t.Fatalf("failure detail artifact projection = %#v, want recorded missing declaration", got)
	}
	if got := byID["fallback"].ExpectedArtifacts; len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationPending {
		t.Fatalf("fallback artifact projection = %#v, want topology-derived pending declarations", got)
	}
	if byID["empty-state"].State != nil || len(byID["plain"].Relations) != 1 || byID["plain"].Relations[0].TargetWorkName != "" {
		t.Fatalf("edge state/relation projection = empty=%#v plain=%#v", byID["empty-state"].State, byID["plain"].Relations)
	}
}

type functionalRecordingsRootFake struct {
	events              []recordings.CanonicalEvent
	worldState          recordings.WorldStateView
	reconstructErr      error
	subscribeRequests   []recordings.SubscribeRequest
	reconstructRequests []recordings.ReconstructWorldStateRequest
}

func (fake *functionalRecordingsRootFake) Append(
	recordings.AppendRecordedEventRequest,
) (recordings.AppendRecordedEventResult, error) {
	return recordings.AppendRecordedEventResult{}, recordings.ErrInvalidAppendEvent
}

func (fake *functionalRecordingsRootFake) SubscribeFrom(
	_ context.Context,
	request recordings.SubscribeRequest,
) (recordings.SubscribeResult, error) {
	fake.subscribeRequests = append(fake.subscribeRequests, request)
	events := make([]recordings.CanonicalEvent, 0, len(fake.events))
	for _, event := range fake.events {
		if event.Scope == request.Scope {
			events = append(events, event)
		}
	}
	index := 0
	return recordings.SubscribeResult{
		Subscription: func(context.Context) recordings.SubscriptionOutcome {
			if index >= len(events) {
				return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
			}
			outcome := recordings.SubscriptionOutcome{
				Kind:  recordings.SubscriptionEvent,
				Event: events[index],
			}
			index++
			return outcome
		},
	}, nil
}

func (fake *functionalRecordingsRootFake) ReconstructWorldState(
	request recordings.ReconstructWorldStateRequest,
) (recordings.ReconstructWorldStateResult, error) {
	fake.reconstructRequests = append(fake.reconstructRequests, request)
	if fake.reconstructErr != nil {
		return recordings.ReconstructWorldStateResult{}, fake.reconstructErr
	}
	return recordings.ReconstructWorldStateResult{WorldState: fake.worldState}, nil
}

func (fake *functionalRecordingsRootFake) QuerySimpleDashboard(
	recordings.SimpleDashboardQueryRequest,
) (recordings.SimpleDashboardQueryResult, error) {
	return recordings.SimpleDashboardQueryResult{}, recordings.ErrInvalidProjectionInput
}

func (fake *functionalRecordingsRootFake) QueryWorkstationRequests(
	recordings.WorkstationRequestsQueryRequest,
) (recordings.WorkstationRequestsQueryResult, error) {
	return recordings.WorkstationRequestsQueryResult{}, recordings.ErrInvalidProjectionInput
}

func (fake *functionalRecordingsRootFake) ValidateReconnectReplayFrom(
	recordings.ValidateReconnectReplayRequest,
) error {
	return recordings.ErrInvalidProjectionInput
}

func (fake *functionalRecordingsRootFake) BindRecording(
	recordings.BindRecordingRequest,
) (recordings.BindRecordingResult, error) {
	return recordings.BindRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *functionalRecordingsRootFake) StartRecording(
	recordings.StartRecordingRequest,
) (recordings.StartRecordingResult, error) {
	return recordings.StartRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *functionalRecordingsRootFake) RecordRecordingEvent(
	recordings.RecordRecordingEventRequest,
) (recordings.RecordRecordingEventResult, error) {
	return recordings.RecordRecordingEventResult{}, recordings.ErrInvalidRecordingEvent
}

func (fake *functionalRecordingsRootFake) RecordRecordingError(
	recordings.RecordRecordingErrorRequest,
) (recordings.RecordRecordingErrorResult, error) {
	return recordings.RecordRecordingErrorResult{}, recordings.ErrInvalidRecordingFailure
}

func (fake *functionalRecordingsRootFake) FlushRecording(
	recordings.FlushRecordingRequest,
) (recordings.FlushRecordingResult, error) {
	return recordings.FlushRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *functionalRecordingsRootFake) StopRecording(
	recordings.StopRecordingRequest,
) (recordings.StopRecordingResult, error) {
	return recordings.StopRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *functionalRecordingsRootFake) FinishRecording(
	recordings.FinishRecordingRequest,
) (recordings.FinishRecordingResult, error) {
	return recordings.FinishRecordingResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *functionalRecordingsRootFake) QueryRecordingStatus(
	recordings.RecordingStatusRequest,
) (recordings.RecordingStatusResult, error) {
	return recordings.RecordingStatusResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *functionalRecordingsRootFake) LoadReplayRecording(
	recordings.LoadReplayRecordingRequest,
) (recordings.LoadReplayRecordingResult, error) {
	return recordings.LoadReplayRecordingResult{}, recordings.ErrMissingReplayArtifact
}

func (fake *functionalRecordingsRootFake) CreateReplayPlan(
	recordings.CreateReplayPlanRequest,
) (recordings.CreateReplayPlanResult, error) {
	return recordings.CreateReplayPlanResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *functionalRecordingsRootFake) ObserveReplay(
	recordings.ObserveReplayRequest,
) (recordings.ObserveReplayResult, error) {
	return recordings.ObserveReplayResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *functionalRecordingsRootFake) BuildPortableArtifact(
	recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	return recordings.BuildPortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *functionalRecordingsRootFake) ValidatePortableArtifact(
	recordings.ValidatePortableArtifactRequest,
) (recordings.ValidatePortableArtifactResult, error) {
	return recordings.ValidatePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *functionalRecordingsRootFake) EncodePortableArtifact(
	recordings.EncodePortableArtifactRequest,
) (recordings.EncodePortableArtifactResult, error) {
	return recordings.EncodePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *functionalRecordingsRootFake) DecodePortableArtifact(
	recordings.DecodePortableArtifactRequest,
) (recordings.DecodePortableArtifactResult, error) {
	return recordings.DecodePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *functionalRecordingsRootFake) SummarizePortableArtifact(
	recordings.SummarizePortableArtifactRequest,
) (recordings.SummarizePortableArtifactResult, error) {
	return recordings.SummarizePortableArtifactResult{}, recordings.ErrInvalidReplayArtifact
}

func (fake *functionalRecordingsRootFake) ExportPortableArtifact(
	context.Context,
	recordings.ExportPortableArtifactRequest,
) (recordings.ExportPortableArtifactResult, error) {
	return recordings.ExportPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func (fake *functionalRecordingsRootFake) ReadPortableArtifact(
	context.Context,
	recordings.ReadPortableArtifactRequest,
) (recordings.ReadPortableArtifactResult, error) {
	return recordings.ReadPortableArtifactResult{}, recordings.ErrMissingRecordingTarget
}

func recordingsBackedRichWorldPayload(t *testing.T) string {
	t.Helper()
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
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	return string(payload)
}

func recordingsBackedWorldPayload(
	t *testing.T,
	workID string,
	workTypeID string,
	stateName string,
) string {
	t.Helper()
	state := interfaces.FactoryWorldState{
		Topology: interfaces.InitialStructurePayload{
			WorkTypes: []interfaces.FactoryWorkType{{
				ID: workTypeID,
				States: []interfaces.FactoryStateDefinition{
					{Value: "init", Category: work.StateTypeInitial},
					{Value: "review", Category: work.StateTypeProcessing},
				},
			}},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			workID: {
				ID:          workID,
				WorkTypeID:  workTypeID,
				DisplayName: "Review PRD",
				State:       stateName,
			},
		},
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal world state: %v", err)
	}
	return string(payload)
}
