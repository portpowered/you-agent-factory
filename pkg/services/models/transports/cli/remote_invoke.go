package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const remoteModelsInvokeScope = "models-cli:remote"

func (service *httpService) invokeRemoteGeneric(
	cfg InvokeConfig,
	modelName string,
	operation string,
) error {
	if service.http == nil {
		return fmt.Errorf("CLI HTTP protocol is required for remote models invoke")
	}
	model, err := queryModel(queryOptions{
		Context: cfg.Context, Server: cfg.Server, ModelName: modelName,
		Verbose: cfg.Verbose, Diagnostics: cfg.Diagnostics, HTTP: service.http,
	})
	if err != nil {
		return err
	}
	catalog := genericCLIModelDetailFromGenerated(model)
	if err := validateCLIOutputShape(cfg, catalog, operation); err != nil {
		return err
	}
	inputs, err := prepareGenericCLIInputsWithReader(cfg, operation, catalog, service.inputFileReader)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.OutputPath) != "" {
		return fmt.Errorf("--output is not supported with explicit generic inputs")
	}
	request := genericCLIRequestFromInputs(modelName, operation, inputs)
	var response factoryapi.GenericModelInvocationResponse
	if err := doModelsPOST(
		cfg.Context, service.http, cfg.Server, "/models/invocations", request, &response,
		requestDiagnostics{
			Enabled: cfg.Verbose, Output: cfg.Diagnostics, Command: "models invoke",
			Server: cfg.Server, ModelName: modelName, Operation: operation,
		},
	); err != nil {
		return err
	}
	if response.Failure != nil {
		return genericCLIInvocationFailureFromGenerated(response.Failure, modelName, operation)
	}
	if err := validateGenericCLIResponse(response); err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	result, err := genericCLIResultFromGenerated(response, modelName, operation)
	if err != nil {
		return malformedModelsResponseError(err)
	}
	return writeGenericCLIOutputWithCatalog(cfg.Output, result, catalog, operation)
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
	return factoryapi.GenericModelInvocationRequest{
		Scope:     remoteModelsInvokeScope,
		Holder:    modelsCLIInvokeHolder,
		Model:     factoryapi.ModelReference{NameOrUri: modelName},
		Operation: &operationValue,
		Inputs:    &generatedInputs,
	}
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

func genericCLIResultFromGenerated(
	response factoryapi.GenericModelInvocationResponse,
	modelName string,
	operation string,
) (modelinference.InvokeModelResult, error) {
	result := modelinference.InvokeModelResult{
		ModelName: modelName,
		Operation: operation,
		Outputs:   make([]modelinference.InferenceOutput, len(response.Outputs)),
	}
	for index, output := range response.Outputs {
		mapped := modelinference.InferenceOutput{
			Name:     output.Name,
			Modality: modelinference.Modality(output.Modality),
		}
		if output.ContentType != nil {
			mapped.ContentType = *output.ContentType
		}
		if output.MediaType != nil {
			mapped.MediaType = *output.MediaType
		}
		if output.Content != nil {
			mapped.Content = *output.Content
		}
		if output.Artifact != nil {
			artifactRef, err := (modelinference.InferenceArtifactRef{}).Parse(output.Artifact.ArtifactRef)
			if err != nil {
				return modelinference.InvokeModelResult{}, fmt.Errorf("malformed models invocation response: output artifact reference is invalid")
			}
			artifact := &modelinference.InferenceArtifact{Artifact: artifactRef}
			if output.Artifact.Name != nil {
				artifact.Name = *output.Artifact.Name
			}
			if output.Artifact.MediaType != nil {
				artifact.MediaType = *output.Artifact.MediaType
			}
			if output.Artifact.SizeBytes != nil {
				artifact.SizeBytes = *output.Artifact.SizeBytes
			}
			if output.Artifact.Properties != nil {
				artifact.Properties = make(map[string]string, len(*output.Artifact.Properties))
				for key, value := range *output.Artifact.Properties {
					artifact.Properties[key] = value
				}
			}
			mapped.Artifact = artifact
		}
		result.Outputs[index] = mapped
	}
	return result, nil
}
