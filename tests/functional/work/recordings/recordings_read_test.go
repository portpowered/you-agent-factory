package workrecordings_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/stateaccessrecordings"
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

	list, err := stateaccessrecordings.ListWorkFromRecordingsRoot(
		ctx,
		"session-recordings-functional",
		fake,
		work.ListOptions{},
	)
	if err != nil {
		t.Fatalf("ListWorkFromRecordingsRoot: %v", err)
	}
	if len(list.Results) != 1 || list.Results[0].WorkID != "work-story" {
		t.Fatalf("ListWorkFromRecordingsRoot = %#v, want one story work item", list)
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

	got, err := stateaccessrecordings.GetWorkFromRecordingsRoot(
		context.Background(),
		"session-recordings-functional",
		"work-active",
		fake,
	)
	if err != nil {
		t.Fatalf("GetWorkFromRecordingsRoot: %v", err)
	}
	if got.WorkID != "work-active" || got.State == nil || got.State.Type != work.StateTypeProcessing {
		t.Fatalf("GetWorkFromRecordingsRoot = %#v, want active processing work item", got)
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

	list, err := stateaccessrecordings.ListWorkFromRecordingsRoot(
		context.Background(),
		"session-recordings-functional",
		fake,
		work.ListOptions{WorkTypeName: "story", MaxResults: 2},
	)
	if err != nil {
		t.Fatalf("ListWorkFromRecordingsRoot: %v", err)
	}
	if len(list.Results) != 2 || list.NextToken == "" {
		t.Fatalf("ListWorkFromRecordingsRoot = %#v, want paginated story items", list)
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

	failed, err := stateaccessrecordings.GetWorkFromRecordingsRoot(
		context.Background(),
		"session-recordings-functional",
		"work-failed",
		fake,
	)
	if err != nil {
		t.Fatalf("GetWorkFromRecordingsRoot(failed): %v", err)
	}
	if failed.State == nil || failed.State.Type != work.StateTypeFailed {
		t.Fatalf("failed work state = %#v, want failed", failed.State)
	}
}

func TestRecordingsBackedWorkReadsSurfaceTypedProjectionFailures(t *testing.T) {
	t.Parallel()

	_, err := stateaccessrecordings.ListWorkFromRecordingsRoot(
		context.Background(),
		"session-recordings-functional",
		&functionalRecordingsRootFake{reconstructErr: recordings.ErrInvalidProjectionInput},
		work.ListOptions{},
	)
	if err == nil {
		t.Fatal("ListWorkFromRecordingsRoot error = nil, want typed projection failure")
	}
	if !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("ListWorkFromRecordingsRoot error = %v, want ErrInvalidProjectionInput", err)
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
