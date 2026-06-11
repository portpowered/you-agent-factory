package factorysession_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

type durableFixtureCatalog struct {
	Scenarios        []durableFixtureScenario       `json:"scenarios"`
	IdempotentReplay durableFixtureIdempotentReplay `json:"idempotentReplay"`
}

type durableFixtureScenario struct {
	ID                string         `json:"id"`
	ExecutionRequest  map[string]any `json:"executionRequest"`
	AsyncResponse     map[string]any `json:"asyncResponse"`
	SyncResponse      map[string]any `json:"syncResponse"`
}

type durableFixtureIdempotentReplay struct {
	ExecutionRequest    map[string]any `json:"executionRequest"`
	AsyncResponse       map[string]any `json:"asyncResponse"`
	ReplayAsyncResponse map[string]any `json:"replayAsyncResponse"`
}

func loadDurableFixtureCatalog(t *testing.T) durableFixtureCatalog {
	t.Helper()
	path := filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
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

func strPtr(value string) *string {
	return &value
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
