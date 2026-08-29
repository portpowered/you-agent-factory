package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
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
	if operation == "" && len(cfg.InputMappings) > 0 {
		operation = inferGenericCLIModelOperation(modelName)
	}
	if operation == "" {
		return fmt.Errorf("--operation is required")
	}
	text := strings.TrimSpace(cfg.Text)
	if text == "" && len(cfg.InputMappings) == 0 && len(cfg.InputSpecs) == 0 {
		return fmt.Errorf("--text is required")
	}
	if text != "" && (len(cfg.InputMappings) > 0 || len(cfg.InputSpecs) > 0) {
		return clidiag.NewFlagConflictFailure(
			"--text", "--input", fmt.Errorf("choose one input form for model invocation"),
		)
	}
	if strings.TrimSpace(cfg.Server) != "" {
		return fmt.Errorf("remote models invoke requires the composition-stable HTTP service")
	}
	if service.openInvokeScope == nil {
		return fmt.Errorf("models invoke runtime scope opener is required")
	}
	scope, err := service.openInvokeScope(cfg.Context, cfg)
	if err != nil {
		return mapModelsClientError(err)
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
	catalog, err := service.catalogForInvoke(cfg, scope, modelName, operation)
	if err != nil {
		return err
	}
	// Catalog identity is static; readiness is an observed runtime fact. Use
	// the current scoped projection before the invoke preflight so a local
	// cache is not reported as missing merely because the catalog detail was
	// assembled without filesystem observations. Keep the unsupported fallback
	// for lightweight embedded Models roots that predate this capability.
	catalog, err = service.refreshInvokeReadiness(cfg, scope, modelName, operation, catalog)
	if err != nil {
		return err
	}
	if err := validateCLIOutputShape(cfg, catalog, operation); err != nil {
		return err
	}
	if validationOnlyModelInvoke(cfg) {
		return writeValidationOnlyModelInvokeResponse(cfg.Output, modelName, operation)
	}
	if !shouldUseGenericCLIInvocation(cfg, catalog, operation) {
		return service.invokePreparedLease(cfg, scope, modelName, operation, text, catalog)
	}
	handled, err := service.invokeGenericInScope(cfg, scope, modelName, operation, text, catalog)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	return service.invokePreparedLease(cfg, scope, modelName, operation, text, catalog)
}

func (service *rootService) catalogForInvoke(
	cfg InvokeConfig,
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
) (modelinference.Detail, error) {
	result, err := service.models.GetCatalogModel(cfg.Context, modelinference.GetModelRequest{
		Scope: scope, Name: modelName, Operation: operation,
	})
	if err == nil {
		return result.Model, nil
	}
	if !cfg.JSON && strings.TrimSpace(cfg.OutputPath) == "" && errors.Is(err, modelinference.ErrUnsupportedOperation) {
		return modelinference.Detail{}, fmt.Errorf("--output is required unless --json is set")
	}
	return modelinference.Detail{}, mapModelsClientError(err)
}

func (service *rootService) refreshInvokeReadiness(
	cfg InvokeConfig,
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	catalog modelinference.Detail,
) (modelinference.Detail, error) {
	readiness, err := service.models.GetModelReadiness(cfg.Context, modelinference.GetModelReadinessRequest{
		Scope: scope, Name: modelName, Operation: operation,
	})
	if err == nil {
		catalog.ManagedRuntime = readiness.Readiness.Clone()
		return catalog, nil
	}
	if errors.Is(err, modelinference.ErrUnsupportedOperation) {
		return catalog, nil
	}
	return modelinference.Detail{}, mapModelsClientError(err)
}

func shouldUseGenericCLIInvocation(cfg InvokeConfig, catalog modelinference.Detail, operation string) bool {
	if cfg.JSON || len(cfg.InputMappings) > 0 || len(cfg.InputSpecs) > 0 || len(cfg.ParameterSpecs) > 0 || len(cfg.OutputMappings) > 0 {
		return true
	}
	if isDirectTTSAlias(cfg) && strings.TrimSpace(cfg.OutputPath) != "" {
		return true
	}
	if strings.TrimSpace(cfg.OutputPath) != "" && genericCLIOutputPathOperation(catalog, operation) {
		return true
	}
	return genericCLIInlineOutput(cfg, catalog, operation)
}

func genericCLIOutputPathOperation(catalog modelinference.Detail, operation string) bool {
	selected, ok := catalogOperationForName(catalog, operation)
	if !ok || len(selected.Outputs) == 0 {
		return false
	}
	for _, output := range selected.Outputs {
		if output.Modality != modelinference.ModalityAudio {
			return true
		}
	}
	return false
}

func (service *rootService) invokeGenericInScope(
	cfg InvokeConfig,
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	text string,
	catalog modelinference.Detail,
) (bool, error) {
	inputs, err := service.prepareGenericCLIInputs(cfg, operation, catalog)
	if err != nil {
		return true, err
	}
	if len(inputs) == 0 && len(cfg.ParameterSpecs) == 0 && !cfg.JSON &&
		len(cfg.OutputMappings) == 0 && strings.TrimSpace(cfg.OutputPath) == "" &&
		!genericCLIStdoutOutput(cfg, catalog, operation) {
		return false, nil
	}
	parameters, err := parseGenericCLIParameterSpecs(cfg.ParameterSpecs)
	if err != nil {
		return true, err
	}
	if err := service.emitAssetEstimate(cfg.Context, scope, modelName, cfg.Diagnostics); err != nil {
		return true, mapModelsClientError(assetPreflightInvocationError(modelName, operation, err))
	}
	request := joinedCLIInvocationRequestFromInputs(scope, modelName, operation, text, inputs, parameters, catalog)
	result, err := service.models.InvokeModel(cfg.Context, request)
	if err == nil {
		return true, service.writeGenericCLIInvocationResult(cfg, result, catalog, operation, text)
	}
	if len(inputs) > 0 || len(parameters) > 0 {
		return true, mapModelsClientError(err)
	}
	if !genericCLIInvocationFallbackError(err) {
		return true, mapModelsClientError(err)
	}
	return false, nil
}

func genericCLIInvocationFallbackError(err error) bool {
	return errors.Is(err, modelinference.ErrUnsupportedOperation) ||
		errors.Is(err, modelinference.ErrModelReferenceUnknown)
}

func (service *rootService) writeGenericCLIInvocationResult(
	cfg InvokeConfig,
	result modelinference.InvokeModelResult,
	catalog modelinference.Detail,
	operation string,
	text string,
) error {
	if len(cfg.OutputMappings) > 0 {
		return service.writeGenericCLIOutputMappings(cfg, result)
	}
	if genericCLIJSONResult(cfg, catalog, operation, result) {
		return json.NewEncoder(cfg.Output).Encode(genericInvocationResponseFromInferenceResult(result))
	}
	if strings.TrimSpace(cfg.OutputPath) != "" && !cfg.JSON {
		return service.writeGenericCLIOutputPath(cfg, result)
	}
	if genericCLIInlineOutput(cfg, catalog, operation) {
		return writeGenericCLIOutputWithCatalog(cfg.Output, result, catalog, operation)
	}
	if genericCLIStdoutOutput(cfg, catalog, operation) {
		return writeGenericCLIOutputWithCatalog(cfg.Output, result, catalog, operation)
	}
	response := modelInvocationResponseFromInferenceResult(result, catalog, text)
	return json.NewEncoder(cfg.Output).Encode(response)
}

func validateCLIOutputShape(
	cfg InvokeConfig,
	catalog modelinference.Detail,
	operation string,
) error {
	selected, ok := catalogCLIOutputOperation(cfg, catalog, operation)
	if len(cfg.OutputMappings) > 0 {
		if strings.TrimSpace(cfg.OutputPath) != "" {
			return fmt.Errorf("--output cannot be combined with explicit output mappings")
		}
		return validateGenericCLIOutputMappings(cfg.OutputMappings, selected, ok)
	}
	if cfg.JSON {
		return nil
	}
	outputSlots := genericCLIOutputSlots(selected.Outputs)
	if ok && len(outputSlots) > 1 {
		return fmt.Errorf(
			"multiple model outputs require --json or explicit output mappings: %s",
			genericOutputSlotNames(outputSlots),
		)
	}
	if strings.TrimSpace(cfg.OutputPath) != "" {
		return nil
	}
	if !ok || len(outputSlots) != 1 || !genericCLIStdoutModality(outputSlots[0].Modality) {
		return fmt.Errorf("--output is required unless --json is set")
	}
	return nil
}

func genericCLIInlineOutput(cfg InvokeConfig, catalog modelinference.Detail, operation string) bool {
	if strings.TrimSpace(cfg.OutputPath) != "" {
		return false
	}
	selected, ok := catalogCLIOutputOperation(cfg, catalog, operation)
	outputSlots := genericCLIOutputSlots(selected.Outputs)
	return ok && len(outputSlots) == 1 && genericCLIInlineModality(outputSlots[0].Modality)
}

func genericCLIInlineModality(modality modelinference.Modality) bool {
	return modality == modelinference.ModalityText || modality == modelinference.ModalityJSON
}

func genericCLIStdoutOutput(cfg InvokeConfig, catalog modelinference.Detail, operation string) bool {
	if cfg.JSON || strings.TrimSpace(cfg.OutputPath) != "" {
		return false
	}
	selected, ok := catalogCLIOutputOperation(cfg, catalog, operation)
	outputSlots := genericCLIOutputSlots(selected.Outputs)
	return ok && len(outputSlots) == 1 && genericCLIStdoutModality(outputSlots[0].Modality)
}

func genericCLIOutputSlots(outputs []modelinference.OperationSlot) []modelinference.OperationSlot {
	if len(outputs) <= 1 {
		return append([]modelinference.OperationSlot(nil), outputs...)
	}
	required := make([]modelinference.OperationSlot, 0, len(outputs))
	for _, output := range outputs {
		if output.Required == nil || *output.Required {
			required = append(required, output)
		}
	}
	return required
}

func catalogCLIOutputOperation(
	cfg InvokeConfig,
	catalog modelinference.Detail,
	operation string,
) (modelinference.Operation, bool) {
	if len(cfg.InputMappings) == 0 && strings.TrimSpace(cfg.Text) != "" {
		if authored, authoredOK := catalogCapabilityOperationForName(catalog, operation); authoredOK {
			return authored, true
		}
	}
	return catalogOperationForName(catalog, operation)
}

func genericCLIStdoutModality(modality modelinference.Modality) bool {
	return genericCLIInlineModality(modality) || modality == modelinference.ModalityAudio
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
	if len(cfg.InputMappings) > 0 || len(cfg.InputSpecs) > 0 || len(cfg.ParameterSpecs) > 0 || len(cfg.OutputMappings) > 0 {
		return true
	}
	if len(result.Outputs) > 1 {
		return true
	}
	selected, ok := catalogOperationForName(catalog, operation)
	return ok && len(selected.Outputs) == 1 && genericCLIInlineModality(selected.Outputs[0].Modality)
}

func writeGenericCLIOutputWithCatalog(
	output io.Writer,
	result modelinference.InvokeModelResult,
	catalog modelinference.Detail,
	operation string,
) error {
	var inline *modelinference.InferenceOutput
	if len(result.Outputs) == 1 {
		inline = &result.Outputs[0]
	} else if selected, ok := catalogOperationForName(catalog, operation); ok {
		for _, slot := range selected.Outputs {
			if slot.Required != nil && !*slot.Required || !genericCLIInlineModality(slot.Modality) {
				continue
			}
			for index := range result.Outputs {
				if result.Outputs[index].Name != slot.Name {
					continue
				}
				if inline != nil {
					return fmt.Errorf("multiple model outputs require --json or explicit output mappings")
				}
				inline = &result.Outputs[index]
			}
		}
	}
	if inline == nil {
		return fmt.Errorf("multiple model outputs require --json or explicit output mappings")
	}
	value := inline.Content
	if value == "" {
		return fmt.Errorf("model invocation returned no inline output")
	}
	_, err := output.Write([]byte(value))
	return err
}

func (service *rootService) writeGenericCLIOutputPath(
	cfg InvokeConfig,
	result modelinference.InvokeModelResult,
) error {
	if service.outputFileSystem == nil {
		return fmt.Errorf("Models CLI output filesystem is required for --output")
	}
	if len(result.Outputs) != 1 {
		return fmt.Errorf("multiple model outputs require --json or explicit output mappings")
	}
	output := result.Outputs[0]
	if strings.TrimSpace(output.Name) == "" {
		return fmt.Errorf("model invocation returned an unnamed output")
	}
	if output.Content == "" {
		return fmt.Errorf("output slot %q has no inline bytes for publication", output.Name)
	}
	mapping := genericCLIOutputMapping{slot: output.Name, path: strings.TrimSpace(cfg.OutputPath)}
	bySlot := map[string]genericCLIOutputMapping{mapping.slot: mapping}
	mappings := []genericCLIOutputMapping{mapping}
	staged := make([]genericCLIOutputStage, 0, 1)
	backups := make([]genericCLIOutputBackup, 0, 1)
	published := 0
	committed := false
	defer func() {
		if committed {
			removeGenericCLIOutputBackups(service.outputFileSystem, backups)
		} else {
			rollbackGenericCLIOutputPublication(service.outputFileSystem, staged, backups, published)
		}
		for _, stagedOutput := range staged {
			if stagedOutput.temporary != "" {
				_ = service.outputFileSystem.Remove(stagedOutput.temporary)
			}
		}
	}()

	var err error
	staged, err = stageGenericCLIOutputs(cfg.Context, service.outputFileSystem, result, bySlot)
	if err != nil {
		return err
	}
	if err := cfg.Context.Err(); err != nil {
		return err
	}
	backups, err = backupGenericCLIOutputTargets(cfg.Context, service.outputFileSystem, mappings)
	if err != nil {
		return err
	}
	published, err = publishGenericCLIOutputs(cfg.Context, service.outputFileSystem, result, staged)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cfg.Output, "Wrote audio: %s\n", mapping.path); err != nil {
		return err
	}
	committed = true
	return nil
}

type genericCLIOutputMapping struct {
	slot string
	path string
}

type genericCLIOutputStage struct {
	targetPath string
	temporary  string
}

type genericCLIOutputBackup struct {
	targetPath string
	backupPath string
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
	staged := make([]genericCLIOutputStage, 0, len(result.Outputs))
	backups := make([]genericCLIOutputBackup, 0, len(result.Outputs))
	published := 0
	committed := false
	defer func() {
		if committed {
			removeGenericCLIOutputBackups(service.outputFileSystem, backups)
		} else {
			rollbackGenericCLIOutputPublication(service.outputFileSystem, staged, backups, published)
		}
		for _, output := range staged {
			if output.temporary != "" {
				_ = service.outputFileSystem.Remove(output.temporary)
			}
		}
	}()
	staged, err = stageGenericCLIOutputs(cfg.Context, service.outputFileSystem, result, bySlot)
	if err != nil {
		return err
	}
	if err := cfg.Context.Err(); err != nil {
		return err
	}
	backups, err = backupGenericCLIOutputTargets(cfg.Context, service.outputFileSystem, mappings)
	if err != nil {
		return err
	}
	published, err = publishGenericCLIOutputs(cfg.Context, service.outputFileSystem, result, staged)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(cfg.Output).Encode(genericInvocationResponseFromInferenceResult(result)); err != nil {
		return err
	}
	committed = true
	return nil
}

func stageGenericCLIOutputs(
	ctx context.Context,
	fileSystem OutputFileSystem,
	result modelinference.InvokeModelResult,
	bySlot map[string]genericCLIOutputMapping,
) ([]genericCLIOutputStage, error) {
	staged := make([]genericCLIOutputStage, 0, len(result.Outputs))
	for _, output := range result.Outputs {
		mapping, ok := bySlot[output.Name]
		if !ok {
			return staged, fmt.Errorf("model invocation returned unmapped output slot %q", output.Name)
		}
		if output.Content == "" {
			return staged, fmt.Errorf("output slot %q has no inline bytes for mapped publication", output.Name)
		}
		temporary, err := stageGenericCLIOutputFile(ctx, fileSystem, mapping.path, []byte(output.Content))
		if err != nil {
			return staged, fmt.Errorf("write mapped output %q: %w", output.Name, err)
		}
		staged = append(staged, genericCLIOutputStage{targetPath: mapping.path, temporary: temporary})
	}
	return staged, nil
}

func publishGenericCLIOutputs(
	ctx context.Context,
	fileSystem OutputFileSystem,
	result modelinference.InvokeModelResult,
	staged []genericCLIOutputStage,
) (int, error) {
	published := 0
	for index, output := range staged {
		if err := ctx.Err(); err != nil {
			return published, err
		}
		if err := fileSystem.Rename(output.temporary, output.targetPath); err != nil {
			return published, fmt.Errorf("publish mapped output %q: %w", result.Outputs[index].Name, err)
		}
		staged[index].temporary = ""
		published++
	}
	return published, nil
}

func backupGenericCLIOutputTargets(
	ctx context.Context,
	fileSystem OutputFileSystem,
	mappings []genericCLIOutputMapping,
) ([]genericCLIOutputBackup, error) {
	backups := make([]genericCLIOutputBackup, 0, len(mappings))
	for _, mapping := range mappings {
		if err := ctx.Err(); err != nil {
			return backups, err
		}
		_, err := fileSystem.Inspect(mapping.path)
		switch {
		case err == nil:
			backupPath, backupErr := reserveGenericCLIOutputPath(ctx, fileSystem, mapping.path)
			if backupErr != nil {
				return backups, fmt.Errorf("prepare mapped output %q backup: %w", mapping.slot, backupErr)
			}
			if backupErr = fileSystem.Rename(mapping.path, backupPath); backupErr != nil {
				_ = fileSystem.Remove(backupPath)
				return backups, fmt.Errorf("backup mapped output %q: %w", mapping.slot, backupErr)
			}
			backups = append(backups, genericCLIOutputBackup{targetPath: mapping.path, backupPath: backupPath})
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			return backups, fmt.Errorf("inspect mapped output %q: %w", mapping.slot, err)
		}
	}
	return backups, nil
}

func reserveGenericCLIOutputPath(
	ctx context.Context,
	fileSystem OutputFileSystem,
	targetPath string,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	temporary, err := fileSystem.CreateTemp(filepath.Dir(targetPath), ".you-model-output-backup-*")
	if err != nil {
		return "", err
	}
	if temporary == nil {
		return "", fmt.Errorf("create backup temporary file returned no named handle")
	}
	backupPath := temporary.Name()
	if strings.TrimSpace(backupPath) == "" {
		_ = temporary.Close()
		return "", fmt.Errorf("create backup temporary file returned no named handle")
	}
	if err := temporary.Close(); err != nil {
		_ = fileSystem.Remove(backupPath)
		return "", err
	}
	if err := fileSystem.Remove(backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

func rollbackGenericCLIOutputPublication(
	fileSystem OutputFileSystem,
	staged []genericCLIOutputStage,
	backups []genericCLIOutputBackup,
	published int,
) {
	for index := published - 1; index >= 0; index-- {
		_ = fileSystem.Remove(staged[index].targetPath)
	}
	for index := len(backups) - 1; index >= 0; index-- {
		_ = fileSystem.Rename(backups[index].backupPath, backups[index].targetPath)
	}
}

func removeGenericCLIOutputBackups(fileSystem OutputFileSystem, backups []genericCLIOutputBackup) {
	for _, backup := range backups {
		_ = fileSystem.Remove(backup.backupPath)
	}
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
			return mapModelsClientError(err)
		}
	}
	leaseResult, err := service.models.AcquireModelLease(cfg.Context, modelinference.AcquireModelLeaseRequest{
		Scope: scope, Name: modelName, Holder: modelsCLIInvokeHolder,
	})
	if err != nil {
		return mapModelsClientError(err)
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
	if strings.TrimSpace(cfg.OutputPath) != "" {
		mode := modelinference.ResponseModeAudioStream
		request.ResponseMode = mode
	}
	result, err := service.models.InvokeModelWithLease(cfg.Context, request)
	if err != nil {
		return mapModelsClientError(err)
	}
	if strings.TrimSpace(cfg.OutputPath) == "" {
		response := modelInvocationResponseFromInferenceResult(result, catalog, text)
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	outputPath := strings.TrimSpace(cfg.OutputPath)
	streamFile, err := inferenceArtifactSourcePath(result)
	if err != nil {
		return mapModelsClientError(err)
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

func joinedCLIInvocationRequestFromInputs(
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	text string,
	inputs []modelinference.InferenceInput,
	parameters []modelinference.OperationParameter,
	catalog modelinference.Detail,
) modelinference.InvokeModelRequest {
	if len(inputs) > 0 || len(parameters) > 0 {
		return modelinference.InvokeModelRequest{
			Scope: scope, Holder: modelsCLIInvokeHolder,
			Model: modelinference.ModelReference{NameOrURI: modelName}, Operation: operation,
			Inputs: inputs, Parameters: parameters,
		}
	}

	inputName := "input"
	modality := modelinference.ModalityText
	contentType := "text/plain"
	if selected, ok := catalogOperationForName(catalog, operation); ok {
		// Catalog projections sort slots by name; bind --text to the required
		// text slot instead of assuming the first slot is the CLI input.
		input := joinedCLITextInput(selected.Inputs)
		if input == nil && len(selected.Inputs) > 0 {
			input = &selected.Inputs[0]
		}
		if input != nil {
			inputName = input.Name
			if input.Modality != "" {
				modality = input.Modality
			}
			if len(input.MediaTypes) > 0 {
				contentType = input.MediaTypes[0]
			}
		}
	}
	return modelinference.InvokeModelRequest{
		Scope: scope, Holder: modelsCLIInvokeHolder,
		Model: modelinference.ModelReference{NameOrURI: modelName}, Operation: operation,
		Inputs: []modelinference.InferenceInput{{
			Name: inputName, Modality: modality, ContentType: contentType, MediaType: contentType, Content: text,
		}},
	}
}

func joinedCLIInvocationRequest(
	scope modelinference.RuntimeScopeRef,
	modelName string,
	operation string,
	text string,
	inputSpecs []string,
	parameterSpecs []string,
	catalog modelinference.Detail,
) (modelinference.InvokeModelRequest, error) {
	inputs, err := parseGenericCLIInputSpecs(inputSpecs)
	if err != nil {
		return modelinference.InvokeModelRequest{}, err
	}
	parameters, err := parseGenericCLIParameterSpecs(parameterSpecs)
	if err != nil {
		return modelinference.InvokeModelRequest{}, err
	}
	return joinedCLIInvocationRequestFromInputs(scope, modelName, operation, text, inputs, parameters, catalog), nil
}

type genericCLIInputSpec struct {
	Name        string `json:"name"`
	Modality    string `json:"modality"`
	ContentType string `json:"contentType"`
	MediaType   string `json:"mediaType"`
	Content     string `json:"content"`
}

func parseGenericCLIInputSpecs(values []string) ([]modelinference.InferenceInput, error) {
	if len(values) == 0 {
		return nil, nil
	}
	inputs := make([]modelinference.InferenceInput, 0, len(values))
	for index, value := range values {
		var spec genericCLIInputSpec
		if err := json.Unmarshal([]byte(value), &spec); err != nil {
			return nil, fmt.Errorf("parse --input %d: %w", index+1, err)
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			return nil, fmt.Errorf("parse --input %d: name is required", index+1)
		}
		modality := modelinference.Modality(strings.ToUpper(strings.TrimSpace(spec.Modality)))
		if modality == "" {
			return nil, fmt.Errorf("parse --input %d (%s): modality is required", index+1, name)
		}
		if strings.TrimSpace(spec.Content) == "" {
			return nil, fmt.Errorf("parse --input %d (%s): content is required", index+1, name)
		}
		inputs = append(inputs, modelinference.InferenceInput{
			Name: name, Modality: modality,
			ContentType: strings.TrimSpace(spec.ContentType),
			MediaType:   strings.TrimSpace(spec.MediaType), Content: spec.Content,
		})
	}
	return inputs, nil
}

type genericCLIParameterSpec struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

func parseGenericCLIParameterSpecs(values []string) ([]modelinference.OperationParameter, error) {
	if len(values) == 0 {
		return nil, nil
	}
	parameters := make([]modelinference.OperationParameter, 0, len(values))
	for index, value := range values {
		var spec genericCLIParameterSpec
		if err := json.Unmarshal([]byte(value), &spec); err != nil {
			return nil, fmt.Errorf("parse --parameter %d: %w", index+1, err)
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			return nil, fmt.Errorf("parse --parameter %d: name is required", index+1)
		}
		if len(spec.Value) == 0 {
			return nil, fmt.Errorf("parse --parameter %d (%s): value is required", index+1, name)
		}
		var parameterValue any
		if err := json.Unmarshal(spec.Value, &parameterValue); err != nil {
			return nil, fmt.Errorf("parse --parameter %d (%s) value: %w", index+1, name, err)
		}
		parameters = append(parameters, modelinference.OperationParameter{Name: name, Value: parameterValue})
	}
	return parameters, nil
}

func joinedCLITextInput(inputs []modelinference.OperationSlot) *modelinference.OperationSlot {
	var optionalText *modelinference.OperationSlot
	for index := range inputs {
		input := &inputs[index]
		if input.Modality != modelinference.ModalityText {
			continue
		}
		if input.Required != nil && *input.Required {
			return input
		}
		if optionalText == nil {
			optionalText = input
		}
	}
	return optionalText
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
