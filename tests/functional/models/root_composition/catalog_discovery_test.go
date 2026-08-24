package root_composition_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestGenericModelContractsRemainDetachedAtApplicationRoot proves generic model
// contracts remain available and detached at the application root.
func TestGenericModelContractsRemainDetachedAtApplicationRoot(t *testing.T) {
	t.Parallel()

	process := support.BuildProcess(t, serviceedges.Edges{})
	help := support.FakeInputs(t.Context(), []string{"you", "session", "show", "--help"})
	if err := process.Execute(help.Input); err != nil {
		t.Fatalf("Process.Execute(session show help) error = %v", err)
	}
	assertGenericOperationCatalog(t)
	assertBuiltInModelCatalog(t)
	assertGenericInvocationContracts(t)
	assertGenericRuntimeFailures(t)
}

func assertGenericOperationCatalog(t *testing.T) {
	t.Helper()
	operationCatalog := modelprovider.GenericOperationCatalog{}
	contracts := operationCatalog.GenericOperationContracts()
	wantNames := []string{
		modelprovider.OperationOMNI,
		modelprovider.OperationEMBED,
		modelprovider.OperationTTS,
		modelprovider.OperationASR,
	}
	if len(contracts) != len(wantNames) {
		t.Fatalf("generic operation count = %d, want %d", len(contracts), len(wantNames))
	}
	for index, wantName := range wantNames {
		if contracts[index].Name != wantName {
			t.Fatalf("generic operation[%d] = %q, want %q", index, contracts[index].Name, wantName)
		}
	}
	omni, ok := operationCatalog.GenericOperationContract(" omni ")
	if !ok || len(omni.Inputs) != 5 || omni.Inputs[1].Name != "image" || !omni.Inputs[1].Repeatable {
		t.Fatalf("OMNI contract = %#v, want named repeatable image slot", omni)
	}
	if _, ok := operationCatalog.GenericOperationContract("unknown"); ok {
		t.Fatal("unknown generic operation unexpectedly resolved")
	}

	clonedOperation := omni.Clone()
	clonedOperation.Inputs[1].MediaTypes[0] = "mutated/image"
	freshOMNI, ok := operationCatalog.GenericOperationContract(modelprovider.OperationOMNI)
	if !ok || freshOMNI.Inputs[1].MediaTypes[0] != "image/*" {
		t.Fatalf("generic operation retained caller mutation: %#v", freshOMNI)
	}
}

func assertBuiltInModelCatalog(t *testing.T) {
	t.Helper()
	builtIns := modelprovider.BuiltInCatalog{}
	definitions := builtIns.ModelDefinitions()
	if len(definitions) != 4 || definitions[0].Name != modelprovider.BuiltInModelNameLLM ||
		definitions[0].LoadPolicy != modelprovider.LoadPolicyOnDemand {
		t.Fatalf("built-in definitions = %#v", definitions)
	}
	catalog := builtIns.ModelCatalog()
	catalog[modelprovider.BuiltInModelNameLLM].Operations[0].Inputs[0].Name = "mutated"
	if _, ok := builtIns.ModelDefinitionFor(" missing "); ok {
		t.Fatal("missing built-in model unexpectedly resolved")
	}
	freshDefinition, ok := builtIns.ModelDefinitionFor(" LLM ")
	if !ok || freshDefinition.Operations[0].Inputs[0].Name != "prompt" {
		t.Fatalf("built-in catalog retained caller mutation: %#v", freshDefinition)
	}
}

func assertGenericInvocationContracts(t *testing.T) {
	t.Helper()
	scope, err := (modelprovider.RuntimeScopeRef{}).Parse("scope-functional-models")
	if err != nil {
		t.Fatalf("parse model scope: %v", err)
	}
	request := modelprovider.GenericInvocationRequest{
		Scope:     scope,
		Holder:    "functional-test",
		Model:     modelprovider.ModelReference{NameOrURI: "llm"},
		Operation: modelprovider.OperationOMNI,
		Inputs: []modelprovider.InferenceInput{
			{Name: "prompt", Modality: modelprovider.ModalityText, Content: "compare"},
			{Name: "image", Modality: modelprovider.ModalityImage, MediaType: "image/png", Content: "first"},
			{Name: "image", Modality: modelprovider.ModalityImage, MediaType: "image/jpeg", Content: "second"},
		},
		Parameters: []modelprovider.OperationParameter{{Name: "temperature", Value: map[string]any{"value": 0.2}}},
		OutputMode: modelprovider.OutputModeJSON,
		Offline:    true,
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("generic request validation error = %v", err)
	}
	if len(request.Inputs) != 3 || request.Inputs[1].Name != "image" || request.Inputs[2].Content != "second" {
		t.Fatalf("ordered generic inputs = %#v", request.Inputs)
	}

	artifact, err := (modelprovider.InferenceArtifactRef{}).Parse("artifact:functional-segments")
	if err != nil {
		t.Fatalf("parse output artifact: %v", err)
	}
	result := modelprovider.GenericInvocationResult{
		Outputs: []modelprovider.InferenceOutput{
			{Name: "transcript", Modality: modelprovider.ModalityText, Content: "hello"},
			{Name: "segments", Modality: modelprovider.ModalityJSON, Artifact: &modelprovider.InferenceArtifact{
				Artifact: artifact, Properties: map[string]string{"format": "json"},
			}},
		},
	}
	clonedResult := result.Clone()
	clonedResult.Outputs[1].Artifact.Properties["format"] = "mutated"
	if result.Outputs[1].Artifact.Properties["format"] != "json" {
		t.Fatal("generic result clone shared artifact properties")
	}
}

func assertGenericRuntimeFailures(t *testing.T) {
	t.Helper()
	ready := modelprovider.Runtime{ReadinessState: modelprovider.ReadinessStateReady}
	if err := ready.InvocationError(); err != nil {
		t.Fatalf("ready runtime invocation error = %v, want nil", err)
	}
	missing := modelprovider.Runtime{
		Identity:       "llm",
		ReadinessState: modelprovider.ReadinessStateMissing,
		LifecycleState: modelprovider.LifecycleStateNotInstalled,
	}
	missingErr := missing.InvocationError()
	if missingErr == nil || !errors.Is(missingErr, modelprovider.ErrMissing) {
		t.Fatalf("missing runtime invocation error = %v, want ErrMissing", missingErr)
	}
	failure := &modelprovider.InvocationFailure{
		Class:     modelprovider.InvocationFailureClassMalformedResponse,
		Message:   "response did not match the selected operation",
		Model:     modelprovider.ModelReference{NameOrURI: "llm"},
		Operation: modelprovider.OperationOMNI,
	}
	if failure.Error() != failure.Message || failure.Unwrap() != nil {
		t.Fatalf("generic invocation failure = %#v", failure)
	}
}

// TestModelsCatalogDiscoveryActivatesThroughRootBuildProcessAfterLifecycle proves
// catalog discovery through GET /models after runtime lifecycle starts on a process
// constructed only through root.BuildProcess with edges.Edges effect replacement.
func TestModelsCatalogDiscoveryActivatesThroughRootBuildProcessAfterLifecycle(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, catalogDiscoveryFactoryConfig())
	support.WriteAgentConfig(t, dir, "tts-worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "OMNIVOICE_Q4_K_M"))

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{},
	})
	t.Cleanup(func() { server.Stop(t) })

	status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("GET /status factory_state = %q, want RUNNING", status.FactoryState)
	}

	models := support.GetJSON[factoryapi.ListModelsResponse](t, server.URL()+"/models")
	var observed *factoryapi.ModelSummary
	for index := range models.Results {
		if models.Results[index].Name == "OMNIVOICE_Q4_K_M" {
			observed = &models.Results[index]
			break
		}
	}
	if observed == nil {
		t.Fatalf("GET /models did not include OMNIVOICE_Q4_K_M; results=%#v", models.Results)
	}
	if observed.ProviderLocality != factoryapi.WorkerModelLocalityCloud {
		t.Fatalf("GET /models provider locality = %q, want CLOUD", observed.ProviderLocality)
	}
	if observed.ManagedRuntime.Identity != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("GET /models managed runtime identity = %q, want OMNIVOICE_Q4_K_M", observed.ManagedRuntime.Identity)
	}
	if observed.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("GET /models managed readiness = %s, want READY", observed.ManagedRuntime.ReadinessState)
	}
}

// TestModelsCatalogDiscoveryProjectsWorkerCapabilitiesAndFactoryPrecedence
// proves the public catalog preserves the authored worker/resource shape while
// keeping a Factory declaration ahead of the built-in definition with the same
// model name.
func TestModelsCatalogDiscoveryProjectsWorkerCapabilitiesAndFactoryPrecedence(t *testing.T) {
	dir := support.ScaffoldFactory(t, richCatalogFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{},
	})
	t.Cleanup(func() { server.Stop(t) })

	listed := support.GetJSON[factoryapi.ListModelsResponse](t, server.URL()+"/models")
	catalogModel := findCatalogModel(t, listed.Results, "OMNIVOICE_Q4_K_M", "GET /models")
	if catalogModel.ProviderLocality != factoryapi.WorkerModelLocalityLocal ||
		catalogModel.Status != factoryapi.ModelStatusREADY ||
		len(catalogModel.Operations) != 2 ||
		catalogModel.Operations[0].Name != "ASR" || catalogModel.Operations[1].Name != "TTS" {
		t.Fatalf("catalog-model summary = %#v, want local READY ASR/TTS catalog", catalogModel)
	}
	if len(catalogModel.Resources) != 2 || catalogModel.Resources[0].Name != "a-cache" || catalogModel.Resources[1].Name != "z-cache" {
		t.Fatalf("catalog-model resources = %#v, want stable a-cache/z-cache order", catalogModel.Resources)
	}

	detail := support.GetJSON[factoryapi.ModelDetail](t, server.URL()+"/models/OMNIVOICE_Q4_K_M")
	if len(detail.Capabilities) != 2 || detail.Capabilities[0].Worker != "a-worker" || detail.Capabilities[1].Worker != "z-worker" {
		t.Fatalf("catalog-model capabilities = %#v, want stable worker order", detail.Capabilities)
	}
	if detail.Capabilities[0].ModelProvider == nil || *detail.Capabilities[0].ModelProvider != "codex" {
		t.Fatalf("first catalog capability provider = %#v (%q), want codex", detail.Capabilities[0].ModelProvider, *detail.Capabilities[0].ModelProvider)
	}
	if len(detail.Capabilities[0].ResourceNames) != 1 || detail.Capabilities[0].ResourceNames[0] != "a-cache" {
		t.Fatalf("first catalog capability resources = %#v, want a-cache", detail.Capabilities[0].ResourceNames)
	}

	builtInOverride := findCatalogModel(t, listed.Results, "tts", "GET /models")
	if len(builtInOverride.Operations) != 1 || builtInOverride.Operations[0].Name != "TTS" {
		t.Fatalf("factory tts override = %#v, want the authored TTS operation", builtInOverride)
	}
	overrideDetail := support.GetJSON[factoryapi.ModelDetail](t, server.URL()+"/models/tts")
	if overrideDetail.Name != "tts" || len(overrideDetail.Capabilities) != 1 || overrideDetail.Capabilities[0].Worker != "factory-tts" {
		t.Fatalf("factory tts detail = %#v, want Factory-owned tts capability", overrideDetail)
	}
}

// TestModelsInvokeReadinessDependencyFailureIsUnavailableAfterCatalogSuccess
// proves direct invocation does not turn a second readiness lookup failure
// into a backend or filesystem diagnostic.
func TestModelsInvokeReadinessDependencyFailureIsUnavailableAfterCatalogSuccess(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	cacheDirectory := t.TempDir()
	var inspections atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetInspectPath: func(string) (os.FileInfo, error) {
			if inspections.Add(1) <= 1 {
				return nil, os.ErrNotExist
			}
			return nil, errors.New(`inspect C:\private\model-cache: access denied`)
		},
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "readiness failure probe",
	})
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Env = append(inputs.Input.Env, runcli.ModelCacheDirEnvironment+"="+cacheDirectory)
	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf("Process.Execute(models invoke) error = nil, want readiness failure; inspections=%d", inspections.Load())
	}
	if inspections.Load() <= 1 {
		t.Fatalf("model cache inspections = %d, want catalog and follow-up readiness observations", inspections.Load())
	}
	combined := inputs.Stdout() + inputs.Stderr()
	if strings.Contains(combined, "private") || strings.Contains(combined, "access denied") {
		t.Fatalf("models invoke leaked readiness dependency details: %s", combined)
	}
}

// TestModelsInvokeCatalogDependencyCancellationIsSafeThroughProcess proves a
// readiness cancellation returned by the composed catalog does not reach the
// provider or expose the dependency's error text through Process.Execute.
func TestModelsInvokeCatalogDependencyCancellationIsSafeThroughProcess(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	cacheDirectory := t.TempDir()
	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetInspectPath: func(string) (os.FileInfo, error) {
			return nil, context.Canceled
		},
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "catalog cancellation probe",
	})
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Env = append(inputs.Input.Env, runcli.ModelCacheDirEnvironment+"="+cacheDirectory)
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(models invoke) error = nil, want dependency cancellation")
	}
	combined := inputs.Stdout() + inputs.Stderr()
	if strings.Contains(combined, "private") || strings.Contains(combined, "access denied") {
		t.Fatalf("models invoke leaked dependency details: %s", combined)
	}
}

// TestModelsInvokeCatalogRequestCancellationStopsReadiness proves cancellation
// between scope opening and catalog discovery stops GetCatalogModel before it
// can return a partial detail or invoke a provider.
func TestModelsInvokeCatalogRequestCancellationStopsReadiness(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	cacheDirectory := t.TempDir()
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetInspectPath: func(string) (os.FileInfo, error) {
			startOnce.Do(func() { close(started) })
			<-release
			return nil, context.Canceled
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(ctx, []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "catalog request cancellation probe",
	})
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Env = append(inputs.Input.Env, runcli.ModelCacheDirEnvironment+"="+cacheDirectory)
	command := support.StartProcessCommand(t, process, inputs.Input)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		cancel()
		close(release)
		t.Fatal("catalog readiness probe did not start")
	}
	cancel()
	close(release)
	select {
	case <-command.Done():
		if command.Err() == nil {
			t.Fatal("Process.Execute(models invoke) error = nil, want caller cancellation")
		}
		command.AcceptError()
	case <-time.After(5 * time.Second):
		t.Fatal("models invoke did not stop after caller cancellation")
	}
}

// TestModelsInvokeReadinessCancellationAfterCatalogSuccessIsSafe proves a
// cancellation from the direct readiness preflight remains typed and safe
// after catalog discovery has already succeeded.
func TestModelsInvokeReadinessCancellationAfterCatalogSuccessIsSafe(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	cacheDirectory := t.TempDir()
	var inspections atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetInspectPath: func(string) (os.FileInfo, error) {
			if inspections.Add(1) == 1 {
				return nil, os.ErrNotExist
			}
			return nil, context.Canceled
		},
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "readiness cancellation probe",
	})
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Env = append(inputs.Input.Env, runcli.ModelCacheDirEnvironment+"="+cacheDirectory)
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(models invoke) error = nil, want readiness cancellation")
	}
	if inspections.Load() < 2 {
		t.Fatalf("model cache inspections = %d, want catalog and readiness probes", inspections.Load())
	}
}

// TestModelsInvokeReadinessCancellationAfterSuccessfulObservationIsSafe
// proves the post-query context guard prevents invocation after a readiness
// collaborator cancels the caller while returning a normal missing-cache
// observation.
func TestModelsInvokeReadinessCancellationAfterSuccessfulObservationIsSafe(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	cacheDirectory := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	var inspections atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{
		ModelAssetInspectPath: func(string) (os.FileInfo, error) {
			if inspections.Add(1) >= 2 {
				cancel()
			}
			return nil, os.ErrNotExist
		},
	})
	inputs := support.FakeInputs(ctx, []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "post-readiness cancellation probe",
	})
	inputs.Input.WorkingDirectory = factoryDir
	inputs.Input.Env = append(inputs.Input.Env, runcli.ModelCacheDirEnvironment+"="+cacheDirectory)
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(models invoke) error = nil, want post-readiness cancellation")
	}
	if inspections.Load() < 2 {
		t.Fatalf("model cache inspections = %d, want catalog and readiness probes", inspections.Load())
	}
}

func findCatalogModel(
	t *testing.T,
	models []factoryapi.ModelSummary,
	name string,
	operation string,
) factoryapi.ModelSummary {
	t.Helper()
	for _, model := range models {
		if model.Name == name {
			return model
		}
	}
	t.Fatalf("%s did not include %q; results=%#v", operation, name, models)
	return factoryapi.ModelSummary{}
}

func richCatalogFactoryConfig() map[string]any {
	return map[string]any{
		"name": "models-rich-catalog",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{
			{
				"name": "z-cache", "type": interfaces.ResourceTypeModel, "capacity": 2,
				"model": "OMNIVOICE_Q4_K_M", "backend": "LLAMACPP", "loadPolicy": "ON_DEMAND", "provider": "LOCAL",
			},
			{
				"name": "a-cache", "type": interfaces.ResourceTypeModel, "capacity": 1,
				"model": "OMNIVOICE_Q4_K_M", "backend": "LLAMACPP", "loadPolicy": "ON_DEMAND", "provider": "LOCAL",
			},
		},
		"workers": []map[string]any{
			{
				"name": "z-worker", "type": interfaces.WorkerTypeModel, "model": "OMNIVOICE_Q4_K_M",
				"modelProvider": "CODEX", "modelLocality": interfaces.ModelLocalityLocal,
				"resources": []map[string]any{{"name": "z-cache", "capacity": 1}},
				"operations": []map[string]any{
					{"name": "TTS", "inputs": []map[string]any{{"name": "text", "contentTypes": []string{interfaces.ModelOperationContentTypeText}, "required": true}}, "outputs": []map[string]any{{"name": "audio", "contentTypes": []string{interfaces.ModelOperationContentTypeAudio}}}},
				},
			},
			{
				"name": "a-worker", "type": interfaces.WorkerTypeModel, "model": "OMNIVOICE_Q4_K_M",
				"modelProvider": "CODEX", "modelLocality": interfaces.ModelLocalityLocal,
				"resources": []map[string]any{{"name": "a-cache", "capacity": 1}},
				"operations": []map[string]any{
					{"name": "ASR", "inputs": []map[string]any{{"name": "audio", "contentTypes": []string{interfaces.ModelOperationContentTypeAudio}, "required": true}}, "outputs": []map[string]any{{"name": "text", "contentTypes": []string{interfaces.ModelOperationContentTypeText}}}},
				},
			},
			{
				"name": "factory-tts", "type": interfaces.WorkerTypeModel, "model": "tts",
				"modelProvider": "CODEX", "modelLocality": interfaces.ModelLocalityCloud,
				"operations": []map[string]any{{"name": "TTS", "inputs": []map[string]any{{"name": "text", "contentTypes": []string{interfaces.ModelOperationContentTypeText}}}, "outputs": []map[string]any{{"name": "audio", "contentTypes": []string{interfaces.ModelOperationContentTypeAudio}}}}},
			},
		},
	}
}

func catalogDiscoveryFactoryConfig() map[string]any {
	return map[string]any{
		"name": "models-catalog-discovery",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "tts-worker",
			"type":          interfaces.WorkerTypeModel,
			"model":         "OMNIVOICE_Q4_K_M",
			"modelProvider": "CODEX",
			"modelLocality": interfaces.ModelLocalityCloud,
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name":         "text",
					"contentTypes": []string{interfaces.ModelOperationContentTypeText},
					"required":     true,
				}},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		}},
	}
}
