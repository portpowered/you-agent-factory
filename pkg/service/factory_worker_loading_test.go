package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

type providerCallRecorder struct {
	mu        sync.Mutex
	calls     []interfaces.ProviderInferenceRequest
	responses []interfaces.InferenceResponse
}

func (p *providerCallRecorder) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, interfaces.CloneProviderInferenceRequest(req))
	if len(p.responses) == 0 {
		return interfaces.InferenceResponse{Content: "ok"}, nil
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return response, nil
}

func (p *providerCallRecorder) Calls() []interfaces.ProviderInferenceRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	calls := make([]interfaces.ProviderInferenceRequest, len(p.calls))
	for i, call := range p.calls {
		calls[i] = interfaces.CloneProviderInferenceRequest(call)
	}
	return calls
}

type providerCommandRunnerRecorder struct {
	mu       sync.Mutex
	requests []workers.CommandRequest
	result   workers.CommandResult
}

func (r *providerCommandRunnerRecorder) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.requests = append(r.requests, workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req)))
	return r.result, nil
}

func (r *providerCommandRunnerRecorder) Requests() []workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	requests := make([]workers.CommandRequest, len(r.requests))
	copy(requests, r.requests)
	return requests
}

func TestLoadWorkersFromConfig_PromptTemplateFromBody(t *testing.T) {
	dir := t.TempDir()

	expectedPrompt := "You are a design reviewer. Evaluate the design for {{ .Payload }}."
	writeWorkstationAgentsMDWithPrompt(t, dir, "review", expectedPrompt)
	writeWorkerAgentsMD(t, dir, "worker-a")

	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	}
	cfg := newLoadedFactoryConfigForServiceTest(t, dir, factoryCfg,
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	wsDef, ok := wsExec.RuntimeConfig.Workstation("review")
	if !ok {
		t.Fatal("expected 'review' workstation in runtime config")
	}
	if wsDef.PromptTemplate != expectedPrompt {
		t.Errorf("expected prompt template %q, got %q", expectedPrompt, wsDef.PromptTemplate)
	}
}

func TestLoadWorkersFromConfig_PromptTemplateFromFile(t *testing.T) {
	dir := t.TempDir()

	expectedPrompt := "Custom prompt loaded from file: {{ .WorkID }}"
	writeWorkstationAgentsMDWithPromptFile(t, dir, "review", "prompt.md", expectedPrompt)
	writeWorkerAgentsMD(t, dir, "worker-a")

	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	}
	cfg := newLoadedFactoryConfigForServiceTest(t, dir, factoryCfg,
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	wsDef, ok := wsExec.RuntimeConfig.Workstation("review")
	if !ok {
		t.Fatal("expected 'review' workstation in runtime config")
	}
	if wsDef.PromptTemplate != expectedPrompt {
		t.Errorf("expected prompt template %q, got %q", expectedPrompt, wsDef.PromptTemplate)
	}
}

func TestLoadWorkersFromConfig_ModelWorkerWithCanonicalExecutorProviderUsesAgentExecutorPath(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
executorProvider: script_wrap
modelProvider: codex
stopToken: COMPLETE
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	if _, ok := wsExec.Executor.(*workers.AgentExecutor); !ok {
		t.Fatalf("expected wrapped executor to be *workers.AgentExecutor, got %T", wsExec.Executor)
	}

	workerDef, ok := wsExec.RuntimeConfig.Worker("worker-a")
	if !ok {
		t.Fatal("expected worker-a in runtime config")
	}
	if workerDef.ExecutorProvider != "script_wrap" {
		t.Fatalf("executor provider = %q, want script_wrap", workerDef.ExecutorProvider)
	}
	if workerDef.ModelProvider != "codex" {
		t.Fatalf("model provider = %q, want codex", workerDef.ModelProvider)
	}
}

func TestLoadWorkersFromConfig_ModelWorkerUsesCanonicalProviderCommandRunnerAndRecordingProvider(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
executorProvider: script_wrap
modelProvider: codex
stopToken: COMPLETE
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{
			Stdout: []byte("provider-output COMPLETE"),
			Stderr: []byte(`{"event":"session.created","session_id":"sess_codex_123"}`),
		},
	}
	recorded := make([]factoryapi.FactoryEvent, 0, 2)
	recorder := func(event factoryapi.FactoryEvent) {
		recorded = append(recorded, event)
	}

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, runner, nil, nil, recorder)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-model-worker-provider-command",
		TransitionID:    "t-model-worker-provider-command",
		WorkerType:      "worker-a",
		WorkstationName: "review",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-model-worker-provider-command",
			Color: interfaces.TokenColor{
				WorkID:  "work-model-worker-provider-command",
				Payload: []byte("helpful input"),
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCanonicalModelWorkerExecutionResult(t, result)
	assertCanonicalProviderCommandRequests(t, runner.Requests())
	assertRecordedInferenceEvents(t, recorded)
}

// backendsizecheck:ignore-function this dual-locality integration test keeps the full model-invoke execution assertion path in one place.
func TestLoadWorkersFromConfig_ModelInvokeContractExecutesAcrossLocalAndCloudWorkers(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		model         string
		modelLocality string
	}{
		{
			name:          "local tts worker",
			model:         "OMNIVOICE_Q4_K_M",
			modelLocality: interfaces.ModelLocalityLocal,
		},
		{
			name:          "cloud tts worker",
			model:         "gpt-4o-mini-tts",
			modelLocality: interfaces.ModelLocalityCloud,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			provider, wsExec := modelInvokeExecutionFixture(t, tt.model, tt.modelLocality)
			result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Outcome != interfaces.OutcomeAccepted {
				t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
			}
			assertModelInvokeProviderCall(t, provider.Calls(), tt.model, tt.modelLocality)
		})
	}
}

func modelInvokeExecutionFixture(t *testing.T, model string, modelLocality string) (*providerCallRecorder, *workers.WorkstationExecutor) {
	t.Helper()
	provider := &providerCallRecorder{responses: []interfaces.InferenceResponse{{Content: "audio-ready"}}}
	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "speak", WorkerTypeName: "tts-worker"}},
		Workers:      []interfaces.WorkerConfig{{Name: "tts-worker"}},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg,
		map[string]*interfaces.WorkerConfig{"tts-worker": modelInvokeRuntimeWorker(model, modelLocality)},
		map[string]*interfaces.FactoryWorkstationConfig{"speak": modelInvokeWorkstationConfig()},
	)
	opts, err := loadWorkersFromConfigForServiceTest("", factoryCfg, "", runtimeCfg, provider, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}
	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}
	exec, ok := fc.WorkerExecutors["tts-worker"]
	if !ok {
		t.Fatal("expected tts-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	return provider, wsExec
}

func modelInvokeRuntimeWorker(model string, modelLocality string) *interfaces.WorkerConfig {
	return &interfaces.WorkerConfig{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeModel,
		Model:         model,
		ModelProvider: interfaces.RunnerIDCodex,
		ModelLocality: modelLocality,
		Body:          "You are a TTS worker.",
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
			Inputs: []interfaces.ModelOperationSlot{
				{Name: "text", ContentTypes: []string{interfaces.ModelOperationContentTypeText}, Required: true},
				{Name: "voice", ContentTypes: []string{interfaces.ModelOperationContentTypeJSON}},
			},
			Outputs: []interfaces.ModelOperationSlot{{Name: "audio", ContentTypes: []string{interfaces.ModelOperationContentTypeAudio}}},
		}},
	}
}

func modelInvokeWorkstationConfig() *interfaces.FactoryWorkstationConfig {
	return &interfaces.FactoryWorkstationConfig{
		Name:           "speak",
		Type:           interfaces.WorkstationTypeInvoke,
		WorkerTypeName: "tts-worker",
		Operation:      "TTS",
		PromptTemplate: "Synthesize {{ (index .Inputs 0).WorkID }}",
		OperationBindings: []interfaces.ModelOperationBinding{
			{
				Slot: "text",
				Selector: &interfaces.ModelOperationBindingSelector{
					Label: "utterance",
					Type:  interfaces.ModelOperationContentTypeText,
				},
			},
			{
				Slot: "voice",
				Config: []interfaces.WorkContentPart{{
					Type: interfaces.WorkContentPartTypeJSON,
					Role: "voice",
					JSON: []byte(`{"name":"alloy"}`),
				}},
			},
		},
	}
}

func modelInvokeDispatch() interfaces.WorkDispatch {
	return interfaces.WorkDispatch{
		DispatchID:      "dispatch-tts",
		TransitionID:    "transition-tts",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-tts",
			Color: interfaces.TokenColor{
				WorkID: "work-tts",
				Content: []interfaces.WorkContentPart{{
					Type:  interfaces.WorkContentPartTypeText,
					Label: "utterance",
					Text:  "hello world",
				}},
			},
		}),
	}
}

func assertModelInvokeProviderCall(t *testing.T, calls []interfaces.ProviderInferenceRequest, wantModel string, wantLocality string) {
	t.Helper()
	if len(calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(calls))
	}
	call := calls[0]
	if call.Model != wantModel {
		t.Fatalf("provider model = %q, want %q", call.Model, wantModel)
	}
	if call.ModelLocality != wantLocality {
		t.Fatalf("provider model locality = %q, want %q", call.ModelLocality, wantLocality)
	}
	if call.ModelOperation != "TTS" {
		t.Fatalf("provider model operation = %q, want TTS", call.ModelOperation)
	}
	if len(call.ModelBindings) != 2 {
		t.Fatalf("provider model bindings = %#v, want 2 entries", call.ModelBindings)
	}
	assertModelInvokeTextBinding(t, call.ModelBindings[0])
	assertModelInvokeVoiceBinding(t, call.ModelBindings[1])
}

func assertModelInvokeTextBinding(t *testing.T, binding interfaces.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "text" || binding.Source != interfaces.ModelOperationBindingSourceInput || binding.Content[0].Text != "hello world" {
		t.Fatalf("text model binding = %#v, want generic text slot from input", binding)
	}
}

func assertModelInvokeVoiceBinding(t *testing.T, binding interfaces.ResolvedModelOperationBinding) {
	t.Helper()
	if binding.Slot != "voice" || binding.Source != interfaces.ModelOperationBindingSourceConfig || string(binding.Content[0].JSON) != `{"name":"alloy"}` {
		t.Fatalf("voice model binding = %#v, want config voice binding", binding)
	}
}

func TestLoadWorkersFromConfig_ReplayEmbeddedRuntimeUsesCanonicalLookup(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	generated, err := replay.GeneratedFactoryFromRuntimeConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}

	opts, err := loadWorkersFromConfigForServiceTest(runtimeCfg.FactoryDir(), runtimeCfg.Factory, "", runtimeCfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	if got := wsExec.RuntimeConfig.FactoryDir(); got != dir {
		t.Fatalf("embedded runtime FactoryDir = %q, want %q", got, dir)
	}
	if got := wsExec.RuntimeConfig.RuntimeBaseDir(); got != dir {
		t.Fatalf("embedded runtime RuntimeBaseDir = %q, want %q", got, dir)
	}
	if _, ok := wsExec.RuntimeConfig.Worker("worker-a"); !ok {
		t.Fatal("expected replay runtime worker lookup for worker-a")
	}
	if _, ok := wsExec.RuntimeConfig.Workstation("review"); !ok {
		t.Fatal("expected replay runtime workstation lookup for review")
	}
}

func TestLoadWorkersFromConfig_LoadedRuntimeBaseDirOverrideFlowsThroughCanonicalLookup(t *testing.T) {
	dir := t.TempDir()
	runtimeBaseDir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)
	loaded.SetRuntimeBaseDir(runtimeBaseDir)

	opts, err := loadWorkersFromConfigForServiceTest(loaded.FactoryDir(), loaded.FactoryConfig(), "", loaded, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	if got := wsExec.RuntimeConfig.FactoryDir(); got != dir {
		t.Fatalf("loaded runtime FactoryDir = %q, want %q", got, dir)
	}
	if got := wsExec.RuntimeConfig.RuntimeBaseDir(); got != runtimeBaseDir {
		t.Fatalf("loaded runtime RuntimeBaseDir = %q, want %q", got, runtimeBaseDir)
	}
}

func TestLoadWorkersFromConfig_CanonicalRuntimeLookupDrivesScriptExecutionWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	runtimeBaseDir := t.TempDir()

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeRuntimeLookupWorkstationAgentsMD(t, dir, "run-script")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "run-script",
			WorkerTypeName: "script-worker",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)
	loaded.SetRuntimeBaseDir(runtimeBaseDir)

	runner := &capturingCommandRunner{}
	opts, err := loadWorkersFromConfigForServiceTest(loaded.FactoryDir(), loaded.FactoryConfig(), "", loaded, nil, nil, runner, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-runtime-lookup-script",
		TransitionID:    "t-runtime-lookup-script",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-runtime-lookup-script",
			Color: interfaces.TokenColor{
				WorkID: "work-runtime-lookup-script",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if got := runner.request.WorkDir; got != filepath.Join(runtimeBaseDir, "workspace") {
		t.Fatalf("command working directory = %q, want %q", got, filepath.Join(runtimeBaseDir, "workspace"))
	}
}

func TestLoadWorkersFromConfig_CanonicalRuntimeLookupResolvesPortableFactoryScriptReferencesAgainstNamedFactoryDir(t *testing.T) {
	rootDir := t.TempDir()
	namedFactoryDir := filepath.Join(rootDir, "beta")

	writeScriptWorkerAgentsMDWithCommand(t, namedFactoryDir, "script-worker", "pwsh", []string{"-File", "factory/scripts/execute-story.ps1"})
	writeRuntimeLookupWorkstationAgentsMD(t, namedFactoryDir, "run-script")
	if err := os.MkdirAll(filepath.Join(namedFactoryDir, "scripts"), 0o755); err != nil {
		t.Fatalf("create scripts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(namedFactoryDir, "scripts", "execute-story.ps1"), []byte("Write-Output 'ok'\n"), 0o644); err != nil {
		t.Fatalf("write portable script: %v", err)
	}

	loaded := newLoadedFactoryConfigForServiceTest(t, namedFactoryDir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "run-script",
			WorkerTypeName: "script-worker",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(namedFactoryDir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(namedFactoryDir, "workstations", "run-script")),
		},
	)
	loaded.SetRuntimeBaseDir(rootDir)

	runner := &capturingCommandRunner{}
	opts, err := loadWorkersFromConfigForServiceTest(loaded.FactoryDir(), loaded.FactoryConfig(), "", loaded, nil, nil, runner, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-runtime-lookup-script-ref",
		TransitionID:    "t-runtime-lookup-script-ref",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-runtime-lookup-script-ref",
			Color: interfaces.TokenColor{
				WorkID: "work-runtime-lookup-script-ref",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if len(runner.request.Args) != 2 {
		t.Fatalf("command args = %#v, want 2 entries", runner.request.Args)
	}
	if got := runner.request.Args[1]; got != filepath.Join(namedFactoryDir, "scripts", "execute-story.ps1") {
		t.Fatalf("portable script arg = %q, want %q", got, filepath.Join(namedFactoryDir, "scripts", "execute-story.ps1"))
	}
	if got := runner.request.WorkDir; got != filepath.Join(rootDir, "workspace") {
		t.Fatalf("command working directory = %q, want %q", got, filepath.Join(rootDir, "workspace"))
	}
}

func TestLoadWorkersFromConfig_ReplayRuntimeLookupDrivesScriptExecutionWorkingDirectory(t *testing.T) {
	dir := t.TempDir()

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeRuntimeLookupWorkstationAgentsMD(t, dir, "run-script")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "run-script",
			WorkerTypeName: "script-worker",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)

	generated, err := replay.GeneratedFactoryFromRuntimeConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}

	runner := &capturingCommandRunner{}
	opts, err := loadWorkersFromConfigForServiceTest(runtimeCfg.FactoryDir(), runtimeCfg.Factory, "", runtimeCfg, nil, nil, runner, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-replay-runtime-lookup-script",
		TransitionID:    "t-replay-runtime-lookup-script",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-replay-runtime-lookup-script",
			Color: interfaces.TokenColor{
				WorkID: "work-replay-runtime-lookup-script",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if got := runner.request.WorkDir; got != filepath.Join(dir, "workspace") {
		t.Fatalf("command working directory = %q, want %q", got, filepath.Join(dir, "workspace"))
	}
}

func TestLoadWorkersFromConfig_ScriptWorkerUsesWorkstationExecutor(t *testing.T) {
	dir := t.TempDir()
	scriptRecorder := func(factoryapi.FactoryEvent) {}

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeWorkstationAgentsMD(t, dir, "run-script")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "run-script"}},
		Workers:      []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, &stubCommandRunner{}, scriptRecorder, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	scriptExec, ok := wsExec.Executor.(*workers.ScriptExecutor)
	if !ok {
		t.Fatalf("expected wrapped executor to be *workers.ScriptExecutor, got %T", wsExec.Executor)
	}
	if recorder := reflect.ValueOf(scriptExec).Elem().FieldByName("recorder"); !recorder.IsValid() || recorder.IsNil() {
		t.Fatal("expected script executor to receive canonical script event recorder")
	}
}

func TestLoadWorkersFromConfig_RegistersWorkerlessLogicalWorkstationByName(t *testing.T) {
	cfg := newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "review-loop-breaker",
			Type: interfaces.WorkstationTypeLogical,
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "failed",
			}},
		}},
	}, nil, map[string]*interfaces.FactoryWorkstationConfig{
		"review-loop-breaker": {
			Name: "review-loop-breaker",
			Type: interfaces.WorkstationTypeLogical,
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "failed",
			}},
		},
	})

	opts, err := loadWorkersFromConfigForServiceTest(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["review-loop-breaker"]
	if !ok {
		t.Fatal("expected workerless logical workstation executor to be registered by workstation name")
	}
	if _, ok := exec.(*workers.WorkstationExecutor); !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
}

func loadWorkersFromConfigForServiceTest(
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	providerOverride workers.Provider,
	providerCommandRunner workers.CommandRunner,
	commandRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
) ([]factory.FactoryOption, error) {
	return loadWorkersFromConfig(
		factoryDir,
		factoryCfg,
		factoryRunnerID,
		runtimeCfg,
		logging.NoopLogger{},
		false,
		providerOverride,
		providerCommandRunner,
		commandRunner,
		scriptRecorder,
		inferenceRecorder,
		nil,
		nil,
		nil,
		nil,
	)
}

func assertCanonicalModelWorkerExecutionResult(t *testing.T, result interfaces.WorkResult) {
	t.Helper()

	if result.Outcome != interfaces.OutcomeAccepted || result.Output != "provider-output COMPLETE" {
		t.Fatalf("result = %#v, want accepted provider output", result)
	}
	if result.ProviderSession == nil || result.ProviderSession.Provider != "codex" || result.ProviderSession.ID != "sess_codex_123" {
		t.Fatalf("provider session = %#v, want canonical codex session metadata", result.ProviderSession)
	}
}

func assertCanonicalProviderCommandRequests(t *testing.T, requests []workers.CommandRequest) {
	t.Helper()

	if len(requests) != 1 {
		t.Fatalf("provider command runner request count = %d, want 1", len(requests))
	}
	if requests[0].Command != string(workers.ModelProviderCodex) {
		t.Fatalf("provider command = %q, want %q", requests[0].Command, workers.ModelProviderCodex)
	}
}

func assertRecordedInferenceEvents(t *testing.T, recorded []factoryapi.FactoryEvent) {
	t.Helper()

	if len(recorded) != 2 {
		t.Fatalf("recorded event count = %d, want 2", len(recorded))
	}
	if recorded[0].Type != factoryapi.FactoryEventTypeInferenceRequest {
		t.Fatalf("first event type = %s, want %s", recorded[0].Type, factoryapi.FactoryEventTypeInferenceRequest)
	}
	if recorded[1].Type != factoryapi.FactoryEventTypeInferenceResponse {
		t.Fatalf("second event type = %s, want %s", recorded[1].Type, factoryapi.FactoryEventTypeInferenceResponse)
	}
}
