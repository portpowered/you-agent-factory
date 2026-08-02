package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

type requestValidationError struct{ message string }

func (e requestValidationError) Error() string { return e.message }

// decodeModelInvocationRequestFromHTTP decodes one owned invoke HTTP body into
// the generated invocation request shape before root mapping.
func decodeModelInvocationRequestFromHTTP(body io.Reader) (factoryapi.ModelInvocationRequest, error) {
	var payload []byte
	var err error
	if body != nil {
		payload, err = io.ReadAll(body)
		if err != nil {
			return factoryapi.ModelInvocationRequest{}, err
		}
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return factoryapi.ModelInvocationRequest{}, requestValidationError{message: "request body is required"}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return factoryapi.ModelInvocationRequest{}, err
	}
	if err := validateWorkContentField(fields, ""); err != nil {
		return factoryapi.ModelInvocationRequest{}, err
	}
	var request factoryapi.ModelInvocationRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return factoryapi.ModelInvocationRequest{}, err
	}
	return request, nil
}

func validateModelInvocationOperation(request factoryapi.ModelInvocationRequest) error {
	if strings.TrimSpace(request.Operation) == "" {
		return requestValidationError{message: "operation is required"}
	}
	return nil
}

type structuredInferenceInput struct {
	Content  []work.WorkContentPart         `json:"content"`
	Bindings []models.ModelOperationBinding `json:"bindings,omitempty"`
}

func inferenceInputFromHTTP(
	request factoryapi.ModelInvocationRequest,
) (models.InferenceInput, []work.WorkContentPart, error) {
	content := contentmapping.PartsFromGenerated(request.Content)
	if len(content) == 0 {
		return models.InferenceInput{}, nil, nil
	}
	if simpleTextInput(content, request.Bindings) {
		var text strings.Builder
		for index, part := range content {
			if index > 0 {
				text.WriteByte('\n')
			}
			text.WriteString(part.Text)
		}
		contentType := strings.TrimSpace(content[0].ContentType)
		if contentType == "" {
			contentType = "text/plain"
		}
		return models.InferenceInput{ContentType: contentType, Content: text.String()}, content, nil
	}

	payload := structuredInferenceInput{
		Content:  work.CloneWorkContentParts(content),
		Bindings: modelBindingsFromHTTP(request.Bindings),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return models.InferenceInput{}, nil, fmt.Errorf("encode model invocation input: %w", err)
	}
	return models.InferenceInput{ContentType: "application/json", Content: string(encoded)}, content, nil
}

func simpleTextInput(content []work.WorkContentPart, bindings *[]factoryapi.WorkstationOperationBinding) bool {
	if bindings != nil && len(*bindings) > 0 {
		return false
	}
	for _, part := range content {
		if part.Type.Normalized() != work.WorkContentPartTypeText ||
			strings.TrimSpace(part.URL) != "" || strings.TrimSpace(part.File) != "" ||
			len(part.JSON) != 0 || strings.TrimSpace(part.Slot) != "" ||
			strings.TrimSpace(part.Label) != "" || strings.TrimSpace(part.Role) != "" ||
			strings.TrimSpace(part.ArtifactID) != "" || len(part.Metadata) != 0 {
			return false
		}
	}
	return true
}

func modelBindingsFromHTTP(bindings *[]factoryapi.WorkstationOperationBinding) []models.ModelOperationBinding {
	if bindings == nil || len(*bindings) == 0 {
		return nil
	}
	result := make([]models.ModelOperationBinding, 0, len(*bindings))
	for _, binding := range *bindings {
		mapped := models.ModelOperationBinding{
			Slot:           strings.TrimSpace(binding.Slot),
			Config:         work.CloneWorkContentParts(contentmapping.PartsFromGenerated(binding.Config)),
			DefaultContent: work.CloneWorkContentParts(contentmapping.PartsFromGenerated(binding.DefaultContent)),
		}
		if binding.Selector != nil {
			mapped.Selector = &models.ModelOperationBindingSelector{
				Slot:  trimmedPointerValue(binding.Selector.Slot),
				Label: trimmedPointerValue(binding.Selector.Label),
				Type:  trimmedPointerValue(binding.Selector.Type),
				Role:  trimmedPointerValue(binding.Selector.Role),
			}
		}
		result = append(result, mapped)
	}
	return result
}

func trimmedPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(string(*value))
}

func modelInvocationResponseFromInferenceResult(
	result models.InvokeModelResult,
	catalog models.Detail,
	inputText string,
	inputContent []work.WorkContentPart,
) factoryapi.ModelInvocationResponse {
	worker, locality := catalogPresentationForOperation(catalog, result.Operation)
	bindings := resolvedPresentationBindings(catalog, result.Operation, inputText, inputContent)
	return factoryapi.ModelInvocationResponse{
		ModelName:        result.ModelName,
		Worker:           worker,
		Operation:        result.Operation,
		ProviderLocality: factoryapi.WorkerModelLocality(locality),
		Content:          derefGeneratedWorkContent(contentmapping.GeneratedPtrFromParts(inferenceContentToWorkParts(result.Content))),
		Bindings:         generatedResolvedModelInvocationBindings(bindings),
	}
}

func catalogPresentationForOperation(catalog models.Detail, operation string) (string, string) {
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
	catalog models.Detail,
	operation string,
	inputText string,
	inputContent []work.WorkContentPart,
) []models.ResolvedModelOperationBinding {
	operationDetail, ok := catalogOperationForName(catalog, operation)
	if !ok {
		return []models.ResolvedModelOperationBinding{}
	}
	for _, input := range operationDetail.Inputs {
		slot := strings.TrimSpace(input.Name)
		if slot == "" {
			continue
		}
		content := work.CloneWorkContentParts(inputContent)
		if len(content) == 0 && strings.TrimSpace(inputText) != "" {
			content = []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: inputText}}
		}
		return []models.ResolvedModelOperationBinding{{Slot: slot, Source: "INPUT", Content: content}}
	}
	return []models.ResolvedModelOperationBinding{}
}

func catalogOperationForName(catalog models.Detail, operation string) (models.Operation, bool) {
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
	return models.Operation{}, false
}

func inferenceContentToWorkParts(content []models.InferenceContent) []work.WorkContentPart {
	if len(content) == 0 {
		return nil
	}
	parts := make([]work.WorkContentPart, 0, len(content))
	for _, item := range content {
		contentType := strings.TrimSpace(item.ContentType)
		value := strings.TrimSpace(item.Content)
		switch {
		case strings.HasPrefix(strings.ToLower(contentType), "audio/"):
			parts = append(parts, work.WorkContentPart{Type: work.WorkContentPartTypeAudio, File: value, ContentType: contentType, Slot: "audio"})
		case strings.HasPrefix(strings.ToLower(contentType), "image/"):
			parts = append(parts, work.WorkContentPart{Type: work.WorkContentPartTypeImage, URL: value, ContentType: contentType, Slot: "image"})
		case strings.EqualFold(contentType, "application/json"):
			parts = append(parts, work.WorkContentPart{Type: work.WorkContentPartTypeJSON, JSON: json.RawMessage(value), Slot: "json"})
		default:
			if contentType == "" {
				contentType = "text/plain"
			}
			parts = append(parts, work.WorkContentPart{Type: work.WorkContentPartTypeText, Text: value, ContentType: contentType, Slot: "text"})
		}
	}
	return parts
}

func generatedResolvedModelInvocationBindings(values []models.ResolvedModelOperationBinding) []factoryapi.ResolvedModelOperationBinding {
	if len(values) == 0 {
		return nil
	}
	bindings := make([]factoryapi.ResolvedModelOperationBinding, 0, len(values))
	for _, binding := range values {
		bindings = append(bindings, factoryapi.ResolvedModelOperationBinding{
			Slot: binding.Slot, Source: factoryapi.ResolvedModelOperationBindingSource(binding.Source),
			Content: derefGeneratedWorkContent(contentmapping.GeneratedPtrFromParts(binding.Content)),
		})
	}
	return bindings
}

func derefGeneratedWorkContent(content *factoryapi.WorkContent) factoryapi.WorkContent {
	if content == nil {
		return nil
	}
	return *content
}

func invocationStreamFromResult(
	result models.InvokeModelResult,
	request factoryapi.ModelInvocationRequest,
) (string, string, error) {
	if request.Options == nil || request.Options.ResponseMode == nil ||
		string(*request.Options.ResponseMode) != string(factoryapi.AUDIOSTREAM) {
		return "", "", nil
	}
	for _, artifact := range result.Artifacts {
		path := strings.TrimSpace(artifact.Artifact.String())
		if path == "" {
			return "", "", fmt.Errorf("%w: streamed audio artifact reference is empty", models.ErrInferenceArtifactInvalid)
		}
		contentType := strings.TrimSpace(artifact.MediaType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		return path, contentType, nil
	}
	return "", "", fmt.Errorf("%w: invocation did not produce audio output", models.ErrUnsupportedResponseMode)
}
