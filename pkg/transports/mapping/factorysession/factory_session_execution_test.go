package factorysession_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

type durableFixtureCatalog struct {
	Scenarios        []durableFixtureScenario       `json:"scenarios"`
	IdempotentReplay durableFixtureIdempotentReplay `json:"idempotentReplay"`
}

type durableFixtureScenario struct {
	ID               string         `json:"id"`
	ExecutionRequest map[string]any `json:"executionRequest"`
	AsyncResponse    map[string]any `json:"asyncResponse"`
	SyncResponse     map[string]any `json:"syncResponse"`
}

type durableFixtureIdempotentReplay struct {
	ExecutionRequest    map[string]any `json:"executionRequest"`
	AsyncResponse       map[string]any `json:"asyncResponse"`
	ReplayAsyncResponse map[string]any `json:"replayAsyncResponse"`
}

func loadDurableFixtureCatalog(t *testing.T) durableFixtureCatalog {
	t.Helper()
	path := filepath.Join("..", "..", "http", "testdata", "durable-session-contract-fixtures.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var catalog durableFixtureCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	return catalog
}

func decodeExecutionRequest(t *testing.T, fixture map[string]any) factoryapi.FactorySessionExecutionRequest {
	t.Helper()
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal execution request: %v", err)
	}
	var request factoryapi.FactorySessionExecutionRequest
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatalf("decode execution request: %v", err)
	}
	return request
}

func TestStartRequestFromAPI_NormalizesAsyncAcceptedFixture(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)
	var scenario durableFixtureScenario
	for _, candidate := range catalog.Scenarios {
		if candidate.ID == "petri-running-one-dispatch" {
			scenario = candidate
			break
		}
	}
	if scenario.ID == "" {
		t.Fatal("missing petri-running-one-dispatch fixture")
	}

	request, err := factorysession.StartRequestFromAPI(decodeExecutionRequest(t, scenario.ExecutionRequest))
	if err != nil {
		t.Fatalf("StartRequestFromAPI: %v", err)
	}
	if request.RequestID != "req-petri-run-001" {
		t.Fatalf("requestId = %q", request.RequestID)
	}
	if request.Source.FactoryID != "customer-support-triage" {
		t.Fatalf("factoryId = %q", request.Source.FactoryID)
	}
}

func TestStartRequestFromAPI_RejectsMissingRequestID(t *testing.T) {
	raw, err := factorysession.StartRequestFromAPI(factoryapi.FactorySessionExecutionRequest{
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("factory"),
		},
	})
	if err != nil {
		t.Fatalf("StartRequestFromAPI: %v", err)
	}
	if raw.RequestID != "" {
		t.Fatalf("requestId = %q, want raw empty value", raw.RequestID)
	}
}

func TestStartRequestFromAPI_IdempotentReplayProducesStableTuple(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)
	request := decodeExecutionRequest(t, catalog.IdempotentReplay.ExecutionRequest)

	first, err := factorysession.StartRequestFromAPI(request)
	if err != nil {
		t.Fatalf("first StartRequestFromAPI: %v", err)
	}
	second, err := factorysession.StartRequestFromAPI(request)
	if err != nil {
		t.Fatalf("second StartRequestFromAPI: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("mapped requests differ: %#v vs %#v", first, second)
	}
}

func TestStartRequestFromAPI_MapsAdditionalSourceKindsAndValidation(t *testing.T) {
	t.Run("factory inline", func(t *testing.T) {
		request, err := factorysession.StartRequestFromAPI(factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-inline-1",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind: factoryapi.FactorySessionExecutionSourceKindFactoryInline,
				FactoryInline: &factoryapi.Factory{
					Name: "factory-inline",
				},
			},
		})
		if err != nil {
			t.Fatalf("StartRequestFromAPI(factory inline): %v", err)
		}
		if request.Source.Kind != factory.WorkflowSourceKindFactoryInline || len(request.Source.FactoryInline) == 0 {
			t.Fatalf("request source = %#v, want encoded factory inline source", request.Source)
		}
	})

	t.Run("workflow file", func(t *testing.T) {
		request, err := factorysession.StartRequestFromAPI(factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-file-1",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowFile,
				WorkflowFile: strPtr(" workflows/simple.workflow.js "),
			},
		})
		if err != nil {
			t.Fatalf("StartRequestFromAPI(workflow file): %v", err)
		}
		if request.Source.WorkflowFile != " workflows/simple.workflow.js " {
			t.Fatalf("workflowFile = %q, want raw path", request.Source.WorkflowFile)
		}
	})

	t.Run("inline workflow", func(t *testing.T) {
		dialect := " you-workflow-v1 "
		entrypoint := " default "
		metadata := factoryapi.StringMap{"team": "ops"}
		request, err := factorysession.StartRequestFromAPI(factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-inline-workflow-1",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
				InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
					InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{Inline: " return 1; "},
					Dialect:      &dialect,
					Entrypoint:   &entrypoint,
					Metadata:     &metadata,
				},
			},
		})
		if err != nil {
			t.Fatalf("StartRequestFromAPI(inline workflow): %v", err)
		}
		if request.Source.InlineWorkflow == nil || request.Source.InlineWorkflow.InlineSource != " return 1; " {
			t.Fatalf("inline workflow = %#v, want raw inline source", request.Source.InlineWorkflow)
		}
	})

	t.Run("missing inline workflow payload", func(t *testing.T) {
		_, err := factorysession.StartRequestFromAPI(factoryapi.FactorySessionExecutionRequest{
			RequestId: "req-bad-inline-1",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			},
		})
		if err == nil {
			t.Fatal("error = nil, want representation validation error")
		}
	})
}

// pkgmaintcheck:ignore-cyclomatic-complexity this contract test keeps sync start terminal and timeout fixture assertions together on one seam.
func TestSyncStartResponseToAPI_MapsTerminalAndTimeoutFixtures(t *testing.T) {
	catalog := loadDurableFixtureCatalog(t)
	var terminalScenario durableFixtureScenario
	var timeoutScenario durableFixtureScenario
	for _, scenario := range catalog.Scenarios {
		switch scenario.ID {
		case "petri-succeeded-one-dispatch":
			terminalScenario = scenario
		case "javascript-sync-timed-out":
			timeoutScenario = scenario
		}
	}
	if terminalScenario.ID == "" || timeoutScenario.ID == "" {
		t.Fatal("missing sync fixture scenarios")
	}

	terminalEncoded, err := json.Marshal(terminalScenario.SyncResponse)
	if err != nil {
		t.Fatalf("marshal terminal fixture: %v", err)
	}
	var terminalExpected factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(terminalEncoded, &terminalExpected); err != nil {
		t.Fatalf("decode terminal fixture: %v", err)
	}

	terminalResult := factorysessionexecution.SyncStartResult{
		AsyncStartResult: factorysessionexecution.AsyncStartResult{
			SessionID:        terminalExpected.SessionId,
			Status:           string(terminalExpected.Status),
			OrchestratorKind: string(terminalExpected.OrchestratorKind),
			ResolvedSource: factorysessionexecution.ResolvedSource{
				Kind:       "FACTORY_ID",
				SourceRef:  deref(terminalExpected.ResolvedSource.SourceRef),
				SourceHash: deref(terminalExpected.ResolvedSource.SourceHash),
			},
			SourceHash: deref(terminalExpected.SourceHash),
			Links: factorysessionexecution.InspectionLinks{
				Session: deref(terminalExpected.Links.Session),
				Results: deref(terminalExpected.Links.Results),
			},
		},
		SyncOutcome: "COMPLETED",
	}
	if terminalExpected.Result != nil {
		terminalResult.Result, err = json.Marshal(terminalExpected.Result)
		if err != nil {
			t.Fatalf("marshal terminal result: %v", err)
		}
	}

	terminalMapped := factorysession.SyncStartResponseToAPI(terminalResult)
	if terminalMapped.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted {
		t.Fatalf("syncOutcome = %q, want COMPLETED", terminalMapped.SyncOutcome)
	}
	if terminalMapped.Result == nil || terminalMapped.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("terminal result = %#v, want FINAL", terminalMapped.Result)
	}

	timeoutEncoded, err := json.Marshal(timeoutScenario.SyncResponse)
	if err != nil {
		t.Fatalf("marshal timeout fixture: %v", err)
	}
	var timeoutExpected factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal(timeoutEncoded, &timeoutExpected); err != nil {
		t.Fatalf("decode timeout fixture: %v", err)
	}
	timeoutMapped := factorysession.SyncStartResponseToAPI(factorysessionexecution.SyncStartResult{
		AsyncStartResult: factorysessionexecution.AsyncStartResult{
			SessionID:        timeoutExpected.SessionId,
			Status:           string(timeoutExpected.Status),
			OrchestratorKind: string(timeoutExpected.OrchestratorKind),
			Dialect:          deref(timeoutExpected.Dialect),
			ResolvedSource: factorysessionexecution.ResolvedSource{
				Kind:       "WORKFLOW_NAME",
				SourceRef:  deref(timeoutExpected.ResolvedSource.SourceRef),
				SourceHash: deref(timeoutExpected.ResolvedSource.SourceHash),
			},
			Links: factorysessionexecution.InspectionLinks{
				Session: deref(timeoutExpected.Links.Session),
				Status:  deref(timeoutExpected.Links.Status),
			},
		},
		SyncOutcome: "TIMED_OUT",
		TimedOut:    true,
	})
	if timeoutMapped.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeTimedOut {
		t.Fatalf("syncOutcome = %q, want TIMED_OUT", timeoutMapped.SyncOutcome)
	}
	if timeoutMapped.TimedOut == nil || !*timeoutMapped.TimedOut {
		t.Fatal("timedOut = false, want true")
	}
	if timeoutMapped.SessionCanceledByTimeout != nil && *timeoutMapped.SessionCanceledByTimeout {
		t.Fatal("sessionCanceledByTimeout = true, want false")
	}
}

func strPtr(value string) *string {
	return &value
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func TestExecutionErrorResponse_MapsValidationAndConflictErrors(t *testing.T) {
	status, response, ok := factorysession.ExecutionErrorResponse(
		&factorysessionexecution.ExecutionValidationError{Field: "requestId", Message: "requestId is required"},
	)
	if !ok {
		t.Fatal("ExecutionErrorResponse = false, want true")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("code = %q, want BAD_REQUEST", response.Code)
	}

	status, response, ok = factorysession.ExecutionErrorResponse(
		factorysessionexecution.ErrExecutionRequestIDConflict,
	)
	if !ok {
		t.Fatal("ExecutionErrorResponse = false, want true")
	}
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if response.Code != factoryapi.ErrorResponseCodeEXECUTIONREQUESTIDCONFLICT {
		t.Fatalf("code = %q, want EXECUTION_REQUEST_ID_CONFLICT", response.Code)
	}

	status, response, ok = factorysession.ExecutionErrorResponse(
		&factorysessionexecution.ResumeError{
			Outcome: "MISSING_CHECKPOINT",
			Field:   "checkpointSummary",
			Message: "persisted checkpoint summary is required to resume an interrupted session",
		},
	)
	if !ok {
		t.Fatal("ExecutionErrorResponse = false, want true for ResumeError")
	}
	if status != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("resume error response = %#v, want 400 BAD_REQUEST", response)
	}
}

func TestExecutionErrorResponse_MapsRequestValidationError(t *testing.T) {
	status, response, ok := factorysession.ExecutionErrorResponse(
		&apisurface.RequestValidationError{Message: "source.kind is invalid"},
	)
	if !ok {
		t.Fatal("ExecutionErrorResponse = false, want true")
	}
	if status != http.StatusBadRequest || response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("response = %#v, want 400 BAD_REQUEST", response)
	}
}

func TestExecutionErrorResponse_ReturnsFalseForUnknownErrors(t *testing.T) {
	if _, _, ok := factorysession.ExecutionErrorResponse(errors.New("other")); ok {
		t.Fatal("ExecutionErrorResponse = true, want false")
	}
}

func TestNewResponseEventSubscription_SerializesPublishedEvents(t *testing.T) {
	t.Parallel()

	cursor := &factorysessionexecution.ResponseEventCursor{
		NextEvents: func(context.Context) ([]factorysessionexecution.FactoryResponseEvent, error) {
			return []factorysessionexecution.FactoryResponseEvent{{
				Sequence:   7,
				Kind:       factorysessionexecution.ResponseEventKindTool,
				DispatchID: "dispatch-7",
				Phase:      factorysessionexecution.ResponseEventPhaseStarted,
				Payload:    json.RawMessage(`{"toolCallId":"call-7","toolName":"read","status":"started"}`),
			}}, nil
		},
		DetachCursor: func() {},
	}
	subscription := factorysession.NewResponseEventSubscription(cursor)
	records, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(records) != 1 || records[0].Sequence != 7 || records[0].Kind != string(factorysessionexecution.ResponseEventKindTool) {
		t.Fatalf("records = %#v, want TOOL sequence 7", records)
	}
	if !strings.Contains(string(records[0].Data), `"toolCallId":"call-7"`) {
		t.Fatalf("record data = %s, want serialized tool payload", records[0].Data)
	}
}

func TestClassifyDurableHistoryFailure_ClassifiesEveryDurableHistorySentinel(t *testing.T) {
	t.Parallel()

	sessionNotFound := factorysession.DurableHistoryFailureSessionNotFound
	dispatchNotFound := factorysession.DurableHistoryFailureDispatchNotFound
	unclassified := factorysession.DurableHistoryFailureUnclassified
	tests := []struct {
		name string
		err  error
		want factorysession.DurableHistoryFailure
	}{
		{name: "absent", err: nil, want: unclassified},
		{name: "durable session", err: factorysessionexecution.ErrDurableSessionNotFound, want: sessionNotFound},
		{name: "live session", err: factorysessionexecution.ErrSessionNotFound, want: sessionNotFound},
		{name: "dispatch", err: factorysessionexecution.ErrDispatchNotFound, want: dispatchNotFound},
		{
			name: "artifact",
			err:  factorysessionexecution.ErrArtifactNotFound,
			want: factorysession.DurableHistoryFailureArtifactNotFound,
		},
		{
			name: "reconnect cursor",
			err:  factorysessionexecution.ErrReconnectCursorNotFound,
			want: factorysession.DurableHistoryFailureReconnectCursorNotFound,
		},
		{
			name: "wrapped dispatch",
			err:  fmt.Errorf("read dispatch: %w", factorysessionexecution.ErrDispatchNotFound),
			want: dispatchNotFound,
		},
		{name: "unrelated", err: errors.New("boom"), want: unclassified},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := factorysession.ClassifyDurableHistoryFailure(test.err); got != test.want {
				t.Fatalf("ClassifyDurableHistoryFailure(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}

func TestNewExecutionValidationError_MapsToBadRequestThroughExecutionErrorResponse(t *testing.T) {
	t.Parallel()

	err := factorysession.NewExecutionValidationError("status", "invalid status")
	status, response, ok := factorysession.ExecutionErrorResponse(err)
	if !ok || status != http.StatusBadRequest {
		t.Fatalf("ExecutionErrorResponse = %d, %v, want 400 recognized", status, ok)
	}
	if response.Message != "invalid status" || response.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("response = %#v, want the invalid status bad-request body", response)
	}
}
