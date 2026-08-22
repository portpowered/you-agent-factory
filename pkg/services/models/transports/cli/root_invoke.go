package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func (service *rootService) Invoke(cfg InvokeConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	operation := strings.TrimSpace(cfg.Operation)
	if operation == "" {
		return fmt.Errorf("--operation is required")
	}
	text := strings.TrimSpace(cfg.Text)
	if text == "" {
		return fmt.Errorf("--text is required")
	}
	if strings.TrimSpace(cfg.Server) != "" {
		return fmt.Errorf("remote models invoke requires the composition-stable HTTP service")
	}
	if service.openInvokeScope == nil {
		return fmt.Errorf("models invoke runtime scope opener is required")
	}
	scope, err := service.openInvokeScope(cfg.Context, cfg)
	if err != nil {
		return mapModelsRootError(err)
	}
	if scope.Close != nil {
		defer func() {
			_ = scope.Close(cfg.Context)
		}()
	}
	return service.invokeInScope(cfg, scope.Scope, modelName, operation, text)
}

func (service *rootService) invokeInScope(
	cfg InvokeConfig,
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	text string,
) error {
	catalogResult, err := service.models.GetCatalogModel(cfg.Context, modelinference.GetModelRequest{
		Scope: scope, Name: modelName, Operation: operation,
	})
	if err != nil {
		if !cfg.JSON && strings.TrimSpace(cfg.OutputPath) == "" && errors.Is(err, modelinference.ErrUnsupportedOperation) {
			return fmt.Errorf("--output is required unless --json is set")
		}
		return mapModelsRootError(err)
	}
	if err := validateCLIOutputShape(cfg, catalogResult.Model, operation); err != nil {
		return err
	}
	if cfg.JSON || len(cfg.OutputMappings) > 0 || genericCLIInlineOutput(cfg, catalogResult.Model, operation) {
		joinedResult, joinedErr := service.models.InvokeModel(cfg.Context, joinedCLIInvocationRequest(
			scope, modelName, operation, text, catalogResult.Model,
		))
		if joinedErr == nil {
			if len(cfg.OutputMappings) > 0 {
				return service.writeGenericCLIOutputMappings(cfg, joinedResult)
			}
			if genericCLIJSONResult(cfg, catalogResult.Model, operation, joinedResult) {
				return json.NewEncoder(cfg.Output).Encode(genericInvocationResponseFromInferenceResult(joinedResult))
			}
			if genericCLIInlineOutput(cfg, catalogResult.Model, operation) {
				return writeGenericCLIOutput(cfg.Output, joinedResult)
			}
			response := modelInvocationResponseFromInferenceResult(joinedResult, catalogResult.Model, text)
			return json.NewEncoder(cfg.Output).Encode(response)
		}
		if !errors.Is(joinedErr, modelinference.ErrUnsupportedOperation) &&
			!errors.Is(joinedErr, modelinference.ErrModelReferenceUnknown) {
			return mapModelsRootError(joinedErr)
		}
	}
	return service.invokePreparedLease(cfg, scope, modelName, operation, text, catalogResult.Model)
}

func validateCLIOutputShape(
	cfg InvokeConfig,
	catalog modelinference.Detail,
	operation string,
) error {
	selected, ok := catalogOperationForName(catalog, operation)
	if len(cfg.OutputMappings) > 0 {
		if strings.TrimSpace(cfg.OutputPath) != "" {
			return fmt.Errorf("--output cannot be combined with explicit output mappings")
		}
		return validateGenericCLIOutputMappings(cfg.OutputMappings, selected, ok)
	}
	if cfg.JSON {
		return nil
	}
	if ok && len(selected.Outputs) > 1 {
		return fmt.Errorf(
			"multiple model outputs require --json or explicit output mappings: %s",
			genericOutputSlotNames(selected.Outputs),
		)
	}
	if strings.TrimSpace(cfg.OutputPath) != "" {
		return nil
	}
	if !ok || len(selected.Outputs) != 1 || !genericCLIInlineModality(selected.Outputs[0].Modality) {
		return fmt.Errorf("--output is required unless --json is set")
	}
	return nil
}

func genericCLIInlineOutput(cfg InvokeConfig, catalog modelinference.Detail, operation string) bool {
	if strings.TrimSpace(cfg.OutputPath) != "" {
		return false
	}
	selected, ok := catalogOperationForName(catalog, operation)
	return ok && len(selected.Outputs) == 1 && genericCLIInlineModality(selected.Outputs[0].Modality)
}

func genericCLIInlineModality(modality modelinference.Modality) bool {
	return modality == modelinference.ModalityText || modality == modelinference.ModalityJSON
}

func genericOutputSlotNames(outputs []modelinference.OperationSlot) string {
	names := make([]string, 0, len(outputs))
	for _, output := range outputs {
		name := strings.TrimSpace(output.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func genericCLIJSONResult(
	cfg InvokeConfig,
	catalog modelinference.Detail,
	operation string,
	result modelinference.InvokeModelResult,
) bool {
	if !cfg.JSON || len(result.Outputs) == 0 {
		return false
	}
	if len(result.Outputs) > 1 {
		return true
	}
	selected, ok := catalogOperationForName(catalog, operation)
	return ok && len(selected.Outputs) == 1 && genericCLIInlineModality(selected.Outputs[0].Modality)
}

func writeGenericCLIOutput(output io.Writer, result modelinference.InvokeModelResult) error {
	if len(result.Outputs) != 1 {
		return fmt.Errorf("multiple model outputs require --json or explicit output mappings")
	}
	value := result.Outputs[0].Content
	if value == "" {
		return fmt.Errorf("model invocation returned no inline output")
	}
	_, err := output.Write([]byte(value))
	return err
}

type genericCLIOutputMapping struct {
	slot string
	path string
}

func parseGenericCLIOutputMappings(values []string) ([]genericCLIOutputMapping, error) {
	mappings := make([]genericCLIOutputMapping, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	paths := make(map[string]string, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid output mapping %q: expected slot=path", value)
		}
		slot := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		if slot == "" || path == "" {
			return nil, fmt.Errorf("invalid output mapping %q: slot and path are required", value)
		}
		if path == "-" {
			return nil, fmt.Errorf("invalid output mapping for slot %q: path '-' is not supported", slot)
		}
		if _, exists := seen[slot]; exists {
			return nil, fmt.Errorf("duplicate output mapping for slot %q", slot)
		}
		canonicalPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve output mapping for slot %q: %w", slot, err)
		}
		if priorSlot, exists := paths[canonicalPath]; exists {
			return nil, fmt.Errorf("output mappings for slots %q and %q use the same path", priorSlot, slot)
		}
		seen[slot] = struct{}{}
		paths[canonicalPath] = slot
		mappings = append(mappings, genericCLIOutputMapping{slot: slot, path: path})
	}
	return mappings, nil
}

func validateGenericCLIOutputMappings(
	values []string,
	operation modelinference.Operation,
	found bool,
) error {
	mappings, err := parseGenericCLIOutputMappings(values)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("cannot map outputs for unknown operation")
	}
	if len(mappings) != len(operation.Outputs) {
		return fmt.Errorf(
			"explicit output mappings must cover every output slot: %s",
			genericOutputSlotNames(operation.Outputs),
		)
	}
	declared := make(map[string]struct{}, len(operation.Outputs))
	for _, output := range operation.Outputs {
		declared[strings.TrimSpace(output.Name)] = struct{}{}
	}
	for _, mapping := range mappings {
		if _, exists := declared[mapping.slot]; !exists {
			return fmt.Errorf("output mapping names unknown slot %q; valid slots: %s", mapping.slot, genericOutputSlotNames(operation.Outputs))
		}
	}
	return nil
}

func (service *rootService) writeGenericCLIOutputMappings(cfg InvokeConfig, result modelinference.InvokeModelResult) error {
	if service.outputFileSystem == nil {
		return fmt.Errorf("Models CLI output filesystem is required for explicit output mappings")
	}
	mappings, err := parseGenericCLIOutputMappings(cfg.OutputMappings)
	if err != nil {
		return err
	}
	bySlot := make(map[string]genericCLIOutputMapping, len(mappings))
	for _, mapping := range mappings {
		bySlot[mapping.slot] = mapping
	}
	if len(result.Outputs) != len(mappings) {
		return fmt.Errorf("model invocation returned %d outputs for %d explicit output mappings", len(result.Outputs), len(mappings))
	}
	type stagedOutput struct {
		targetPath string
		temporary  string
	}
	staged := make([]stagedOutput, 0, len(result.Outputs))
	defer func() {
		for _, output := range staged {
			if output.temporary != "" {
				_ = service.outputFileSystem.Remove(output.temporary)
			}
		}
	}()
	for _, output := range result.Outputs {
		mapping, ok := bySlot[output.Name]
		if !ok {
			return fmt.Errorf("model invocation returned unmapped output slot %q", output.Name)
		}
		if output.Content == "" {
			return fmt.Errorf("output slot %q has no inline bytes for mapped publication", output.Name)
		}
		temporary, err := stageGenericCLIOutputFile(cfg.Context, service.outputFileSystem, mapping.path, []byte(output.Content))
		if err != nil {
			return fmt.Errorf("write mapped output %q: %w", output.Name, err)
		}
		staged = append(staged, stagedOutput{targetPath: mapping.path, temporary: temporary})
	}
	if err := cfg.Context.Err(); err != nil {
		return err
	}
	for index, output := range staged {
		if err := service.outputFileSystem.Rename(output.temporary, output.targetPath); err != nil {
			return fmt.Errorf("publish mapped output %q: %w", result.Outputs[index].Name, err)
		}
		staged[index].temporary = ""
	}
	return json.NewEncoder(cfg.Output).Encode(genericInvocationResponseFromInferenceResult(result))
}

func stageGenericCLIOutputFile(ctx context.Context, fileSystem OutputFileSystem, path string, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	directory := filepath.Dir(path)
	temporary, err := fileSystem.CreateTemp(directory, ".you-model-output-*")
	if err != nil {
		return "", err
	}
	if temporary == nil {
		return "", fmt.Errorf("create temporary output file returned no handle")
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = fileSystem.Remove(temporaryPath)
		}
	}()
	if written, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return "", err
	} else if written != len(data) {
		_ = temporary.Close()
		return "", io.ErrShortWrite
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	removeTemporary = false
	return temporaryPath, nil
}

func (service *rootService) invokePreparedLease(
	cfg InvokeConfig,
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	text string,
	catalog modelinference.Detail,
) error {
	if runtime := catalog.ManagedRuntime; strings.TrimSpace(runtime.Identity) != "" {
		if err := runtime.InvocationError(); err != nil {
			return mapModelsRootError(err)
		}
	}
	leaseResult, err := service.models.AcquireModelLease(cfg.Context, modelinference.AcquireModelLeaseRequest{
		Scope: scope, Name: modelName, Holder: modelsCLIInvokeHolder,
	})
	if err != nil {
		return mapModelsRootError(err)
	}
	request := modelinference.InvokeModelRequest{
		Scope:     scope,
		Lease:     leaseResult.Lease.Lease,
		Holder:    modelsCLIInvokeHolder,
		ModelName: modelName,
		Operation: operation,
		Input: modelinference.InferenceInput{
			ContentType: "text/plain",
			Content:     text,
		},
	}
	if !cfg.JSON {
		mode := modelinference.ResponseModeAudioStream
		request.ResponseMode = mode
	}
	result, err := service.models.InvokeModelWithLease(cfg.Context, request)
	if err != nil {
		return mapModelsRootError(err)
	}
	if cfg.JSON {
		response := modelInvocationResponseFromInferenceResult(result, catalog, text)
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	outputPath := strings.TrimSpace(cfg.OutputPath)
	streamFile, err := inferenceArtifactSourcePath(result)
	if err != nil {
		return mapModelsRootError(err)
	}
	if service.artifacts == nil {
		return fmt.Errorf("model invocation artifact exporter is required")
	}
	if err := service.artifacts.ExportInvocationArtifact(streamFile, outputPath); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cfg.Output, "Wrote audio: %s\n", outputPath)
	return err
}

func joinedCLIInvocationRequest(
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	text string,
	catalog modelinference.Detail,
) modelinference.InvokeModelRequest {
	inputName := "input"
	modality := modelinference.ModalityText
	contentType := "text/plain"
	if selected, ok := catalogOperationForName(catalog, operation); ok && len(selected.Inputs) > 0 {
		inputName = selected.Inputs[0].Name
		if selected.Inputs[0].Modality != "" {
			modality = selected.Inputs[0].Modality
		}
		if len(selected.Inputs[0].MediaTypes) > 0 {
			contentType = selected.Inputs[0].MediaTypes[0]
		}
	}
	return modelinference.InvokeModelRequest{
		Scope: scope, Holder: modelsCLIInvokeHolder,
		Model: modelinference.ModelReference{NameOrURI: modelName}, Operation: operation,
		Inputs: []modelinference.InferenceInput{{
			Name: inputName, Modality: modality, ContentType: contentType, Content: text,
		}},
	}
}

func modelInvocationResponseFromInferenceResult(
	result modelinference.InvokeModelResult,
	catalog modelinference.Detail,
	inputText string,
) factoryapi.ModelInvocationResponse {
	worker, locality := catalogPresentationForOperation(catalog, result.Operation)
	bindings := resolvedPresentationBindings(catalog, result.Operation, inputText)
	content := contentcontract.GeneratedPtrFromParts(inferenceContentToWorkParts(result.Content))
	return factoryapi.ModelInvocationResponse{
		ModelName:        result.ModelName,
		Worker:           worker,
		Operation:        result.Operation,
		ProviderLocality: factoryapi.WorkerModelLocality(locality),
		Content:          derefGeneratedWorkContent(content),
		Bindings:         generatedResolvedModelInvocationBindings(bindings),
	}
}

func genericInvocationResponseFromInferenceResult(
	result modelinference.InvokeModelResult,
) factoryapi.GenericModelInvocationResponse {
	outputs := make([]factoryapi.ModelInvocationOutput, len(result.Outputs))
	for index, output := range result.Outputs {
		projected := factoryapi.ModelInvocationOutput{
			Name:     output.Name,
			Modality: factoryapi.ModelInvocationContentType(output.Modality),
		}
		projected.ContentType = genericCLIStringPointer(output.ContentType)
		projected.MediaType = genericCLIStringPointer(output.MediaType)
		projected.Content = genericCLIStringPointer(output.Content)
		if output.Artifact != nil && !output.Artifact.Artifact.IsZero() {
			artifact := factoryapi.ModelInvocationArtifact{ArtifactRef: output.Artifact.Artifact.String()}
			artifact.Name = genericCLIStringPointer(output.Artifact.Name)
			artifact.MediaType = genericCLIStringPointer(output.Artifact.MediaType)
			if output.Artifact.SizeBytes >= 0 {
				size := output.Artifact.SizeBytes
				artifact.SizeBytes = &size
			}
			if len(output.Artifact.Properties) > 0 {
				properties := make(factoryapi.StringMap, len(output.Artifact.Properties))
				for key, value := range output.Artifact.Properties {
					properties[key] = value
				}
				artifact.Properties = &properties
			}
			projected.Artifact = &artifact
		}
		outputs[index] = projected
	}
	return factoryapi.GenericModelInvocationResponse{Outputs: outputs}
}

func genericCLIStringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	copy := value
	return &copy
}

func catalogPresentationForOperation(catalog modelinference.Detail, operation string) (string, string) {
	for _, capability := range catalog.Capabilities {
		for _, catalogOperation := range capability.Operations {
			if catalogOperation.Name == operation {
				return capability.Worker, string(capability.ProviderLocality)
			}
		}
	}
	return "", string(catalog.ProviderLocality)
}

func resolvedPresentationBindings(
	catalog modelinference.Detail,
	operation string,
	inputText string,
) []modelinference.ResolvedModelOperationBinding {
	operationDetail, ok := catalogOperationForName(catalog, operation)
	if !ok {
		return []modelinference.ResolvedModelOperationBinding{}
	}
	for _, input := range operationDetail.Inputs {
		slot := strings.TrimSpace(input.Name)
		if slot == "" {
			continue
		}
		return []modelinference.ResolvedModelOperationBinding{{
			Slot:   slot,
			Source: "INPUT",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: inputText,
			}},
		}}
	}
	return []modelinference.ResolvedModelOperationBinding{}
}

func catalogOperationForName(catalog modelinference.Detail, operation string) (modelinference.Operation, bool) {
	for _, catalogOperation := range catalog.Operations {
		if catalogOperation.Name == operation {
			return catalogOperation, true
		}
	}
	for _, capability := range catalog.Capabilities {
		for _, catalogOperation := range capability.Operations {
			if catalogOperation.Name == operation {
				return catalogOperation, true
			}
		}
	}
	return modelinference.Operation{}, false
}

func inferenceContentToWorkParts(content []modelinference.InferenceContent) []work.WorkContentPart {
	if len(content) == 0 {
		return nil
	}
	parts := make([]work.WorkContentPart, 0, len(content))
	for _, item := range content {
		parts = append(parts, inferenceContentToWorkPart(item))
	}
	return parts
}

func inferenceContentToWorkPart(item modelinference.InferenceContent) work.WorkContentPart {
	contentType := strings.TrimSpace(item.ContentType)
	value := strings.TrimSpace(item.Content)
	switch {
	case strings.HasPrefix(strings.ToLower(contentType), "audio/"):
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeAudio,
			File:        value,
			ContentType: contentType,
			Slot:        "audio",
		}
	case strings.HasPrefix(strings.ToLower(contentType), "image/"):
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeImage,
			URL:         value,
			ContentType: contentType,
			Slot:        "image",
		}
	case strings.EqualFold(contentType, "application/json"):
		return work.WorkContentPart{
			Type: work.WorkContentPartTypeJSON,
			JSON: json.RawMessage(value),
			Slot: "json",
		}
	default:
		if contentType == "" {
			contentType = "text/plain"
		}
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeText,
			Text:        value,
			ContentType: contentType,
			Slot:        "text",
		}
	}
}

func inferenceArtifactSourcePath(result modelinference.InvokeModelResult) (string, error) {
	for _, artifact := range result.Artifacts {
		if path := strings.TrimSpace(artifact.Artifact.String()); path != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("models invoke returned no streamed audio output")
}
