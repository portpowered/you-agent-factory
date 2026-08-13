package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	previous := func(context.Context, workers.ExecuteRequest) (attemptTerminalFunc, error) {
		return func(context.Context, workers.ExecuteRequest, workers.ExecuteResult, error) {
			previousTerminalCalled = true
		}, nil
	}
	cfg := &runtimeConfig{
		executeService: modelRecordingExecuteService{owns: true},
		eventHistory:   ledger,
		clock:          testRuntimeClock{},
	}
	request := modelRecordingRequest()
	prepared := prepareDetachedModelRecording(cfg, func(ctx context.Context, req workers.ExecuteRequest) (attemptTerminalFunc, error) {
		previousCalled = true
		return previous(ctx, req)
	})
	terminal, err := prepared(context.Background(), request)
	if err != nil {
		t.Fatalf("prepared() error = %v", err)
	}
	if !previousCalled {
		t.Fatal("previous preparation was not called")
	}
	if len(ledger.events) != 1 || ledger.events[0].Kind != workers.ModelEventKindRequest {
		t.Fatalf("request events = %#v, want one model request", ledger.events)
	}
	recordedRequest := ledger.events[0]
	if recordedRequest.ID != "factory-event/model-request/dispatch-1/model-request/1" ||
		recordedRequest.Tick != 7 || recordedRequest.RequestID != "request-1" ||
		len(recordedRequest.TraceIDs) != 1 || recordedRequest.TraceIDs[0] != "trace-1" {
		t.Fatalf("recorded request identity = %#v, want detached correlation", recordedRequest)
	}
	if recordedRequest.Request == nil || recordedRequest.Request.Operation != "summarize" ||
		recordedRequest.Request.Worker != "worker-1" || recordedRequest.Request.Model != "model-1" ||
		recordedRequest.Request.ProviderLocality != "remote" {
		t.Fatalf("recorded request payload = %#v, want resolved model fields", recordedRequest.Request)
	}
	if recordedRequest.Request.WorkingDirectory == nil || *recordedRequest.Request.WorkingDirectory != "/workspace" ||
		recordedRequest.Request.Worktree == nil || *recordedRequest.Request.Worktree != "feature-1" {
		t.Fatalf("recorded request paths = %#v, want working directory and worktree", recordedRequest.Request)
	}

	terminal(context.Background(), request, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "model output",
		}}},
		Continuation: &workers.ProviderContinuationRef{Provider: "provider-1", ProviderSessionID: "session-1"},
		Diagnostics:  &workers.SafeDiagnostics{Metadata: map[string]string{"source": "test"}},
		Metrics:      workers.ExecutionMetrics{Duration: 1500 * time.Millisecond},
	}, nil)
	if !previousTerminalCalled {
		t.Fatal("previous terminal hook was not called")
	}
	if len(ledger.events) != 2 || ledger.events[1].Kind != workers.ModelEventKindResponse {
		t.Fatalf("response events = %#v, want one model response", ledger.events)
	}
	recordedResponse := ledger.events[1]
	if recordedResponse.Response == nil || recordedResponse.Response.Outcome != workers.InferenceOutcomeSucceeded ||
		recordedResponse.Response.ModelRequestID != "dispatch-1/model-request/1" ||
		recordedResponse.Response.DurationMillis != 1500 || recordedResponse.Response.ProviderSession == nil ||
		recordedResponse.Response.ProviderSession.ID != "session-1" {
		t.Fatalf("recorded response = %#v, want successful detached model response", recordedResponse.Response)
	}
	if recordedResponse.Response.OutputContent == nil || len(*recordedResponse.Response.OutputContent) != 1 ||
		(*recordedResponse.Response.OutputContent)[0].Text != "model output" {
		t.Fatalf("recorded response output = %#v, want detached content", recordedResponse.Response.OutputContent)
	}
	if recordedResponse.Response.Bindings == nil || len(*recordedResponse.Response.Bindings) != 1 || (*recordedResponse.Response.Bindings)[0].Slot != "summary" {
		t.Fatalf("recorded response bindings = %#v, want detached bindings", recordedResponse.Response.Bindings)
	}
	if len(recordedResponse.Response.Diagnostics) == 0 || !strings.Contains(string(recordedResponse.Response.Diagnostics), "source") {
		t.Fatalf("recorded response diagnostics = %s, want safe diagnostics", recordedResponse.Response.Diagnostics)
	}

	request.Input.ModelBindings[0].Content[0].Text = "mutated"
	if (*recordedResponse.Response.Bindings)[0].Content[0].Text != "binding" {
		t.Fatal("recorded response bindings alias the Execute request")
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
	request.Attempt.Number = 2
	request.Target.WorkerName = ""
	request.Target.WorkerType = "worker-type"
	request.Correlation.TraceID = ""
	request.Input.Dispatch.Execution.CurrentTick = 9

	recordDetachedModelResponse(cfg, request, workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeFailed,
		Failure: &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeTimeout,
			Message: "timeout",
			Detail:  &workers.FailureDetail{Reason: workers.WorkFailureTypeTimeout, Message: "safe timeout"},
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
	if got := ledger.events[0].Response.FailureDetail; got == nil || got.Message != "safe timeout" {
		t.Fatalf("detailed failure = %#v, want cloned safe detail", got)
	}
	if got := ledger.events[1].Response.FailureDetail; got == nil || got.Reason != workers.WorkFailureTypeUnknown || got.Message != "cancelled" {
		t.Fatalf("fallback failure = %#v, want execution failure fields", got)
	}
	if got := ledger.events[2].Response.FailureDetail; got == nil || got.Reason != workers.WorkFailureTypeUnknown {
		t.Fatalf("error failure = %#v, want unknown detail", got)
	}
	for _, event := range ledger.events {
		if event.Response == nil || event.Response.Outcome != workers.InferenceOutcomeFailed || event.Tick != 9 {
			t.Fatalf("failure response = %#v, want failed response at current tick", event.Response)
		}
	}

	if got := providerSessionFromExecuteResult(workers.ExecuteResult{
		Continuation: &workers.ProviderContinuationRef{Provider: "provider-2", ExternalRef: "external-2"},
	}); got == nil || got.ID != "external-2" || got.Provider != "provider-2" {
		t.Fatalf("external continuation session = %#v, want external reference", got)
	}
}

func TestRuntimeModelRecordingHelpersNormalizeOptionalAndExecutionValues(t *testing.T) {
	t.Parallel()

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
	if providerSessionFromExecuteResult(workers.ExecuteResult{}) != nil {
		t.Fatal("empty continuation returned a provider session")
	}
	if providerSessionFromExecuteResult(workers.ExecuteResult{Continuation: &workers.ProviderContinuationRef{}}) != nil {
		t.Fatal("empty provider continuation returned a provider session")
	}
}

func TestPrepareDetachedModelRecordingPreservesDisabledAndPreviousErrors(t *testing.T) {
	t.Parallel()

	previousCalled := false
	previous := func(context.Context, workers.ExecuteRequest) (attemptTerminalFunc, error) {
		previousCalled = true
		return nil, nil
	}
	prepared := prepareDetachedModelRecording(nil, previous)
	if _, err := prepared(context.Background(), modelRecordingRequest()); err != nil {
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
	prepared = prepareDetachedModelRecording(cfg, func(context.Context, workers.ExecuteRequest) (attemptTerminalFunc, error) {
		return nil, wantErr
	})
	if _, err := prepared(context.Background(), modelRecordingRequest()); !errors.Is(err, wantErr) {
		t.Fatalf("previous preparation error = %v, want %v", err, wantErr)
	}
}

func modelRecordingRequest() workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "dispatch-1",
			RequestID:  "request-1",
			TraceID:    "trace-1",
		},
		Target: workers.ExecutionTarget{
			WorkerName:  "worker-1",
			WorkerType:  "agent",
			RunnerID:    "codex",
			Model:       workers.ModelReference{Name: "model-1", Locality: "remote"},
			Environment: workers.EnvironmentPolicy{WorkingDirectory: " /workspace "},
			Workspace:   workers.WorkspacePolicy{Worktree: " feature-1 "},
		},
		Input: workers.ExecutionInput{
			ModelOperation: " summarize ",
			ModelBindings: []workers.ResolvedModelOperationBinding{{
				Slot:    "summary",
				Source:  workers.ModelOperationBindingSourceInput,
				Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "binding"}},
			}},
			Dispatch: work.WorkDispatch{
				Execution: work.ExecutionMetadata{
					DispatchCreatedTick: 7,
					RequestID:           "request-1",
					TraceID:             "trace-1",
					WorkIDs:             []string{"work-1"},
				},
			},
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

type runtimeLedgerWithoutWorkerRecorder struct {
	recordings.RuntimeLedger
}
