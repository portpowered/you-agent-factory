package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

const genericCLIInputMaxFileBytes int64 = 8 * 1024 * 1024

// CompositionModelsRoot exposes the accepted Models root from injected
// composition collaborators when the invocation operation can supply it.
type CompositionModelsRoot interface {
	CompositionModelsRoot() modelinference.Service
}

// CompositionOpenCatalogScope exposes process-scoped catalog opening from
// injected composition collaborators for local list/inspect/pull behavior.
type CompositionOpenCatalogScope interface {
	CompositionOpenCatalogScope(context.Context) (InvokeRuntimeScope, error)
}

// CompositionInvokeScopeOpener exposes invoke-scope opening from injected
// composition collaborators when the invocation operation can supply it.
type CompositionInvokeScopeOpener interface {
	CompositionOpenInvokeScope(context.Context, InvokeConfig) (InvokeRuntimeScope, error)
}

// CompositionScopeProvider is the Models transport's explicit composition
// port. Wire supplies the Models root and scope operations; the transport does
// not discover a collaborator through a Sessions operation value.
type CompositionScopeProvider interface {
	CompositionModelsRoot
	CompositionOpenCatalogScope
	CompositionInvokeScopeOpener
}

// BindService returns the composition-facing Models CLI adapter Service
// constructed from the accepted Models root. Wire and other composition roots
// inject the returned Service without constructing adapter behavior at the
// composition boundary.
func BindService(cfg Config) Service {
	return NewService(cfg)
}

// ConfigFromComposition maps composition-stable collaborator shapes onto the
// owned adapter Config without requiring composition roots to construct the
// Service directly.
func ConfigFromComposition(
	httpProtocol clihttp.Protocol,
	invocation InvocationOperation,
	providers ...CompositionScopeProvider,
) Config {
	cfg := Config{HTTP: httpProtocol}
	if invocation == nil {
		return cfg
	}
	cfg.Artifacts = compositionArtifactExporter{invocation: invocation}
	var provider CompositionScopeProvider
	if len(providers) > 0 {
		provider = providers[0]
	} else if candidate, ok := invocation.(CompositionScopeProvider); ok {
		// Keep direct transport composition usable for small embedded callers
		// that already provide the Models transport port. The canonical process
		// graph passes this port explicitly from Wire.
		provider = candidate
	}
	if provider != nil {
		cfg.Models = provider.CompositionModelsRoot()
		cfg.OpenCatalogScope = provider.CompositionOpenCatalogScope
		cfg.OpenInvokeScope = provider.CompositionOpenInvokeScope
		return cfg
	}
	// Preserve the independently injectable composition shapes used by
	// embedded callers. The process graph above supplies the aggregate port;
	// these fallbacks do not inspect or depend on Factory Sessions contracts.
	if root, ok := invocation.(CompositionModelsRoot); ok {
		cfg.Models = root.CompositionModelsRoot()
	}
	if opener, ok := invocation.(CompositionOpenCatalogScope); ok {
		cfg.OpenCatalogScope = opener.CompositionOpenCatalogScope
	}
	if opener, ok := invocation.(CompositionInvokeScopeOpener); ok {
		cfg.OpenInvokeScope = opener.CompositionOpenInvokeScope
	}
	return cfg
}

type compositionArtifactExporter struct {
	invocation InvocationOperation
}

func (exporter compositionArtifactExporter) ExportInvocationArtifact(sourcePath, destinationPath string) error {
	if exporter.invocation == nil {
		return nil
	}
	return exporter.invocation.ExportModelInvocationArtifact(sourcePath, destinationPath)
}

type genericCLIInputMapping struct {
	slot  string
	value string
}

func (service *rootService) prepareGenericCLIInputs(
	cfg InvokeConfig,
	operation string,
	catalog modelinference.Detail,
) ([]modelinference.InferenceInput, error) {
	return prepareGenericCLIInputsWithReader(cfg, operation, catalog, service.inputFileReader)
}

func prepareGenericCLIInputsWithReader(
	cfg InvokeConfig,
	operation string,
	catalog modelinference.Detail,
	inputFileReader InputFileReader,
) ([]modelinference.InferenceInput, error) {
	rawValues := append([]string(nil), cfg.InputMappings...)
	rawValues = append(rawValues, cfg.InputSpecs...)
	if len(rawValues) == 0 {
		return nil, nil
	}
	selected, ok := catalogOperationForName(catalog, operation)
	if !ok {
		return nil, genericCLIInputFailure(
			modelinference.InvocationFailureClassInvalidOperation,
			fmt.Sprintf("unknown operation %q", operation), operation, nil,
		)
	}
	mappingValues, specValues := splitGenericCLIInputValues(rawValues)
	var inputs []modelinference.InferenceInput
	if len(mappingValues) > 0 {
		mappings, err := parseGenericCLIInputMappings(mappingValues)
		if err != nil {
			return nil, err
		}
		slots, validNames := genericCLIInputSlots(selected.Inputs)
		counts, err := validateGenericCLIInputMappings(mappings, slots, validNames)
		if err != nil {
			return nil, err
		}
		if err := validateMissingGenericCLIInputSlots(selected.Inputs, counts, validNames); err != nil {
			return nil, err
		}
		inputs, err = bindGenericCLIInputsWithReader(cfg, mappings, slots, inputFileReader)
		if err != nil {
			return nil, err
		}
	}
	if len(specValues) > 0 {
		specInputs, err := parseGenericCLIInputSpecs(specValues)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, specInputs...)
	}
	return inputs, nil
}

func splitGenericCLIInputValues(values []string) (mappings, specs []string) {
	for _, value := range values {
		if strings.HasPrefix(strings.TrimSpace(value), "{") {
			specs = append(specs, value)
			continue
		}
		mappings = append(mappings, value)
	}
	return mappings, specs
}

func genericCLIInputSlots(inputSlots []modelinference.OperationSlot) (map[string]modelinference.OperationSlot, []string) {
	slots := make(map[string]modelinference.OperationSlot, len(inputSlots))
	validNames := make([]string, 0, len(inputSlots))
	for _, slot := range inputSlots {
		name := strings.TrimSpace(slot.Name)
		if name == "" {
			continue
		}
		slots[name] = slot
		validNames = append(validNames, name)
	}
	sort.Strings(validNames)
	return slots, validNames
}

func validateGenericCLIInputMappings(
	mappings []genericCLIInputMapping,
	slots map[string]modelinference.OperationSlot,
	validNames []string,
) (map[string]int, error) {
	counts := make(map[string]int, len(mappings))
	for _, mapping := range mappings {
		slot, exists := slots[mapping.slot]
		if !exists {
			return nil, genericCLIInputFailure(
				modelinference.InvocationFailureClassInvalidSlot,
				fmt.Sprintf("unknown input slot %q; valid slots: %s", mapping.slot, strings.Join(validNames, ", ")),
				mapping.slot, validNames,
			)
		}
		counts[mapping.slot]++
		if !slot.Repeatable && counts[mapping.slot] > 1 {
			return nil, genericCLIInputFailure(
				modelinference.InvocationFailureClassSlotArity,
				fmt.Sprintf("input slot %q accepts at most one value", mapping.slot),
				mapping.slot, []string{"1"},
			)
		}
	}
	return counts, nil
}

func validateMissingGenericCLIInputSlots(
	slots []modelinference.OperationSlot,
	counts map[string]int,
	validNames []string,
) error {
	missing := make([]string, 0)
	for _, slot := range slots {
		if slot.Required == nil || !*slot.Required || counts[strings.TrimSpace(slot.Name)] != 0 {
			continue
		}
		missing = append(missing, strings.TrimSpace(slot.Name))
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return genericCLIInputFailure(
		modelinference.InvocationFailureClassInvalidSlot,
		"required input slot is missing: "+strings.Join(missing, ", "), missing[0], validNames,
	)
}

func (service *rootService) bindGenericCLIInputs(
	cfg InvokeConfig,
	mappings []genericCLIInputMapping,
	slots map[string]modelinference.OperationSlot,
) ([]modelinference.InferenceInput, error) {
	return bindGenericCLIInputsWithReader(cfg, mappings, slots, service.inputFileReader)
}

func bindGenericCLIInputsWithReader(
	cfg InvokeConfig,
	mappings []genericCLIInputMapping,
	slots map[string]modelinference.OperationSlot,
	inputFileReader InputFileReader,
) ([]modelinference.InferenceInput, error) {
	inputs := make([]modelinference.InferenceInput, 0, len(mappings))
	for _, mapping := range mappings {
		if err := cfg.Context.Err(); err != nil {
			return nil, err
		}
		input, err := genericCLIInputWithReader(cfg, mapping, slots[mapping.slot], inputFileReader)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func parseGenericCLIInputMappings(values []string) ([]genericCLIInputMapping, error) {
	mappings := make([]genericCLIInputMapping, 0, len(values))
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 {
			return nil, genericCLIInputFailure(
				modelinference.InvocationFailureClassInvalidSlot,
				fmt.Sprintf("invalid input mapping %q: expected slot=value", value), "", nil,
			)
		}
		slot := strings.TrimSpace(parts[0])
		if slot == "" {
			return nil, genericCLIInputFailure(
				modelinference.InvocationFailureClassInvalidSlot,
				fmt.Sprintf("invalid input mapping %q: slot is required", value), "", nil,
			)
		}
		if strings.TrimSpace(parts[1]) == "" {
			return nil, genericCLIInputFailure(
				modelinference.InvocationFailureClassInvalidParameter,
				fmt.Sprintf("input slot %q requires a value", slot), slot, nil,
			)
		}
		mappings = append(mappings, genericCLIInputMapping{slot: slot, value: parts[1]})
	}
	return mappings, nil
}

func (service *rootService) genericCLIInput(
	cfg InvokeConfig,
	mapping genericCLIInputMapping,
	slot modelinference.OperationSlot,
) (modelinference.InferenceInput, error) {
	return genericCLIInputWithReader(cfg, mapping, slot, service.inputFileReader)
}

func genericCLIInputWithReader(
	cfg InvokeConfig,
	mapping genericCLIInputMapping,
	slot modelinference.OperationSlot,
	inputFileReader InputFileReader,
) (modelinference.InferenceInput, error) {
	value := mapping.value
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "@") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmed, "@"))
		if path == "" {
			return modelinference.InferenceInput{}, genericCLIInputFailure(
				modelinference.InvocationFailureClassInvalidParameter,
				fmt.Sprintf("input slot %q requires a file path after @", mapping.slot), mapping.slot, nil,
			)
		}
		if inputFileReader == nil {
			return modelinference.InferenceInput{}, clidiag.NewLocalInputFailure(
				"--input", path, errors.New("Models CLI input filesystem is not configured"),
			)
		}
		data, err := inputFileReader(cfg.Context, path, genericCLIInputMaxFileBytes)
		if err != nil {
			return modelinference.InferenceInput{}, clidiag.NewLocalInputFailure("--input", path, err)
		}
		if err := cfg.Context.Err(); err != nil {
			return modelinference.InferenceInput{}, clidiag.NewLocalInputFailure("--input", path, err)
		}
		if len(data) == 0 {
			return modelinference.InferenceInput{}, clidiag.NewLocalInputFailure(
				"--input", path, errors.New("file is empty"),
			)
		}
		if int64(len(data)) > genericCLIInputMaxFileBytes {
			return modelinference.InferenceInput{}, clidiag.NewLocalInputFailure(
				"--input", path,
				fmt.Errorf("file content exceeds the %d-byte limit", genericCLIInputMaxFileBytes),
			)
		}
		mediaType := genericCLIInputMediaType(path, data)
		if !genericCLIInputAcceptsMediaType(slot, mediaType) {
			return modelinference.InferenceInput{}, genericCLIInputFailure(
				modelinference.InvocationFailureClassMediaCapability,
				fmt.Sprintf("input slot %q does not accept media type %q", mapping.slot, mediaType),
				mapping.slot, slot.MediaTypes,
			)
		}
		return modelinference.InferenceInput{
			Name: mapping.slot, Modality: slot.Modality, ContentType: mediaType,
			MediaType: mediaType, Content: string(data),
		}, nil
	}
	if slot.Modality == modelinference.ModalityAudio ||
		slot.Modality == modelinference.ModalityImage ||
		slot.Modality == modelinference.ModalityVideo ||
		slot.Modality == modelinference.ModalityBinary {
		return modelinference.InferenceInput{}, genericCLIInputFailure(
			modelinference.InvocationFailureClassMediaCapability,
			fmt.Sprintf("input slot %q requires a file value prefixed with @", mapping.slot),
			mapping.slot, slot.MediaTypes,
		)
	}
	contentType := genericCLIInputContentType(slot)
	if slot.Modality == modelinference.ModalityJSON {
		value = genericCLIJSONInputValue(value)
		if !json.Valid([]byte(value)) {
			return modelinference.InferenceInput{}, genericCLIInputFailure(
				modelinference.InvocationFailureClassInvalidParameter,
				fmt.Sprintf("input slot %q must contain valid JSON", mapping.slot), mapping.slot, nil,
			)
		}
	}
	return modelinference.InferenceInput{
		Name: mapping.slot, Modality: slot.Modality, ContentType: contentType,
		MediaType: contentType, Content: value,
	}, nil
}

func genericCLIJSONInputValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "json:") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "json:"))
	}
	return value
}

func genericCLIInputFailure(
	class modelinference.InvocationFailureClass,
	message string,
	slot string,
	validNames []string,
) error {
	return &modelinference.InvocationFailure{
		Class: class, Message: message, Operation: "", Slot: slot,
		ValidNames: append([]string(nil), validNames...),
	}
}

func genericCLIInputContentType(slot modelinference.OperationSlot) string {
	for _, mediaType := range slot.MediaTypes {
		mediaType = strings.TrimSpace(mediaType)
		if mediaType != "" && !strings.HasSuffix(mediaType, "/*") {
			return mediaType
		}
	}
	switch slot.Modality {
	case modelinference.ModalityJSON:
		return "application/json"
	default:
		return "text/plain"
	}
}

var genericCLIInputMediaTypes = map[string]string{
	".wav": "audio/wav", ".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".aac": "audio/aac",
	".flac": "audio/flac", ".ogg": "audio/ogg", ".oga": "audio/ogg", ".opus": "audio/opus",
	".webm": "audio/webm", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".mp4": "video/mp4", ".mov": "video/quicktime",
	".json": "application/json", ".txt": "text/plain", ".md": "text/plain",
}

func genericCLIInputMediaType(path string, data []byte) string {
	extension := strings.ToLower(filepath.Ext(path))
	if mediaType, ok := genericCLIInputMediaTypes[extension]; ok {
		return mediaType
	}
	if detected := mime.TypeByExtension(filepath.Ext(path)); strings.TrimSpace(detected) != "" {
		return genericCLIInputNormalizeMediaType(detected)
	}
	return genericCLIInputNormalizeMediaType(http.DetectContentType(data))
}

func genericCLIInputNormalizeMediaType(value string) string {
	value = strings.TrimSpace(strings.SplitN(value, ";", 2)[0])
	if strings.EqualFold(value, "audio/x-wav") {
		return "audio/wav"
	}
	if strings.EqualFold(value, "application/ogg") {
		return "audio/ogg"
	}
	return strings.ToLower(value)
}

func genericCLIInputAcceptsMediaType(slot modelinference.OperationSlot, mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	for _, declared := range slot.MediaTypes {
		declared = strings.ToLower(strings.TrimSpace(declared))
		if declared == "" || declared == "*/*" || declared == mediaType {
			return true
		}
		if strings.HasSuffix(declared, "/*") && strings.HasPrefix(mediaType, strings.TrimSuffix(declared, "*")) {
			return true
		}
	}
	return false
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
