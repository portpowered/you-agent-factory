package models_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func (unsupportedRuntimeScopePeer) ListCatalog(
	context.Context,
	models.ListModelsRequest,
) (models.ListModelsResult, error) {
	return models.ListModelsResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) GetCatalogModel(
	context.Context,
	models.GetModelRequest,
) (models.GetModelResult, error) {
	return models.GetModelResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) GetModelReadiness(
	context.Context,
	models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	return models.GetModelReadinessResult{}, models.ErrUnsupportedOperation
}

// catalogPeerService is a fake peer implementer of Models root Service that
// exercises scoped catalog and readiness contracts using only root types.
type catalogPeerService struct {
	*runtimeScopePeerService
	unavailable bool
	entries     map[string]models.Detail
}

func (s catalogPeerService) ListModels(context.Context) (models.List, error) {
	if s.unavailable {
		return models.List{}, models.ErrUnavailable
	}
	results := make([]models.Summary, 0, len(s.entries))
	for _, detail := range s.entries {
		results = append(results, detail.Summary)
	}
	return models.List{Results: results}, nil
}

func (s catalogPeerService) ListCatalog(
	ctx context.Context,
	request models.ListModelsRequest,
) (models.ListModelsResult, error) {
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.ListModelsResult{}, err
	}
	list, err := s.ListModels(ctx)
	if err != nil {
		return models.ListModelsResult{}, err
	}
	result := models.ListModelsResult{Models: make([]models.Summary, len(list.Results))}
	for i := range list.Results {
		result.Models[i] = list.Results[i].Clone()
	}
	return result, nil
}

func (s catalogPeerService) GetCatalogModel(
	ctx context.Context,
	request models.GetModelRequest,
) (models.GetModelResult, error) {
	if err := request.Validate(); err != nil {
		return models.GetModelResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.GetModelResult{}, err
	}
	detail, err := s.GetModel(ctx, request.Name)
	if err != nil {
		return models.GetModelResult{}, err
	}
	if request.Operation != "" && !detailSupportsOperation(detail, request.Operation) {
		return models.GetModelResult{}, models.ErrUnsupportedOperation
	}
	return models.GetModelResult{Model: detail.Clone()}, nil
}

func (s catalogPeerService) GetModelReadiness(
	ctx context.Context,
	request models.GetModelReadinessRequest,
) (models.GetModelReadinessResult, error) {
	if err := request.Validate(); err != nil {
		return models.GetModelReadinessResult{}, err
	}
	found, err := s.GetCatalogModel(ctx, models.GetModelRequest{
		Scope:     request.Scope,
		Name:      request.Name,
		Operation: request.Operation,
	})
	if err != nil {
		return models.GetModelReadinessResult{}, err
	}
	return models.GetModelReadinessResult{
		ModelName: found.Model.Name,
		Readiness: found.Model.ManagedRuntime,
	}, nil
}

func detailSupportsOperation(detail models.Detail, operation string) bool {
	for _, configured := range detail.Operations {
		if configured.Name == operation {
			return true
		}
	}
	for _, capability := range detail.Capabilities {
		for _, configured := range capability.Operations {
			if configured.Name == operation {
				return true
			}
		}
	}
	return false
}

func TestCatalogDiscovery_ScopedListGetAndReadinessReturnDetachedRootValues(t *testing.T) {
	t.Parallel()

	fake := catalogPeerService{
		runtimeScopePeerService: newRuntimeScopePeerService("factory-session-a"),
		entries: map[string]models.Detail{
			"local-model": {
				Summary: models.Summary{
					Name:             "local-model",
					ProviderLocality: models.LocalityLocal,
					Status:           models.StatusReady,
					Operations:       []models.Operation{{Name: "generate"}},
					ManagedRuntime: models.Runtime{
						Identity:            "local-model",
						ReadinessState:      models.ReadinessStateReady,
						LifecycleState:      models.LifecycleStateInstalled,
						SupportedOperations: []models.Operation{{Name: "generate"}},
					},
				},
				Capabilities: []models.Capability{{
					Worker:           "writer",
					ProviderLocality: models.LocalityLocal,
					Operations:       []models.Operation{{Name: "generate"}},
				}},
				Sources: []models.SourceMetadata{{
					Provider:  "hugging-face",
					Reference: "org/local-model",
					Revision:  "sha256:abc",
				}},
			},
		},
	}
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}

	list, err := service.ListCatalog(context.Background(), models.ListModelsRequest{Scope: opened.Scope})
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(list.Models) != 1 || list.Models[0].Name != "local-model" {
		t.Fatalf("ListCatalog = %#v, want detached local-model summary", list)
	}

	got, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: opened.Scope, Name: "local-model", Operation: "generate",
	})
	if err != nil {
		t.Fatalf("GetCatalogModel: %v", err)
	}
	assertCatalogDetail(t, got.Model)

	readiness, err := service.GetModelReadiness(context.Background(), models.GetModelReadinessRequest{
		Scope: opened.Scope, Name: "local-model", Operation: "generate",
	})
	if err != nil {
		t.Fatalf("GetModelReadiness: %v", err)
	}
	if readiness.ModelName != "local-model" || readiness.Readiness.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("GetModelReadiness = %#v, want ready local-model", readiness)
	}
}

func assertCatalogDetail(t *testing.T, got models.Detail) {
	t.Helper()
	if len(got.Sources) != 1 || got.Sources[0].Reference != "org/local-model" {
		t.Fatalf("GetCatalogModel = %#v, want detached source metadata", got)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].Worker != "writer" {
		t.Fatalf("GetCatalogModel = %#v, want configured binding facts", got)
	}
}

func TestCatalogDiscovery_ResultsAreDetachedFromPeerState(t *testing.T) {
	t.Parallel()

	fake := catalogPeerService{
		runtimeScopePeerService: newRuntimeScopePeerService("factory-session-detached"),
		entries: map[string]models.Detail{
			"local-model": {
				Summary:      models.Summary{Name: "local-model", Operations: []models.Operation{{Name: "generate"}}},
				Capabilities: []models.Capability{{Worker: "writer"}},
				Sources:      []models.SourceMetadata{{Reference: "org/local-model"}},
			},
		},
	}
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	got, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: opened.Scope, Name: "local-model",
	})
	if err != nil {
		t.Fatalf("GetCatalogModel: %v", err)
	}
	got.Model.Sources[0].Reference = "mutated"
	got.Model.Capabilities[0].Worker = "mutated"
	got.Model.Operations[0].Name = "mutated"

	again, err := service.GetCatalogModel(context.Background(), models.GetModelRequest{
		Scope: opened.Scope, Name: "local-model",
	})
	if err != nil {
		t.Fatalf("GetCatalogModel after mutation: %v", err)
	}
	if again.Model.Sources[0].Reference != "org/local-model" ||
		again.Model.Capabilities[0].Worker != "writer" ||
		again.Model.Operations[0].Name != "generate" {
		t.Fatalf("GetCatalogModel retained caller mutation: %#v", again)
	}
}

func TestCatalogDiscovery_NormalizedFailuresStayDistinct(t *testing.T) {
	t.Parallel()

	fake := catalogPeerService{
		runtimeScopePeerService: newRuntimeScopePeerService("factory-session-a"),
		entries: map[string]models.Detail{
			"local-model": {Summary: models.Summary{Name: "local-model"}},
		},
	}
	var service models.Service = fake
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	stale, _ := (models.RuntimeScopeRef{}).Parse("factory-session-a:stale")
	foreign, _ := (models.RuntimeScopeRef{}).Parse("factory-session-b:1")

	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: models.RuntimeScopeRef{}, Name: "local-model"}, models.ErrRuntimeScopeInvalid)
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: stale, Name: "local-model"}, models.ErrRuntimeScopeStale)
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: foreign, Name: "local-model"}, models.ErrRuntimeScopeForeign)
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: opened.Scope, Name: "missing"}, models.ErrNotFound)
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{
		Scope: opened.Scope, Name: "local-model", Operation: "unsupported",
	}, models.ErrUnsupportedOperation)

	if _, err := service.CloseRuntimeScope(context.Background(), models.CloseRuntimeScopeRequest{Scope: opened.Scope}); err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	assertCatalogGetErrorIs(t, service, models.GetModelRequest{Scope: opened.Scope, Name: "local-model"}, models.ErrRuntimeScopeClosed)

	unavailableFake := catalogPeerService{
		runtimeScopePeerService: newRuntimeScopePeerService("factory-session-c"),
		unavailable:             true,
	}
	var unavailable models.Service = unavailableFake
	unavailableScope, err := unavailable.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope unavailable fake: %v", err)
	}
	_, err = unavailable.ListCatalog(context.Background(), models.ListModelsRequest{Scope: unavailableScope.Scope})
	if !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("ListCatalog unavailable = %v, want ErrUnavailable", err)
	}
}

func assertCatalogGetErrorIs(
	t *testing.T,
	service models.Service,
	request models.GetModelRequest,
	want error,
) {
	t.Helper()
	_, err := service.GetCatalogModel(context.Background(), request)
	if !errors.Is(err, want) {
		t.Fatalf("GetCatalogModel(%+v) = %v, want %v", request, err, want)
	}
}

func TestGenericOperationContractsDescribeExactSlotShapes(t *testing.T) {
	t.Parallel()

	operationCatalog := models.GenericOperationCatalog{}
	contracts := operationCatalog.GenericOperationContracts()
	wantNames := []string{models.OperationOMNI, models.OperationEMBED, models.OperationTTS, models.OperationASR}
	if len(contracts) != len(wantNames) {
		t.Fatalf("GenericOperationContracts length = %d, want %d", len(contracts), len(wantNames))
	}
	for index, wantName := range wantNames {
		if contracts[index].Name != wantName {
			t.Fatalf("operation[%d].Name = %q, want %q", index, contracts[index].Name, wantName)
		}
	}

	assertOperationSlots(t, contracts[0], []operationSlotExpectation{
		{name: "prompt", modality: models.ModalityText, required: true, mediaType: "text/plain"},
		{name: "image", modality: models.ModalityImage, repeatable: true, mediaType: "image/*"},
		{name: "audio", modality: models.ModalityAudio, mediaType: "audio/*"},
		{name: "video", modality: models.ModalityVideo, mediaType: "video/*"},
		{name: "parameters", modality: models.ModalityJSON, mediaType: "application/json"},
	}, []operationSlotExpectation{
		{name: "text", modality: models.ModalityText, required: true, mediaType: "text/plain"},
		{name: "usage", modality: models.ModalityJSON, mediaType: "application/json"},
	})
	assertOperationSlots(t, contracts[1], []operationSlotExpectation{
		{name: "text", modality: models.ModalityText, required: true, mediaType: "text/plain"},
		{name: "parameters", modality: models.ModalityJSON, mediaType: "application/json"},
	}, []operationSlotExpectation{
		{name: "embedding", modality: models.ModalityJSON, required: true, mediaType: "application/json"},
	})
	assertOperationSlots(t, contracts[2], []operationSlotExpectation{
		{name: "text", modality: models.ModalityText, required: true, mediaType: "text/plain"},
		{name: "voice", modality: models.ModalityAudio, mediaType: "audio/*"},
		{name: "parameters", modality: models.ModalityJSON, mediaType: "application/json"},
	}, []operationSlotExpectation{
		{name: "audio", modality: models.ModalityAudio, required: true, mediaType: "audio/*"},
	})
	assertOperationSlots(t, contracts[3], []operationSlotExpectation{
		{name: "audio", modality: models.ModalityAudio, required: true, mediaType: "audio/*"},
		{name: "prompt", modality: models.ModalityText, mediaType: "text/plain"},
		{name: "parameters", modality: models.ModalityJSON, mediaType: "application/json"},
	}, []operationSlotExpectation{
		{name: "transcript", modality: models.ModalityText, required: true, mediaType: "text/plain"},
		{name: "segments", modality: models.ModalityJSON, required: true, mediaType: "application/json"},
	})
}

type operationSlotExpectation struct {
	name       string
	modality   models.Modality
	required   bool
	repeatable bool
	mediaType  string
}

func assertOperationSlots(
	t *testing.T,
	operation models.Operation,
	wantInputs []operationSlotExpectation,
	wantOutputs []operationSlotExpectation,
) {
	t.Helper()
	assertOperationSlotList(t, operation.Name+" inputs", operation.Inputs, wantInputs)
	assertOperationSlotList(t, operation.Name+" outputs", operation.Outputs, wantOutputs)
}

func assertOperationSlotList(
	t *testing.T,
	label string,
	slots []models.OperationSlot,
	want []operationSlotExpectation,
) {
	t.Helper()
	if len(slots) != len(want) {
		t.Fatalf("%s length = %d, want %d", label, len(slots), len(want))
	}
	for index, expected := range want {
		actual := slots[index]
		if actual.Name != expected.name || actual.Modality != expected.modality || actual.Repeatable != expected.repeatable {
			t.Fatalf("%s[%d] = %#v, want name=%q modality=%q repeatable=%t", label, index, actual, expected.name, expected.modality, expected.repeatable)
		}
		if actual.Required == nil || *actual.Required != expected.required {
			t.Fatalf("%s[%d].Required = %v, want %t", label, index, actual.Required, expected.required)
		}
		if len(actual.MediaTypes) != 1 || actual.MediaTypes[0] != expected.mediaType {
			t.Fatalf("%s[%d].MediaTypes = %#v, want [%q]", label, index, actual.MediaTypes, expected.mediaType)
		}
	}
}

func TestGenericOperationContractsAreDetached(t *testing.T) {
	t.Parallel()

	operationCatalog := models.GenericOperationCatalog{}
	first, ok := operationCatalog.GenericOperationContract(" omni ")
	if !ok {
		t.Fatal("GenericOperationContract(omni) did not find OMNI")
	}
	first.Inputs[1].MediaTypes[0] = "mutated"
	first.Outputs[0].ContentTypes[0] = "mutated"

	second, ok := operationCatalog.GenericOperationContract(models.OperationOMNI)
	if !ok {
		t.Fatal("GenericOperationContract(OMNI) did not find OMNI")
	}
	if second.Inputs[1].MediaTypes[0] != "image/*" || second.Outputs[0].ContentTypes[0] != string(models.ModalityText) {
		t.Fatalf("GenericOperationContract retained caller mutation: %#v", second)
	}
}

func TestBuiltInModelCatalogPublishesCanonicalDefinitions(t *testing.T) {
	t.Parallel()

	builtIns := models.BuiltInCatalog{}
	operationCatalog := models.GenericOperationCatalog{}

	want := []struct {
		name      string
		source    string
		backend   string
		operation string
	}{
		{
			name:      "llm",
			source:    "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3",
			backend:   "localai-llamacpp",
			operation: models.OperationOMNI,
		},
		{
			name:      "asr",
			source:    "hf://ggerganov/whisper.cpp/ggml-base.en.bin@5359861c739e955e79d9a303bcbc70fb988958b1",
			backend:   "localai-whisper",
			operation: models.OperationASR,
		},
		{
			name:      "tts",
			source:    "hf://vibevoice/VibeVoice-7B@505114ae6ad17be74df98e6939707434ec49c187",
			backend:   "localai-vibevoice",
			operation: models.OperationTTS,
		},
		{
			name:      "embed",
			source:    "hf://Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-f16.gguf@370f27d7550e0def9b39c1f16d3fbaa13aa67728",
			backend:   "localai-llamacpp",
			operation: models.OperationEMBED,
		},
	}

	definitions := builtIns.ModelDefinitions()
	if len(definitions) != len(want) {
		t.Fatalf("BuiltInModelDefinitions length = %d, want %d", len(definitions), len(want))
	}
	for index, expected := range want {
		definition := definitions[index]
		if definition.Name != expected.name || definition.Source != expected.source ||
			definition.Backend != expected.backend || definition.LoadPolicy != models.LoadPolicyOnDemand {
			t.Fatalf("definition[%d] = %#v, want name=%q source=%q backend=%q loadPolicy=%q", index, definition, expected.name, expected.source, expected.backend, models.LoadPolicyOnDemand)
		}
		if len(definition.Operations) != 1 || definition.Operations[0].Name != expected.operation {
			t.Fatalf("definition[%d].Operations = %#v, want one %s operation", index, definition.Operations, expected.operation)
		}
		canonical, ok := operationCatalog.GenericOperationContract(expected.operation)
		if !ok || !reflect.DeepEqual(definition.Operations[0], canonical) {
			t.Fatalf("definition[%d].Operations[0] = %#v, want canonical %s contract %#v", index, definition.Operations[0], expected.operation, canonical)
		}
	}

	catalog := builtIns.ModelCatalog()
	if len(catalog) != len(want) {
		t.Fatalf("BuiltInModelCatalog length = %d, want %d", len(catalog), len(want))
	}
	for _, expected := range want {
		definition, ok := catalog[expected.name]
		if !ok || definition.Name != expected.name {
			t.Fatalf("BuiltInModelCatalog[%q] = %#v, present = %t", expected.name, definition, ok)
		}
	}
}

func TestBuiltInModelCatalogReturnsDetachedDefinitions(t *testing.T) {
	t.Parallel()

	builtIns := models.BuiltInCatalog{}

	first := builtIns.ModelCatalog()
	first["llm"].Operations[0].Inputs[0].Name = "mutated"
	delete(first, "asr")

	second := builtIns.ModelCatalog()
	if len(second) != 4 {
		t.Fatalf("second catalog length = %d, want 4", len(second))
	}
	if second["llm"].Operations[0].Inputs[0].Name != "prompt" {
		t.Fatalf("second catalog retained nested mutation: %#v", second["llm"])
	}
	if _, ok := second["asr"]; !ok {
		t.Fatal("second catalog retained map mutation")
	}

	definition, ok := builtIns.ModelDefinitionFor(" LLM ")
	if !ok {
		t.Fatal("BuiltInModelDefinitionFor(LLM) did not find the built-in")
	}
	definition.Operations[0].Outputs[0].Name = "mutated"
	fresh, ok := builtIns.ModelDefinitionFor("llm")
	if !ok || fresh.Operations[0].Outputs[0].Name != "text" {
		t.Fatalf("BuiltInModelDefinitionFor retained nested mutation: %#v", fresh)
	}
}

func TestGenericInvocationRequestPreservesOrderedRepeatedInputs(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("scope-generic-001")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	request := models.GenericInvocationRequest{
		Scope:     scope,
		Holder:    "cli",
		Model:     models.ModelReference{NameOrURI: "llm"},
		Operation: models.OperationOMNI,
		Inputs: []models.InferenceInput{
			{Name: "prompt", Modality: models.ModalityText, Content: "compare"},
			{Name: "image", Modality: models.ModalityImage, MediaType: "image/png", Content: "first"},
			{Name: "image", Modality: models.ModalityImage, MediaType: "image/jpeg", Content: "second"},
		},
		Parameters: []models.OperationParameter{{Name: "temperature", Value: map[string]any{"value": 0.2}}},
		OutputMode: models.OutputModeJSON,
		Offline:    true,
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if len(request.Inputs) != 3 || request.Inputs[1].Name != "image" || request.Inputs[2].Content != "second" {
		t.Fatalf("ordered inputs = %#v", request.Inputs)
	}
	if request.Parameters[0].Name != "temperature" || request.OutputMode != models.OutputModeJSON || !request.Offline {
		t.Fatalf("generic request controls = %#v", request)
	}
}

func TestGenericInvocationResultPreservesNamedASROutputsAndTypedFailures(t *testing.T) {
	t.Parallel()

	artifact, err := (models.InferenceArtifactRef{}).Parse("artifact:segments")
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	result := models.GenericInvocationResult{
		Outputs: []models.InferenceOutput{
			{Name: "transcript", Modality: models.ModalityText, MediaType: "text/plain", Content: "hello"},
			{Name: "segments", Modality: models.ModalityJSON, MediaType: "application/json", Artifact: &models.InferenceArtifact{
				Artifact: artifact, Name: "segments.json", MediaType: "application/json", SizeBytes: 12,
			}},
		},
	}
	cloned := result.Clone()
	cloned.Outputs[1].Artifact.Properties = map[string]string{"mutated": "true"}
	if len(cloned.Outputs) != 2 || cloned.Outputs[0].Name != "transcript" || cloned.Outputs[1].Name != "segments" {
		t.Fatalf("named outputs = %#v", cloned.Outputs)
	}

	classes := []models.InvocationFailureClass{
		models.InvocationFailureClassInvalidModelReference,
		models.InvocationFailureClassInvalidOperation,
		models.InvocationFailureClassInvalidSlot,
		models.InvocationFailureClassSlotArity,
		models.InvocationFailureClassInvalidParameter,
		models.InvocationFailureClassMediaCapability,
		models.InvocationFailureClassConfiguration,
		models.InvocationFailureClassOfflineCache,
		models.InvocationFailureClassArtifact,
		models.InvocationFailureClassBackendReadiness,
		models.InvocationFailureClassBackendProtocol,
		models.InvocationFailureClassCancellation,
		models.InvocationFailureClassTimeout,
		models.InvocationFailureClassMalformedResponse,
	}
	seen := make(map[models.InvocationFailureClass]struct{}, len(classes))
	for _, class := range classes {
		if _, duplicate := seen[class]; duplicate {
			t.Fatalf("duplicate invocation failure class %q", class)
		}
		seen[class] = struct{}{}
	}
	var typed *models.InvocationFailure
	if !errors.As((&models.InvocationFailure{Class: models.InvocationFailureClassTimeout, Message: "timed out"}), &typed) || typed.Class != models.InvocationFailureClassTimeout {
		t.Fatal("typed invocation failure did not preserve class identity")
	}
}

func TestGenericInvocationValidationReportsTypedContractFailures(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("scope-generic-validation")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	base := models.GenericInvocationRequest{
		Scope:     scope,
		Holder:    "worker-1",
		Model:     models.ModelReference{NameOrURI: "llm"},
		Operation: models.OperationOMNI,
		Inputs:    []models.InferenceInput{{Name: "prompt"}},
	}
	assertGenericInvocationFailureCases(t, base)
	assertGenericInvocationOutputModes(t, base)
	assertGenericInvocationScopeAndHolderValidation(t, base)
}

func assertGenericInvocationFailureCases(t *testing.T, base models.GenericInvocationRequest) {
	t.Helper()
	tests := []struct {
		name  string
		setup func(*models.GenericInvocationRequest)
		class models.InvocationFailureClass
	}{
		{
			name: "missing model reference",
			setup: func(request *models.GenericInvocationRequest) {
				request.Model = models.ModelReference{}
			},
			class: models.InvocationFailureClassInvalidModelReference,
		},
		{
			name: "unnamed input slot",
			setup: func(request *models.GenericInvocationRequest) {
				request.Inputs = []models.InferenceInput{{}}
			},
			class: models.InvocationFailureClassInvalidSlot,
		},
		{
			name: "unnamed parameter",
			setup: func(request *models.GenericInvocationRequest) {
				request.Parameters = []models.OperationParameter{{Value: true}}
			},
			class: models.InvocationFailureClassInvalidParameter,
		},
		{
			name: "unsupported output mode",
			setup: func(request *models.GenericInvocationRequest) {
				request.OutputMode = models.OutputMode("UNSUPPORTED")
			},
			class: models.InvocationFailureClassInvalidParameter,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.setup(&request)
			err := request.Validate()
			var failure *models.InvocationFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Validate() error = %v, want *InvocationFailure", err)
			}
			if failure.Class != test.class {
				t.Fatalf("Validate() failure = %#v, want class %q", failure, test.class)
			}
		})
	}
	request := base
	request.Operation = ""
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate() with omitted operation = %v, want deferred operation selection", err)
	}
}

func assertGenericInvocationOutputModes(t *testing.T, base models.GenericInvocationRequest) {
	t.Helper()
	for _, outputMode := range []models.OutputMode{
		"",
		models.OutputModeAuto,
		models.OutputModeInline,
		models.OutputModeJSON,
		models.OutputModeArtifact,
	} {
		request := base
		request.OutputMode = outputMode
		if err := request.ValidateGeneric(); err != nil {
			t.Fatalf("ValidateGeneric(%q) error = %v, want nil", outputMode, err)
		}
	}
}

func assertGenericInvocationScopeAndHolderValidation(t *testing.T, base models.GenericInvocationRequest) {
	t.Helper()
	for _, test := range []struct {
		name string
		edit func(*models.GenericInvocationRequest)
		want error
	}{
		{
			name: "invalid scope",
			edit: func(request *models.GenericInvocationRequest) { request.Scope = models.RuntimeScopeRef{} },
			want: models.ErrRuntimeScopeInvalid,
		},
		{
			name: "invalid holder",
			edit: func(request *models.GenericInvocationRequest) { request.Holder = "" },
			want: models.ErrHostInvalidHolder,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.edit(&request)
			if err := request.ValidateGeneric(); !errors.Is(err, test.want) {
				t.Fatalf("ValidateGeneric() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestGenericInvocationValuesCloneNestedPayloadsAndArtifacts(t *testing.T) {
	t.Parallel()

	parameter := models.OperationParameter{Value: map[string]any{
		"nested":  []any{map[string]any{"value": "original"}},
		"strings": []string{"first"},
		"bytes":   []byte("payload"),
		"scalar":  42,
	}}
	clonedParameter := parameter.Clone()
	clonedValue := clonedParameter.Value.(map[string]any)
	clonedValue["nested"].([]any)[0].(map[string]any)["value"] = "mutated"
	clonedValue["strings"].([]string)[0] = "mutated"
	clonedValue["bytes"].([]byte)[0] = 'M'
	originalValue := parameter.Value.(map[string]any)
	if originalValue["nested"].([]any)[0].(map[string]any)["value"] != "original" ||
		originalValue["strings"].([]string)[0] != "first" ||
		string(originalValue["bytes"].([]byte)) != "payload" {
		t.Fatalf("OperationParameter.Clone retained nested mutation: %#v", originalValue)
	}
	if (models.OperationParameter{Value: nil}).Clone().Value != nil {
		t.Fatal("OperationParameter.Clone(nil) did not preserve nil")
	}

	artifactRef, err := (models.InferenceArtifactRef{}).Parse("artifact:clone")
	if err != nil {
		t.Fatalf("parse artifact reference: %v", err)
	}
	originalArtifact := models.InferenceArtifact{
		Artifact:   artifactRef,
		Properties: map[string]string{"digest": "original"},
	}
	result := models.GenericInvocationResult{Outputs: []models.InferenceOutput{
		{Name: "artifact", Artifact: &originalArtifact},
		{Name: "inline"},
	}}
	clonedResult := result.Clone()
	clonedResult.Outputs[0].Artifact.Properties["digest"] = "mutated"
	if originalArtifact.Properties["digest"] != "original" || clonedResult.Outputs[1].Artifact != nil {
		t.Fatalf("InvokeModelResult.Clone did not detach output artifacts: %#v", clonedResult.Outputs)
	}
}

func TestGenericCatalogMissingLookupAndReadinessValuesRemainDetached(t *testing.T) {
	t.Parallel()

	operation, ok := (models.GenericOperationCatalog{}).GenericOperationContract("missing")
	if ok || operation.Name != "" {
		t.Fatalf("missing operation = %#v, present = %t", operation, ok)
	}
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor("missing")
	if ok || definition.Name != "" {
		t.Fatalf("missing model definition = %#v, present = %t", definition, ok)
	}

	original := models.Operation{
		Name: "custom",
		Inputs: []models.OperationSlot{{
			Name:         "input",
			ContentTypes: []string{"TEXT"},
			Modality:     models.ModalityText,
			Required:     boolPointer(true),
			MediaTypes:   []string{"text/plain"},
		}},
	}
	cloned := original.Clone()
	cloned.Inputs[0].ContentTypes[0] = "mutated"
	cloned.Inputs[0].MediaTypes[0] = "mutated"
	*cloned.Inputs[0].Required = false
	if original.Inputs[0].ContentTypes[0] != "TEXT" || original.Inputs[0].MediaTypes[0] != "text/plain" || !*original.Inputs[0].Required {
		t.Fatalf("Operation.Clone retained nested mutation: %#v", original)
	}

	runtime := models.Runtime{
		Identity:            "runtime",
		ReadinessState:      models.ReadinessStateReady,
		SupportedOperations: []models.Operation{original},
		Diagnostics:         map[string]string{"status": "ready"},
	}
	clonedRuntime := runtime.Clone()
	clonedRuntime.SupportedOperations[0].Inputs[0].Name = "mutated"
	clonedRuntime.Diagnostics["status"] = "mutated"
	if runtime.SupportedOperations[0].Inputs[0].Name != "input" || runtime.Diagnostics["status"] != "ready" {
		t.Fatalf("Runtime.Clone retained nested mutation: %#v", runtime)
	}
}

func TestInvocationFailureMethodsExposeStableSafeIdentity(t *testing.T) {
	t.Parallel()

	cause := errors.New("private backend detail")
	withMessage := &models.InvocationFailure{Message: "safe message", Cause: cause}
	if withMessage.Error() != "safe message" || !errors.Is(withMessage, cause) {
		t.Fatalf("InvocationFailure with message = %v, want safe message and cause", withMessage)
	}
	withoutMessage := &models.InvocationFailure{Class: models.InvocationFailureClassTimeout}
	if withoutMessage.Error() != "model invocation failed: TIMEOUT" {
		t.Fatalf("InvocationFailure without message = %q", withoutMessage.Error())
	}
	withoutClass := &models.InvocationFailure{}
	if withoutClass.Error() != "model invocation failed" {
		t.Fatalf("InvocationFailure without class = %q", withoutClass.Error())
	}
	var nilFailure *models.InvocationFailure
	if nilFailure.Error() != "" || nilFailure.Unwrap() != nil {
		t.Fatal("nil InvocationFailure methods did not remain safe")
	}
}

func TestInvocationErrorProjectsManagedRuntimeReadiness(t *testing.T) {
	t.Parallel()

	readinessCases := []struct {
		state  models.ReadinessState
		cause  error
		action string
	}{
		{state: models.ReadinessStateMissing, cause: models.ErrMissing, action: "pull or install"},
		{state: models.ReadinessStateLoading, cause: models.ErrLoading, action: "wait for"},
		{state: models.ReadinessStateFailed, cause: models.ErrFailed, action: "resolve"},
		{state: models.ReadinessStateUnsupported, cause: models.ErrUnsupported, action: "use a supported"},
		{state: models.ReadinessState("UNKNOWN"), cause: models.ErrUnsupported, action: "resolve managed runtime readiness"},
	}
	for _, test := range readinessCases {
		t.Run(string(test.state), func(t *testing.T) {
			err := (models.Runtime{
				Identity:       "runtime",
				ReadinessState: test.state,
				LifecycleState: models.LifecycleStateLoaded,
			}).InvocationError()
			if !errors.Is(err, test.cause) || !strings.Contains(err.Error(), test.action) {
				t.Fatalf("InvocationError() = %v, want %v and action %q", err, test.cause, test.action)
			}
			var readinessErr *models.InvocationError
			if !errors.As(err, &readinessErr) || readinessErr.ManagedRuntimeReadinessState() != test.state || readinessErr.Unwrap() != test.cause {
				t.Fatalf("InvocationError projection = %#v, want state %q and cause %v", readinessErr, test.state, test.cause)
			}
		})
	}
	var nilReadiness *models.InvocationError
	if nilReadiness.Error() != "" || nilReadiness.Unwrap() != nil || nilReadiness.ManagedRuntimeReadinessState() != "" {
		t.Fatal("nil InvocationError methods did not remain safe")
	}
}

func boolPointer(value bool) *bool {
	return &value
}
