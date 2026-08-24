package inference

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func genericInvocationRequest(
	request workers.RunnerExecutionRequest,
	scope models.RuntimeScopeRef,
	worker models.LocalWorker,
) (models.InvokeModelRequest, error) {
	inputs, parameters, err := genericInvocationInputs(request.ModelBindings)
	if err != nil {
		return models.InvokeModelRequest{}, err
	}
	return models.InvokeModelRequest{
		Scope:      scope,
		Holder:     invocationHolder(request),
		Model:      models.ModelReference{NameOrURI: strings.TrimSpace(worker.Model)},
		Operation:  strings.TrimSpace(request.ModelOperation),
		Inputs:     inputs,
		Parameters: parameters,
		OutputMode: models.OutputModeAuto,
	}, nil
}

func genericInvocationInputs(
	bindings []workers.ResolvedModelOperationBinding,
) ([]models.InferenceInput, []models.OperationParameter, error) {
	var inputs []models.InferenceInput
	var parameters []models.OperationParameter
	for bindingIndex, binding := range bindings {
		slot := strings.TrimSpace(binding.Slot)
		if strings.EqualFold(slot, "parameters") {
			for partIndex, part := range binding.Content {
				decoded, err := operationParametersFromWorkPart(part)
				if err != nil {
					return nil, nil, badRequest(
						fmt.Sprintf("inference model binding[%d] parameters[%d] is invalid", bindingIndex, partIndex),
						err,
					)
				}
				parameters = append(parameters, decoded...)
			}
			continue
		}
		for partIndex, part := range binding.Content {
			if workContentPartEmpty(part) {
				continue
			}
			input, err := inferenceInputFromWorkPart(slot, part)
			if err != nil {
				return nil, nil, badRequest(
					fmt.Sprintf("inference model binding[%d] content[%d] is invalid", bindingIndex, partIndex),
					err,
				)
			}
			inputs = append(inputs, input)
		}
	}
	return inputs, parameters, nil
}

func inferenceInputFromWorkPart(slot string, part work.WorkContentPart) (models.InferenceInput, error) {
	input := models.InferenceInput{Name: slot}
	var content string
	switch normalized := part.Type.Normalized(); normalized {
	case work.WorkContentPartTypeText:
		input.Modality = models.ModalityText
		content = part.Text
		input.ContentType, input.MediaType = inputContentMetadata(part.ContentType, "text/plain")
	case work.WorkContentPartTypeJSON:
		input.Modality = models.ModalityJSON
		content = string(part.JSON)
		input.ContentType, input.MediaType = inputContentMetadata(part.ContentType, "application/json")
	case work.WorkContentPartTypeImage:
		input.Modality = models.ModalityImage
		content = firstNonEmpty(part.URL, part.File, part.Text)
		input.ContentType, input.MediaType = inputContentMetadata(part.ContentType, "")
	case work.WorkContentPartTypeAudio:
		input.Modality = models.ModalityAudio
		content = firstNonEmpty(part.URL, part.File, part.Text)
		input.ContentType, input.MediaType = inputContentMetadata(part.ContentType, "")
	case work.WorkContentPartTypeBinary:
		input.Modality = models.ModalityBinary
		content = firstNonEmpty(part.URL, part.File, part.Text)
		input.ContentType, input.MediaType = inputContentMetadata(part.ContentType, "")
	default:
		return models.InferenceInput{}, fmt.Errorf("unsupported Work content type %q", part.Type)
	}
	input.Content = content
	if artifactID := strings.TrimSpace(part.ArtifactID); artifactID != "" {
		artifact, err := (models.InferenceArtifactRef{}).Parse(artifactID)
		if err != nil {
			return models.InferenceInput{}, err
		}
		input.Artifact = &artifact
	}
	return input, nil
}

func inputContentMetadata(contentType, fallback string) (string, string) {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return fallback, fallback
	}
	if strings.Contains(contentType, "/") {
		return contentType, contentType
	}
	return contentType, ""
}

func operationParametersFromWorkPart(part work.WorkContentPart) ([]models.OperationParameter, error) {
	raw, err := parameterJSONFromWorkPart(part)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, fmt.Errorf("parameters must contain valid JSON")
	}
	return decodeOperationParameters(raw)
}

func parameterJSONFromWorkPart(part work.WorkContentPart) ([]byte, error) {
	switch part.Type.Normalized() {
	case work.WorkContentPartTypeJSON:
		return append([]byte(nil), part.JSON...), nil
	case work.WorkContentPartTypeText:
		return []byte(part.Text), nil
	default:
		return nil, fmt.Errorf("parameters must be JSON content")
	}
}

func decodeOperationParameters(raw []byte) ([]models.OperationParameter, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := first.(json.Delim)
	if !ok || delimiter != '{' {
		return nil, fmt.Errorf("parameters must be a JSON object")
	}
	var parameters []models.OperationParameter
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("parameter name must be a non-empty string")
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		parameters = append(parameters, models.OperationParameter{Name: name, Value: value})
	}
	last, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := last.(json.Delim); !ok || delimiter != '}' {
		return nil, fmt.Errorf("parameters object is malformed")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parameters contain trailing JSON")
		}
		return nil, err
	}
	return parameters, nil
}

func workContentPartEmpty(part work.WorkContentPart) bool {
	switch part.Type.Normalized() {
	case work.WorkContentPartTypeText:
		return strings.TrimSpace(part.Text) == "" && strings.TrimSpace(part.ArtifactID) == ""
	case work.WorkContentPartTypeJSON:
		return len(part.JSON) == 0 && strings.TrimSpace(part.ArtifactID) == ""
	default:
		return firstNonEmpty(part.URL, part.File, part.Text, part.ArtifactID) == ""
	}
}

func proposedOutputFromModelResult(result models.InvokeModelResult) (workers.ProposedOutput, error) {
	outputs := append([]models.InferenceOutput(nil), result.Outputs...)
	if len(outputs) == 0 {
		for index, content := range result.Content {
			name := strings.TrimSpace(content.Name)
			if name == "" {
				name = defaultOutputName(content.Modality, index)
			}
			outputs = append(outputs, models.InferenceOutput{
				Name:        name,
				Modality:    content.Modality,
				ContentType: content.ContentType,
				MediaType:   content.MediaType,
				Content:     content.Content,
			})
		}
	}
	if len(outputs) == 0 {
		return workers.ProposedOutput{}, badRequest("Models returned no model output", models.ErrInferenceFailed)
	}
	proposal := workers.ProposedOutput{Primary: make([]work.WorkContentPart, 0, len(outputs))}
	for index, output := range outputs {
		part, err := workContentPartFromModelOutput(output, index)
		if err != nil {
			return workers.ProposedOutput{}, err
		}
		proposal.Primary = append(proposal.Primary, part)
		if output.Artifact != nil && !output.Artifact.Artifact.IsZero() {
			proposal.ArtifactRefs = append(proposal.ArtifactRefs, workers.ArtifactRef{
				ArtifactID: output.Artifact.Artifact.String(),
				Label:      output.Artifact.Name,
				URI:        part.URL,
			})
		}
	}
	return proposal, nil
}

func workContentPartFromModelOutput(output models.InferenceOutput, index int) (work.WorkContentPart, error) {
	part, modality := baseWorkContentPartFromModelOutput(output, index)
	switch {
	case strings.EqualFold(string(modality), string(models.ModalityText)):
		return textWorkContentPart(part, output), nil
	case strings.EqualFold(string(modality), string(models.ModalityJSON)):
		return jsonWorkContentPart(part, output)
	case strings.EqualFold(string(modality), string(models.ModalityAudio)):
		return audioWorkContentPart(part, output)
	case strings.EqualFold(string(modality), string(models.ModalityImage)):
		return mediaWorkContentPart(part, output, work.WorkContentPartTypeImage, "Models returned empty image output")
	case strings.EqualFold(string(modality), string(models.ModalityBinary)):
		return mediaWorkContentPart(part, output, work.WorkContentPartTypeBinary, "Models returned empty binary output")
	default:
		return work.WorkContentPart{}, badRequest(
			fmt.Sprintf("Models returned unsupported output modality %q", modality),
			nil,
		)
	}
}

func baseWorkContentPartFromModelOutput(output models.InferenceOutput, index int) (work.WorkContentPart, models.Modality) {
	modality := output.Modality
	if modality == "" {
		modality = modalityFromMediaType(output.MediaType, output.ContentType)
	}
	part := work.WorkContentPart{
		Slot:        strings.TrimSpace(output.Name),
		ContentType: firstNonEmpty(output.MediaType, output.ContentType),
	}
	if part.Slot == "" {
		part.Slot = defaultOutputName(modality, index)
	}
	if output.Artifact != nil && !output.Artifact.Artifact.IsZero() {
		part.ArtifactID = output.Artifact.Artifact.String()
		part.Label = output.Artifact.Name
		part.Metadata = workMetadataFromArtifact(output.Artifact)
	}
	return part, modality
}

func textWorkContentPart(part work.WorkContentPart, output models.InferenceOutput) work.WorkContentPart {
	part.Type = work.WorkContentPartTypeText
	part.Text = output.Content
	if part.ContentType == "" {
		part.ContentType = "text/plain"
	}
	return part
}

func jsonWorkContentPart(part work.WorkContentPart, output models.InferenceOutput) (work.WorkContentPart, error) {
	part.Type = work.WorkContentPartTypeJSON
	part.JSON = []byte(output.Content)
	if !json.Valid(part.JSON) {
		return work.WorkContentPart{}, badRequest(
			fmt.Sprintf("Models returned invalid JSON output %q", part.Slot),
			nil,
		)
	}
	if part.ContentType == "" {
		part.ContentType = "application/json"
	}
	return part, nil
}

func audioWorkContentPart(part work.WorkContentPart, output models.InferenceOutput) (work.WorkContentPart, error) {
	part.Type = work.WorkContentPartTypeAudio
	part.URL = inlineOutputURL(output.Content, firstNonEmpty(output.MediaType, output.ContentType, "audio/wav"))
	if part.URL == "" {
		return work.WorkContentPart{}, badRequest(
			fmt.Sprintf("Models returned empty audio output %q", part.Slot),
			models.ErrInferenceFailed,
		)
	}
	if part.ContentType == "" {
		part.ContentType = firstNonEmpty(output.MediaType, output.ContentType, "audio/wav")
	}
	return part, nil
}

func mediaWorkContentPart(
	part work.WorkContentPart,
	output models.InferenceOutput,
	partType work.WorkContentPartType,
	emptyMessage string,
) (work.WorkContentPart, error) {
	part.Type = partType
	part.URL = inlineOutputURL(output.Content, firstNonEmpty(output.MediaType, output.ContentType, "application/octet-stream"))
	if part.URL == "" {
		return work.WorkContentPart{}, badRequest(emptyMessage, models.ErrInferenceFailed)
	}
	return part, nil
}

func workMetadataFromArtifact(artifact *models.InferenceArtifact) map[string]any {
	if artifact == nil || len(artifact.Properties) == 0 {
		return nil
	}
	metadata := make(map[string]any, len(artifact.Properties))
	for key, value := range artifact.Properties {
		metadata[key] = value
	}
	return metadata
}

func inlineOutputURL(content, mediaType string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	lower := strings.ToLower(strings.TrimSpace(content))
	for _, prefix := range []string{"data:", "file://", "http://", "https://"} {
		if strings.HasPrefix(lower, prefix) {
			return content
		}
	}
	mediaType = strings.TrimSpace(mediaType)
	if mediaType == "" || !strings.Contains(mediaType, "/") {
		mediaType = "application/octet-stream"
	}
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString([]byte(content))
}

func modalityFromMediaType(values ...string) models.Modality {
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		switch {
		case strings.HasPrefix(value, "text/"):
			return models.ModalityText
		case strings.HasPrefix(value, "audio/"):
			return models.ModalityAudio
		case strings.HasPrefix(value, "image/"):
			return models.ModalityImage
		case value == "application/json":
			return models.ModalityJSON
		}
	}
	return ""
}

func defaultOutputName(modality models.Modality, index int) string {
	switch {
	case strings.EqualFold(string(modality), string(models.ModalityAudio)):
		return "audio"
	case strings.EqualFold(string(modality), string(models.ModalityText)):
		return "text"
	case strings.EqualFold(string(modality), string(models.ModalityJSON)):
		return "json"
	default:
		return fmt.Sprintf("output-%d", index+1)
	}
}

func textContentFromProposedOutput(output workers.ProposedOutput) string {
	var values []string
	for _, part := range output.Primary {
		if part.Type.Normalized() != work.WorkContentPartTypeText {
			continue
		}
		if text := strings.TrimSpace(part.Text); text != "" {
			values = append(values, text)
		}
	}
	return strings.Join(values, "\n")
}
