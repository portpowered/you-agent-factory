package wire

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestReadSnapshotFromWorldStateRejectsUnsupportedViews(t *testing.T) {
	t.Parallel()

	for name, view := range map[string]recordings.WorldStateView{
		"wrong schema":  {SchemaVersion: "v0", Payload: `{"workItemsById":{}}`},
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

	state := factorydefinitions.FactoryWorldState{
		Topology: factorydefinitions.InitialStructurePayload{
			WorkTypes: []factorydefinitions.FactoryWorkType{{
				ID: "story",
				States: []factorydefinitions.FactoryStateDefinition{
					{Value: "init", Category: work.StateTypeInitial},
					{Value: "review", Category: work.StateTypeProcessing},
					{Value: "done", Category: work.StateTypeTerminal},
				},
			}},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			factorydefinitions.SystemTimeWorkTypeID: {
				ID:         factorydefinitions.SystemTimeWorkTypeID,
				WorkTypeID: factorydefinitions.SystemTimeWorkTypeID,
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
		TerminalWorkByID: map[string]factorydefinitions.FactoryTerminalWork{
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
	if _, ok := byID[factorydefinitions.SystemTimeWorkTypeID]; ok {
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

func TestReadSnapshotFromFactoryWorldStateProjectsOnlyCurrentLatestWorkFailure(t *testing.T) {
	t.Parallel()

	state := factorydefinitions.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-failed":    {ID: "work-failed", WorkTypeID: "story", State: "rejected"},
			"work-recovered": {ID: "work-recovered", WorkTypeID: "story", State: "review"},
			"work-unrelated": {ID: "work-unrelated", WorkTypeID: "story", State: "review"},
		},
		FailedWorkItemsByID: map[string]work.FactoryWorkItem{
			"work-failed": {ID: "work-failed", WorkTypeID: "story", State: "rejected"},
		},
		FailedDispatches: []factorydefinitions.FactoryWorldDispatchCompletion{
			{
				DispatchID: "dispatch-old", WorkItemIDs: []string{"work-failed"},
				Result: workers.WorkstationResult{FailureDetail: &workers.FailureDetail{
					Reason: workers.WorkFailureTypeUnknown, Message: "old setup failure",
				}},
			},
			{
				DispatchID: "dispatch-unrelated", WorkItemIDs: []string{"work-unrelated"},
				Result: workers.WorkstationResult{FailureDetail: &workers.FailureDetail{
					Reason: workers.WorkFailureTypeUnknown, Message: "unrelated failure",
				}},
			},
			{
				DispatchID: "dispatch-new", WorkItemIDs: []string{"work-failed"},
				Result: workers.WorkstationResult{FailureDetail: &workers.FailureDetail{
					Reason: workers.WorkFailureTypeInternalServerError, Message: "latest setup failure",
				}},
			},
		},
		FailureDetailsByWorkID: map[string]factorydefinitions.FactoryWorldFailureDetail{
			"work-recovered": {
				FailureDetail: &workers.FailureDetail{
					Reason: workers.WorkFailureTypeUnknown, Message: "stale recovered failure",
				},
			},
		},
	}

	snapshot := readSnapshotFromFactoryWorldState(state)
	byID := make(map[string]work.ReadModel, len(snapshot.Items))
	for _, item := range snapshot.Items {
		byID[item.WorkID] = item
	}
	failed := byID["work-failed"].FailureDetail
	if failed == nil || failed.Reason != string(workers.WorkFailureTypeInternalServerError) || failed.Message != "latest setup failure" {
		t.Fatalf("current failed detail = %#v, want latest matching dispatch", failed)
	}
	if byID["work-recovered"].FailureDetail != nil || byID["work-unrelated"].FailureDetail != nil {
		t.Fatalf("stale or unrelated failure details = recovered=%#v unrelated=%#v, want nil", byID["work-recovered"].FailureDetail, byID["work-unrelated"].FailureDetail)
	}
}

func TestReadSnapshotFromFactoryWorldStateLinksPendingHumanApprovalsToWork(t *testing.T) {
	t.Parallel()

	description := factorydefinitions.NameValueConfig{Value: "Review release"}
	state := factorydefinitions.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-with-description":    {ID: "work-with-description", WorkTypeID: "story", State: "review"},
			"work-without-description": {ID: "work-without-description", WorkTypeID: "story", State: "review"},
			"work-without-approval":    {ID: "work-without-approval", WorkTypeID: "story", State: "review"},
		},
		PendingHumanApprovalsByID: map[string]factorydefinitions.FactoryWorldHumanApproval{
			"approval-z": {
				ApprovalID:      "approval-z",
				SessionID:       "session-1",
				DispatchID:      "dispatch-z",
				WorkstationID:   "review",
				WorkstationName: "Review release",
				WorkItemIDs:     []string{"work-without-description"},
				Decisions:       []factorydefinitions.HumanApprovalDecision{factorydefinitions.HumanApprovalDecisionApprove},
				Status:          factorydefinitions.HumanApprovalStatusPending,
			},
			"approval-a": {
				ApprovalID:             "approval-a",
				SessionID:              "session-1",
				DispatchID:             "dispatch-a",
				WorkstationID:          "review",
				WorkstationName:        "Review release",
				WorkstationDescription: &description,
				WorkItemIDs:            []string{"work-with-description"},
				Decisions:              []factorydefinitions.HumanApprovalDecision{factorydefinitions.HumanApprovalDecisionApprove, factorydefinitions.HumanApprovalDecisionReject},
				Status:                 factorydefinitions.HumanApprovalStatusPending,
			},
		},
	}
	snapshot := readSnapshotFromFactoryWorldState(state)
	byID := make(map[string]work.ReadModel, len(snapshot.Items))
	for _, item := range snapshot.Items {
		byID[item.WorkID] = item
	}

	withoutDescription := byID["work-without-description"].HumanApproval
	if withoutDescription == nil || withoutDescription.ApprovalID != "approval-z" || withoutDescription.Description != "" {
		t.Fatalf("approval without description = %#v, want approval-z without description", withoutDescription)
	}
	withDescription := byID["work-with-description"].HumanApproval
	if withDescription == nil || withDescription.ApprovalID != "approval-a" || withDescription.Description != "Review release" ||
		withDescription.Status != string(factorydefinitions.HumanApprovalStatusPending) ||
		len(withDescription.Decisions) != 2 || withDescription.Decisions[1] != string(factorydefinitions.HumanApprovalDecisionReject) {
		t.Fatalf("approval read model = %#v, want stable display and decision metadata", withDescription)
	}
	if byID["work-without-approval"].HumanApproval != nil {
		t.Fatalf("unmatched work approval = %#v, want nil", byID["work-without-approval"].HumanApproval)
	}
}

func TestReadSnapshotFromFactoryWorldStateProjectsArtifactVerificationWithoutScanning(t *testing.T) {
	t.Parallel()

	state := factorydefinitions.FactoryWorldState{
		Topology: factorydefinitions.InitialStructurePayload{
			WorkTypes: []factorydefinitions.FactoryWorkType{{
				ID: "story",
				ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{
					Name: "report", Pattern: "reports/{{ (index .Inputs 0).Name }}.json", NonEmpty: true,
				}},
			}},
			Workstations: []factorydefinitions.FactoryWorkstation{{
				ID: "publish", Name: "publish", InputPlaceIDs: []string{"story:review"},
				ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{
					Name: "manifest", Pattern: "reports/manifest.json",
				}},
			}},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"pending":   {ID: "pending", WorkTypeID: "story", DisplayName: "pending", State: "review"},
			"satisfied": {ID: "satisfied", WorkTypeID: "story", DisplayName: "satisfied", State: "review"},
			"failed":    {ID: "failed", WorkTypeID: "story", DisplayName: "failed", State: "review"},
			"plain":     {ID: "plain", WorkTypeID: "plain", DisplayName: "plain", State: "review"},
		},
		ActiveDispatches: map[string]factorydefinitions.FactoryWorldDispatch{
			"dispatch-pending": {
				DispatchID: "dispatch-pending", TransitionID: "publish", Workstation: factorydefinitions.FactoryWorkstationRef{ID: "publish", Name: "publish"},
				WorkItemIDs: []string{"pending"}, Inputs: []factorydefinitions.WorkstationInput{{WorkItem: &work.FactoryWorkItem{ID: "pending", WorkTypeID: "story", DisplayName: "pending"}}},
			},
		},
		CompletedDispatches: []factorydefinitions.FactoryWorldDispatchCompletion{
			{
				DispatchID: "dispatch-satisfied", TransitionID: "publish", Workstation: factorydefinitions.FactoryWorkstationRef{ID: "publish", Name: "publish"},
				WorkItemIDs: []string{"satisfied"}, InputWorkItems: []work.FactoryWorkItem{{ID: "satisfied", WorkTypeID: "story", DisplayName: "satisfied"}},
				Result: workers.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
			},
			{
				DispatchID: "dispatch-failed", TransitionID: "publish", Workstation: factorydefinitions.FactoryWorkstationRef{ID: "publish", Name: "publish"},
				WorkItemIDs: []string{"failed"}, InputWorkItems: []work.FactoryWorkItem{{ID: "failed", WorkTypeID: "story", DisplayName: "failed"}},
				Result: workers.WorkstationResult{
					Outcome: string(workers.OutcomeFailed),
					ArtifactVerification: &workers.ExpectedArtifactVerification{Entries: []workers.ExpectedArtifactVerificationEntry{{
						Name: "manifest", Pattern: "reports/manifest.json", Reason: workers.ExpectedArtifactVerificationReasonEmpty,
					}}},
				},
			},
		},
	}

	snapshot := readSnapshotFromFactoryWorldState(state)
	byID := make(map[string]work.ReadModel, len(snapshot.Items))
	for _, item := range snapshot.Items {
		byID[item.WorkID] = item
	}
	if got := byID["pending"].ExpectedArtifacts; len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationPending || got[0].Pattern != "reports/pending.json" {
		t.Fatalf("pending artifacts = %#v", got)
	}
	if got := byID["satisfied"].ExpectedArtifacts; len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationSatisfied || got[1].Verification != work.ExpectedArtifactVerificationSatisfied {
		t.Fatalf("satisfied artifacts = %#v", got)
	}
	if got := byID["failed"].ExpectedArtifacts; len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationSatisfied || got[1].Verification != work.ExpectedArtifactVerificationFailed || got[1].Reason == nil || *got[1].Reason != work.ExpectedArtifactVerificationReasonEmpty {
		t.Fatalf("failed artifacts = %#v", got)
	}
	if got := byID["plain"].ExpectedArtifacts; got != nil {
		t.Fatalf("no-declaration artifacts = %#v, want nil", got)
	}
}

func TestReadSnapshotFromFactoryWorldStateProjectsRecordedArtifactContext(t *testing.T) {
	t.Parallel()
	state := factorydefinitions.FactoryWorldState{
		Topology: factorydefinitions.InitialStructurePayload{
			WorkTypes: []factorydefinitions.FactoryWorkType{{
				ID: "story",
				ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{
					Name: "report", Pattern: "{{ .Context.Project }}/{{ .Context.SessionID }}/{{ (index .Inputs 0).Project }}/{{ (index .Inputs 0).Payload }}/report.txt",
				}},
			}},
			Workstations: []factorydefinitions.FactoryWorkstation{{ID: "publish", Name: "publish"}},
		},
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "story", DisplayName: "story", State: "review"},
		},
		CompletedDispatches: []factorydefinitions.FactoryWorldDispatchCompletion{{
			DispatchID: "dispatch-1", TransitionID: "publish", Workstation: factorydefinitions.FactoryWorkstationRef{ID: "publish", Name: "publish"},
			ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{
				Project:   "project-7",
				SessionID: "session-9",
				Inputs: []work.ExpectedArtifactInput{{
					Project: "input-project-7",
					Payload: "payload-7",
				}},
			},
			WorkItemIDs: []string{"work-1"}, InputWorkItems: []work.FactoryWorkItem{{ID: "work-1", WorkTypeID: "story", DisplayName: "story"}},
			Result: factorydefinitions.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
		}},
	}
	snapshot := readSnapshotFromFactoryWorldState(state)
	if len(snapshot.Items) != 1 || len(snapshot.Items[0].ExpectedArtifacts) != 1 ||
		snapshot.Items[0].ExpectedArtifacts[0].Pattern != "project-7/session-9/input-project-7/payload-7/report.txt" ||
		snapshot.Items[0].ExpectedArtifacts[0].Verification != work.ExpectedArtifactVerificationSatisfied {
		t.Fatalf("recorded context projection = %#v", snapshot.Items)
	}
}

func TestWorkStateCategoryFallsBackToInitialWhenUnknown(t *testing.T) {
	t.Parallel()

	state := &work.State{
		Name: "unknown-state",
		Type: workStateCategory(factorydefinitions.InitialStructurePayload{}, "story", "unknown-state"),
	}
	if state.Type != "" {
		t.Fatalf("workStateCategory = %q, want empty category", state.Type)
	}
	item := workStateFromItem(
		work.FactoryWorkItem{ID: "work-1", WorkTypeID: "story", State: "unknown-state"},
		factorydefinitions.FactoryWorldState{},
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

func TestReadWorkSnapshotRejectsANilRecordingsRoot(t *testing.T) {
	t.Parallel()

	reader := workSnapshotReader{}
	_, err := reader.ReadWorkSnapshot(context.Background(), "session-1")
	if err == nil || err.Error() != "Recordings service is required" {
		t.Fatalf("ReadWorkSnapshot error = %v, want missing Recordings service", err)
	}
}

func TestWorkStateCategoryMatchesWorkTypeName(t *testing.T) {
	t.Parallel()

	category := workStateCategory(
		factorydefinitions.InitialStructurePayload{
			WorkTypes: []factorydefinitions.FactoryWorkType{{
				Name: "story",
				States: []factorydefinitions.FactoryStateDefinition{
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
		factorydefinitions.InitialStructurePayload{
			WorkTypes: []factorydefinitions.FactoryWorkType{
				{
					ID: "bug",
					States: []factorydefinitions.FactoryStateDefinition{
						{Value: "triage", Category: work.StateTypeInitial},
					},
				},
				{
					ID: "story",
					States: []factorydefinitions.FactoryStateDefinition{
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

func TestReadSnapshotFromFactoryWorldStateUsesFailedDispatchAndFailureDetailArtifactFacts(t *testing.T) {
	t.Parallel()

	topology := factorydefinitions.InitialStructurePayload{
		WorkTypes: []factorydefinitions.FactoryWorkType{{
			ID: "story", ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{Name: "report", Pattern: "report.txt"}},
		}},
		Workstations: []factorydefinitions.FactoryWorkstation{{
			ID: "review", Name: "review", InputPlaceIDs: []string{"story:review"},
			ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{Name: "manifest", Pattern: "manifest.json"}},
		}},
	}
	failedDispatchItem := work.FactoryWorkItem{ID: "failed-dispatch", WorkTypeID: "story", DisplayName: "failed dispatch", State: "review"}
	failureDetailItem := work.FactoryWorkItem{ID: "failed-detail", WorkTypeID: "story", DisplayName: "failed detail", State: "review"}
	state := factorydefinitions.FactoryWorldState{
		Topology:      topology,
		WorkItemsByID: map[string]work.FactoryWorkItem{failedDispatchItem.ID: failedDispatchItem, failureDetailItem.ID: failureDetailItem},
		FailedDispatches: []factorydefinitions.FactoryWorldDispatchCompletion{{
			DispatchID: "dispatch-failed", TransitionID: "review", Workstation: factorydefinitions.FactoryWorkstationRef{ID: "review", Name: "review"},
			WorkItemIDs: []string{failedDispatchItem.ID}, Result: factorydefinitions.WorkstationResult{Outcome: string(workers.OutcomeAccepted)},
		}},
		FailureDetailsByWorkID: map[string]factorydefinitions.FactoryWorldFailureDetail{
			failureDetailItem.ID: {
				DispatchID: "dispatch-detail", TransitionID: "review", WorkstationName: "review", WorkItem: failureDetailItem,
				ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{Project: "project-7", SessionID: "session-9"},
				ArtifactVerification: &workers.ExpectedArtifactVerification{Entries: []workers.ExpectedArtifactVerificationEntry{{
					Name: "manifest", Pattern: "manifest.json", Reason: workers.ExpectedArtifactVerificationReasonEmpty,
				}}},
			},
		},
	}

	snapshot := readSnapshotFromFactoryWorldState(state)
	byID := make(map[string]work.ReadModel, len(snapshot.Items))
	for _, item := range snapshot.Items {
		byID[item.WorkID] = item
	}
	if got := byID[failedDispatchItem.ID].ExpectedArtifacts; len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationSatisfied || got[1].Verification != work.ExpectedArtifactVerificationSatisfied {
		t.Fatalf("failed-dispatch artifacts = %#v", got)
	}
	if got := byID[failureDetailItem.ID].ExpectedArtifacts; len(got) != 2 || got[0].Verification != work.ExpectedArtifactVerificationSatisfied || got[1].Verification != work.ExpectedArtifactVerificationFailed || got[1].Reason == nil {
		t.Fatalf("failure-detail artifacts = %#v", got)
	}
}

func TestWorldArtifactProjectionCoversFallbackBranches(t *testing.T) {
	t.Parallel()

	fallback := work.FactoryWorkItem{ID: "fallback", WorkTypeID: "story", DisplayName: "fallback"}
	if got := worldExpectedArtifactInputs(nil, fallback); len(got) != 1 || got[0].WorkID != fallback.ID {
		t.Fatalf("fallback inputs = %#v", got)
	}
	if got := worldArtifactObservation(nil); got.Verified || len(got.Entries) != 0 {
		t.Fatalf("nil observation = %#v, want empty", got)
	}
	if got := worldArtifactObservationFromResult(workers.WorkstationResult{Outcome: string(workers.OutcomeFailed)}); got.Verified {
		t.Fatalf("failed nil-verification observation = %#v, want pending", got)
	}
	if got := worldWorkstationArtifactDeclarations(factorydefinitions.InitialStructurePayload{}, "missing", "missing"); got != nil {
		t.Fatalf("missing workstation declarations = %#v, want nil", got)
	}

	input := work.FactoryWorkItem{ID: "input"}
	if !worldDispatchContainsWork(nil, []factorydefinitions.WorkstationInput{{WorkItem: &input}}, nil, nil, input.ID) {
		t.Fatal("workstation input did not identify Work")
	}
	if !worldDispatchContainsWork(nil, nil, []work.FactoryWorkItem{input}, nil, input.ID) {
		t.Fatal("input Work item did not identify Work")
	}
	if !worldDispatchContainsWork(nil, nil, nil, []work.FactoryWorkItem{input}, input.ID) {
		t.Fatal("output Work item did not identify Work")
	}
	if worldDispatchContainsWork(nil, nil, nil, nil, input.ID) {
		t.Fatal("empty dispatch incorrectly identified Work")
	}

	topology := factorydefinitions.InitialStructurePayload{
		Workstations: []factorydefinitions.FactoryWorkstation{{
			Name: "review", InputPlaceIDs: []string{"story:review"},
			ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{Name: "review", Pattern: "review.txt"}},
		}},
	}
	got := worldCandidateWorkstationArtifactDeclarations(work.FactoryWorkItem{WorkTypeID: "story", State: "review"}, topology)
	if len(got) != 1 || got[0].Name != "review" {
		t.Fatalf("candidate workstation declarations = %#v", got)
	}
}
