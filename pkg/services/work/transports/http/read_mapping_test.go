package http

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestListOptionsFromAPI_MapsQueryParameters(t *testing.T) {
	t.Parallel()

	stateType := factoryapi.StateType(work.StateTypeProcessing)
	sortBy := factoryapi.ListWorkBySessionIdParamsSortBy(work.SortByStateType)
	maxResults := factoryapi.MaxResults(25)
	nextToken := factoryapi.NextToken("Y3Vyc29y")
	stateName := factoryapi.StateName("review")
	name := factoryapi.WorkListName("story")
	workTypeName := factoryapi.WorkListWorkTypeName("prd")
	traceID := factoryapi.WorkListTraceId("trace-1")
	terminal := factoryapi.WorkListTerminal(true)
	nonTerminal := factoryapi.WorkListNonTerminal(false)
	counts := factoryapi.WorkListCounts(true)
	includeSuperseded := factoryapi.WorkListIncludeSuperseded(true)

	options, err := ListOptionsFromAPI(factoryapi.ListWorkBySessionIdParams{
		StateName:         &stateName,
		StateType:         &stateType,
		Name:              &name,
		WorkTypeName:      &workTypeName,
		TraceId:           &traceID,
		Terminal:          &terminal,
		NonTerminal:       &nonTerminal,
		SortBy:            &sortBy,
		MaxResults:        &maxResults,
		NextToken:         &nextToken,
		Counts:            &counts,
		IncludeSuperseded: &includeSuperseded,
	})
	if err != nil {
		t.Fatalf("ListOptionsFromAPI: %v", err)
	}
	if options.StateName != "review" ||
		options.StateType != work.StateTypeProcessing ||
		options.Name != "story" ||
		options.WorkTypeName != "prd" ||
		options.TraceID != "trace-1" ||
		!options.Terminal ||
		options.NonTerminal ||
		options.SortBy != work.SortByStateType ||
		options.MaxResults != 25 ||
		options.NextToken != "Y3Vyc29y" ||
		!options.Counts || !options.IncludeSuperseded {
		t.Fatalf("options = %#v, want mapped list query", options)
	}
}

func TestListOptionsFromAPI_RejectsInvalidStateTypeBeforeRoot(t *testing.T) {
	t.Parallel()

	invalid := factoryapi.StateType("RUNNING")
	if _, err := ListOptionsFromAPI(factoryapi.ListWorkBySessionIdParams{
		StateType: &invalid,
	}); err == nil {
		t.Fatal("ListOptionsFromAPI must reject unsupported state.type before root invocation")
	}
}

func TestListOptionsFromAPI_MapsNonTerminalFlag(t *testing.T) {
	t.Parallel()

	nonTerminal := factoryapi.WorkListNonTerminal(true)
	options, err := ListOptionsFromAPI(factoryapi.ListWorkBySessionIdParams{NonTerminal: &nonTerminal})
	if err != nil {
		t.Fatalf("ListOptionsFromAPI: %v", err)
	}
	if !options.NonTerminal || options.Terminal {
		t.Fatalf("options = %#v, want non-terminal-only selection", options)
	}
}

func TestListWorkResponseToAPI_EncodesDetachedReadModels(t *testing.T) {
	t.Parallel()

	response := ListWorkResponseToAPI(work.ListResult{
		Results: []work.ReadModel{{
			CursorID:     "tok-1",
			WorkID:       "work-1",
			Name:         "Review PRD",
			WorkTypeName: "prd",
			State:        &work.State{Name: "init", Type: work.StateTypeInitial},
			SupersededBy: "work-new",
		}},
		MaxResults: 50,
		NextToken:  "next-page",
	})
	if len(response.Results) != 1 ||
		response.Results[0].Name != "Review PRD" ||
		response.PaginationContext == nil ||
		response.PaginationContext.MaxResults != 50 ||
		response.PaginationContext.NextToken == nil ||
		*response.PaginationContext.NextToken != "next-page" {
		t.Fatalf("response = %#v, want encoded list page", response)
	}
	if response.Results[0].SupersededBy == nil || *response.Results[0].SupersededBy != "work-new" {
		t.Fatalf("supersededBy = %#v, want work-new", response.Results[0].SupersededBy)
	}
}

func TestListWorkResponseToAPI_EncodesRequestedCountIncludingZero(t *testing.T) {
	t.Parallel()

	response := ListWorkResponseToAPI(work.ListResult{
		MaxResults: 50,
		Counts:     &work.ListCountSummary{Total: 0},
	})
	if response.Counts == nil || response.Counts.Total != 0 {
		t.Fatalf("counts = %#v, want explicit zero total", response.Counts)
	}
}

func TestWorkReadModelToAPI_EncodesDetachedReadModel(t *testing.T) {
	t.Parallel()

	got := WorkReadModelToAPI(work.ReadModel{
		CursorID:                 "tok-prd-1",
		WorkID:                   "work-prd-1",
		Name:                     "Review PRD",
		WorkTypeName:             "prd",
		State:                    &work.State{Name: "init", Type: work.StateTypeInitial},
		ConfirmationState:        work.ConfirmationStateConfirmed,
		ChainingTraceDepth:       4,
		CurrentChainingTraceID:   "chain-1",
		PreviousChainingTraceIDs: []string{"chain-a", "chain-b"},
		TraceID:                  "trace-1",
		Content: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "Review screenshot"},
		},
		Tags: map[string]string{"owner": "docs"},
		FailureDetail: &work.FailureDetail{
			Reason: "internal_server_error", Message: "repository root is dirty",
		},
		Relations: []work.ReadRelation{{
			Type:           work.RelationDependsOn,
			SourceWorkName: "Review PRD",
			TargetWorkName: "Draft PRD",
			TargetWorkID:   "work-draft",
			RequiredState:  "complete",
		}},
		HumanApproval: &work.HumanApprovalReadModel{
			ApprovalID:      "approval-dispatch-1",
			SessionID:       "session-1",
			DispatchID:      "dispatch-1",
			WorkstationID:   "release-approval",
			WorkstationName: "Release Approval",
			Description:     "Approve the release",
			Decisions:       []string{"APPROVE", "REJECT"},
			Status:          "PENDING",
		},
		ExpectedArtifacts: []work.ExpectedArtifactReadModel{{
			Name: "report", Pattern: "reports/review.json", NonEmpty: true,
			Verification: work.ExpectedArtifactVerificationSatisfied,
		}},
	})
	if got.ConfirmationState == nil || *got.ConfirmationState != factoryapi.CONFIRMED {
		t.Fatalf("confirmationState = %v, want CONFIRMED", got.ConfirmationState)
	}
	assertDetachedWorkFields(t, got)
	assertDetachedWorkCollections(t, got)
	if got.HumanApproval == nil || got.HumanApproval.ApprovalId != "approval-dispatch-1" ||
		got.HumanApproval.SessionId != "session-1" ||
		got.HumanApproval.WorkstationName != "Release Approval" ||
		got.HumanApproval.Description == nil || *got.HumanApproval.Description != "Approve the release" ||
		got.HumanApproval.Status != factoryapi.HumanApprovalStatus("PENDING") ||
		len(got.HumanApproval.Decisions) != 2 {
		t.Fatalf("human approval = %#v, want safe pending approval projection", got.HumanApproval)
	}
	assertExpectedArtifactAPI(t, got)
	if got.FailureDetail == nil || got.FailureDetail.Reason != factoryapi.WorkFailureTypeInternalServerError || got.FailureDetail.Message != "repository root is dirty" {
		t.Fatalf("failure detail = %#v, want mapped current Work failure", got.FailureDetail)
	}
}

func TestWorkReadModelToAPI_PreservesOrderedMixedContentParts(t *testing.T) {
	t.Parallel()

	got := WorkReadModelToAPI(work.ReadModel{
		Content: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "first", Slot: "prompt"},
			{Type: work.WorkContentPartTypeJSON, JSON: json.RawMessage(`{"answer":42}`)},
			{Type: work.WorkContentPartTypeImage, URL: "you-artifact://session-1/image-1"},
		},
	})
	if got.Content == nil || len(*got.Content) != 3 {
		t.Fatalf("content = %#v, want three ordered parts", got.Content)
	}

	textPart, err := (*got.Content)[0].AsWorkTextContentPart()
	if err != nil || textPart.Type != factoryapi.WorkContentPartTypeText || textPart.Text != "first" {
		t.Fatalf("content[0] = %#v (decode error %v), want first text part", textPart, err)
	}

	jsonPart, err := (*got.Content)[1].AsWorkJsonContentPart()
	if err != nil || jsonPart.Type != factoryapi.WorkContentPartTypeJSON {
		t.Fatalf("content[1] = %#v (decode error %v), want JSON part", jsonPart, err)
	}
	jsonValue, ok := jsonPart.Json.(map[string]any)
	if !ok || jsonValue["answer"] != float64(42) {
		t.Fatalf("content[1].json = %#v, want answer=42", jsonPart.Json)
	}

	imagePart, err := (*got.Content)[2].AsWorkImageContentPart()
	if err != nil || imagePart.Type != factoryapi.WorkContentPartTypeImage || imagePart.Url != "you-artifact://session-1/image-1" {
		t.Fatalf("content[2] = %#v (decode error %v), want image artifact reference", imagePart, err)
	}
}

func assertDetachedWorkFields(t *testing.T, got factoryapi.Work) {
	t.Helper()
	if got.Name != "Review PRD" {
		t.Fatalf("Work name = %q, want Review PRD", got.Name)
	}
	if got.WorkId == nil || *got.WorkId != "work-prd-1" {
		t.Fatalf("Work ID = %#v, want work-prd-1", got.WorkId)
	}
	if got.ChainingTraceDepth == nil || *got.ChainingTraceDepth != 4 {
		t.Fatalf("trace depth = %#v, want 4", got.ChainingTraceDepth)
	}
}

func assertDetachedWorkCollections(t *testing.T, got factoryapi.Work) {
	t.Helper()
	if got.Content == nil || len(*got.Content) != 1 {
		t.Fatalf("content = %#v, want one item", got.Content)
	}
	if got.Tags == nil || (*got.Tags)["owner"] != "docs" {
		t.Fatalf("tags = %#v, want owner=docs", got.Tags)
	}
	if got.Relations == nil || len(*got.Relations) != 1 {
		t.Fatalf("relations = %#v, want one item", got.Relations)
	}
}

func assertExpectedArtifactAPI(t *testing.T, got factoryapi.Work) {
	t.Helper()
	if got.ExpectedArtifacts == nil || len(*got.ExpectedArtifacts) != 1 {
		t.Fatalf("expected artifacts = %#v, want one item", got.ExpectedArtifacts)
	}
	artifact := (*got.ExpectedArtifacts)[0]
	if artifact.Pattern != "reports/review.json" || artifact.Verification != factoryapi.WorkExpectedArtifactVerificationSatisfied {
		t.Fatalf("expected artifact = %#v, want encoded artifact projection", artifact)
	}
}

func TestWorkReadModelToAPI_PreservesNativeStructuredResultAndNullPresence(t *testing.T) {
	t.Parallel()

	structured := map[string]any{"nested": map[string]any{"label": "ready"}, "items": []any{float64(1), float64(2)}}
	got := WorkReadModelToAPI(work.ReadModel{
		WorkID:                  "work-structured",
		StructuredResult:        structured,
		StructuredResultPresent: true,
	})
	if !reflect.DeepEqual(got.StructuredResult, structured) {
		t.Fatalf("structured result = %#v, want native object %#v", got.StructuredResult, structured)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal structured Work: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode structured Work: %v", err)
	}
	if _, ok := fields["structuredResult"]; !ok {
		t.Fatalf("structuredResult omitted from %s", encoded)
	}

	nullWork := WorkReadModelToAPI(work.ReadModel{WorkID: "work-null", StructuredResultPresent: true})
	nullEncoded, err := json.Marshal(nullWork)
	if err != nil {
		t.Fatalf("marshal null Work: %v", err)
	}
	var nullFields map[string]json.RawMessage
	if err := json.Unmarshal(nullEncoded, &nullFields); err != nil {
		t.Fatalf("decode null Work: %v", err)
	}
	if string(nullFields["structuredResult"]) != "null" {
		t.Fatalf("structuredResult = %s, want null", nullFields["structuredResult"])
	}
}

func TestWorkReadModelToAPI_EncodesStopSummary(t *testing.T) {
	t.Parallel()

	status := "PAUSED"
	workID := "work-1"
	workName := "Review PRD"
	workType := "prd"
	state := "review"
	got := WorkReadModelToAPI(work.ReadModel{
		CursorID: "tok-1",
		WorkID:   workID,
		Name:     workName,
		StopSummary: &work.StopSummary{
			SessionID:              "session-1",
			StopKind:               "BLOCKED",
			SessionLifecycleStatus: &status,
			WorkID:                 &workID,
			WorkName:               &workName,
			WorkTypeName:           &workType,
			WorkState:              &state,
			LatestDispatch: &work.StopDispatchSummary{
				DispatchID:   "dispatch-1",
				Status:       "FAILED",
				DispatchKind: "WORK",
				FailureDetail: &work.StopFailureDetail{
					Reason:  "PROVIDER_ERROR",
					Message: "provider failed",
				},
			},
		},
	})
	if got.StopSummary == nil ||
		string(got.StopSummary.StopKind) != "BLOCKED" ||
		got.StopSummary.LatestDispatch == nil ||
		got.StopSummary.LatestDispatch.ConfirmationState != factoryapi.UNCONFIRMED ||
		got.StopSummary.LatestDispatch.FailureDetail == nil {
		t.Fatalf("stop summary = %#v, want encoded blocked stop summary", got.StopSummary)
	}
}

func TestWorkReadModelToAPI_OmitsEmptyOptionalCollections(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(WorkReadModelToAPI(work.ReadModel{
		CursorID: "tok-1",
		WorkID:   "work-1",
		Name:     "one",
	}))
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"content", "tags", "relations", "expectedArtifacts", "previousChainingTraceIds", "failureDetail", "stopSummary"} {
		if _, present := fields[field]; present {
			t.Fatalf("optional field %q unexpectedly present: %s", field, string(raw))
		}
	}
}
