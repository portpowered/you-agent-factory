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

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

type strictDurableEventExecution struct {
	factorysession.DurableExecution
	read func(context.Context, string, factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error)
}

func (s strictDurableEventExecution) ReadEvents(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.EventReconnectRequest,
) (factorysessionexecution.EventReadResult, error) {
	return s.read(ctx, sessionID, request)
}

func TestDurableAPIEventRead_DelegatesMaterializedStreamToFactorySessions(t *testing.T) {
	t.Parallel()

	sequence := factoryapi.AfterSequence(4)
	api := factorysession.NewDurableAPI(strictDurableEventExecution{
		read: func(_ context.Context, sessionID string, request factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			if sessionID != "dur-sess-1" || request.AfterSequence == nil || *request.AfterSequence != 4 {
				t.Fatalf("read request = session %q %#v", sessionID, request)
			}
			return factorysessionexecution.EventReadResult{
				SessionID: sessionID,
				Events:    []json.RawMessage{json.RawMessage(`{"id":"event-1"}`)},
			}, nil
		},
	})
	raw, _ := factorysession.EventReconnectRequestFromAPI(factoryapi.GetEventsBySessionIdParams{AfterSequence: &sequence})
	got, err := api.ReadDurableFactorySessionEvents(context.Background(), "dur-sess-1", raw)
	if err != nil {
		t.Fatalf("ReadDurableFactorySessionEvents: %v", err)
	}
	if got == nil || len(got.History) != 1 || got.History[0].Id != "event-1" {
		t.Fatalf("stream = %#v, want materialized durable event history", got)
	}
}

func TestDurableAPIEventProbe_DelegatesWithoutReadingStream(t *testing.T) {
	t.Parallel()

	called := false
	api := factorysession.NewDurableAPI(strictDurableEventExecution{
		read: func(_ context.Context, sessionID string, request factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
			called = true
			if sessionID != "dur-sess-1" || request.AfterEventID != " event-1 " {
				t.Fatalf("probe request = session %q %#v", sessionID, request)
			}
			return factorysessionexecution.EventReadResult{SessionID: sessionID}, nil
		},
	})
	after := factoryapi.AfterEventId(" event-1 ")
	raw, _ := factorysession.EventReconnectRequestFromAPI(factoryapi.GetEventsBySessionIdParams{AfterEventId: &after})
	if err := api.ProbeDurableFactorySessionEvents(context.Background(), "dur-sess-1", raw); err != nil {
		t.Fatalf("ProbeDurableFactorySessionEvents: %v", err)
	}
	if !called {
		t.Fatal("Factory Sessions probe operation was not called")
	}
}

func TestDurableAPIListRequiresExecutionService(t *testing.T) {
	t.Parallel()
	api := factorysession.NewDurableAPI(nil)
	if _, err := api.ListDurableFactorySessions(context.Background(), factorysessionexecution.ListSessionsRequest{}); err == nil {
		t.Fatal("ListDurableFactorySessions succeeded without an execution service")
	}
}

type durableControlExecution struct {
	factorysession.DurableExecution
	pause func(context.Context, string, factorysessionexecution.DurableControlRequest) (factorysessionexecution.DurableControlResult, error)
}

func (fake durableControlExecution) Pause(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.DurableControlRequest,
) (factorysessionexecution.DurableControlResult, error) {
	return fake.pause(ctx, sessionID, request)
}

func TestDurableAPIControl_UsesOwnerPublishedDurableCapability(t *testing.T) {
	t.Parallel()

	api := factorysession.NewDurableAPI(durableControlExecution{
		pause: func(_ context.Context, sessionID string, request factorysessionexecution.DurableControlRequest) (factorysessionexecution.DurableControlResult, error) {
			if sessionID != "dur-sess-1" || request.RequestID != "control-1" || request.Reason != "operator pause" {
				t.Fatalf("Pause request = session %q %#v", sessionID, request)
			}
			return factorysessionexecution.DurableControlResult{
				SessionID: sessionID,
				Operation: factorysessionexecution.LifecycleControlPause,
				Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
				Status:    factorysessionexecution.LifecycleStatusPaused,
			}, nil
		},
	})

	result, err := api.PauseDurableFactorySession(context.Background(), "dur-sess-1", factorysessionexecution.DurableControlRequest{
		RequestID: "control-1", Reason: "operator pause",
	})
	if err != nil {
		t.Fatalf("PauseDurableFactorySession: %v", err)
	}
	if result.SessionId != "dur-sess-1" ||
		result.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
		result.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("PauseDurableFactorySession = %#v, want mapped accepted pause", result)
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

func TestEventReconnectRequestFromInput_MapsAfterEventIDAndSequence(t *testing.T) {
	sequence := 3
	req, err := factorysession.EventReconnectRequestFromInput(factorysession.DurableEventReconnectInput{
		AfterEventID:  " session-started/dur-sess-js-run-n-001 ",
		AfterSequence: &sequence,
	})
	if err != nil {
		t.Fatalf("EventReconnectRequestFromInput: %v", err)
	}
	if req.AfterEventID != " session-started/dur-sess-js-run-n-001 " {
		t.Fatalf("afterEventId = %q", req.AfterEventID)
	}
	if req.AfterSequence == nil || *req.AfterSequence != 3 {
		t.Fatalf("afterSequence = %#v, want 3", req.AfterSequence)
	}
}

func TestResultRequestFromInput_MapsModeAndIncludeArtifacts(t *testing.T) {
	req, err := factorysession.ResultRequestFromInput(factorysession.DurableResultInput{
		Mode:             "partial",
		IncludeArtifacts: true,
	})

	if err != nil {
		t.Fatalf("ResultRequestFromInput: %v", err)
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

type durableResponseEventsExecutionFake struct {
	factorysession.DurableExecution
	subscribeDurable func(context.Context, factorysessionexecution.ResponseEventSubscriptionRequest) (*factorysessionexecution.ResponseEventCursor, error)
	subscribeDirect  func(context.Context, string, factorysessionexecution.ResponseEventSubscriptionRequest) (*factorysessionexecution.ResponseEventCursor, error)
}

func (fake durableResponseEventsExecutionFake) SubscribeDurableFactoryResponseEvents(
	ctx context.Context,
	request factorysessionexecution.ResponseEventSubscriptionRequest,
) (*factorysessionexecution.ResponseEventCursor, error) {
	if fake.subscribeDurable == nil {
		panic("unexpected SubscribeDurableFactoryResponseEvents call")
	}
	return fake.subscribeDurable(ctx, request)
}

func (fake durableResponseEventsExecutionFake) SubscribeResponseEvents(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.ResponseEventSubscriptionRequest,
) (*factorysessionexecution.ResponseEventCursor, error) {
	if fake.subscribeDirect == nil {
		panic("unexpected SubscribeResponseEvents call")
	}
	return fake.subscribeDirect(ctx, sessionID, request)
}

func TestDurableAPIResponseEvents_SubscriberDelegatesToExecution(t *testing.T) {
	t.Parallel()

	wantCursor := &factorysessionexecution.ResponseEventCursor{
		NextEvents: func(context.Context) ([]factorysessionexecution.FactoryResponseEvent, error) {
			return []factorysessionexecution.FactoryResponseEvent{{Sequence: 3, Kind: factorysessionexecution.ResponseEventKindMessage}}, nil
		},
		DetachCursor: func() {},
	}
	api := factorysession.NewDurableAPI(durableResponseEventsExecutionFake{
		subscribeDurable: func(_ context.Context, request factorysessionexecution.ResponseEventSubscriptionRequest) (*factorysessionexecution.ResponseEventCursor, error) {
			if request.SessionID != "dur-sess-1" || request.DispatchID != "dispatch-1" {
				t.Fatalf("subscribe request = %#v", request)
			}
			return wantCursor, nil
		},
	})

	subscription, err := api.SubscribeDurableFactoryResponseEvents(context.Background(), factorysessionexecution.ResponseEventSubscriptionRequest{
		SessionID:  "dur-sess-1",
		DispatchID: "dispatch-1",
	})
	if err != nil {
		t.Fatalf("SubscribeDurableFactoryResponseEvents: %v", err)
	}
	defer subscription.Detach()

	records, err := subscription.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(records) != 1 || records[0].Sequence != 3 || records[0].Kind != string(factorysessionexecution.ResponseEventKindMessage) {
		t.Fatalf("records = %#v, want mapped MESSAGE sequence 3", records)
	}
}

type directResponseEventsExecutionFake struct {
	factorysession.DurableExecution
	subscribeDirect func(context.Context, string, factorysessionexecution.ResponseEventSubscriptionRequest) (*factorysessionexecution.ResponseEventCursor, error)
}

func (fake directResponseEventsExecutionFake) SubscribeResponseEvents(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.ResponseEventSubscriptionRequest,
) (*factorysessionexecution.ResponseEventCursor, error) {
	if fake.subscribeDirect == nil {
		panic("unexpected SubscribeResponseEvents call")
	}
	return fake.subscribeDirect(ctx, sessionID, request)
}

func TestDurableAPIResponseEvents_DirectExecutionPathMapsSessionNotFound(t *testing.T) {
	t.Parallel()

	api := factorysession.NewDurableAPI(directResponseEventsExecutionFake{
		subscribeDirect: func(_ context.Context, sessionID string, request factorysessionexecution.ResponseEventSubscriptionRequest) (*factorysessionexecution.ResponseEventCursor, error) {
			if sessionID != "dur-sess-2" || request.SessionID != "dur-sess-2" {
				t.Fatalf("direct subscribe = session %q request %#v", sessionID, request)
			}
			return nil, factorysessionexecution.ErrSessionNotFound
		},
	})

	_, err := api.SubscribeDurableFactoryResponseEvents(context.Background(), factorysessionexecution.ResponseEventSubscriptionRequest{
		SessionID: "dur-sess-2",
	})
	if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("SubscribeDurableFactoryResponseEvents error = %v, want ErrFactorySessionNotFound", err)
	}
}

func TestDurableAPIResponseEvents_DurableExecutionPathMapsStoreExpired(t *testing.T) {
	t.Parallel()

	api := factorysession.NewDurableAPI(durableResponseEventsExecutionFake{
		subscribeDurable: func(_ context.Context, request factorysessionexecution.ResponseEventSubscriptionRequest) (*factorysessionexecution.ResponseEventCursor, error) {
			if request.SessionID != "dur-sess-expired" {
				t.Fatalf("subscribe request = %#v", request)
			}
			return nil, factorysessionexecution.ErrResponseEventStoreExpired
		},
	})

	_, err := api.SubscribeDurableFactoryResponseEvents(context.Background(), factorysessionexecution.ResponseEventSubscriptionRequest{
		SessionID: "dur-sess-expired",
	})
	if !errors.Is(err, apisurface.ErrFactoryResponseEventStreamExpired) {
		t.Fatalf("SubscribeDurableFactoryResponseEvents error = %v, want ErrFactoryResponseEventStreamExpired", err)
	}
}

func TestDurableAPIResponseEvents_DirectExecutionPathMapsStoreExpired(t *testing.T) {
	t.Parallel()

	api := factorysession.NewDurableAPI(directResponseEventsExecutionFake{
		subscribeDirect: func(_ context.Context, sessionID string, request factorysessionexecution.ResponseEventSubscriptionRequest) (*factorysessionexecution.ResponseEventCursor, error) {
			if sessionID != "dur-sess-expired" || request.SessionID != "dur-sess-expired" {
				t.Fatalf("direct subscribe = session %q request %#v", sessionID, request)
			}
			return nil, factorysessionexecution.ErrResponseEventStoreExpired
		},
	})

	_, err := api.SubscribeDurableFactoryResponseEvents(context.Background(), factorysessionexecution.ResponseEventSubscriptionRequest{
		SessionID: "dur-sess-expired",
	})
	if !errors.Is(err, apisurface.ErrFactoryResponseEventStreamExpired) {
		t.Fatalf("SubscribeDurableFactoryResponseEvents error = %v, want ErrFactoryResponseEventStreamExpired", err)
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

type durableHistorySourceFake struct {
	resultRequest    factorysessionexecution.ResultRequest
	reconnectRequest factorysessionexecution.EventReconnectRequest
	dispatchID       string
	artifactID       string
	sessionID        string
}

func (fake *durableHistorySourceFake) GetDurableFactorySessionResult(_ context.Context, sessionID string, request factorysessionexecution.ResultRequest) (factoryapi.FactorySessionResult, error) {
	fake.sessionID, fake.resultRequest = sessionID, request
	return factoryapi.FactorySessionResult{SessionId: sessionID}, nil
}

func (fake *durableHistorySourceFake) ReadDurableFactorySessionEvents(_ context.Context, sessionID string, request factorysessionexecution.EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error) {
	fake.sessionID, fake.reconnectRequest = sessionID, request
	return &factorydefinitions.FactoryEventStream{FactorySessionID: sessionID}, nil
}

func (fake *durableHistorySourceFake) ProbeDurableFactorySessionEvents(_ context.Context, sessionID string, request factorysessionexecution.EventReconnectRequest) error {
	fake.sessionID, fake.reconnectRequest = sessionID, request
	return nil
}

func (fake *durableHistorySourceFake) ListDurableFactorySessionDispatches(_ context.Context, sessionID string, _ factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	fake.sessionID = sessionID
	return factoryapi.ListFactorySessionDispatchesResponse{SessionId: sessionID}, nil
}

func (fake *durableHistorySourceFake) GetDurableFactorySessionDispatch(_ context.Context, sessionID, dispatchID string) (factoryapi.FactoryDispatch, error) {
	fake.sessionID, fake.dispatchID = sessionID, dispatchID
	return factoryapi.FactoryDispatch{Id: dispatchID}, nil
}

func (fake *durableHistorySourceFake) ListDurableFactorySessionArtifacts(_ context.Context, sessionID string) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	fake.sessionID = sessionID
	return factoryapi.ListFactorySessionArtifactsResponse{SessionId: sessionID}, nil
}

func (fake *durableHistorySourceFake) GetDurableFactorySessionArtifact(_ context.Context, sessionID, artifactID string) (factoryapi.FactorySessionArtifactDetail, error) {
	fake.sessionID, fake.artifactID = sessionID, artifactID
	return factoryapi.FactorySessionArtifactDetail{Id: artifactID}, nil
}

func TestDurableHistoryBridge_CarriesTransportInputsOntoTheServiceRequest(t *testing.T) {
	t.Parallel()

	source := &durableHistorySourceFake{}
	bridge := factorysession.NewDurableHistoryBridge(source)
	if bridge == nil {
		t.Fatal("durable history source should produce a bound bridge")
	}
	resultInput := factorysession.DurableResultInput{Mode: "partial", IncludeArtifacts: true}
	result, err := bridge.GetDurableFactorySessionResult(context.Background(), "dur-sess-bridge-001", resultInput)
	if err != nil || result.SessionId != "dur-sess-bridge-001" {
		t.Fatalf("GetDurableFactorySessionResult = %#v, %v", result, err)
	}
	if source.resultRequest.Mode != factorysessionexecution.ResultMode("partial") ||
		!source.resultRequest.IncludeArtifacts {
		t.Fatalf("result request = %#v, want partial with artifacts", source.resultRequest)
	}

	sequence := 12
	reconnect := factorysession.DurableEventReconnectInput{AfterEventID: "event-7", AfterSequence: &sequence}
	stream, err := bridge.ReadDurableFactorySessionEvents(context.Background(), "dur-sess-bridge-002", reconnect)
	if err != nil || stream == nil || stream.FactorySessionID != "dur-sess-bridge-002" {
		t.Fatalf("ReadDurableFactorySessionEvents = %#v, %v", stream, err)
	}
	if source.reconnectRequest.AfterEventID != "event-7" ||
		source.reconnectRequest.AfterSequence == nil || *source.reconnectRequest.AfterSequence != 12 {
		t.Fatalf("reconnect request = %#v, want event-7/12", source.reconnectRequest)
	}
	if err := bridge.ProbeDurableFactorySessionEvents(context.Background(), "dur-sess-bridge-003", reconnect); err != nil {
		t.Fatalf("ProbeDurableFactorySessionEvents: %v", err)
	}
	if source.sessionID != "dur-sess-bridge-003" {
		t.Fatalf("probe sessionId = %q, want dur-sess-bridge-003", source.sessionID)
	}
}

func TestDurableHistoryBridge_ForwardsAlreadyNeutralReads(t *testing.T) {
	t.Parallel()

	source := &durableHistorySourceFake{}
	bridge := factorysession.NewDurableHistoryBridge(source)
	ctx := context.Background()
	listParams := factoryapi.ListFactorySessionDispatchesParams{}
	dispatches, err := bridge.ListDurableFactorySessionDispatches(ctx, "dur-sess-bridge-004", listParams)
	if err != nil || dispatches.SessionId != "dur-sess-bridge-004" {
		t.Fatalf("ListDurableFactorySessionDispatches = %#v, %v", dispatches, err)
	}
	dispatch, err := bridge.GetDurableFactorySessionDispatch(ctx, "dur-sess-bridge-005", "dispatch-3")
	if err != nil || dispatch.Id != "dispatch-3" {
		t.Fatalf("GetDurableFactorySessionDispatch = %#v, %v", dispatch, err)
	}
	artifacts, err := bridge.ListDurableFactorySessionArtifacts(ctx, "dur-sess-bridge-006")
	if err != nil || artifacts.SessionId != "dur-sess-bridge-006" {
		t.Fatalf("ListDurableFactorySessionArtifacts = %#v, %v", artifacts, err)
	}
	artifact, err := bridge.GetDurableFactorySessionArtifact(ctx, "dur-sess-bridge-007", "artifact-4")
	if err != nil || artifact.Id != "artifact-4" {
		t.Fatalf("GetDurableFactorySessionArtifact = %#v, %v", artifact, err)
	}
}

func TestNewDurableHistoryBridge_RejectsAbsentSources(t *testing.T) {
	t.Parallel()

	if factorysession.NewDurableHistoryBridge(nil) != nil {
		t.Fatal("absent history source should not produce a bound bridge")
	}
	var typedNil *durableHistorySourceFake
	if factorysession.NewDurableHistoryBridge(typedNil) != nil {
		t.Fatal("typed-nil history source should not produce a bound bridge")
	}
}

type durableInspectionSourceFake struct {
	reconnect  factorysessionexecution.EventReconnectRequest
	sessionID  string
	dispatches []factorysessionexecution.DispatchSummary
	artifacts  []factorysessionexecution.ArtifactSummary
	events     []json.RawMessage
}

func (fake *durableInspectionSourceFake) QueryDispatches(_ context.Context, request factorysessionexecution.DispatchQueryRequest) (factorysessionexecution.ListDispatchesResult, error) {
	fake.sessionID = request.SessionID
	return factorysessionexecution.ListDispatchesResult{Dispatches: fake.dispatches}, nil
}

func (fake *durableInspectionSourceFake) ListArtifacts(_ context.Context, sessionID string) (factorysessionexecution.ListArtifactsResult, error) {
	fake.sessionID = sessionID
	return factorysessionexecution.ListArtifactsResult{Artifacts: fake.artifacts}, nil
}

func (fake *durableInspectionSourceFake) ReadEvents(_ context.Context, sessionID string, request factorysessionexecution.EventReconnectRequest) (factorysessionexecution.EventReadResult, error) {
	fake.sessionID, fake.reconnect = sessionID, request
	return factorysessionexecution.EventReadResult{Events: fake.events}, nil
}

func newDurableInspectionSourceFake() *durableInspectionSourceFake {
	return &durableInspectionSourceFake{
		dispatches: []factorysessionexecution.DispatchSummary{{
			ID:           "dispatch-1",
			Status:       factorysessionexecution.DispatchStatus("COMPLETED"),
			DispatchKind: "PETRI_TRANSITION",
		}},
		artifacts: []factorysessionexecution.ArtifactSummary{{
			ID: "artifact-1", Kind: "LOG", Visibility: "PUBLIC", Label: "log",
			ContentHash: "hash", SizeBytes: 9, DispatchID: "dispatch-1",
		}},
		events: []json.RawMessage{json.RawMessage(`{"id":"event-1"}`)},
	}
}

func TestDurableInspectionBridge_RestatesDispatchReadsInTransportVocabulary(t *testing.T) {
	t.Parallel()

	source := newDurableInspectionSourceFake()
	bridge := factorysession.NewDurableInspectionBridge(source)
	if bridge == nil {
		t.Fatal("durable inspection source should produce a bound bridge")
	}
	dispatches, err := bridge.QueryDispatches(context.Background(), "dur-sess-inspect-001")
	if err != nil {
		t.Fatalf("QueryDispatches: %v", err)
	}
	want := []factorysession.HistoricalDispatchInput{
		{ID: "dispatch-1", Status: "COMPLETED", DispatchKind: "PETRI_TRANSITION"},
	}
	if !reflect.DeepEqual(dispatches, want) {
		t.Fatalf("QueryDispatches = %#v, want %#v", dispatches, want)
	}
	if source.sessionID != "dur-sess-inspect-001" {
		t.Fatalf("query sessionId = %q, want dur-sess-inspect-001", source.sessionID)
	}
}

func TestDurableInspectionBridge_RestatesArtifactReadsInTransportVocabulary(t *testing.T) {
	t.Parallel()

	source := newDurableInspectionSourceFake()
	bridge := factorysession.NewDurableInspectionBridge(source)
	artifacts, err := bridge.ListArtifacts(context.Background(), "dur-sess-inspect-002")
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	want := []factorysession.DurableArtifactFact{{
		ID: "artifact-1", Kind: "LOG", Visibility: "PUBLIC", Label: "log",
		ContentHash: "hash", SizeBytes: 9, DispatchID: "dispatch-1",
	}}
	if !reflect.DeepEqual(artifacts, want) {
		t.Fatalf("ListArtifacts = %#v, want %#v", artifacts, want)
	}
	if source.sessionID != "dur-sess-inspect-002" {
		t.Fatalf("artifact sessionId = %q, want dur-sess-inspect-002", source.sessionID)
	}
}

func TestDurableInspectionBridge_CarriesReconnectCursorsOntoEventReads(t *testing.T) {
	t.Parallel()

	source := newDurableInspectionSourceFake()
	bridge := factorysession.NewDurableInspectionBridge(source)
	sequence := 3
	events, err := bridge.ReadEvents(context.Background(), "dur-sess-inspect-003",
		factorysession.DurableEventReconnectInput{AfterEventID: "event-0", AfterSequence: &sequence})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 1 || string(events[0]) != `{"id":"event-1"}` {
		t.Fatalf("ReadEvents = %#v, want the retained event payload", events)
	}
	if source.reconnect.AfterEventID != "event-0" {
		t.Fatalf("reconnect afterEventId = %q, want event-0", source.reconnect.AfterEventID)
	}
	if source.reconnect.AfterSequence == nil || *source.reconnect.AfterSequence != 3 {
		t.Fatalf("reconnect request = %#v, want afterSequence=3", source.reconnect)
	}
}

func TestNewDurableInspectionBridge_RejectsAbsentAndForeignSources(t *testing.T) {
	t.Parallel()

	if factorysession.NewDurableInspectionBridge(nil) != nil {
		t.Fatal("absent inspection source should not produce a bound bridge")
	}
	if factorysession.NewDurableInspectionBridge(struct{}{}) != nil {
		t.Fatal("foreign value should not produce a bound bridge")
	}
	var typedNil *durableInspectionSourceFake
	if factorysession.NewDurableInspectionBridge(typedNil) != nil {
		t.Fatal("typed-nil inspection source should not produce a bound bridge")
	}
}

func TestDurableInputsFromAPI_CarryPublicParametersWithoutTheServiceContract(t *testing.T) {
	t.Parallel()

	mode := factoryapi.FactorySessionResultMode("partial")
	include := factoryapi.FactorySessionResultIncludeArtifacts(true)
	result, err := factorysession.DurableResultInputFromAPI(factoryapi.GetFactorySessionResultsParams{
		Mode: &mode, IncludeArtifacts: &include,
	})
	if err != nil || result.Mode != "partial" || !result.IncludeArtifacts {
		t.Fatalf("DurableResultInputFromAPI = %#v, %v", result, err)
	}
	empty, err := factorysession.DurableResultInputFromAPI(factoryapi.GetFactorySessionResultsParams{})
	if err != nil || empty.Mode != "" || empty.IncludeArtifacts {
		t.Fatalf("DurableResultInputFromAPI(empty) = %#v, %v", empty, err)
	}

	eventID := factoryapi.AfterEventId("event-5")
	sequence := factoryapi.AfterSequence(11)
	reconnect, err := factorysession.DurableEventReconnectInputFromAPI(factoryapi.GetEventsBySessionIdParams{
		AfterEventId: &eventID, AfterSequence: &sequence,
	})
	if err != nil || reconnect.AfterEventID != "event-5" ||
		reconnect.AfterSequence == nil || *reconnect.AfterSequence != 11 {
		t.Fatalf("DurableEventReconnectInputFromAPI = %#v, %v", reconnect, err)
	}
	blank, err := factorysession.DurableEventReconnectInputFromAPI(factoryapi.GetEventsBySessionIdParams{})
	if err != nil || blank.AfterEventID != "" || blank.AfterSequence != nil {
		t.Fatalf("DurableEventReconnectInputFromAPI(empty) = %#v, %v", blank, err)
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
