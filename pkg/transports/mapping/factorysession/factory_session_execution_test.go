package factorysession_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestDurableAPIListRequiresExecutionService(t *testing.T) {
	t.Parallel()
	api := factorysession.NewDurableAPI(nil, nil)
	if _, err := api.ListDurableFactorySessions(context.Background(), factoryapi.ListFactorySessionsParams{}); err == nil {
		t.Fatal("ListDurableFactorySessions succeeded without an execution service")
	}
}

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
	_, err := factorysession.StartRequestFromAPI(factoryapi.FactorySessionExecutionRequest{
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("factory"),
		},
	})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *apisurface.RequestValidationError
	if !errors.As(err, &validationErr) {
		var domainErr *factorysessionexecution.ValidationError
		if !errors.As(err, &domainErr) {
			t.Fatalf("error = %T, want validation error", err)
		}
	}
}

func TestStartRequestFromCLI_NormalizesFixtureBackedRequest(t *testing.T) {
	request, err := factorysession.StartRequestFromCLI(factorysession.CLIStartInput{
		RequestID: "req-petri-success-001",
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	if err != nil {
		t.Fatalf("StartRequestFromCLI: %v", err)
	}
	if request.RequestID != "req-petri-success-001" {
		t.Fatalf("requestId = %q", request.RequestID)
	}
	if request.Source.FactoryID != "customer-support-triage" {
		t.Fatalf("factoryId = %q", request.Source.FactoryID)
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

	firstHash, err := factorysessionexecution.IdempotencyTupleHash(first)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	secondHash, err := factorysessionexecution.IdempotencyTupleHash(second)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("idempotent tuple hash mismatch: %q vs %q", firstHash, secondHash)
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
		if request.Source.Kind != workflowsource.KindFactoryInline || len(request.Source.FactoryInline) == 0 {
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
		if request.Source.WorkflowFile != "workflows/simple.workflow.js" {
			t.Fatalf("workflowFile = %q, want trimmed path", request.Source.WorkflowFile)
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
		if request.Source.InlineWorkflow == nil || request.Source.InlineWorkflow.InlineSource != "return 1;" {
			t.Fatalf("inline workflow = %#v, want trimmed inline source", request.Source.InlineWorkflow)
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
			t.Fatal("error = nil, want request validation error")
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
		SyncOutcome: factorysessionexecution.SyncOutcomeCompleted,
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
		SyncOutcome: factorysessionexecution.SyncOutcomeTimedOut,
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

func TestEventReconnectRequestFromCLI_MapsAfterEventIDAndSequence(t *testing.T) {
	sequence := 3
	req, err := factorysession.EventReconnectRequestFromCLI(factorysession.CLIEventReconnectInput{
		AfterEventID:  " session-started/dur-sess-js-run-n-001 ",
		AfterSequence: &sequence,
	})
	if err != nil {
		t.Fatalf("EventReconnectRequestFromCLI: %v", err)
	}
	if req.AfterEventID != "session-started/dur-sess-js-run-n-001" {
		t.Fatalf("afterEventId = %q", req.AfterEventID)
	}
	if req.AfterSequence == nil || *req.AfterSequence != 3 {
		t.Fatalf("afterSequence = %#v, want 3", req.AfterSequence)
	}
}

func TestResultRequestFromCLI_MapsModeAndIncludeArtifacts(t *testing.T) {
	req, err := factorysession.ResultRequestFromCLI(factorysession.CLIResultInput{
		Mode:             "partial",
		IncludeArtifacts: true,
	})
	if err != nil {
		t.Fatalf("ResultRequestFromCLI: %v", err)
	}
	if req.Mode != factorysessionexecution.ResultModePartial {
		t.Fatalf("mode = %q, want partial", req.Mode)
	}
	if !req.IncludeArtifacts {
		t.Fatal("includeArtifacts = false, want true")
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
		factorysessionexecution.NewValidationError("requestId", "requestId is required"),
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
			Outcome: factorysessionexecution.ResumeOutcomeMissingCheckpoint,
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
