package executor

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestAgentExecutor_ModelOperationOutputUsesCanonicalWorkContent(t *testing.T) {
	audioPath := "/tmp/agent-canonical-output.wav"
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{
		Content: `[{"type":"AUDIO","file":"` + audioPath + `","contentType":"audio/wav"}]`,
	}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"tts-worker": {
				Type: workerconfig.WorkerTypeModel,
				Operations: []workerconfig.ModelOperation{{
					Name: "TTS",
					Outputs: []workerconfig.ModelOperationSlot{{
						Name:         "audio",
						ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio},
					}},
				}},
			},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "dispatch-tts",
			TransitionID: "transition-tts",
			WorkerType:   "tts-worker",
		},
		withAgentModelOperation("TTS", nil),
	))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}

	var content []work.WorkContentPart
	if err := json.Unmarshal([]byte(result.Output), &content); err != nil {
		t.Fatalf("decode canonical output: %v", err)
	}
	if len(content) != 1 || content[0].Type != work.WorkContentPartTypeAudio || content[0].File != audioPath {
		t.Fatalf("output = %#v, want canonical audio WorkContent", content)
	}
}

func TestAgentExecutor_RetryableFailureRetriesWithPreservedSession(t *testing.T) {
	const sessionID = "675f9238-5f05-456c-9a9f-f8fe486f49e4"
	throttleErr := workerexecution.NewProviderErrorWithSession(
		workerexecution.WorkFailureTypeThrottled,
		"temporarily unavailable",
		nil,
		(&providers.SessionMetadata{
			Provider: string(modelprovider.ProviderKiro),
			Kind:     providerSessionKindSessionID,
			ID:       sessionID,
		}).ContinuationRef(),
	)
	provider := &agentMockProvider{
		errors: []error{throttleErr, nil},
		responses: []workerexecution.InferenceResponse{
			{},
			{Content: "ok"},
		},
	}
	executor := NewAgentExecutor(
		staticRuntimeConfig{
			Workers: map[string]*workerconfig.FactoryWorkerConfig{
				"worker-a": {Model: "kiro-auto", ModelProvider: string(modelprovider.ProviderKiro)},
			},
		},
		provider,
		nil,
		time.Now,
	)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{
			DispatchID:   "d-kiro-retry",
			TransitionID: "t-1",
			WorkerType:   "worker-a",
		},
		withAgentPrompts("sys", "msg"),
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if provider.callCount != 2 {
		t.Fatalf("provider call count = %d, want 2", provider.callCount)
	}
	if provider.lastReq.SessionID != sessionID {
		t.Fatalf("retry SessionID = %q, want %q", provider.lastReq.SessionID, sessionID)
	}
	if !containsRunnerCapability(
		provider.lastReq.RequiredOptionalCapabilities,
		workerexecution.RunnerOptionalCapabilitySessionResume,
	) {
		t.Fatalf("retry capabilities = %#v, want session resume", provider.lastReq.RequiredOptionalCapabilities)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, workerexecution.OutcomeAccepted)
	}
}

func containsRunnerCapability(
	capabilities []workerexecution.RunnerOptionalCapability,
	want workerexecution.RunnerOptionalCapability,
) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

func TestInferenceRequestForExecutionRequest_ForwardsModelOperationContract(t *testing.T) {
	req := testAgentRequest(
		work.WorkDispatch{
			DispatchID:      "d-tts",
			TransitionID:    "t-tts",
			WorkerType:      "worker-a",
			WorkstationName: "speak",
		},
		withAgentPrompts("System prompt", "Speak"),
		withAgentModelOperation("TTS", []workerexecution.ResolvedModelOperationBinding{
			{
				Slot:   "text",
				Source: workerexecution.ModelOperationBindingSourceInput,
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeText,
					Text: "hello",
				}},
			},
			{
				Slot:   "voice",
				Source: workerexecution.ModelOperationBindingSourceConfig,
				Content: []work.WorkContentPart{{
					Type: work.WorkContentPartTypeJSON,
					JSON: []byte(`{"name":"alloy"}`),
				}},
			},
		}),
	)

	got := inferenceRequestForExecutionRequest(req, &workerconfig.FactoryWorkerConfig{
		Model:         "OMNIVOICE_Q4_K_M",
		ModelProvider: workerexecution.RunnerIDCodex,
		ModelLocality: workerconfig.ModelLocalityLocal,
	}, nil)

	if got.ModelOperation != "TTS" {
		t.Fatalf("model operation = %q, want TTS", got.ModelOperation)
	}
	if got.ModelLocality != workerconfig.ModelLocalityLocal {
		t.Fatalf("model locality = %q, want %q", got.ModelLocality, workerconfig.ModelLocalityLocal)
	}
	if len(got.ModelBindings) != 2 {
		t.Fatalf("model bindings = %#v, want 2 entries", got.ModelBindings)
	}
	if got.ModelBindings[0].Slot != "text" || got.ModelBindings[0].Content[0].Text != "hello" {
		t.Fatalf("text binding = %#v, want forwarded text binding", got.ModelBindings[0])
	}
	if got.ModelBindings[1].Slot != "voice" || string(got.ModelBindings[1].Content[0].JSON) != `{"name":"alloy"}` {
		t.Fatalf("voice binding = %#v, want forwarded config binding", got.ModelBindings[1])
	}

	got.ModelBindings[0].Content[0].Text = "changed"
	if req.ModelBindings[0].Content[0].Text != "hello" {
		t.Fatalf("request bindings mutated original execution request: %#v", req.ModelBindings)
	}
}

func TestInferenceRequestForExecutionRequest_ForwardsAntigravityPrintTimeout(t *testing.T) {
	got := inferenceRequestForExecutionRequest(
		testAgentRequest(work.WorkDispatch{
			DispatchID: "d-agy-timeout",
			WorkerType: "agy-worker",
		}),
		&workerconfig.FactoryWorkerConfig{
			Model:         "gemini-3.6-flash-high",
			ModelProvider: string(modelprovider.ProviderAntigravity),
			Timeout:       "8m",
		},
		nil,
	)

	if got.PrintTimeout != 8*time.Minute {
		t.Fatalf("PrintTimeout = %s, want 8m", got.PrintTimeout)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this inference request contract test keeps the canonical dispatch payload assertions together on the worker seam.
func TestAgentExecutor_InferenceRequestUsesCanonicalWorkDispatchPayload(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: "done"}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {
				Model:         "claude-sonnet-4-20250514",
				ModelProvider: string(modelprovider.ProviderClaude),
				SessionID:     "session-1",
			},
		},
	}, provider, nil, time.Now)

	inputToken := factoryruntime.RuntimeToken{
		ID: "token-1",
		Color: factoryruntime.RuntimeTokenColor{
			WorkID:     "work-1",
			WorkTypeID: "task",
			TraceID:    "trace-1",
		},
	}
	dispatch := work.WorkDispatch{
		DispatchID:      "dispatch-1",
		TransitionID:    "transition-1",
		WorkerType:      "worker-a",
		WorkstationName: "review",
		Execution:       work.ExecutionMetadata{ReplayKey: "transition-1/trace-1/work-1", TraceID: "trace-1", WorkIDs: []string{"work-1"}},
		InputTokens:     InputTokens(inputToken),
		InputBindings:   map[string][]string{"task": {"token-1"}},
	}
	request := testAgentRequest(
		dispatch,
		withAgentWorktree("feature-worktree"),
		withAgentWorkingDirectory("C:\\repo"),
		withAgentEnvVars(map[string]string{"PORTOS_TEST_ENV": "enabled"}),
		withAgentPrompts("system prompt", "user prompt"),
		withAgentOutputSchema(`{"type":"object"}`),
	)
	reference := providers.SessionRef{Provider: providers.IDClaude, Kind: "provider-native-thread", ID: "opaque-provider-session"}
	continuation := reference.ContinuationRef()
	request.Continuation = &continuation

	_, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := provider.lastReq
	if req.Dispatch.DispatchID != dispatch.DispatchID || req.Dispatch.TransitionID != dispatch.TransitionID || req.Dispatch.WorkerType != dispatch.WorkerType {
		t.Fatalf("request identity = %#v, want dispatch identity %#v", req, dispatch)
	}
	if req.Dispatch.WorkstationName != dispatch.WorkstationName || req.WorkstationType != dispatch.WorkstationName {
		t.Fatalf("request workstation fields = name %q type %q, want %q", req.Dispatch.WorkstationName, req.WorkstationType, dispatch.WorkstationName)
	}
	if req.SystemPrompt != request.SystemPrompt || req.UserMessage != request.UserMessage || req.OutputSchema != request.OutputSchema {
		t.Fatalf("request prompt fields differ from execution request: %#v", req)
	}
	if req.Worktree != request.Worktree || req.WorkingDirectory != request.WorkingDirectory {
		t.Fatalf("request paths = worktree %q working_directory %q", req.Worktree, req.WorkingDirectory)
	}
	if req.Model != "claude-sonnet-4-20250514" || req.ModelProvider != string(modelprovider.ProviderClaude) || req.SessionID != "session-1" {
		t.Fatalf("request provider fields = model %q provider %q session %q", req.Model, req.ModelProvider, req.SessionID)
	}
	if req.Continuation == nil || *req.Continuation != continuation {
		t.Fatalf("request Continuation = %#v, want exact %#v", req.Continuation, continuation)
	}
	request.Continuation.ProviderSessionID = "caller-mutated"
	if req.Continuation.ProviderSessionID != "opaque-provider-session" {
		t.Fatalf("request Continuation retained caller mutation: %#v", req.Continuation)
	}
	if req.EnvVars["PORTOS_TEST_ENV"] != "enabled" {
		t.Fatalf("request env vars = %#v", req.EnvVars)
	}
	if got := req.Dispatch.InputBindings["task"]; len(got) != 1 || got[0] != "token-1" {
		t.Fatalf("request input bindings = %#v", req.Dispatch.InputBindings)
	}
	tokens := cloneInputTokens(req.InputTokens)
	if len(tokens) != 1 || tokens[0].ID != inputToken.ID || tokens[0].Color.WorkID != inputToken.Color.WorkID {
		t.Fatalf("request input tokens = %#v, want %#v", tokens, inputToken)
	}
	assertExecutionMetadataEqual(t, dispatch.Execution, req.Dispatch.Execution)
}

func TestInferenceRequestForExecutionRequest_DefaultWorkingDirectoryDoesNotRequireRunnerCapability(t *testing.T) {
	got := inferenceRequestForExecutionRequest(testAgentRequest(work.WorkDispatch{
		DispatchID: "d-default-workdir", TransitionID: "t-default-workdir", WorkerType: "worker-a", WorkstationName: "review",
	}, withAgentPrompts("System prompt", "Review"), withAgentWorkingDirectory("/tmp/runtime-session")), &workerconfig.FactoryWorkerConfig{
		Model: "gemini-1.5-pro", ModelProvider: workerexecution.RunnerIDGemini,
	}, nil)
	if got.WorkingDirectory != "/tmp/runtime-session" {
		t.Fatalf("working directory = %q, want forwarded runtime path", got.WorkingDirectory)
	}
	for _, capability := range got.RequiredOptionalCapabilities {
		if capability == workerexecution.RunnerOptionalCapabilityWorkingDirectory {
			t.Fatalf("capabilities = %#v, want default working directory omitted", got.RequiredOptionalCapabilities)
		}
	}
}

func TestAgentExecutorAcceptsSchemaValidatedStructuredOutput(t *testing.T) {
	const schema = `{"type":"object","properties":{"verdict":{"type":"string","enum":["pass","reroll"]},"confidence":{"type":"number"}},"required":["verdict","confidence"]}`
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{
		Content: `{"verdict":"pass","confidence":0.95}`,
	}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"clip-qa": {Model: "gemini-3.6-flash-high", ModelProvider: workerexecution.RunnerIDAntigravity},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "structured-pass", TransitionID: "qa", WorkerType: "clip-qa"},
		withAgentOutputSchema(schema),
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %q, want %q", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if result.Output != `{"verdict":"pass","confidence":0.95}` {
		t.Fatalf("output = %q, want structured JSON", result.Output)
	}
	wantStructured := map[string]any{"verdict": "pass", "confidence": json.Number("0.95")}
	if !reflect.DeepEqual(result.StructuredResult, wantStructured) || !result.StructuredResultPresent {
		t.Fatalf("structured result = %#v (present=%t), want detached native object %#v", result.StructuredResult, result.StructuredResultPresent, wantStructured)
	}
	if result.FailureMetadata != nil {
		t.Fatalf("failure metadata = %#v, want nil", result.FailureMetadata)
	}
}

func TestAgentExecutorClassifiesStructuredOutputSchemaViolations(t *testing.T) {
	const schema = `{"type":"object","properties":{"verdict":{"type":"string"}},"required":["verdict"]}`
	const rejectedMarker = "do-not-leak-this-rejected-value"
	tests := []struct {
		name              string
		schema            string
		response          string
		validationSummary string
	}{
		{name: "malformed_json", schema: schema, response: `{"verdict":`},
		{
			name:              "schema_mismatch_pattern",
			schema:            `{"type":"string","pattern":"^ok$"}`,
			response:          `"` + rejectedMarker + `"`,
			validationSummary: "pattern",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			assertAgentExecutorStructuredSchemaViolation(t, test.schema, rejectedMarker, test.name, test.response, test.validationSummary)
		})
	}
}

func assertAgentExecutorStructuredSchemaViolation(t *testing.T, schema, rejectedMarker, name, response, validationSummary string) {
	t.Helper()
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: response}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model", ModelProvider: workerexecution.RunnerIDAntigravity},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "structured-schema-" + name, TransitionID: "parse", WorkerType: "worker-a"},
		withAgentOutputSchema(schema),
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertAgentExecutorStructuredSchemaFailure(t, result)
	assertAgentExecutorStructuredSchemaOutput(t, result, response)
	assertAgentExecutorStructuredSchemaDiagnostic(t, result.Error, rejectedMarker, validationSummary)
}

func assertAgentExecutorStructuredSchemaFailure(t *testing.T, result workerexecution.WorkResult) {
	t.Helper()
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want failed", result.Outcome)
	}
	if result.FailureMetadata == nil || result.FailureMetadata.Family != workerexecution.WorkFailureFamilyTerminal ||
		result.FailureMetadata.Type != workerexecution.WorkFailureTypeStructuredOutputSchemaViolation {
		t.Fatalf("failure metadata = %#v, want terminal structured schema violation", result.FailureMetadata)
	}
	decision := workerexecution.FailureDecisionFromMetadata(result.FailureMetadata)
	if decision.Retryable || !decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("failure decision = %#v, want terminal non-retryable non-throttle", decision)
	}
	if result.StructuredResult != nil || result.StructuredResultPresent {
		t.Fatalf("structured result = %#v (present=%t), want rejected value omitted", result.StructuredResult, result.StructuredResultPresent)
	}
}

func assertAgentExecutorStructuredSchemaOutput(t *testing.T, result workerexecution.WorkResult, response string) {
	t.Helper()
	if result.Output != response {
		t.Fatalf("raw output = %q, want provider response retained for diagnostics", result.Output)
	}
}

func assertAgentExecutorStructuredSchemaDiagnostic(t *testing.T, diagnostic, rejectedMarker, validationSummary string) {
	t.Helper()
	if !strings.HasPrefix(diagnostic, "structured output schema violation: ") {
		t.Fatalf("error = %q, want stable schema-violation prefix", diagnostic)
	}
	if validationSummary != "" && !strings.Contains(diagnostic, validationSummary) {
		t.Fatalf("error = %q, want validation summary containing %q", diagnostic, validationSummary)
	}
	if validationSummary == "pattern" {
		for _, want := range []string{"instance $", "schema output schema#/pattern", `keyword "pattern"`} {
			if !strings.Contains(diagnostic, want) {
				t.Fatalf("error = %q, want actionable schema location %q", diagnostic, want)
			}
		}
	}
	if strings.Contains(diagnostic, rejectedMarker) {
		t.Fatalf("error = %q, want rejected response value excluded from diagnostic", diagnostic)
	}
}

func TestStructuredOutputValidationSummaryIsBounded(t *testing.T) {
	response := `"` + strings.Repeat("rejected-value-", 512) + `"`
	_, err := parseOutputAgainstSchema(response, []byte(`{"type":"string","pattern":"^ok$"}`))
	if err == nil {
		t.Fatal("parseOutputAgainstSchema() error = nil, want pattern validation failure")
	}
	if len(err.Error()) > structuredOutputValidationDetailLimit+128 {
		t.Fatalf("validation error length = %d, want bounded diagnostic: %q", len(err.Error()), err)
	}
	if strings.Contains(err.Error(), "rejected-value-") {
		t.Fatalf("validation error = %q, want rejected response content omitted", err)
	}
}

func TestAttachStructuredResultClassifiesStructuredOutputSchemaViolation(t *testing.T) {
	const response = `{"wrong":"do-not-leak-this-rejected-value"}`
	result := attachStructuredResult(
		workerexecution.WorkstationExecutionRequest{
			OutputSchema: `{"type":"object","required":["verdict"]}`,
		},
		workerexecution.WorkResult{
			Outcome: workerexecution.OutcomeAccepted,
			Output:  response,
		},
	)

	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want failed", result.Outcome)
	}
	if result.FailureMetadata == nil || result.FailureMetadata.Type != workerexecution.WorkFailureTypeStructuredOutputSchemaViolation {
		t.Fatalf("failure metadata = %#v, want structured schema violation", result.FailureMetadata)
	}
	if result.StructuredResult != nil || result.StructuredResultPresent {
		t.Fatalf("structured result = %#v (present=%t), want rejected value omitted", result.StructuredResult, result.StructuredResultPresent)
	}
	if result.Output != response {
		t.Fatalf("raw output = %q, want provider response retained for diagnostics", result.Output)
	}
}

func TestAgentExecutorPreservesNativeStructuredResultTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		schema   string
		response string
		want     any
	}{
		{
			name:     "object",
			schema:   `{"type":"object","properties":{"nested":{"type":"object"}},"required":["nested"]}`,
			response: `{"nested":{"label":"ready"}}`,
			want:     map[string]any{"nested": map[string]any{"label": "ready"}},
		},
		{
			name:     "array",
			schema:   `{"type":"array","items":{"type":"integer"}}`,
			response: `[1,2,3]`,
			want:     []any{json.Number("1"), json.Number("2"), json.Number("3")},
		},
		{
			name:     "string",
			schema:   `{"type":"string"}`,
			response: `"ready"`,
			want:     "ready",
		},
		{
			name:     "number",
			schema:   `{"type":"number"}`,
			response: `0.95`,
			want:     json.Number("0.95"),
		},
		{
			name:     "boolean",
			schema:   `{"type":"boolean"}`,
			response: `true`,
			want:     true,
		},
		{
			name:     "null",
			schema:   `{"type":"null"}`,
			response: `null`,
			want:     nil,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: test.response}}
			executor := NewAgentExecutor(staticRuntimeConfig{
				Workers: map[string]*workerconfig.FactoryWorkerConfig{
					"worker-a": {Model: "test-model", ModelProvider: workerexecution.RunnerIDAntigravity},
				},
			}, provider, nil, time.Now)

			result, err := executor.Execute(context.Background(), testAgentRequest(
				work.WorkDispatch{DispatchID: "structured-" + test.name, TransitionID: "parse", WorkerType: "worker-a"},
				withAgentOutputSchema(test.schema),
			))
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.Outcome != workerexecution.OutcomeAccepted {
				t.Fatalf("outcome = %q, want %q (error=%q)", result.Outcome, workerexecution.OutcomeAccepted, result.Error)
			}
			if !result.StructuredResultPresent || !reflect.DeepEqual(result.StructuredResult, test.want) {
				t.Fatalf("structured result = %#v (present=%t), want %#v", result.StructuredResult, result.StructuredResultPresent, test.want)
			}
			if result.Output != test.response {
				t.Fatalf("raw output = %q, want %q", result.Output, test.response)
			}
		})
	}
}

func TestAgentExecutorRejectsInvalidOutputSchemaBeforeProviderResponse(t *testing.T) {
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: `{"ok":true}`}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model", ModelProvider: workerexecution.RunnerIDAntigravity},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "invalid-schema", TransitionID: "parse", WorkerType: "worker-a"},
		withAgentOutputSchema(`{"type":`),
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want failed", result.Outcome)
	}
	if result.FailureMetadata == nil || result.FailureMetadata.Type != workerexecution.WorkFailureTypeMisconfigured || result.FailureMetadata.Family != workerexecution.WorkFailureFamilyTerminal {
		t.Fatalf("failure metadata = %#v, want terminal misconfigured", result.FailureMetadata)
	}
	if provider.callCount != 0 {
		t.Fatalf("provider calls = %d, want zero for invalid configuration", provider.callCount)
	}
	if result.Output != "" || result.StructuredResultPresent {
		t.Fatalf("result = %#v, want no accepted response data", result)
	}
}

func TestAgentExecutorAcceptsExplicitStructuredRerollVerdict(t *testing.T) {
	const schema = `{"type":"object","properties":{"action_completed":{"type":"boolean"},"verdict":{"type":"string","enum":["pass","reroll"]}},"required":["action_completed","verdict"]}`
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{
		Content: `{"action_completed":false,"verdict":"reroll"}`,
	}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"clip-qa": {Model: "gemini-3.6-flash-high", ModelProvider: workerexecution.RunnerIDAntigravity},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "structured-reroll", TransitionID: "qa", WorkerType: "clip-qa"},
		withAgentOutputSchema(schema),
		withAgentOutputContract(outputContractStructuredClipQAVerdictV1),
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("outcome = %q, want %q", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if result.Error != "" || result.FailureMetadata != nil {
		t.Fatalf("failure fields = error %q metadata %#v, want no execution failure", result.Error, result.FailureMetadata)
	}
}

func TestAgentExecutorRejectsStructuredRerollWithFailureStatus(t *testing.T) {
	const schema = `{"type":"object","properties":{"action_completed":{"type":"boolean"},"verdict":{"type":"string","enum":["pass","reroll"]},"status":{"type":"string"}},"required":["action_completed","verdict"]}`
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{
		Content: `{"action_completed":false,"verdict":"reroll","status":"error"}`,
	}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"clip-qa": {Model: "gemini-3.6-flash-high", ModelProvider: workerexecution.RunnerIDAntigravity},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "structured-reroll-status", TransitionID: "qa", WorkerType: "clip-qa"},
		withAgentOutputSchema(schema),
		withAgentOutputContract(outputContractStructuredClipQAVerdictV1),
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want failed for mixed reroll/status output", result.Outcome)
	}
	if !strings.Contains(result.Error, "status=error") {
		t.Fatalf("error = %q, want status diagnostic", result.Error)
	}
}

func TestAgentExecutorRejectsUnscopedStructuredReroll(t *testing.T) {
	const schema = `{"type":"object","properties":{"verdict":{"type":"string","enum":["pass","reroll"]}},"required":["verdict"]}`
	provider := &agentMockProvider{response: workerexecution.InferenceResponse{Content: `{"verdict":"reroll"}`}}
	executor := NewAgentExecutor(staticRuntimeConfig{
		Workers: map[string]*workerconfig.FactoryWorkerConfig{
			"worker-a": {Model: "test-model", ModelProvider: workerexecution.RunnerIDAntigravity},
		},
	}, provider, nil, time.Now)

	result, err := executor.Execute(context.Background(), testAgentRequest(
		work.WorkDispatch{DispatchID: "structured-reroll-unscoped", TransitionID: "qa", WorkerType: "worker-a"},
		withAgentOutputSchema(schema),
	))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("outcome = %q, want failed for unscoped reroll", result.Outcome)
	}
	if !strings.Contains(result.Error, "explicit structured QA output contract") {
		t.Fatalf("error = %q, want explicit-contract diagnostic", result.Error)
	}
}

func TestParseOutputAgainstSchemaRejectsSchemaInvalidStructuredOutput(t *testing.T) {
	_, err := parseOutputAgainstSchema(
		`{"verdict":"pass"}`,
		[]byte(`{"type":"object","required":["confidence"]}`),
	)
	if err == nil {
		t.Fatal("parseOutputAgainstSchema() error = nil, want schema validation failure")
	}
}

func TestPrintTimeoutFromWorkerTimeoutPreservesValidNativePrintLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "unset", raw: "", want: 0},
		{name: "media", raw: "8m", want: 8 * time.Minute},
		{name: "invalid", raw: "not-a-duration", want: 0},
		{name: "non-positive", raw: "0s", want: 0},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := PrintTimeoutFromWorkerTimeout(test.raw); got != test.want {
				t.Fatalf("PrintTimeoutFromWorkerTimeout(%q) = %s, want %s", test.raw, got, test.want)
			}
		})
	}
}

func (m *agentMockProvider) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	if request.Identity == "" {
		request.Identity = "codex"
	}
	return m.ProviderServiceAdapter.ResolveIdentity(ctx, request)
}

func (m *agentMockProvider) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	if request.ID == "" {
		request.ID = providers.IDCodex
	}
	return m.ProviderServiceAdapter.ValidatePrerequisites(ctx, request)
}

func (m *agentMockProvider) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	adapter := m.ProviderServiceAdapter
	adapter.InferFunc = m.Infer
	return adapter.Execute(ctx, request)
}

func (m *agentMockProvider) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	adapter := m.ProviderServiceAdapter
	adapter.InferFunc = m.Infer
	return adapter.Continue(ctx, request)
}

func (m *agentMockProvider) ContinueReference(
	ctx context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, providers.ContinuationFailure{
			Kind:      providers.ContinuationFailureKindInvalid,
			Message:   err.Error(),
			Reference: providers.SessionRef{},
		}
	}
	continued, err := m.Continue(ctx, providers.ContinueRequest{
		Reference: reference,
		Attempt:   request.Attempt,
	})
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	continuedReference := continued.Reference
	if continuedReference.Provider == "" {
		continuedReference = reference
	}
	resultReference := continuedReference.ContinuationRef()
	resultReference.ExternalRef = request.Reference.Normalize().ExternalRef
	return providers.ContinueReferenceResult{
		Reference: resultReference,
		Outcome:   continued.Outcome,
		Result:    continued.Result,
	}, nil
}
