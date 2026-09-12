package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type compositionService struct {
	owned  Service
	legacy *httpService
}

func bindCompositionService(
	httpProtocol clihttp.Protocol,
	pullHTTPProtocol clihttp.Protocol,
	invocation InvocationOperation,
	outputFileSystem OutputFileSystem,
	inputFileReader InputFileReader,
	now func() time.Time,
	providers ...CompositionScopeProvider,
) Service {
	if httpProtocol == nil || invocation == nil {
		return nil
	}
	if pullHTTPProtocol == nil {
		pullHTTPProtocol = httpProtocol
	}
	cfg := ConfigFromComposition(httpProtocol, invocation, providers...)
	cfg.PullHTTP = pullHTTPProtocol
	cfg.OutputFileSystem = outputFileSystem
	cfg.InputFileReader = inputFileReader
	cfg.Clock = now
	var openInvokeScopeWithCache func(context.Context, InvokeScopeRequest) (InvokeRuntimeScope, error)
	var openCatalogScopeWithCache func(context.Context, CatalogScopeRequest) (InvokeRuntimeScope, error)
	compositionCandidates := make([]interface{}, 0, 2)
	if len(providers) > 0 {
		compositionCandidates = append(compositionCandidates, providers[0])
	}
	compositionCandidates = append(compositionCandidates, invocation)
	for _, candidate := range compositionCandidates {
		if opener, ok := candidate.(CompositionInvokeScopeWithModelCacheOpener); ok {
			openInvokeScopeWithCache = opener.CompositionOpenInvokeScopeWithModelCache
		}
		if opener, ok := candidate.(CompositionCatalogScopeWithModelCacheOpener); ok {
			openCatalogScopeWithCache = opener.CompositionOpenCatalogScopeWithModelCache
		}
		if openInvokeScopeWithCache != nil && openCatalogScopeWithCache != nil {
			break
		}
	}
	legacy := &httpService{
		http:             httpProtocol,
		pullHTTP:         pullHTTPProtocol,
		invocation:       invocation,
		now:              now,
		models:           cfg.Models,
		openCatalogScope: cfg.OpenCatalogScope,
		openInvokeScope:  cfg.OpenInvokeScope,
		outputFileSystem: outputFileSystem,
		inputFileReader:  inputFileReader,
	}
	owned := NewService(cfg)
	if owned == nil {
		return legacy
	}
	if root, ok := owned.(*rootService); ok {
		root.openInvokeScopeWithCache = openInvokeScopeWithCache
		root.openCatalogScopeWithCache = openCatalogScopeWithCache
	}
	return &compositionService{owned: owned, legacy: legacy}
}

func (service *compositionService) List(cfg ListConfig) error {
	if service.owned != nil {
		return service.owned.List(cfg)
	}
	return service.legacy.List(cfg)
}

func (service *compositionService) Inspect(cfg InspectConfig) error {
	if service.owned != nil {
		return service.owned.Inspect(cfg)
	}
	return service.legacy.Inspect(cfg)
}

func (service *compositionService) Pull(cfg PullConfig) error {
	if service.owned != nil {
		return service.owned.Pull(cfg)
	}
	return service.legacy.Pull(cfg)
}

func (service *compositionService) Remove(cfg RemoveConfig) error {
	if service.owned != nil {
		return service.owned.Remove(cfg)
	}
	return service.legacy.Remove(cfg)
}

func (service *compositionService) ListWithModelCache(cfg ListConfig, modelCacheDir string) error {
	if strings.TrimSpace(modelCacheDir) == "" || strings.TrimSpace(cfg.Server) != "" {
		return service.List(cfg)
	}
	catalog, ok := service.owned.(ModelCacheCatalog)
	if !ok {
		return fmt.Errorf("Models owned service does not support catalog model cache selection")
	}
	return catalog.ListWithModelCache(cfg, modelCacheDir)
}

func (service *compositionService) InspectWithModelCache(cfg InspectConfig, modelCacheDir string) error {
	if strings.TrimSpace(modelCacheDir) == "" || strings.TrimSpace(cfg.Server) != "" {
		return service.Inspect(cfg)
	}
	catalog, ok := service.owned.(ModelCacheCatalog)
	if !ok {
		return fmt.Errorf("Models owned service does not support catalog model cache selection")
	}
	return catalog.InspectWithModelCache(cfg, modelCacheDir)
}

func (service *compositionService) PullWithModelCache(cfg PullConfig, modelCacheDir string) error {
	if strings.TrimSpace(modelCacheDir) == "" || strings.TrimSpace(cfg.Server) != "" {
		return service.Pull(cfg)
	}
	catalog, ok := service.owned.(ModelCacheCatalog)
	if !ok {
		return fmt.Errorf("Models owned service does not support catalog model cache selection")
	}
	return catalog.PullWithModelCache(cfg, modelCacheDir)
}

func (service *compositionService) RemoveWithModelCache(cfg RemoveConfig, modelCacheDir string) error {
	if strings.TrimSpace(modelCacheDir) == "" || strings.TrimSpace(cfg.Server) != "" {
		return service.Remove(cfg)
	}
	catalog, ok := service.owned.(ModelCacheCatalog)
	if !ok {
		return fmt.Errorf("Models owned service does not support catalog model cache selection")
	}
	return catalog.RemoveWithModelCache(cfg, modelCacheDir)
}

func (service *compositionService) Invoke(cfg InvokeConfig) error {
	if service.owned != nil && service.canInvokeThroughOwned(cfg) {
		return service.owned.Invoke(cfg)
	}
	return service.legacy.Invoke(cfg)
}

func (service *compositionService) InvokeWithScope(request InvokeScopeRequest) error {
	if service.owned == nil {
		if request.Offline {
			return fmt.Errorf("offline models invoke requires the local Models composition")
		}
		return service.legacy.Invoke(request.Config)
	}
	invoker, ok := service.owned.(InvokeScopeInvoker)
	if !ok {
		return fmt.Errorf("Models owned service does not support invocation scope policy")
	}
	if request.Offline || service.canInvokeThroughOwned(request.Config) {
		return invoker.InvokeWithScope(request)
	}
	return service.legacy.Invoke(request.Config)
}

func (service *compositionService) InvokeWithModelCache(cfg InvokeConfig, modelCacheDir string) error {
	if strings.TrimSpace(modelCacheDir) == "" {
		return service.Invoke(cfg)
	}
	if service.owned != nil && service.canInvokeThroughOwned(cfg) {
		invoker, ok := service.owned.(ModelCacheInvoker)
		if !ok {
			return fmt.Errorf("Models owned service does not support invocation-local model cache selection")
		}
		return invoker.InvokeWithModelCache(cfg, modelCacheDir)
	}
	return service.legacy.Invoke(cfg)
}

func (service *compositionService) canInvokeThroughOwned(cfg InvokeConfig) bool {
	if strings.TrimSpace(cfg.Server) != "" {
		return false
	}
	if len(cfg.InputMappings) > 0 || len(cfg.InputSpecs) > 0 || len(cfg.ParameterSpecs) > 0 {
		return true
	}
	// Existing named-worker models retain the bootstrap audio artifact contract.
	// The effective built-in tts alias is a generic operation, so its legacy
	// --text/--output spelling must share the same owned path as --input text=.
	if !cfg.JSON && strings.TrimSpace(cfg.OutputPath) != "" && !isDirectTTSAlias(cfg) {
		return false
	}
	root, ok := service.owned.(*rootService)
	return ok && root != nil && root.openInvokeScope != nil
}

func isDirectTTSAlias(cfg InvokeConfig) bool {
	return strings.EqualFold(strings.TrimSpace(cfg.ModelName), modelinference.BuiltInModelNameTTS) &&
		strings.EqualFold(strings.TrimSpace(cfg.Operation), modelinference.OperationTTS)
}

func genericCLIInvocationFailureFromGenerated(
	failure *factoryapi.ModelInvocationFailure,
	modelName string,
	operation string,
) error {
	if failure == nil {
		return nil
	}
	if strings.TrimSpace(string(failure.Class)) == "" || strings.TrimSpace(failure.Message) == "" {
		return malformedModelsResponseError(nil)
	}
	mapped := &modelinference.InvocationFailure{
		Class:     modelinference.InvocationFailureClass(failure.Class),
		Message:   strings.TrimSpace(failure.Message),
		Model:     modelinference.ModelReference{NameOrURI: modelName},
		Operation: operation,
	}
	if failure.Model != nil && strings.TrimSpace(failure.Model.NameOrUri) != "" {
		mapped.Model = modelinference.ModelReference{NameOrURI: failure.Model.NameOrUri}
	}
	if failure.Slot != nil {
		mapped.Slot = strings.TrimSpace(*failure.Slot)
	}
	if failure.Parameter != nil {
		mapped.Parameter = strings.TrimSpace(*failure.Parameter)
	}
	if failure.Field != nil {
		mapped.Field = strings.TrimSpace(*failure.Field)
	}
	if failure.Operation != nil && strings.TrimSpace(*failure.Operation) != "" {
		mapped.Operation = strings.TrimSpace(*failure.Operation)
	}
	return mapModelsClientError(mapped)
}
func validateGenericCLIResponse(response factoryapi.GenericModelInvocationResponse) error {
	if len(response.Outputs) == 0 {
		return malformedModelsResponseError(nil)
	}
	for _, output := range response.Outputs {
		if strings.TrimSpace(output.Name) == "" || !genericCLIResponseModalityValid(output.Modality) {
			return malformedModelsResponseError(nil)
		}
		if output.Content == nil && output.Artifact == nil {
			return malformedModelsResponseError(nil)
		}
		if output.Content != nil && *output.Content == "" && output.Artifact == nil {
			return malformedModelsResponseError(nil)
		}
		if output.Artifact != nil && strings.TrimSpace(output.Artifact.ArtifactRef) == "" {
			return malformedModelsResponseError(nil)
		}
	}
	return nil
}
func genericCLIResponseModalityValid(modality factoryapi.ModelInvocationContentType) bool {
	switch modality {
	case factoryapi.ModelInvocationContentTypeText,
		factoryapi.ModelInvocationContentTypeImage,
		factoryapi.ModelInvocationContentTypeAudio,
		factoryapi.ModelInvocationContentTypeVideo,
		factoryapi.ModelInvocationContentTypeJSON,
		factoryapi.ModelInvocationContentTypeBinary:
		return true
	default:
		return false
	}
}
func genericCLIRequestFromInputs(
	modelName string,
	operation string,
	inputs []modelinference.InferenceInput,
	parameters []modelinference.OperationParameter,
) factoryapi.GenericModelInvocationRequest {
	generatedInputs := make([]factoryapi.ModelInvocationInput, len(inputs))
	for index, input := range inputs {
		generated := factoryapi.ModelInvocationInput{
			Name:     input.Name,
			Modality: factoryapi.ModelInvocationContentType(input.Modality),
		}
		generated.ContentType = genericCLIStringPointer(input.ContentType)
		generated.MediaType = genericCLIStringPointer(input.MediaType)
		if input.Artifact != nil && !input.Artifact.IsZero() {
			artifactRef := input.Artifact.String()
			generated.ArtifactRef = &artifactRef
		} else if genericCLIInputUsesBinaryCarrier(input.Modality) {
			content := []byte(input.Content)
			generated.ContentBase64 = &content
		} else {
			generated.Content = genericCLIStringPointer(input.Content)
		}
		generatedInputs[index] = generated
	}
	operationValue := operation
	request := factoryapi.GenericModelInvocationRequest{
		Scope:     remoteModelsInvokeScope,
		Holder:    modelsCLIInvokeHolder,
		Model:     factoryapi.ModelReference{NameOrUri: modelName},
		Operation: &operationValue,
		Inputs:    &generatedInputs,
	}
	if len(parameters) > 0 {
		generatedParameters := make([]factoryapi.ModelInvocationParameter, len(parameters))
		for index, parameter := range parameters {
			generatedParameters[index] = factoryapi.ModelInvocationParameter{
				Name: parameter.Name, Value: parameter.Value,
			}
		}
		request.Parameters = &generatedParameters
	}
	return request
}
func genericCLIInputUsesBinaryCarrier(modality modelinference.Modality) bool {
	switch modality {
	case modelinference.ModalityAudio, modelinference.ModalityImage,
		modelinference.ModalityVideo, modelinference.ModalityBinary:
		return true
	default:
		return false
	}
}
