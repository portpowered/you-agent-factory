package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCapturedTerminalSummaryRetainsDetachedMetadata(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"completed", "failed", "owner-loss", "legacy", "wrong-attempt"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			metadata := &workersessions.SessionMetadata{Requester: &workersessions.Requester{Kind: "WORKER_SESSION", WorkerSessionID: "lead", WorkID: "project"}, Correlation: &workersessions.Correlation{WorkID: "work", FactorySessionID: "original"}, Labels: []string{"project:example"}}
			captured := workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "original", AttemptID: "attempt", State: workersessions.StateCompleted, Requester: metadata.Requester, Correlation: metadata.Correlation, Labels: metadata.Labels}
			item := recordings.WorkerCapturedCatalogItem{Catalog: recordings.WorkerSessionCatalogEntry{FactorySessionID: "original", CommittedPosition: 3}, Terminal: &recordings.WorkerRecordingTerminal{Position: 3}}
			service := &recordedWorkerSessionObservation{}
			switch name {
			case "failed":
				captured.State = workersessions.StateFailed
			case "owner-loss":
				captured.State = workersessions.StateFailed
				service.restoredWorkerScopes = map[string]string{"worker": "original"}
				item.Terminal, item.Health = nil, recordings.WorkerRecordingStatusIncomplete
			case "legacy":
				captured.Requester, captured.Correlation, captured.Labels = nil, nil, nil
			case "wrong-attempt":
				captured.AttemptID = "foreign"
			}
			service.Service = &capturedForceService{observation: captured}
			got, err := service.withSelectedCapturedTerminal(t.Context(), workersessions.Observation{WorkerSessionID: "worker", AttemptID: "attempt", State: workersessions.StateCompleted}, item)
			if err != nil {
				t.Fatal(err)
			}
			assertCapturedTerminalMetadata(t, name, got, metadata)
		})
	}
}

func assertCapturedTerminalMetadata(t *testing.T, name string, got workersessions.Observation, metadata *workersessions.SessionMetadata) {
	t.Helper()
	if name == "legacy" || name == "wrong-attempt" {
		if got.Requester != nil || got.Correlation != nil || got.Labels != nil {
			t.Fatalf("invented metadata: %+v", got)
		}
		return
	}
	actual := &workersessions.SessionMetadata{Requester: got.Requester, Correlation: got.Correlation, Labels: got.Labels}
	if !reflect.DeepEqual(actual, metadata) {
		t.Fatalf("captured metadata lost: %+v", got)
	}
	got.Requester.WorkerSessionID, got.Correlation.WorkID, got.Labels[0] = "changed", "changed", "changed"
	if metadata.Requester.WorkerSessionID != "lead" || metadata.Correlation.WorkID != "work" || metadata.Labels[0] != "project:example" {
		t.Fatal("returned metadata aliases capture")
	}
}

func TestCapturedTerminalSummaryUsesExactOriginalScopeAndCaptureTiming(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"captured", "legacy", "wrong-attempt", "different-outcome", "owner-loss", "owner-loss-wrong-attempt"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			start := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
			end, duration := start.Add(time.Second), time.Second
			captured := workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "original", AttemptID: "attempt", State: workersessions.StateFailed,
				Provider: "codex", ContinuationHeadWorkerSessionID: "worker",
				StartedAt: &start, EndedAt: &end, Duration: &duration, DurationBasis: workersessions.DurationBasisRecordedTimestamps,
				Failure: &workersessions.FailureCause{Kind: workersessions.FailureCauseWorkersExecutionFailure, Detail: "expected artifact was not produced"}}
			if name == "legacy" || name == "owner-loss" || name == "owner-loss-wrong-attempt" {
				captured.EndedAt, captured.Duration = nil, nil
				captured.DurationBasis = workersessions.DurationBasisUnavailable
			}
			if name == "wrong-attempt" || name == "owner-loss-wrong-attempt" {
				captured.AttemptID = "other"
			}
			if name == "different-outcome" {
				captured.State = workersessions.StateCompleted
				captured.Failure = nil
				cause := "COMPLETED"
				captured.TerminalCause = &cause
			}
			peer := &capturedForceService{observation: captured}
			service := &recordedWorkerSessionObservation{Service: peer}
			item := recordings.WorkerCapturedCatalogItem{Catalog: recordings.WorkerSessionCatalogEntry{FactorySessionID: "original", CommittedPosition: 3}, Terminal: &recordings.WorkerRecordingTerminal{Position: 3}}
			if name == "owner-loss" || name == "owner-loss-wrong-attempt" {
				service.restoredWorkerScopes = map[string]string{"worker": "original"}
				item.Terminal, item.Health = nil, recordings.WorkerRecordingStatusIncomplete
				cause := "OWNER_LOST"
				captured.TerminalCause = &cause
				peer.observation = captured
			}
			got, err := service.withSelectedCapturedTerminal(t.Context(), workersessions.Observation{WorkerSessionID: "worker", FactorySessionID: "rebound", AttemptID: "attempt", State: workersessions.StateFailed, EndedAt: &start}, item)
			wantScope, wantState := "original", captured.State
			if name == "wrong-attempt" || name == "owner-loss-wrong-attempt" {
				wantScope, wantState = "rebound", workersessions.StateFailed
			}
			if err != nil || got.FactorySessionID != wantScope || got.State != wantState || peer.request.FactorySessionID != "original" {
				t.Fatalf("physical selection lost exact captured identity/state: %+v %v %+v", got, err, peer.request)
			}
			assertSelectedTerminalSummaryTiming(t, name, got, end)
			if name != "wrong-attempt" && name != "owner-loss-wrong-attempt" && (got.Provider != "codex" || got.ContinuationHeadWorkerSessionID != "worker") {
				t.Fatalf("captured provider/head lost: %+v", got)
			}
			if name == "owner-loss" && (got.TerminalCause == nil || *got.TerminalCause != "OWNER_LOST" || got.Failure.Detail != captured.Failure.Detail) {
				t.Fatalf("captured owner-loss facts lost: %+v", got)
			}
		})
	}
}

func assertSelectedTerminalSummaryTiming(t *testing.T, name string, got workersessions.Observation, end time.Time) {
	t.Helper()
	if name == "captured" || name == "different-outcome" {
		if got.EndedAt == nil || !got.EndedAt.Equal(end) || got.Duration == nil || *got.Duration != time.Second {
			t.Fatalf("captured terminal facts lost: %+v", got)
		}
		assertSelectedTerminalOutcome(t, name, got)
	} else if got.EndedAt != nil || got.Duration != nil || got.DurationBasis != workersessions.DurationBasisUnavailable {
		t.Fatalf("invented terminal timing: %+v", got)
	}
}

func assertSelectedTerminalOutcome(t *testing.T, name string, got workersessions.Observation) {
	t.Helper()
	if name == "captured" && (got.Failure == nil || got.Failure.Detail != "expected artifact was not produced") {
		t.Fatalf("physical failure lost: %+v", got)
	}
	if name == "different-outcome" && (got.Failure != nil || got.TerminalCause == nil || *got.TerminalCause != "COMPLETED") {
		t.Fatalf("Work rejection overwrote physical success: %+v", got)
	}
}

func TestRuntimeModelRecordingEnabledRequiresRuntimeOwnerAndRecorder(t *testing.T) {
	t.Parallel()

	ledger := &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	cases := []struct {
		name string
		cfg  *runtimeConfig
		want bool
	}{
		{name: "nil config", cfg: nil, want: false},
		{name: "missing ledger", cfg: &runtimeConfig{executeService: modelRecordingExecuteService{owns: true}}, want: false},
		{name: "missing runtime owner", cfg: &runtimeConfig{eventHistory: ledger, executeService: modelRecordingExecuteService{}}, want: false},
		{name: "runtime owner disabled", cfg: &runtimeConfig{eventHistory: ledger, executeService: modelRecordingExecuteService{}}, want: false},
		{
			name: "ledger does not expose worker recorder",
			cfg: &runtimeConfig{
				eventHistory:   runtimeLedgerWithoutWorkerRecorder{RuntimeLedger: ledger.ScriptedRuntimeLedger},
				executeService: modelRecordingExecuteService{owns: true},
			},
			want: false,
		},
		{name: "runtime owner and recorder", cfg: &runtimeConfig{eventHistory: ledger, executeService: modelRecordingExecuteService{owns: true}}, want: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeModelRecordingEnabled(test.cfg); got != test.want {
				t.Fatalf("runtimeModelRecordingEnabled() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestPrepareDetachedModelRecordingRecordsDetachedRequestAndResponse(t *testing.T) {
	t.Parallel()

	ledger := &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	previousCalled := false
	previousTerminalCalled := false
	cfg := &runtimeConfig{
		executeService: modelRecordingExecuteService{owns: true},
		eventHistory:   ledger,
		clock:          testRuntimeClock{},
	}
	request := modelRecordingRequest()
	prepared := prepareDetachedModelRecording(cfg, func(context.Context, *workers.ExecuteRequest) (attemptCompletionFunc, error) {
		previousCalled = true
		return func(_ context.Context, _ workers.ExecuteRequest, result workers.ExecuteResult, err error) (workers.ExecuteResult, error) {
			previousTerminalCalled = true
			return result, err
		}, nil
	})
	terminal, err := prepared(context.Background(), &request)
	if err != nil {
		t.Fatalf("prepared() error = %v", err)
	}
	if !previousCalled {
		t.Fatal("previous preparation was not called")
	}
	request.Input.PreparedRequestObserver(request)
	assertDetachedModelRequestEvent(t, ledger.events)
	_, terminalErr := terminal(context.Background(), request, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "model output",
		}}},
		Continuation: &workers.ProviderContinuationRef{Provider: "provider-1", ProviderSessionID: "session-1"},
		Diagnostics: &workers.SafeDiagnostics{Provider: &workers.SafeProviderDiagnostic{
			Provider:         "agy",
			ResponseMetadata: map[string]string{"input_tokens": "3"},
		}},
		Metrics: workers.ExecutionMetrics{Duration: 1500 * time.Millisecond},
	}, nil)
	if terminalErr != nil {
		t.Fatalf("terminal model recording: %v", terminalErr)
	}
	if !previousTerminalCalled {
		t.Fatal("previous terminal hook was not called")
	}
	assertDetachedModelResponseEvent(t, ledger.events)
	assertDetachedModelResponseContentDecodesStructuredWorkerText(t)
	request.Input.ModelBindings[0].Content[0].Text = "mutated"
	if (*ledger.events[1].Response.Bindings)[0].Content[0].Text != "binding" {
		t.Fatal("recorded response bindings alias the Execute request")
	}
}

func assertDetachedModelRequestEvent(t *testing.T, events []workers.ModelEvent) {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("request events = %#v, want one model request", events)
	}
	recorded := events[0]
	if recorded.Kind != workers.ModelEventKindRequest {
		t.Fatalf("request event kind = %q, want request", recorded.Kind)
	}
	assertDetachedModelRequestIdentity(t, recorded)
	assertDetachedModelRequestPayload(t, recorded.Request)
}

func assertDetachedModelRequestIdentity(t *testing.T, recorded workers.ModelEvent) {
	t.Helper()
	if recorded.ID != "factory-event/model-request/dispatch-1/model-request/1" || recorded.Tick != 7 {
		t.Fatalf("recorded request identity = %#v, want detached correlation", recorded)
	}
	if recorded.RequestID != "request-1" || len(recorded.TraceIDs) != 1 || recorded.TraceIDs[0] != "trace-1" {
		t.Fatalf("recorded request correlation = %#v, want detached correlation", recorded)
	}
}

func assertDetachedModelRequestPayload(t *testing.T, request *workers.ModelRequestEventPayload) {
	t.Helper()
	if request == nil {
		t.Fatal("recorded request payload is nil")
	}
	if request.Operation != "summarize" || request.Worker != "worker-1" || request.Model != "model-1" || request.ProviderLocality != "remote" {
		t.Fatalf("recorded request payload = %#v, want resolved model fields", request)
	}
	if request.WorkingDirectory == nil || *request.WorkingDirectory != "/workspace" {
		t.Fatalf("recorded request working directory = %#v, want /workspace", request.WorkingDirectory)
	}
	if request.Worktree == nil || *request.Worktree != "feature-1" {
		t.Fatalf("recorded request worktree = %#v, want feature-1", request.Worktree)
	}
}

func assertDetachedModelResponseEvent(t *testing.T, events []workers.ModelEvent) {
	t.Helper()
	if len(events) != 2 {
		t.Fatalf("response events = %#v, want one model response", events)
	}
	if events[1].Kind != workers.ModelEventKindResponse {
		t.Fatalf("response event kind = %q, want response", events[1].Kind)
	}
	response := events[1].Response
	if response == nil {
		t.Fatal("recorded response is nil")
	}
	assertDetachedModelResponseMetadata(t, response)
	assertDetachedModelResponseContent(t, response)
}

func assertDetachedModelResponseMetadata(t *testing.T, response *workers.ModelResponseEventPayload) {
	t.Helper()
	if response.Outcome != workers.InferenceOutcomeSucceeded || response.ModelRequestID != "dispatch-1/model-request/1" || response.DurationMillis != 1500 {
		t.Fatalf("recorded response = %#v, want successful detached model response", response)
	}
	providerSession := (response.Continuation).SessionMetadata()
	if providerSession == nil || providerSession.ID != "session-1" {
		t.Fatalf("recorded provider session = %#v, want session-1", providerSession)
	}
}

func assertDetachedModelResponseContent(t *testing.T, response *workers.ModelResponseEventPayload) {
	t.Helper()
	if response.OutputContent == nil || len(*response.OutputContent) != 1 || (*response.OutputContent)[0].Text != "model output" {
		t.Fatalf("recorded response output = %#v, want detached content", response.OutputContent)
	}
	if response.Bindings == nil || len(*response.Bindings) != 1 || (*response.Bindings)[0].Slot != "summary" {
		t.Fatalf("recorded response bindings = %#v, want detached bindings", response.Bindings)
	}
	diagnostics, err := workers.SafeWorkDiagnosticsFromEventPayload(response.Diagnostics)
	if err != nil {
		t.Fatalf("decode recorded response diagnostics: %v", err)
	}
	if diagnostics == nil || diagnostics.Provider == nil || diagnostics.Provider.ResponseMetadata["input_tokens"] != "3" {
		t.Fatalf("recorded response diagnostics = %#v, want public provider metadata", diagnostics)
	}
}

func assertDetachedModelResponseContentDecodesStructuredWorkerText(t *testing.T) {
	t.Helper()
	raw, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        "C:/tmp/factory-work.wav",
		ContentType: "audio/wav",
	}})
	if err != nil {
		t.Fatalf("marshal structured worker output: %v", err)
	}
	got := detachedModelResponseContent([]work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: string(raw),
	}})
	if len(got) != 1 || got[0].Type != work.WorkContentPartTypeAudio || got[0].File != "C:/tmp/factory-work.wav" || got[0].ContentType != "audio/wav" {
		t.Fatalf("decoded detached response content = %#v, want one audio part", got)
	}

	plain := detachedModelResponseContent([]work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: "plain provider response",
	}})
	if len(plain) != 1 || plain[0].Type != work.WorkContentPartTypeText || plain[0].Text != "plain provider response" {
		t.Fatalf("plain detached response content = %#v, want unchanged text", plain)
	}
}

func TestRuntimeModelResponseRecordingPreservesProviderSuccessForOutputContractFailure(t *testing.T) {
	t.Parallel()

	ledger := &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	cfg := &runtimeConfig{
		executeService: modelRecordingExecuteService{owns: true},
		eventHistory:   ledger,
		clock:          testRuntimeClock{},
	}
	recordDetachedModelResponse(cfg, modelRecordingRequest(), workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypePermanentBadRequest,
			Message: "output contract failed",
		},
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "provider response",
		}}},
	}, nil)

	if len(ledger.events) != 1 || ledger.events[0].Response == nil {
		t.Fatalf("recorded events = %#v, want one model response", ledger.events)
	}
	response := ledger.events[0].Response
	if response.Outcome != workers.InferenceOutcomeSucceeded {
		t.Fatalf("provider response outcome = %q, want SUCCEEDED", response.Outcome)
	}
	if response.OutputContent == nil || len(*response.OutputContent) != 1 || (*response.OutputContent)[0].Text != "provider response" {
		t.Fatalf("provider response output = %#v, want raw provider response", response.OutputContent)
	}
}

func TestRuntimeModelResponseRecordingClassifiesFailuresAndContinuationFallbacks(t *testing.T) {
	t.Parallel()

	ledger := &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}}
	cfg := &runtimeConfig{
		executeService: modelRecordingExecuteService{owns: true},
		eventHistory:   ledger,
		clock:          testRuntimeClock{},
	}
	request := modelRecordingRequest()
	request.Target.Provider.ID = "antigravity"
	request.Attempt.Number = 2
	request.Target.WorkerName = ""
	request.Target.WorkerType = "worker-type"
	request.Correlation.TraceID = ""
	request.Input.Dispatch.Execution.CurrentTick = 9

	recordDetachedModelResponse(cfg, request, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type: workers.WorkFailureTypeTimeout, Message: "timeout",
			Detail: &workers.FailureDetail{Reason: workers.WorkFailureTypeTimeout, Message: "safe timeout"},
		},
	}, nil)
	recordDetachedModelResponse(cfg, request, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeCanceled,
		Failure: &workers.ExecutionFailure{Type: workers.WorkFailureTypeUnknown, Message: "cancelled"},
	}, nil)
	recordDetachedModelResponse(cfg, request, workers.ExecuteResult{}, errors.New("transport failed"))

	if len(ledger.events) != 3 {
		t.Fatalf("failure events = %#v, want three responses", ledger.events)
	}
	assertModelFailureEvents(t, ledger.events)
	if got := providerSessionFromExecuteResult(request, workers.ExecuteResult{
		Continuation: &workers.ProviderContinuationRef{Provider: "provider-2", ExternalRef: "external-2"},
	}); got == nil || got.ID != "external-2" || got.Provider != "provider-2" {
		t.Fatalf("external continuation session = %#v, want external reference", got)
	}
}

func assertModelFailureEvents(t *testing.T, events []workers.ModelEvent) {
	t.Helper()
	assertModelFailureProviderSessions(t, events)
	if got := events[0].Response.FailureDetail; got == nil || got.Message != "safe timeout" {
		t.Fatalf("detailed failure = %#v, want cloned safe detail", got)
	}
	if got := events[1].Response.FailureDetail; got == nil || got.Reason != workers.WorkFailureTypeUnknown || got.Message != "cancelled" {
		t.Fatalf("fallback failure = %#v, want execution failure fields", got)
	}
	if got := events[2].Response.FailureDetail; got == nil || got.Reason != workers.WorkFailureTypeUnknown {
		t.Fatalf("error failure = %#v, want unknown detail", got)
	}
	for _, event := range events {
		if event.Response == nil || event.Response.Outcome != workers.InferenceOutcomeFailed || event.Tick != 9 {
			t.Fatalf("failure response = %#v, want failed response at current tick", event.Response)
		}
	}
}

func assertModelFailureProviderSessions(t *testing.T, events []workers.ModelEvent) {
	t.Helper()
	for _, event := range events {
		if event.Response == nil {
			t.Fatal("failure response is nil")
		}
		providerSession := event.Response.ProviderSession
		if providerSession == nil || providerSession.Provider != "antigravity" || providerSession.ID != "" {
			t.Fatalf("failure provider session = %#v, want provider-only antigravity metadata", event.Response)
		}
	}
}

func TestRuntimeModelRecordingHelpersNormalizeOptionalAndExecutionValues(t *testing.T) {
	t.Parallel()
	assertModelExecutionClassification(t)
	assertModelCorrelationHelpers(t)
	assertModelOptionalHelpers(t)
}

func assertModelExecutionClassification(t *testing.T) {
	t.Helper()
	if isModelExecution(workers.ExecuteRequest{Target: workers.ExecutionTarget{RunnerID: "script", Model: workers.ModelReference{Name: "model"}}}) {
		t.Fatal("script execution was classified as model execution")
	}
	if isModelExecution(workers.ExecuteRequest{Target: workers.ExecutionTarget{RunnerID: "inference", Model: workers.ModelReference{Name: "model"}}}) {
		t.Fatal("inference execution was classified as model execution")
	}
	if isModelExecution(workers.ExecuteRequest{Target: workers.ExecutionTarget{RunnerID: "codex"}}) {
		t.Fatal("runner without model was classified as model execution")
	}
	if !isModelExecution(modelRecordingRequest()) {
		t.Fatal("model execution was not recognized")
	}
}

func assertModelCorrelationHelpers(t *testing.T) {
	t.Helper()
	if got := detachedModelRequestID(" dispatch ", 3); got != "dispatch/model-request/3" {
		t.Fatalf("detachedModelRequestID() = %q", got)
	}
	if got := modelEventTick(modelRecordingRequest()); got != 7 {
		t.Fatalf("modelEventTick() = %d, want dispatch-created tick", got)
	}
	request := modelRecordingRequest()
	request.Input.Dispatch.Execution.CurrentTick = 8
	if got := modelEventTick(request); got != 8 {
		t.Fatalf("modelEventTick() = %d, want current tick", got)
	}
	if got := executionWorkerName(request); got != "worker-1" {
		t.Fatalf("executionWorkerName() = %q, want worker name", got)
	}
	request.Target.WorkerName = ""
	request.Target.WorkerType = "worker-type"
	if got := executionWorkerName(request); got != "worker-type" {
		t.Fatalf("executionWorkerName() fallback = %q, want worker type", got)
	}
}

func assertModelOptionalHelpers(t *testing.T) {
	t.Helper()
	if optionalString(" ") != nil || nonEmptyStrings(" ") != nil {
		t.Fatal("blank optional values were retained")
	}
	if got := optionalString(" value "); got == nil || *got != "value" {
		t.Fatalf("optionalString() = %#v, want trimmed pointer", got)
	}
	if got := nonEmptyStrings(" trace "); len(got) != 1 || got[0] != " trace " {
		t.Fatalf("nonEmptyStrings() = %#v, want original non-empty value", got)
	}
	if resolvedModelBindings(nil) != nil {
		t.Fatal("empty model bindings returned non-nil pointer")
	}
	bindings := []workers.ResolvedModelOperationBinding{{Slot: "slot", Content: []work.WorkContentPart{{Text: "content"}}}}
	cloned := resolvedModelBindings(bindings)
	if cloned == nil || len(*cloned) != 1 || (*cloned)[0].Slot != "slot" {
		t.Fatalf("resolvedModelBindings() = %#v, want cloned bindings", cloned)
	}
	assertModelProviderSessionFallbacks(t)
}

func assertModelProviderSessionFallbacks(t *testing.T) {
	t.Helper()
	request := modelRecordingRequest()
	if providerSessionFromExecuteResult(request, workers.ExecuteResult{}) != nil {
		t.Fatal("empty continuation returned a provider session")
	}
	if providerSessionFromExecuteResult(request, workers.ExecuteResult{Continuation: &workers.ProviderContinuationRef{}}) != nil {
		t.Fatal("empty provider continuation returned a provider session")
	}
	request.Target.Provider.ID = "agent"
	if got := providerSessionFromExecuteResult(request, workers.ExecuteResult{}); got == nil || got.Provider != "cursor" || got.ID != "" {
		t.Fatalf("provider fallback = %#v, want canonical provider identity without a fabricated session id", got)
	}
}

func TestPrepareDetachedModelRecordingPreservesDisabledAndPreviousErrors(t *testing.T) {
	t.Parallel()

	previousCalled := false
	previous := func(context.Context, *workers.ExecuteRequest) (attemptCompletionFunc, error) {
		previousCalled = true
		return nil, nil
	}
	prepared := prepareDetachedModelRecording(nil, previous)
	disabledRequest := modelRecordingRequest()
	if _, err := prepared(context.Background(), &disabledRequest); err != nil {
		t.Fatalf("disabled preparation error = %v", err)
	}
	if !previousCalled {
		t.Fatal("disabled preparation did not preserve previous preparation")
	}

	cfg := &runtimeConfig{
		executeService: modelRecordingExecuteService{owns: true},
		eventHistory:   &modelRecordingLedger{ScriptedRuntimeLedger: &recordingfixtures.ScriptedRuntimeLedger{}},
		clock:          testRuntimeClock{},
	}
	wantErr := errors.New("previous preparation failed")
	prepared = prepareDetachedModelRecording(cfg, func(context.Context, *workers.ExecuteRequest) (attemptCompletionFunc, error) {
		return nil, wantErr
	})
	failedPreparationRequest := modelRecordingRequest()
	if _, err := prepared(context.Background(), &failedPreparationRequest); !errors.Is(err, wantErr) {
		t.Fatalf("previous preparation error = %v, want %v", err, wantErr)
	}
}

func modelRecordingRequest() workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-1", RequestID: "request-1", TraceID: "trace-1"},
		Target: workers.ExecutionTarget{
			WorkerName: "worker-1", WorkerType: "agent", RunnerID: "codex",
			Model:       workers.ModelReference{Name: "model-1", Locality: "remote"},
			Environment: workers.EnvironmentPolicy{WorkingDirectory: " /workspace "},
			Workspace:   workers.WorkspacePolicy{Worktree: " feature-1 "},
		},
		Input: workers.ExecutionInput{
			ModelOperation: " summarize ",
			ModelBindings: []workers.ResolvedModelOperationBinding{{
				Slot: "summary", Source: workers.ModelOperationBindingSourceInput,
				Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "binding"}},
			}},
			Dispatch: work.WorkDispatch{Execution: work.ExecutionMetadata{
				DispatchCreatedTick: 7, RequestID: "request-1", TraceID: "trace-1", WorkIDs: []string{"work-1"},
			}},
		},
	}
}

type modelRecordingExecuteService struct{ owns bool }

func (service modelRecordingExecuteService) Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{}, nil
}

func (service modelRecordingExecuteService) RuntimeOwnsModelEventRecording() bool {
	return service.owns
}

type modelRecordingLedger struct {
	*recordingfixtures.ScriptedRuntimeLedger
	events []workers.ModelEvent
}

func (ledger *modelRecordingLedger) RecordModelEvent(event workers.ModelEvent) {
	ledger.events = append(ledger.events, event)
}

type runtimeLedgerWithoutWorkerRecorder struct{ recordings.RuntimeLedger }
