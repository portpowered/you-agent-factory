package cli

import (
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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
