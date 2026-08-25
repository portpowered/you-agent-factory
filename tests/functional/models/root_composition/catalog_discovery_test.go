package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
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
	if observed.ManagedRuntime.Diagnostics == nil ||
		(*observed.ManagedRuntime.Diagnostics)["readinessState"] != string(observed.ManagedRuntime.ReadinessState) ||
		(*observed.ManagedRuntime.Diagnostics)["lifecycleState"] != string(observed.ManagedRuntime.LifecycleState) {
		t.Fatalf("GET /models managed diagnostics = %#v, want state projection", observed.ManagedRuntime.Diagnostics)
	}
	for _, name := range []string{modelprovider.BuiltInModelNameASR, modelprovider.BuiltInModelNameEmbed, modelprovider.BuiltInModelNameLLM, modelprovider.BuiltInModelNameTTS} {
		detail := support.GetJSON[factoryapi.ModelDetail](t, server.URL()+"/models/"+name)
		if detail.Name != name || detail.ManagedRuntime.Identity != name {
			t.Fatalf("GET /models/%s = %#v, want stable effective catalog detail", name, detail)
		}
	}
	unknownResponse, err := http.Get(server.URL() + "/models/catalog-unknown")
	if err != nil {
		t.Fatalf("GET /models/catalog-unknown: %v", err)
	}
	defer unknownResponse.Body.Close()
	if unknownResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /models/catalog-unknown status = %d, want 404", unknownResponse.StatusCode)
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
		t.Fatalf("first catalog capability provider = %#v, want codex", detail.Capabilities[0].ModelProvider)
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
	content := factoryapi.WorkContent{mustFunctionalTextPart(t, "catalog capability probe")}
	body, err := json.Marshal(factoryapi.ModelInvocationRequest{Operation: "TTS", Content: &content})
	if err != nil {
		t.Fatalf("marshal Factory-owned model invocation: %v", err)
	}
	response, err := http.Post(server.URL()+"/models/OMNIVOICE_Q4_K_M/invocations", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /models/OMNIVOICE_Q4_K_M/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("Factory-owned model invocation status = %d, want execution failure after catalog capability resolution", response.StatusCode)
	}
	unsupportedBody, err := json.Marshal(factoryapi.ModelInvocationRequest{Operation: "ASR"})
	if err != nil {
		t.Fatalf("marshal unsupported Factory-owned model invocation: %v", err)
	}
	unsupported, err := http.Post(server.URL()+"/models/tts/invocations", "application/json", bytes.NewReader(unsupportedBody))
	if err != nil {
		t.Fatalf("POST /models/tts/invocations: %v", err)
	}
	defer unsupported.Body.Close()
	if unsupported.StatusCode != http.StatusBadRequest {
		t.Fatalf("unsupported Factory-owned model invocation status = %d, want 400", unsupported.StatusCode)
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

func TestModelsInstalledDetailDiagnosticsFollowResolvedRuntime(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCachedOmniVoiceAssets(t, cacheDirectory)
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       append(functionalHomeEnvironment(cacheDirectory), runcli.ModelCacheDirEnvironment+"="+cacheDirectory),
	})
	t.Cleanup(func() { server.Stop(t) })

	detail := support.GetJSON[factoryapi.ModelDetail](t, server.URL()+"/models/"+story003ModelName)
	if detail.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		detail.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("installed detail managed runtime = %s/%s, want READY/INSTALLED", detail.ManagedRuntime.ReadinessState, detail.ManagedRuntime.LifecycleState)
	}
	if detail.ManagedRuntime.CacheBytes == nil || *detail.ManagedRuntime.CacheBytes <= 0 {
		t.Fatalf("installed detail cache bytes = %v, want non-empty cache", detail.ManagedRuntime.CacheBytes)
	}
	managedDiagnostics := detail.ManagedRuntime.Diagnostics
	if detail.Diagnostics["readinessState"] != string(detail.ManagedRuntime.ReadinessState) ||
		detail.Diagnostics["lifecycleState"] != string(detail.ManagedRuntime.LifecycleState) ||
		managedDiagnostics == nil || (*managedDiagnostics)["readinessState"] != string(detail.ManagedRuntime.ReadinessState) ||
		(*managedDiagnostics)["lifecycleState"] != string(detail.ManagedRuntime.LifecycleState) {
		t.Fatalf("installed detail diagnostics diverged: detail=%#v managedRuntime=%#v", detail.Diagnostics, detail.ManagedRuntime.Diagnostics)
	}

	list := support.GetJSON[factoryapi.ListModelsResponse](t, server.URL()+"/models")
	var listed *factoryapi.ModelSummary
	for index := range list.Results {
		if list.Results[index].Name == story003ModelName {
			listed = &list.Results[index]
			break
		}
	}
	if listed == nil {
		t.Fatalf("GET /models did not include %q; results=%#v", story003ModelName, list.Results)
	}
	if listed.ManagedRuntime.ReadinessState != detail.ManagedRuntime.ReadinessState ||
		listed.ManagedRuntime.LifecycleState != detail.ManagedRuntime.LifecycleState ||
		listed.ManagedRuntime.CacheBytes == nil || detail.ManagedRuntime.CacheBytes == nil ||
		*listed.ManagedRuntime.CacheBytes != *detail.ManagedRuntime.CacheBytes {
		t.Fatalf("installed list/detail managed runtime diverged: list=%#v detail=%#v", listed.ManagedRuntime, detail.ManagedRuntime)
	}
	if listed.ManagedRuntime.Diagnostics == nil ||
		(*listed.ManagedRuntime.Diagnostics)["readinessState"] != string(listed.ManagedRuntime.ReadinessState) ||
		(*listed.ManagedRuntime.Diagnostics)["lifecycleState"] != string(listed.ManagedRuntime.LifecycleState) {
		t.Fatalf("installed list diagnostics = %#v, want state projection", listed.ManagedRuntime.Diagnostics)
	}
}

func TestModelsCatalogReadinessFailureKeepsPublicUnavailableTaxonomy(t *testing.T) {
	t.Parallel()

	t.Run("list projects missing readiness", testListProjectsMissingReadiness)
	t.Run("detail maps cache inspection failure", testDetailMapsCacheInspectionFailure)
	t.Run("list maps readiness dependency failure", testListMapsReadinessDependencyFailure)
	t.Run("detail preserves readiness cancellation", testDetailPreservesReadinessCancellation)
	t.Run("HTTP list does not publish a partial response after cancellation", testHTTPListNoPartialResponseAfterCancellation)
	t.Run("HTTP list observes caller cancellation after a dependency error", testHTTPListCallerCancellationAfterDependencyError)
	t.Run("invoke maps readiness dependency failures", testInvokeMapsReadinessDependencyFailures)
	t.Run("invoke stops after successful readiness observes cancellation", testInvokeStopsAfterSuccessfulReadinessCancellation)
	t.Run("invoke stops while catalog readiness is blocked", testInvokeStopsWhileCatalogReadinessBlocked)
}

func testListProjectsMissingReadiness(t *testing.T) {
	cacheDirectory := t.TempDir()
	dir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env: append(functionalHomeEnvironment(t.TempDir()), runcli.ModelCacheDirEnvironment+"="+cacheDirectory),
	})
	t.Cleanup(func() { server.Stop(t) })

	response, err := http.Get(server.URL() + "/models")
	if err != nil {
		t.Fatalf("GET /models: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET /models status = %d, want 200: %s", response.StatusCode, body)
	}
	var list factoryapi.ListModelsResponse
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		t.Fatalf("decode GET /models: %v", err)
	}
	model := findCatalogModel(t, list.Results, story003ModelName, "GET /models")
	if model.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING ||
		model.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("GET /models readiness = %s/%s, want MISSING/NOT_INSTALLED", model.ManagedRuntime.ReadinessState, model.ManagedRuntime.LifecycleState)
	}
}

func testDetailMapsCacheInspectionFailure(t *testing.T) {
	cacheDirectory := t.TempDir()
	writeCachedOmniVoiceAssets(t, cacheDirectory)
	dir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Env: append(functionalHomeEnvironment(t.TempDir()), runcli.ModelCacheDirEnvironment+"="+cacheDirectory),
		Edges: serviceedges.Edges{ModelAssetInspectPath: func(string) (os.FileInfo, error) {
			return nil, errors.New("cache inspection failed")
		}},
	})
	t.Cleanup(func() { server.Stop(t) })

	response, err := http.Get(server.URL() + "/models/" + story003ModelName)
	if err != nil {
		t.Fatalf("GET /models/%s: %v", story003ModelName, err)
	}
	defer response.Body.Close()
	var failure factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
		t.Fatalf("decode GET /models/%s failure: %v", story003ModelName, err)
	}
	if response.StatusCode != http.StatusNotFound || failure.Family != factoryapi.ErrorFamilyNotFound ||
		failure.Code != factoryapi.ErrorResponseCode("MODEL_NOT_AVAILABLE") {
		t.Fatalf("GET /models/%s failure = status %d %#v, want public unavailable model taxonomy", story003ModelName, response.StatusCode, failure)
	}
}

func testListMapsReadinessDependencyFailure(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	var failInspection atomic.Bool
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ModelAssetInspectPath: func(string) (os.FileInfo, error) {
			if !failInspection.Load() {
				return nil, os.ErrNotExist
			}
			return nil, errors.New("cache inspection failed")
		}},
	})
	t.Cleanup(func() { server.Stop(t) })
	failInspection.Store(true)

	response, err := http.Get(server.URL() + "/models")
	if err != nil {
		t.Fatalf("GET /models: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("GET /models status = %d body=%s, want 404", response.StatusCode, body)
	}
}

func testDetailPreservesReadinessCancellation(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	process := support.BuildProcess(t, serviceedges.Edges{ModelAssetInspectPath: func(string) (os.FileInfo, error) {
		return nil, context.Canceled
	}})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "inspect", story003ModelName})
	inputs.Input.Env = functionalHomeEnvironment(t.TempDir())
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(models inspect) error = nil, want readiness cancellation")
	}
}

func testHTTPListNoPartialResponseAfterCancellation(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	var allowInspection atomic.Bool
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ModelAssetInspectPath: func(string) (os.FileInfo, error) {
			if !allowInspection.Load() {
				return nil, os.ErrNotExist
			}
			return nil, context.Canceled
		}},
	})
	t.Cleanup(func() { server.Stop(t) })
	allowInspection.Store(true)

	response, err := http.Get(server.URL() + "/models")
	if err != nil {
		t.Fatalf("GET /models after readiness cancellation: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatalf("read GET /models cancellation response: %v", readErr)
	}
	if response.StatusCode != http.StatusOK || len(body) != 0 {
		t.Fatalf("GET /models after readiness cancellation = status %d body=%q, want empty response", response.StatusCode, body)
	}
}

func testHTTPListCallerCancellationAfterDependencyError(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	requestContext, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ModelAssetInspectPath: func(string) (os.FileInfo, error) {
			startOnce.Do(func() { close(started) })
			<-release
			return nil, errors.New("readiness dependency failed")
		}},
	})
	t.Cleanup(func() { server.Stop(t) })

	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, server.URL()+"/models", nil)
	if err != nil {
		t.Fatalf("create GET /models cancellation request: %v", err)
	}
	result := make(chan struct {
		response *http.Response
		err      error
	}, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		result <- struct {
			response *http.Response
			err      error
		}{response: response, err: err}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		cancel()
		close(release)
		t.Fatal("GET /models readiness probe did not start")
	}
	cancel()
	close(release)
	select {
	case completed := <-result:
		if completed.response != nil {
			completed.response.Body.Close()
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GET /models did not stop after caller cancellation")
	}
}

func testInvokeMapsReadinessDependencyFailures(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure error
	}{
		{name: "unavailable", failure: errors.New("cache inspection failed")},
		{name: "canceled", failure: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
			cacheDirectory := t.TempDir()
			var inspections atomic.Int32
			process := support.BuildProcess(t, serviceedges.Edges{ModelAssetInspectPath: func(string) (os.FileInfo, error) {
				if inspections.Add(1) == 1 {
					return nil, os.ErrNotExist
				}
				return nil, test.failure
			}})
			support.CleanupProcess(t, process)
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
				"--operation", "TTS", "--text", "readiness failure probe",
			})
			inputs.Input.Env = append(functionalHomeEnvironment(t.TempDir()), runcli.ModelCacheDirEnvironment+"="+cacheDirectory)
			inputs.Input.WorkingDirectory = factoryDir
			if err := process.Execute(inputs.Input); err == nil {
				t.Fatalf("Process.Execute(models invoke) error = nil, want %s", test.name)
			}
			if inspections.Load() <= 1 {
				t.Fatalf("model cache inspections = %d, want catalog and follow-up readiness observations", inspections.Load())
			}
		})
	}
}

func testInvokeStopsAfterSuccessfulReadinessCancellation(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	cacheDirectory := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	var inspections atomic.Int32
	process := support.BuildProcess(t, serviceedges.Edges{ModelAssetInspectPath: func(string) (os.FileInfo, error) {
		if inspections.Add(1) >= 2 {
			cancel()
		}
		return nil, os.ErrNotExist
	}})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(ctx, []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "post-readiness cancellation probe",
	})
	inputs.Input.Env = append(functionalHomeEnvironment(t.TempDir()), runcli.ModelCacheDirEnvironment+"="+cacheDirectory)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(models invoke) error = nil, want post-readiness cancellation")
	}
	if inspections.Load() < 2 {
		t.Fatalf("model cache inspections = %d, want catalog and readiness probes", inspections.Load())
	}
}

func testInvokeStopsWhileCatalogReadinessBlocked(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, localModelReadinessAssetsHostFactoryConfig("http://127.0.0.1:1"))
	cacheDirectory := t.TempDir()
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	process := support.BuildProcess(t, serviceedges.Edges{ModelAssetInspectPath: func(string) (os.FileInfo, error) {
		startOnce.Do(func() { close(started) })
		<-release
		return nil, context.Canceled
	}})
	support.CleanupProcess(t, process)
	ctx, cancel := context.WithCancel(context.Background())
	inputs := support.FakeInputs(ctx, []string{
		"you", "--json", "models", "invoke", "OMNIVOICE_Q4_K_M",
		"--operation", "TTS", "--text", "catalog request cancellation probe",
	})
	inputs.Input.Env = append(functionalHomeEnvironment(t.TempDir()), runcli.ModelCacheDirEnvironment+"="+cacheDirectory)
	inputs.Input.WorkingDirectory = factoryDir
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
