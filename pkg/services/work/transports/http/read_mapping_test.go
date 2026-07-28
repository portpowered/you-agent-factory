package http

import (
	"encoding/json"
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

	options, err := ListOptionsFromAPI(factoryapi.ListWorkBySessionIdParams{
		StateName:    &stateName,
		StateType:    &stateType,
		Name:         &name,
		WorkTypeName: &workTypeName,
		TraceId:      &traceID,
		SortBy:       &sortBy,
		MaxResults:   &maxResults,
		NextToken:    &nextToken,
	})
	if err != nil {
		t.Fatalf("ListOptionsFromAPI: %v", err)
	}
	if options.StateName != "review" ||
		options.StateType != work.StateTypeProcessing ||
		options.Name != "story" ||
		options.WorkTypeName != "prd" ||
		options.TraceID != "trace-1" ||
		options.SortBy != work.SortByStateType ||
		options.MaxResults != 25 ||
		options.NextToken != "Y3Vyc29y" {
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

func TestListWorkResponseToAPI_EncodesDetachedReadModels(t *testing.T) {
	t.Parallel()

	response := ListWorkResponseToAPI(work.ListResult{
		Results: []work.ReadModel{{
			CursorID:     "tok-1",
			WorkID:       "work-1",
			Name:         "Review PRD",
			WorkTypeName: "prd",
			State:        &work.State{Name: "init", Type: work.StateTypeInitial},
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
}

func TestWorkReadModelToAPI_EncodesDetachedReadModel(t *testing.T) {
	t.Parallel()

	got := WorkReadModelToAPI(work.ReadModel{
		CursorID:                 "tok-prd-1",
		WorkID:                   "work-prd-1",
		Name:                     "Review PRD",
		WorkTypeName:             "prd",
		State:                    &work.State{Name: "init", Type: work.StateTypeInitial},
		ChainingTraceDepth:       4,
		CurrentChainingTraceID:   "chain-1",
		PreviousChainingTraceIDs: []string{"chain-a", "chain-b"},
		TraceID:                  "trace-1",
		Content: []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "Review screenshot"},
		},
		Tags: map[string]string{"owner": "docs"},
		Relations: []work.ReadRelation{{
			Type:           work.RelationDependsOn,
			SourceWorkName: "Review PRD",
			TargetWorkName: "Draft PRD",
			TargetWorkID:   "work-draft",
			RequiredState:  "complete",
		}},
	})
	if got.Name != "Review PRD" ||
		got.WorkId == nil || *got.WorkId != "work-prd-1" ||
		got.ChainingTraceDepth == nil || *got.ChainingTraceDepth != 4 ||
		got.Content == nil || len(*got.Content) != 1 ||
		got.Tags == nil || (*got.Tags)["owner"] != "docs" ||
		got.Relations == nil || len(*got.Relations) != 1 {
		t.Fatalf("Work = %#v, want encoded detached read model", got)
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
	for _, field := range []string{"content", "tags", "relations", "previousChainingTraceIds", "stopSummary"} {
		if _, present := fields[field]; present {
			t.Fatalf("optional field %q unexpectedly present: %s", field, string(raw))
		}
	}
}
