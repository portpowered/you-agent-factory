package factorysession_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

type strictDurableEventLifecycle struct {
	factorysession.DurableLifecycleAPI
	read  func(context.Context, string, factorysessionexecution.EventReconnectRequest) (*interfaces.FactoryEventStream, error)
	probe func(context.Context, string, factorysessionexecution.EventReconnectRequest) error
}

func (s strictDurableEventLifecycle) ReadDurableFactorySessionEventStream(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.EventReconnectRequest,
) (*interfaces.FactoryEventStream, error) {
	return s.read(ctx, sessionID, request)
}

func (s strictDurableEventLifecycle) ProbeDurableFactorySessionEvents(
	ctx context.Context,
	sessionID string,
	request factorysessionexecution.EventReconnectRequest,
) error {
	return s.probe(ctx, sessionID, request)
}

func TestDurableAPIEventRead_DelegatesMaterializedStreamToFactorySessions(t *testing.T) {
	t.Parallel()

	wantStream := &interfaces.FactoryEventStream{History: []interfaces.FactoryEvent{{Id: "event-1"}}}
	sequence := factoryapi.AfterSequence(4)
	api := factorysession.NewDurableAPI(nil, strictDurableEventLifecycle{
		read: func(_ context.Context, sessionID string, request factorysessionexecution.EventReconnectRequest) (*interfaces.FactoryEventStream, error) {
			if sessionID != "dur-sess-1" || request.AfterSequence == nil || *request.AfterSequence != 4 {
				t.Fatalf("read request = session %q %#v", sessionID, request)
			}
			return wantStream, nil
		},
		probe: func(context.Context, string, factorysessionexecution.EventReconnectRequest) error { return nil },
	})
	raw, _ := factorysession.EventReconnectRequestFromAPI(factoryapi.GetEventsBySessionIdParams{AfterSequence: &sequence})
	got, err := api.ReadDurableFactorySessionEvents(context.Background(), "dur-sess-1", raw)
	if err != nil {
		t.Fatalf("ReadDurableFactorySessionEvents: %v", err)
	}
	if got != wantStream {
		t.Fatalf("stream = %#v, want exact service-materialized stream %#v", got, wantStream)
	}
}

func TestDurableAPIEventProbe_DelegatesWithoutReadingStream(t *testing.T) {
	t.Parallel()

	called := false
	api := factorysession.NewDurableAPI(nil, strictDurableEventLifecycle{
		read: func(context.Context, string, factorysessionexecution.EventReconnectRequest) (*interfaces.FactoryEventStream, error) {
			t.Fatal("probe called stream read")
			return nil, nil
		},
		probe: func(_ context.Context, sessionID string, request factorysessionexecution.EventReconnectRequest) error {
			called = true
			if sessionID != "dur-sess-1" || request.AfterEventID != " event-1 " {
				t.Fatalf("probe request = session %q %#v", sessionID, request)
			}
			return nil
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
	api := factorysession.NewDurableAPI(nil, nil)
	if _, err := api.ListDurableFactorySessions(context.Background(), factorysessionexecution.ListSessionsRequest{}); err == nil {
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

func TestEventReconnectRequestFromCLI_MapsAfterEventIDAndSequence(t *testing.T) {
	sequence := 3
	req, err := factorysession.EventReconnectRequestFromCLI(factorysession.CLIEventReconnectInput{
		AfterEventID:  " session-started/dur-sess-js-run-n-001 ",
		AfterSequence: &sequence,
	})
	if err != nil {
		t.Fatalf("EventReconnectRequestFromCLI: %v", err)
	}
	if req.AfterEventID != " session-started/dur-sess-js-run-n-001 " {
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
	factorysessionexecution.ExecutionService
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
	}, nil)

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
	factorysessionexecution.ExecutionService
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
	}, nil)

	_, err := api.SubscribeDurableFactoryResponseEvents(context.Background(), factorysessionexecution.ResponseEventSubscriptionRequest{
		SessionID: "dur-sess-2",
	})
	if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("SubscribeDurableFactoryResponseEvents error = %v, want ErrFactorySessionNotFound", err)
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
