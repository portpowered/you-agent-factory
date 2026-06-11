package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestWorkerWorkstationTaxonomyRuntime_InferencePairingExecutesLikeLegacyModelInvoke(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		publicWorkerType       string
		publicWorkstationType  string
		wantRuntimeWorkerType  string
		wantRuntimeWorkstation string
	}{
		{
			name:                   "inference taxonomy",
			publicWorkerType:       interfaces.WorkerTypeInference,
			publicWorkstationType:    interfaces.WorkstationTypeInference,
			wantRuntimeWorkerType:  interfaces.WorkerTypeModel,
			wantRuntimeWorkstation: interfaces.WorkstationTypeInvoke,
		},
		{
			name:                   "legacy model invoke",
			publicWorkerType:       interfaces.WorkerTypeModel,
			publicWorkstationType:  interfaces.WorkstationTypeInvoke,
			wantRuntimeWorkerType:  interfaces.WorkerTypeModel,
			wantRuntimeWorkstation: interfaces.WorkstationTypeInvoke,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
				t,
				tt.publicWorkerType,
				tt.publicWorkstationType,
				tt.wantRuntimeWorkerType,
				tt.wantRuntimeWorkstation,
			)
			provider, wsExec := taxonomyModelInvokeExecutionFixtureFromRuntimeConfig(t, cfg)

			result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if result.Outcome != interfaces.OutcomeAccepted {
				t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
			}
			assertModelInvokeProviderCall(t, provider.Calls(), "gpt-4o-mini-tts", interfaces.ModelLocalityCloud)
		})
	}
}

func TestWorkerWorkstationTaxonomyRuntime_OmniVoiceInferenceExecutesWithoutAgentLoopFields(t *testing.T) {
	t.Parallel()

	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cfg := mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
		t,
		interfaces.WorkerTypeInference,
		interfaces.WorkstationTypeInference,
		interfaces.WorkerTypeModel,
		interfaces.WorkstationTypeInvoke,
	)
	wsExec := taxonomyOmniVoiceInferenceWorkstationExecutor(t, runtime, cfg)

	result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	assertModelInvokeAcceptedAudioOutput(t, result.Output, audioPath)
	if runtime.invocationCount() != 1 {
		t.Fatalf("managed runtime invocation count = %d, want 1 bounded inference operation", runtime.invocationCount())
	}
	invocations := runtime.invocationRequests()
	if len(invocations) != 1 {
		t.Fatalf("invocation requests = %d, want 1", len(invocations))
	}
	request := invocations[0].Request
	if request.ModelOperation != "TTS" {
		t.Fatalf("model operation = %q, want TTS inference operation", request.ModelOperation)
	}
	if strings.TrimSpace(request.OpenCodeAgent) != "" {
		t.Fatalf("open code agent = %q, want empty for inference-run execution", request.OpenCodeAgent)
	}
	if request.Model != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model = %q, want OMNIVOICE_Q4_K_M inference model identity", request.Model)
	}
	if request.ModelLocality != interfaces.ModelLocalityLocal {
		t.Fatalf("model locality = %q, want %q", request.ModelLocality, interfaces.ModelLocalityLocal)
	}
	if len(request.ModelBindings) != 1 || request.ModelBindings[0].Slot != "text" {
		t.Fatalf("model bindings = %#v, want one resolved text input binding", request.ModelBindings)
	}
}

func TestWorkerWorkstationTaxonomyRuntime_InferenceTaxonomyRecordsModelExecutionEvents(t *testing.T) {
	t.Parallel()

	eventTime := taxonomyRuntimeEventTime()
	audioPath := filepath.Join(t.TempDir(), "speech.wav")
	runtime := &fakeLocalModelRuntime{
		response: interfaces.InferenceResponse{
			Content: mustMarshalAudioContentResponse(t, audioPath),
		},
	}
	cfg := mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
		t,
		interfaces.WorkerTypeInference,
		interfaces.WorkstationTypeInference,
		interfaces.WorkerTypeModel,
		interfaces.WorkstationTypeInvoke,
	)
	wsExec, history := taxonomyOmniVoiceInferenceWorkstationExecutorWithEvents(t, runtime, cfg, eventTime)

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "dispatch-taxonomy-inference",
		TransitionID:    "transition-taxonomy-inference",
		WorkerType:      "tts-worker",
		WorkstationName: "speak",
		Execution: interfaces.ExecutionMetadata{
			CurrentTick: 2,
			RequestID:   "request-taxonomy-inference",
			TraceID:     "trace-taxonomy-inference",
			WorkIDs:     []string{"work-taxonomy-inference"},
		},
		InputTokens: modelInvokeDispatch().InputTokens,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	assertRecordedLocalModelExecutionEvents(t, history.Events(), audioPath)
}

func TestWorkerWorkstationTaxonomyRuntime_AgentRunWithInferenceWorkerFailsValidationBeforeDispatch(t *testing.T) {
	t.Parallel()

	generated := taxonomyRuntimeIncompatibleInferenceWorkerAgentRunFactory()

	result, err := validationentry.ValidateFactoryAPI(context.Background(), generated, factoryvalidation.Options{})
	if err != nil {
		t.Fatalf("ValidateFactoryAPI: %v", err)
	}
	validationassert.HasDomainTargetCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationIncompatibleBehavior)
	target := taxonomyRuntimeFindTargetByCode(t, result.Targets, factoryvalidation.CodeWorkerWorkstationIncompatibleBehavior)
	if !strings.Contains(target.Message, interfaces.WorkstationTypeAgent) {
		t.Fatalf("message %q missing workstation type %q", target.Message, interfaces.WorkstationTypeAgent)
	}
	if !strings.Contains(target.Message, interfaces.WorkerTypeInference) {
		t.Fatalf("message %q missing worker type %q", target.Message, interfaces.WorkerTypeInference)
	}
}

func mustTaxonomyRuntimeFactoryConfigFromOpenAPI(
	t *testing.T,
	publicWorkerType string,
	publicWorkstationType string,
	wantRuntimeWorkerType string,
	wantRuntimeWorkstationType string,
) *interfaces.FactoryConfig {
	t.Helper()

	generated, err := factoryconfig.GeneratedFactoryFromOpenAPIJSON(
		taxonomyRuntimeModelInvokeFactoryJSON(publicWorkerType, publicWorkstationType),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	cfg, err := factoryconfig.FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	if len(cfg.Workers) != 1 {
		t.Fatalf("workers = %#v, want one worker", cfg.Workers)
	}
	if cfg.Workers[0].Type != wantRuntimeWorkerType {
		t.Fatalf("runtime worker type = %q, want %q", cfg.Workers[0].Type, wantRuntimeWorkerType)
	}
	if len(cfg.Workstations) != 1 {
		t.Fatalf("workstations = %#v, want one workstation", cfg.Workstations)
	}
	if cfg.Workstations[0].Type != wantRuntimeWorkstationType {
		t.Fatalf("runtime workstation type = %q, want %q", cfg.Workstations[0].Type, wantRuntimeWorkstationType)
	}
	return &cfg
}

func taxonomyModelInvokeExecutionFixtureFromRuntimeConfig(
	t *testing.T,
	cfg *interfaces.FactoryConfig,
) (*providerCallRecorder, *workers.WorkstationExecutor) {
	t.Helper()

	provider := &providerCallRecorder{responses: []interfaces.InferenceResponse{{Content: "audio-ready"}}}
	runtimeCfg := newLoadedFactoryConfigForServiceTest(
		t,
		"",
		cfg,
		map[string]*interfaces.WorkerConfig{"tts-worker": taxonomyRuntimeModelInvokeWorker(cfg.Workers[0].Type)},
		map[string]*interfaces.FactoryWorkstationConfig{"speak": taxonomyRuntimeModelInvokeWorkstation(cfg.Workstations[0].Type)},
	)
	opts, err := loadWorkersFromConfigForServiceTest("", cfg, "", runtimeCfg, provider, nil, nil, nil, nil)
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

func taxonomyOmniVoiceInferenceWorkstationExecutor(
	t *testing.T,
	runtime *fakeLocalModelRuntime,
	cfg *interfaces.FactoryConfig,
) *workers.WorkstationExecutor {
	t.Helper()

	wsExec, _ := taxonomyOmniVoiceInferenceWorkstationExecutorWithEvents(t, runtime, cfg, taxonomyRuntimeEventTime())
	return wsExec
}

func taxonomyOmniVoiceInferenceWorkstationExecutorWithEvents(
	t *testing.T,
	runtime *fakeLocalModelRuntime,
	cfg *interfaces.FactoryConfig,
	eventTime time.Time,
) (*workers.WorkstationExecutor, *factoryevents.FactoryEventHistory) {
	t.Helper()

	cache := localModelTestCacheLayout(t)
	factoryCfg := localModelFactoryConfig()
	runtimeWorkers := localModelRuntimeWorkers()
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, runtimeWorkers, map[string]*interfaces.FactoryWorkstationConfig{
		"speak": taxonomyRuntimeModelInvokeWorkstation(cfg.Workstations[0].Type),
	})
	history := factoryevents.NewFactoryEventHistory(nil, func() time.Time { return eventTime }, runtimeCfg)
	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		nil,
		logging.NoopLogger{},
		true,
		nil,
		nil,
		nil,
		nil,
		nil,
		history.RecordModelEvent,
		func() time.Time { return eventTime },
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
	return wsExec, history
}

func taxonomyRuntimeModelInvokeFactoryJSON(workerType, workstationType string) []byte {
	payload := map[string]any{
		"name": "taxonomy-runtime-factory",
		"workTypes": []map[string]any{{
			"name": "speech",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "tts-worker",
			"type":          workerType,
			"model":         "gpt-4o-mini-tts",
			"modelProvider": "CODEX",
			"modelLocality": interfaces.ModelLocalityCloud,
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{
					{
						"name":         "text",
						"contentTypes": []string{interfaces.ModelOperationContentTypeText},
						"required":     true,
					},
					{
						"name":         "voice",
						"contentTypes": []string{interfaces.ModelOperationContentTypeJSON},
					},
				},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		}},
		"workstations": []map[string]any{{
			"name":      "speak",
			"type":      workstationType,
			"worker":    "tts-worker",
			"operation": "TTS",
			"operationBindings": []map[string]any{
				{
					"slot": "text",
					"selector": map[string]any{
						"label": "utterance",
						"type":  interfaces.ModelOperationContentTypeText,
					},
				},
				{
					"slot": "voice",
					"config": []map[string]any{{
						"type": interfaces.WorkContentPartTypeJSON,
						"role": "voice",
						"json": map[string]string{"name": "alloy"},
					}},
				},
			},
			"inputs":    []map[string]string{{"workType": "speech", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "speech", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "speech", "state": "failed"}},
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return data
}

func taxonomyRuntimeModelInvokeWorker(runtimeWorkerType string) *interfaces.WorkerConfig {
	worker := modelInvokeRuntimeWorker("gpt-4o-mini-tts", interfaces.ModelLocalityCloud)
	worker.Type = runtimeWorkerType
	return worker
}

func taxonomyRuntimeModelInvokeWorkstation(runtimeWorkstationType string) *interfaces.FactoryWorkstationConfig {
	workstation := modelInvokeWorkstationConfig()
	workstation.Type = runtimeWorkstationType
	return workstation
}

func taxonomyRuntimeEventTime() time.Time {
	return time.Date(2026, time.June, 11, 18, 0, 0, 0, time.UTC)
}

func taxonomyRuntimeIncompatibleInferenceWorkerAgentRunFactory() factoryapi.Factory {
	workerType := factoryapi.WorkerTypeInferenceWorker
	workstationType := factoryapi.WorkstationTypeAgentRun
	modelProvider := factoryapi.WorkerModelProviderCodex
	return factoryapi.Factory{
		Name: "taxonomy-runtime-factory",
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "speech",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
				{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			},
		}},
		Workers: &[]factoryapi.Worker{{
			Name:          "tts-worker",
			Type:          &workerType,
			Model:         stringPtr("gpt-4o-mini-tts"),
			ModelProvider: &modelProvider,
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "speak",
			Type:   &workstationType,
			Worker: "tts-worker",
			Inputs: []factoryapi.WorkstationIO{{
				WorkType: "speech",
				State:    "init",
			}},
			Outputs: &[]factoryapi.WorkstationIO{{
				WorkType: "speech",
				State:    "complete",
			}},
			OnFailure: &[]factoryapi.WorkstationIO{{
				WorkType: "speech",
				State:    "failed",
			}},
		}},
	}
}

func taxonomyRuntimeFindTargetByCode(t *testing.T, targets []factoryvalidation.Target, code string) factoryvalidation.Target {
	t.Helper()
	for _, target := range targets {
		if target.Code == code {
			return target
		}
	}
	t.Fatalf("target with code %q not found in %#v", code, targets)
	return factoryvalidation.Target{}
}
