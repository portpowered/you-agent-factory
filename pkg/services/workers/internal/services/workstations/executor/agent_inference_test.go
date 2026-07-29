package executor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

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
		&workerexecution.ProviderSessionMetadata{
			Provider: string(modelprovider.ProviderKiro),
			Kind:     providerSessionKindSessionID,
			ID:       sessionID,
		},
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
