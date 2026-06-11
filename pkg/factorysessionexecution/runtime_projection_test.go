package factorysessionexecution_test

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestRuntimeService_ReadProjections_MatchAPIShapedExpectations(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "simple-final.workflow.js", "simple-final")

	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-projection-complete",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   3,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Status != factorysessionexecution.LifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", read.Status)
	}
	if read.SourceHash == "" || read.Policy.EffectiveHash == "" {
		t.Fatal("expected source and policy hashes on session read")
	}
	if read.ResultSummary == nil || read.ResultSummary.ResultStatus != string(factorysessionexecution.ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", read.ResultSummary)
	}
	if read.Progress == nil {
		t.Fatal("expected progress projection")
	}
	if read.Links.Session == "" || read.Links.Results == "" {
		t.Fatal("expected inspection links on session read")
	}

	mappedSession := factorysession.SessionReadResponseToAPI(read)
	if mappedSession.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("mapped status = %q, want SUCCEEDED", mappedSession.Status)
	}
	if mappedSession.SourceHash == nil || *mappedSession.SourceHash == "" {
		t.Fatal("mapped sourceHash missing")
	}
	if mappedSession.EffectivePolicyHash == nil || *mappedSession.EffectivePolicyHash == "" {
		t.Fatal("mapped effectivePolicyHash missing")
	}
	if mappedSession.ResultSummary == nil || mappedSession.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("mapped resultSummary = %#v, want FINAL", mappedSession.ResultSummary)
	}

	listSummary := factorysessionexecution.DurableListSummaryFromSessionRead(read)
	if listSummary.Actions.CanCancel {
		t.Fatal("terminal session should not allow cancel")
	}
	if listSummary.Recoverable {
		t.Fatal("succeeded session should not be recoverable")
	}
	if listSummary.Links.Session != read.Links.Session {
		t.Fatalf("list summary link = %q, want %q", listSummary.Links.Session, read.Links.Session)
	}

	result, err := service.GetResult(context.Background(), completed.SessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != factorysessionexecution.ResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	if err := factorysessionexecution.ValidateResultMatchesSessionRead(read, result); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}

	mappedResult := factorysession.ResultResponseToAPI(result)
	if mappedResult.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("mapped resultStatus = %q, want FINAL", mappedResult.ResultStatus)
	}
	if mappedResult.PrimaryResult == nil || len(*mappedResult.PrimaryResult) != 1 {
		t.Fatalf("mapped primaryResult = %#v, want one work content part", mappedResult.PrimaryResult)
	}
	jsonPart, err := (*mappedResult.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("primary result json part: %v", err)
	}
	payload, ok := jsonPart.Json.(map[string]any)
	if !ok {
		t.Fatalf("primary json payload = %#v, want object", jsonPart.Json)
	}
	if payload["echo"] != "you:workflows" {
		t.Fatalf("echo = %#v, want you:workflows", payload["echo"])
	}

	events, err := service.ReadEvents(context.Background(), completed.SessionID, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if err := factorysessionexecution.ValidateResultMatchesEventProjection(result, events.Events); err != nil {
		t.Fatalf("ValidateResultMatchesEventProjection: %v", err)
	}

	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if err := factorysessionexecution.ValidateDispatchListMatchesSessionProgress(read, dispatches.Dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}
}

func TestRuntimeService_GetResult_RunningSessionReportsAvailability(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")

	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-projection-running",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	result, err := service.GetResult(context.Background(), started.SessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != factorysessionexecution.ResultStatusNotReady {
		t.Fatalf("resultStatus = %q, want NOT_READY", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("availability = %#v, want RESULT_NOT_READY", result.Availability)
	}
	if err := factorysessionexecution.ValidateResultMatchesSessionRead(read, result); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}

	mapped := factorysession.ResultResponseToAPI(result)
	if mapped.Availability == nil || mapped.Availability.Reason == nil || *mapped.Availability.Reason != "RESULT_NOT_READY" {
		t.Fatalf("mapped availability = %#v, want RESULT_NOT_READY", mapped.Availability)
	}
}

func TestRuntimeService_GetResult_SyncTimeoutReportsAvailability(t *testing.T) {
	_, service := newRuntimeServiceWithFixture(t, "busy-loop.workflow.js", "busy-loop")
	timeoutMillis := int64(25)

	timedOut, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-runtime-projection-timeout",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "busy-loop",
		},
		Wait: &factorysessionexecution.WaitOptions{
			TimeoutMillis: &timeoutMillis,
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	result, err := service.GetResult(context.Background(), timedOut.SessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != factorysessionexecution.ResultStatusNotReady {
		t.Fatalf("resultStatus = %q, want NOT_READY", result.ResultStatus)
	}
	if result.Availability == nil || result.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("availability = %#v, want SYNC_WAIT_TIMED_OUT", result.Availability)
	}

	mapped := factorysession.ResultResponseToAPI(result)
	if mapped.Availability == nil || mapped.Availability.Reason == nil || *mapped.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("mapped availability = %#v, want SYNC_WAIT_TIMED_OUT", mapped.Availability)
	}
}
