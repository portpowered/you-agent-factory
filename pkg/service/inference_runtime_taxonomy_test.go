package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type inferenceRuntimeTaxonomyCase struct {
	name            string
	workerType      string
	workstationType string
}

func inferenceRuntimeTaxonomyCases() []inferenceRuntimeTaxonomyCase {
	return []inferenceRuntimeTaxonomyCase{
		{
			name:            "inference worker and run",
			workerType:      interfaces.WorkerTypeInference,
			workstationType: interfaces.WorkstationTypeInference,
		},
		{
			name:            "legacy model worker and invoke",
			workerType:      interfaces.WorkerTypeModel,
			workstationType: interfaces.WorkstationTypeInvoke,
		},
	}
}

func TestInferenceRuntimeTaxonomy_ExecutesBoundedInferenceWithCanonicalEvents(t *testing.T) {
	t.Parallel()

	for _, tc := range inferenceRuntimeTaxonomyCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider, wsExec := inferenceTaxonomyExecutionFixture(t, tc.workerType, tc.workstationType)
			result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			assertInferenceRunExecutionResult(t, result, provider.Calls())
		})
	}
}

func TestInferenceRuntimeTaxonomy_InferenceWorkerEmitsCanonicalInferenceEvents(t *testing.T) {
	t.Parallel()

	for _, tc := range inferenceRuntimeTaxonomyCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recorded := inferenceWorkerInferenceEventsFixture(t, tc.workerType)
			assertRecordedInferenceEvents(t, recorded)
		})
	}
}

func TestInferenceRuntimeTaxonomy_ModernAndLegacyProduceEquivalentResultShape(t *testing.T) {
	t.Parallel()

	modernProvider, modernExec := inferenceTaxonomyExecutionFixture(
		t,
		interfaces.WorkerTypeInference,
		interfaces.WorkstationTypeInference,
	)
	legacyProvider, legacyExec := inferenceTaxonomyExecutionFixture(
		t,
		interfaces.WorkerTypeModel,
		interfaces.WorkstationTypeInvoke,
	)

	modernResult, err := modernExec.Execute(context.Background(), modelInvokeDispatch())
	if err != nil {
		t.Fatalf("modern Execute: %v", err)
	}
	legacyResult, err := legacyExec.Execute(context.Background(), modelInvokeDispatch())
	if err != nil {
		t.Fatalf("legacy Execute: %v", err)
	}

	assertInferenceRunResultShapeEqual(t, modernResult, legacyResult)
	assertInferenceProviderCallShapeEqual(t, modernProvider.Calls(), legacyProvider.Calls())
}

func TestInferenceRuntimeTaxonomy_OmniVoicePackagedFactoryUsesInferenceBehavior(t *testing.T) {
	t.Parallel()

	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInTTSFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if len(cfg.Workers) != 1 || len(cfg.Workstations) != 1 {
		t.Fatalf("workers/workstations = %d/%d, want 1/1", len(cfg.Workers), len(cfg.Workstations))
	}

	worker := cfg.Workers[0]
	workstation := cfg.Workstations[0]
	if !interfaces.IsInferenceWorkerType(worker.Type) {
		t.Fatalf("worker type %q is not accepted inference-worker behavior", worker.Type)
	}
	if interfaces.ProjectWorkerBehaviorClass(worker.Type) != interfaces.WorkerTypeInference {
		t.Fatalf("worker behavior class = %q, want %q", interfaces.ProjectWorkerBehaviorClass(worker.Type), interfaces.WorkerTypeInference)
	}
	if !interfaces.IsInferenceRunWorkstationType(workstation.Type) {
		t.Fatalf("workstation type %q is not accepted inference-run behavior", workstation.Type)
	}
	if interfaces.ProjectWorkstationBehaviorClass(workstation.Type, workstation.Kind) != interfaces.WorkstationTypeInference {
		t.Fatalf("workstation behavior class = %q, want %q", interfaces.ProjectWorkstationBehaviorClass(workstation.Type, workstation.Kind), interfaces.WorkstationTypeInference)
	}
	if strings.TrimSpace(worker.StopToken) != "" || strings.TrimSpace(worker.OpenCodeAgent) != "" {
		t.Fatalf("omnivoice worker = %#v, want inference behavior without agent-loop fields", worker)
	}
	if !tts.ShouldFormatInvocationMetadata(&workstation) {
		t.Fatal("packaged invoke workstation should format inference-run TTS metadata")
	}

	validation := factoryconfig.NewConfigValidator().Validate(cfg)
	for _, finding := range validation.Findings {
		if finding.Rule == "workstation-worker-behavior-compatibility" {
			t.Fatalf("packaged omnivoice factory should validate, got %+v", finding)
		}
	}

	provider, wsExec := inferenceTaxonomyExecutionFixture(
		t,
		worker.Type,
		workstation.Type,
	)
	result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	assertInferenceRunExecutionResult(t, result, provider.Calls())
}

func TestInferenceRuntimeTaxonomy_InferenceWorkerRoutesOmniVoiceThroughManagedRuntime(t *testing.T) {
	t.Parallel()

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	provider := &providerCallRecorder{}
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}

	worker := modelInvokeLocalManagedRuntimeWorker()
	worker.Type = interfaces.WorkerTypeInference
	workstation := modelInvokeWorkstationConfig()
	workstation.Type = interfaces.WorkstationTypeInference

	factoryCfg := localModelFactoryConfig()
	cache := localModelTestCacheLayout(t)
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, map[string]*interfaces.WorkerConfig{
		"tts-worker": worker,
	}, map[string]*interfaces.FactoryWorkstationConfig{
		"speak": workstation,
	})

	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		nil,
		logging.NoopLogger{},
		true,
		provider,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		newLocalModelResourceLimiter(),
		newManagedLocalModelManager(staticModelAssetPuller{cache: cache}, runtime),
	)
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

	result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("outcome = %s error = %q, want %s", result.Outcome, result.Error, interfaces.OutcomeAccepted)
	}
	assertModelInvokeAcceptedAudioOutput(t, result.Output, audioPath)
	if runtime.invocationCount() != 1 {
		t.Fatalf("managed runtime invocation count = %d, want 1", runtime.invocationCount())
	}
	if calls := provider.Calls(); len(calls) != 0 {
		t.Fatalf("provider calls = %#v, want inference worker to bypass cloud provider path", calls)
	}
	assertManagedRuntimeModelInvokeInvocation(t, runtime.invocationRequests(), interfaces.ModelLocalityLocal)
}

func TestInferenceRuntimeTaxonomy_AgentRunRejectsInferenceWorkerBeforeDispatch(t *testing.T) {
	t.Parallel()

	cfg := omnivoiceInferenceFactoryConfig()
	cfg.Workstations[0].Type = interfaces.WorkstationTypeAgent

	validation := factoryconfig.NewConfigValidator().Validate(cfg)
	var matched bool
	for _, finding := range validation.Findings {
		if finding.Rule != "workstation-worker-behavior-compatibility" {
			continue
		}
		if !strings.Contains(finding.Path, "workstations[0]") {
			continue
		}
		if !strings.Contains(strings.ToLower(finding.Message), "agent-run") {
			continue
		}
		if !strings.Contains(strings.ToLower(finding.Message), "inference") {
			continue
		}
		matched = true
		break
	}
	if !matched {
		t.Fatalf("findings = %#v, want agent-run/inference compatibility finding before dispatch", validation.Findings)
	}
}

func inferenceTaxonomyExecutionFixture(
	t *testing.T,
	workerType string,
	workstationType string,
) (*providerCallRecorder, *workers.WorkstationExecutor) {
	t.Helper()

	provider := &providerCallRecorder{responses: []interfaces.InferenceResponse{{Content: "audio-ready"}}}

	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "speak", WorkerTypeName: "tts-worker"}},
		Workers:      []interfaces.WorkerConfig{{Name: "tts-worker"}},
	}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg,
		map[string]*interfaces.WorkerConfig{
			"tts-worker": inferenceTaxonomyRuntimeWorker(workerType),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"speak": inferenceTaxonomyRuntimeWorkstation(workstationType),
		},
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

func inferenceWorkerInferenceEventsFixture(t *testing.T, workerType string) []factoryapi.FactoryEvent {
	t.Helper()

	dir := t.TempDir()
	writeWorkerAgentsMDWithContent(t, dir, "worker-a", strings.TrimSpace(`
---
type: `+workerType+`
model: gpt-5.4
executorProvider: script_wrap
modelProvider: codex
stopToken: COMPLETE
---
You are a helpful assistant.
`))
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
		DispatchID:      "dispatch-inference-taxonomy",
		TransitionID:    "transition-inference-taxonomy",
		WorkerType:      "worker-a",
		WorkstationName: "review",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "token-inference-taxonomy",
			Color: interfaces.TokenColor{
				WorkID:  "work-inference-taxonomy",
				Payload: []byte("helpful input"),
			},
		}),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	assertCanonicalModelWorkerExecutionResult(t, result)
	return recorded
}

func inferenceTaxonomyRuntimeWorker(workerType string) *interfaces.WorkerConfig {
	worker := modelInvokeRuntimeWorker("OMNIVOICE_Q4_K_M", interfaces.ModelLocalityLocal)
	worker.Type = workerType
	return worker
}

func inferenceTaxonomyRuntimeWorkstation(workstationType string) *interfaces.FactoryWorkstationConfig {
	workstation := modelInvokeWorkstationConfig()
	workstation.Type = workstationType
	return workstation
}

func omnivoiceInferenceFactoryConfig() *interfaces.FactoryConfig {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(factoryconfig.BuiltInTTSFactoryJSON)
	if err != nil {
		panic(err)
	}
	return cfg
}

func assertInferenceRunExecutionResult(t *testing.T, result interfaces.WorkResult, calls []interfaces.ProviderInferenceRequest) {
	t.Helper()
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if len(calls) != 1 {
		t.Fatalf("provider calls = %d, want one bounded inference operation", len(calls))
	}
	if calls[0].ModelOperation != "TTS" {
		t.Fatalf("model operation = %q, want TTS", calls[0].ModelOperation)
	}
}

func assertInferenceRunResultShapeEqual(t *testing.T, left, right interfaces.WorkResult) {
	t.Helper()
	if left.Outcome != right.Outcome {
		t.Fatalf("outcome mismatch: left=%s right=%s", left.Outcome, right.Outcome)
	}
	if left.Error != right.Error {
		t.Fatalf("error mismatch: left=%q right=%q", left.Error, right.Error)
	}
	if left.Output != right.Output {
		t.Fatalf("output mismatch: left=%q right=%q", left.Output, right.Output)
	}
}

func assertInferenceProviderCallShapeEqual(t *testing.T, left, right []interfaces.ProviderInferenceRequest) {
	t.Helper()
	if len(left) != len(right) {
		t.Fatalf("call count mismatch: left=%d right=%d", len(left), len(right))
	}
	for i := range left {
		if left[i].Model != right[i].Model {
			t.Fatalf("call[%d] model mismatch: left=%q right=%q", i, left[i].Model, right[i].Model)
		}
		if left[i].ModelLocality != right[i].ModelLocality {
			t.Fatalf("call[%d] locality mismatch: left=%q right=%q", i, left[i].ModelLocality, right[i].ModelLocality)
		}
		if left[i].ModelOperation != right[i].ModelOperation {
			t.Fatalf("call[%d] operation mismatch: left=%q right=%q", i, left[i].ModelOperation, right[i].ModelOperation)
		}
		if len(left[i].ModelBindings) != len(right[i].ModelBindings) {
			t.Fatalf("call[%d] binding count mismatch: left=%d right=%d", i, len(left[i].ModelBindings), len(right[i].ModelBindings))
		}
	}
}
