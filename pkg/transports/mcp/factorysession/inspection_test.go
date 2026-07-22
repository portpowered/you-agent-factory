package factorysession_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/transports/mcp/factorysession"
)

const (
	pausedSessionID  = "dur-sess-js-paused-001"
	runningPetriID   = "dur-sess-petri-run-001"
	dispatchID       = "disp-petri-success-001"
	pausedDispatchID = "disp-js-pause-001"
)

func TestMockClient_RuntimeService_DocumentedHostConversationUsesRegisteredTools(t *testing.T) {
	statusReads := []factorysessions.LifecycleStatus{
		factorysessions.LifecycleStatusRunning,
		factorysessions.LifecycleStatusPaused,
		factorysessions.LifecycleStatusRunning,
		factorysessions.LifecycleStatus("CANCELED"),
	}
	readIndex := 0
	service := scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return runningAsyncStart(), nil
		},
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			status := statusReads[readIndex]
			readIndex++
			read := runningSessionRead()
			read.Status = status
			return read, nil
		},
		getResult: func(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
			return notReadyResult(), nil
		},
		listDispatches: func(context.Context, string) (factorysessions.ListDispatchesResult, error) {
			return factorysessions.ListDispatchesResult{SessionID: runningSessionID}, nil
		},
		listArtifacts: func(context.Context, string) (factorysessions.ListArtifactsResult, error) {
			return factorysessions.ListArtifactsResult{SessionID: runningSessionID}, nil
		},
		readEvents: func(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
			return lifecycleEvents(runningSessionID), nil
		},
		pause: func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			return acceptedControl(runningSessionID, "PAUSE", factorysessions.LifecycleStatusPaused), nil
		},
		resume: func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			return acceptedControl(runningSessionID, "RESUME", factorysessions.LifecycleStatusRunning), nil
		},
		cancel: func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			return acceptedControl(runningSessionID, "CANCEL", factorysessions.LifecycleStatus("CANCELED")), nil
		},
	}
	workflows := scriptedWorkflowDefinitions{
		defaultSourceContext: func(string) (factoryruntime.WorkflowSourceContext, error) {
			return factoryruntime.WorkflowSourceContext{}, nil
		},
		buildPreview: func(factoryruntime.WorkflowPreviewRequest) factoryruntime.WorkflowPreview {
			return factoryruntime.WorkflowPreview{Valid: true}
		},
	}
	client := newTestClientWithService(service, canonicalMCPRequestPreparation, workflows)

	assertDocumentedSourceValid(t, client)
	started, err := client.StartAsync(context.Background(), runtimeBusyLoopAsyncRequest("req-host-demo-001"))
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("StartAsync = %#v, %v; want success", started, err)
	}
	sessionID := started.Result.SessionId
	assertClientSessionStatus(t, client, sessionID, factoryapi.FactorySessionDurableLifecycleStatusRunning)
	mode := factoryapi.FactorySessionResultModeFinal
	notReady, err := client.GetResult(context.Background(), mcpfactorysession.GetResultInput{SessionID: sessionID, Mode: &mode})
	if err != nil || notReady.Error == nil || notReady.Error.Code != "factory_session.result.not_ready" {
		t.Fatalf("GetResult = %#v, %v; want not-ready", notReady, err)
	}
	assertDocumentedInspectionReads(t, client, sessionID)
	assertDocumentedPauseResume(t, client, sessionID)
	assertDocumentedLifecycleEvents(t, client, sessionID)
	assertDocumentedCancel(t, client, sessionID)
}

func assertDocumentedSourceValid(t *testing.T, client *testClient) {
	t.Helper()
	inlineSource, projectRoot := `while (true) {}`, t.TempDir()
	raw, err := client.CallTool(context.Background(), mcpfactorysession.ToolValidateSource, mustJSON(t, factoryapi.FactoryPreviewRequest{
		SourceKind:   factoryapi.INLINEWORKFLOW,
		InlineSource: &inlineSource,
		ProjectRoot:  &projectRoot,
	}))
	if err != nil {
		t.Fatalf("ValidateSource: %v", err)
	}
	var response mcpfactorysession.ToolResponse[factoryapi.FactoryPreviewResult]
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode ValidateSource: %v", err)
	}
	if response.Error != nil || response.Result == nil || !response.Result.Valid ||
		response.Result.SourceValidationIssues == nil {
		t.Fatalf("validation = %#v, want valid source with sourceValidationIssues", response)
	}
}

func assertDocumentedInspectionReads(t *testing.T, client *testClient, sessionID string) {
	t.Helper()
	dispatches, dispatchErr := client.ListDispatches(context.Background(), mcpfactorysession.ListDispatchesInput{SessionID: sessionID})
	artifacts, artifactErr := client.ListArtifacts(context.Background(), mcpfactorysession.ListArtifactsInput{SessionID: sessionID})
	events, eventErr := client.ReadEvents(context.Background(), mcpfactorysession.ReadEventsInput{SessionID: sessionID})
	if dispatchErr != nil || dispatches.Error != nil || dispatches.Result == nil {
		t.Fatalf("ListDispatches: response=%#v err=%v", dispatches, dispatchErr)
	}
	if artifactErr != nil || artifacts.Error != nil || artifacts.Result == nil {
		t.Fatalf("ListArtifacts: response=%#v err=%v", artifacts, artifactErr)
	}
	if eventErr != nil || events.Error != nil || events.Result == nil || len(events.Result.Events) == 0 {
		t.Fatalf("ReadEvents: response=%#v err=%v", events, eventErr)
	}
}

func assertDocumentedPauseResume(t *testing.T, client *testClient, sessionID string) {
	t.Helper()
	pauseID, pauseReason := "req-pause-host-demo-01", "host maintenance"
	paused, err := client.Control(context.Background(), mcpfactorysession.ControlInput{
		SessionID: sessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindPause,
		RequestID: &pauseID,
		Reason:    &pauseReason,
	})
	if err != nil || paused.Error != nil || paused.Result == nil ||
		paused.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause: response=%#v err=%v", paused, err)
	}
	assertClientSessionStatus(t, client, sessionID, factoryapi.FactorySessionDurableLifecycleStatusPaused)
	resumeID := "req-resume-host-demo-01"
	resumed, err := client.Control(context.Background(), mcpfactorysession.ControlInput{
		SessionID: sessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindResume,
		RequestID: &resumeID,
	})
	if err != nil || resumed.Error != nil || resumed.Result == nil ||
		resumed.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume: response=%#v err=%v", resumed, err)
	}
	assertClientSessionStatus(t, client, sessionID, factoryapi.FactorySessionDurableLifecycleStatusRunning)
}

func assertDocumentedLifecycleEvents(t *testing.T, client *testClient, sessionID string) {
	t.Helper()
	response, err := client.ReadEvents(context.Background(), mcpfactorysession.ReadEventsInput{SessionID: sessionID})
	if err != nil || response.Error != nil || response.Result == nil {
		t.Fatalf("ReadEvents after controls: response=%#v err=%v", response, err)
	}
	controls := 0
	for _, event := range response.Result.Events {
		if event.Type == factoryapi.FactoryEventTypeSessionLifecycleControl {
			controls++
		}
	}
	if controls < 2 {
		t.Fatalf("SESSION_LIFECYCLE_CONTROL events = %d, want pause and resume", controls)
	}
}

func assertDocumentedCancel(t *testing.T, client *testClient, sessionID string) {
	t.Helper()
	requestID, reason := "req-cancel-host-demo-01", "example complete"
	response, err := client.Control(context.Background(), mcpfactorysession.ControlInput{
		SessionID: sessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindCancel,
		RequestID: &requestID,
		Reason:    &reason,
	})
	if err != nil || response.Error != nil || response.Result == nil ||
		response.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("cancel: response=%#v err=%v", response, err)
	}
	assertClientSessionStatus(t, client, sessionID, factoryapi.FactorySessionDurableLifecycleStatusCanceled)
}

func assertClientSessionStatus(
	t *testing.T,
	client *testClient,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
) {
	t.Helper()
	response, err := client.GetSession(context.Background(), mcpfactorysession.GetSessionInput{SessionID: sessionID})
	if err != nil || response.Error != nil || response.Result == nil || response.Result.Status != want {
		t.Fatalf("GetSession: response=%#v err=%v, want %s", response, err, want)
	}
}

func TestMockClient_ListDispatches_DispatchInspectionFixtureReturnsStableSummaries(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startSync: func(context.Context, factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
			return successfulSyncStart(), nil
		},
		listDispatches: func(context.Context, string) (factorysessions.ListDispatchesResult, error) {
			return dispatchInspection(), nil
		},
	})
	if _, err := client.StartSync(context.Background(), syncSuccessExecutionRequest()); err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	response, err := client.ListDispatches(context.Background(), mcpfactorysession.ListDispatchesInput{SessionID: successSessionID})
	if err != nil || response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, %v; want dispatch list", response, err)
	}
	if response.Result.SessionId != successSessionID || len(response.Result.Dispatches) != 1 {
		t.Fatalf("response = %#v, want one dispatch", response.Result)
	}
	dispatch := response.Result.Dispatches[0]
	if dispatch.Id != dispatchID || dispatch.Status == "" || dispatch.DispatchKind == "" {
		t.Fatalf("dispatch = %#v, want stable populated dispatch", dispatch)
	}
}

func TestMockClient_ListDispatches_FiltersAndRejectsInvalidStatus(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		queryDispatches: func(_ context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
			if request.SessionID != successSessionID {
				t.Fatalf("query sessionID = %q", request.SessionID)
			}
			switch {
			case request.Filters.Phase == "unknown" && request.Filters.Status == "COMPLETED":
				return factorysessions.ListDispatchesResult{SessionID: successSessionID, Dispatches: []factorysessions.DispatchSummary{}}, nil
			case request.Filters.Status == "BROKEN":
				return factorysessions.ListDispatchesResult{}, &factorysessions.ExecutionValidationError{Field: "status", Message: "invalid status"}
			default:
				t.Fatalf("unexpected dispatch query = %#v", request)
				return factorysessions.ListDispatchesResult{}, nil
			}
		},
	})
	empty, err := client.ListDispatches(context.Background(), mcpfactorysession.ListDispatchesInput{
		SessionID: successSessionID,
		Phase:     "unknown",
		Status:    "COMPLETED",
	})
	if err != nil || empty.Error != nil || empty.Result == nil || len(empty.Result.Dispatches) != 0 {
		t.Fatalf("unknown phase response = %#v, %v", empty, err)
	}
	invalid, err := client.ListDispatches(context.Background(), mcpfactorysession.ListDispatchesInput{
		SessionID: successSessionID,
		Status:    "BROKEN",
	})
	if err != nil || invalid.Error == nil || invalid.Result != nil || invalid.Error.Code != "BAD_REQUEST" {
		t.Fatalf("invalid status response = %#v, %v", invalid, err)
	}
}

func TestMockClient_ListArtifacts_ArtifactInspectionFixtureReturnsStableSummaries(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return factorysessions.AsyncStartResult{SessionID: pausedSessionID, Status: "PAUSED"}, nil
		},
		listArtifacts: func(context.Context, string) (factorysessions.ListArtifactsResult, error) {
			return artifactInspection(), nil
		},
	})
	started, err := client.StartAsync(context.Background(), artifactInspectionExecutionRequest())
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, %v; want success", started, err)
	}
	response, err := client.ListArtifacts(context.Background(), mcpfactorysession.ListArtifactsInput{SessionID: pausedSessionID})
	if err != nil || response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, %v; want artifact list", response, err)
	}
	if response.Result.SessionId != pausedSessionID || len(response.Result.Artifacts) != 1 {
		t.Fatalf("response = %#v, want one artifact", response.Result)
	}
	artifact := response.Result.Artifacts[0]
	if artifact.Id != "art-js-pause-001" || artifact.Kind == "" ||
		artifact.DispatchId == nil || *artifact.DispatchId != pausedDispatchID {
		t.Fatalf("artifact = %#v, want stable dispatch artifact", artifact)
	}
}

func TestMockClient_ReadEvents_EventReconnectFixtureReturnsOrderedCanonicalEvents(t *testing.T) {
	client := clientWithScript(scriptedExecutionService{
		startAsync: func(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
			return runningAsyncStart(), nil
		},
		readEvents: func(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
			return startedAndProgressEvents(runningSessionID), nil
		},
	})
	started, err := client.StartAsync(context.Background(), asyncRunningExecutionRequest())
	if err != nil || started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, %v; want success", started, err)
	}
	response, err := client.ReadEvents(context.Background(), mcpfactorysession.ReadEventsInput{SessionID: runningSessionID})
	if err != nil || response.Error != nil || response.Result == nil {
		t.Fatalf("response = %#v, %v; want event read", response, err)
	}
	if response.Result.SessionID != runningSessionID || len(response.Result.Events) != 2 ||
		response.Result.Events[0].Type != "SESSION_STARTED" {
		t.Fatalf("events = %#v, want ordered started/progress events", response.Result)
	}
	for _, event := range response.Result.Events {
		if strings.Contains(strings.ToUpper(string(event.Type)), "PETRI") {
			t.Fatalf("event type exposes internal vocabulary: %q", event.Type)
		}
		if event.Context.SessionId == nil || *event.Context.SessionId != runningSessionID {
			t.Fatalf("event context sessionId = %#v, want %q", event.Context.SessionId, runningSessionID)
		}
	}
}

func TestMockClient_Control_LifecycleFixtureReturnsAcceptedRejectedAndIsolatesSessions(t *testing.T) {
	service := scriptedExecutionService{
		pause: func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			return controlResult(pausedSessionID, "PAUSE", "NO_OP", factorysessions.LifecycleStatusPaused), nil
		},
		resume: func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			return acceptedControl(pausedSessionID, "RESUME", factorysessions.LifecycleStatusRunning), nil
		},
		terminate: func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			return acceptedControl(runningPetriID, "TERMINATE", factorysessions.LifecycleStatus("TERMINATED")), nil
		},
		cancel: func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
			return controlResult(successSessionID, "CANCEL", "TERMINAL_SESSION", factorysessions.LifecycleStatusSucceeded), nil
		},
		getSession: func(context.Context, string) (factorysessions.SessionReadResult, error) {
			read := runningSessionRead()
			read.SessionID = pausedSessionID
			return read, nil
		},
	}
	client := clientWithScript(service)
	noOp, err := client.Control(context.Background(), mcpfactorysession.ControlInput{
		SessionID: pausedSessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindPause,
	})
	if err != nil || noOp.Error != nil || noOp.Result == nil ||
		noOp.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("pause no-op = %#v, %v", noOp, err)
	}
	resumed, err := client.Control(context.Background(), mcpfactorysession.ControlInput{
		SessionID: pausedSessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindResume,
	})
	if err != nil || resumed.Error != nil || resumed.Result == nil ||
		resumed.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
		resumed.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume = %#v, %v; want accepted RUNNING", resumed, err)
	}
	terminated, err := client.Control(context.Background(), mcpfactorysession.ControlInput{
		SessionID: runningPetriID,
		Operation: factoryapi.FactorySessionLifecycleControlKindTerminate,
	})
	if err != nil || terminated.Error != nil || terminated.Result == nil ||
		terminated.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("terminate = %#v, %v; want accepted", terminated, err)
	}
	rejected, err := client.Control(context.Background(), mcpfactorysession.ControlInput{
		SessionID: successSessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKindCancel,
	})
	if err != nil || rejected.Error != nil || rejected.Result == nil ||
		rejected.Result.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("terminal cancel = %#v, %v", rejected, err)
	}
	pausedAfter, err := client.GetSession(context.Background(), mcpfactorysession.GetSessionInput{SessionID: pausedSessionID})
	if err != nil || pausedAfter.Error != nil || pausedAfter.Result == nil ||
		pausedAfter.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("isolated read = %#v, %v; want RUNNING", pausedAfter, err)
	}
}

type scriptedWorkflowDefinitions struct {
	factoryruntime.JavaScriptWorkflowDefinitions
	defaultSourceContext func(string) (factoryruntime.WorkflowSourceContext, error)
	buildPreview         func(factoryruntime.WorkflowPreviewRequest) factoryruntime.WorkflowPreview
}

func (service scriptedWorkflowDefinitions) DefaultSourceContext(root string) (factoryruntime.WorkflowSourceContext, error) {
	return service.defaultSourceContext(root)
}

func (service scriptedWorkflowDefinitions) BuildPreview(request factoryruntime.WorkflowPreviewRequest) factoryruntime.WorkflowPreview {
	return service.buildPreview(request)
}

func (service scriptedWorkflowDefinitions) PreviewWorkflow(_ context.Context, input factoryruntime.WorkflowPreviewInput) (factoryruntime.WorkflowPreview, error) {
	context, err := service.DefaultSourceContext(input.ProjectRoot)
	if err != nil {
		return factoryruntime.WorkflowPreview{}, err
	}
	return service.BuildPreview(factoryruntime.WorkflowPreviewRequest{Source: input.Source, Context: context}), nil
}

func dispatchInspection() factorysessions.ListDispatchesResult {
	return factorysessions.ListDispatchesResult{
		SessionID: successSessionID,
		Dispatches: []factorysessions.DispatchSummary{{
			ID:           dispatchID,
			Status:       factorysessions.DispatchStatus("COMPLETED"),
			DispatchKind: "WORK",
			Phase:        "execute",
		}},
	}
}

func artifactInspection() factorysessions.ListArtifactsResult {
	return factorysessions.ListArtifactsResult{
		SessionID: pausedSessionID,
		Artifacts: []factorysessions.ArtifactSummary{{
			ID:         "art-js-pause-001",
			Kind:       "CHECKPOINT",
			DispatchID: pausedDispatchID,
		}},
	}
}

func startedAndProgressEvents(sessionID string) factorysessions.EventReadResult {
	return factorysessions.EventReadResult{
		SessionID: sessionID,
		Events: []json.RawMessage{
			canonicalEvent("evt-started", "SESSION_STARTED", sessionID, 1),
			canonicalEvent("evt-progress", "SESSION_PROGRESS", sessionID, 2),
		},
	}
}

func lifecycleEvents(sessionID string) factorysessions.EventReadResult {
	return factorysessions.EventReadResult{
		SessionID: sessionID,
		Events: []json.RawMessage{
			canonicalEvent("evt-started", "SESSION_STARTED", sessionID, 1),
			canonicalEvent("evt-pause", "SESSION_LIFECYCLE_CONTROL", sessionID, 2),
			canonicalEvent("evt-resume", "SESSION_LIFECYCLE_CONTROL", sessionID, 3),
		},
	}
}

func canonicalEvent(id, eventType, sessionID string, sequence int) json.RawMessage {
	return json.RawMessage(
		`{"schemaVersion":"agent-factory.event.v1","id":"` + id +
			`","type":"` + eventType +
			`","context":{"sequence":` + jsonNumber(sequence) +
			`,"tick":` + jsonNumber(sequence) +
			`,"eventTime":"2026-01-01T00:00:00Z","sessionId":"` + sessionID +
			`"},"payload":{}}`,
	)
}

func jsonNumber(value int) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func acceptedControl(
	sessionID string,
	operation string,
	status factorysessions.LifecycleStatus,
) factorysessions.LifecycleControlResult {
	return controlResult(sessionID, operation, "ACCEPTED", status)
}

func controlResult(
	sessionID string,
	operation string,
	outcome string,
	status factorysessions.LifecycleStatus,
) factorysessions.LifecycleControlResult {
	return factorysessions.LifecycleControlResult{
		SessionID: sessionID,
		Operation: factorysessions.LifecycleControlKind(operation),
		Outcome:   factorysessions.LifecycleControlOutcome(outcome),
		Status:    status,
		Links:     factorysessions.LifecycleControlLinks{Session: "/factory-sessions/" + sessionID},
	}
}

func artifactInspectionExecutionRequest() factoryapi.FactorySessionExecutionRequest {
	return factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-js-paused-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: strPtr("customer-support-triage"),
		},
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return encoded
}
