package http

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/workerinference"
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

// decodeGenericModelInvocationRequestFromHTTP decodes the generic collection
// endpoint with strict JSON fields so a misspelled slot or output control is
// rejected before the Models root can perform any side effect.
func decodeGenericModelInvocationRequestFromHTTP(body io.Reader) (factoryapi.GenericModelInvocationRequest, error) {
	if body == nil {
		return factoryapi.GenericModelInvocationRequest{}, requestValidationError{message: "request body is required"}
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var request factoryapi.GenericModelInvocationRequest
	if err := decoder.Decode(&request); err != nil {
		return factoryapi.GenericModelInvocationRequest{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return factoryapi.GenericModelInvocationRequest{}, requestValidationError{message: "request body must contain one JSON value"}
		}
		return factoryapi.GenericModelInvocationRequest{}, err
	}
	return request, nil
}

func validateModelInvocationOperation(request factoryapi.ModelInvocationRequest) error {
	if strings.TrimSpace(request.Operation) == "" {
		return requestValidationError{message: "operation is required"}
	}
	return nil
}

func modelInvocationRequestFromHTTP(request factoryapi.ModelInvocationRequest) models.Request {
	return invocationRequestFromGenerated(request)
}

func invocationRequestFromGenerated(request factoryapi.ModelInvocationRequest) models.Request {
	domain := models.Request{
		Operation: request.Operation,
		Content:   contentmapping.PartsFromGenerated(request.Content),
		Bindings:  modelBindingsFromGenerated(request.Bindings),
	}
	if request.Options != nil {
		domain.Options = &models.Options{}
		if request.Options.ResponseMode != nil {
			domain.Options.ResponseMode = models.ResponseMode(*request.Options.ResponseMode)
		}
	}
	return domain
}

func modelBindingsFromGenerated(bindings *[]factoryapi.WorkstationOperationBinding) []models.ModelOperationBinding {
	authored := workerinference.OperationBindingsFromGenerated(bindings)
	result := make([]models.ModelOperationBinding, len(authored))
	for i := range authored {
		result[i] = models.ModelOperationBinding{
			Slot: authored[i].Slot, Config: work.CloneWorkContentParts(authored[i].Config),
			DefaultContent: work.CloneWorkContentParts(authored[i].DefaultContent),
		}
		if authored[i].Selector != nil {
			result[i].Selector = &models.ModelOperationBindingSelector{
				Slot: authored[i].Selector.Slot, Label: authored[i].Selector.Label,
				Type: authored[i].Selector.Type, Role: authored[i].Selector.Role,
			}
		}
	}
	return result
}

func modelInvocationResponseFromResult(result models.Result) factoryapi.ModelInvocationResponse {
	return factoryapi.ModelInvocationResponse{
		ModelName:        result.ModelName,
		Worker:           result.Worker,
		Operation:        result.Operation,
		ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Content:          derefGeneratedWorkContent(contentmapping.GeneratedPtrFromParts(result.Content)),
		Bindings:         generatedResolvedModelInvocationBindings(result.Bindings),
	}
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

// GenericInvocationRequestFromGenerated normalizes the additive generic HTTP
// request into Models-owned values without resolving the model or starting any
// runtime. Ordered slices are copied in their authored order, including
// repeated slot names.
func GenericInvocationRequestFromGenerated(
	request factoryapi.GenericModelInvocationRequest,
) (models.GenericInvocationRequest, error) {
	scope, err := (models.RuntimeScopeRef{}).Parse(request.Scope)
	if err != nil {
		return models.GenericInvocationRequest{}, err
	}

	var inputs []models.InferenceInput
	if request.Inputs != nil {
		inputs = make([]models.InferenceInput, len(*request.Inputs))
		for index, input := range *request.Inputs {
			mapped, mapErr := genericInferenceInputFromGenerated(input)
			if mapErr != nil {
				return models.GenericInvocationRequest{}, mapErr
			}
			inputs[index] = mapped
		}
	}

	var parameters []models.OperationParameter
	if request.Parameters != nil {
		parameters = make([]models.OperationParameter, len(*request.Parameters))
		for index, parameter := range *request.Parameters {
			parameters[index] = models.OperationParameter{
				Name:  parameter.Name,
				Value: cloneGenericJSONValue(parameter.Value),
			}
		}
	}
	operation := ""
	if request.Operation != nil {
		operation = string(*request.Operation)
	}

	result := models.GenericInvocationRequest{
		Scope:      scope,
		Holder:     request.Holder,
		Model:      models.ModelReference{NameOrURI: request.Model.NameOrUri},
		Operation:  operation,
		Inputs:     inputs,
		Parameters: parameters,
	}
	if request.OutputMode != nil {
		result.OutputMode = models.OutputMode(*request.OutputMode)
	}
	if request.Offline != nil {
		result.Offline = *request.Offline
	}
	return result, nil
}

// GenericInvocationResponseToGenerated projects a detached generic Models
// result while retaining output order and distinct slot names.
func GenericInvocationResponseToGenerated(
	result models.GenericInvocationResult,
) factoryapi.GenericModelInvocationResponse {
	outputs := make([]factoryapi.ModelInvocationOutput, len(result.Outputs))
	for index, output := range result.Outputs {
		outputs[index] = genericInferenceOutputToGenerated(output)
	}
	return factoryapi.GenericModelInvocationResponse{Outputs: outputs}
}

// GenericInvocationFailureToGenerated projects the stable typed failure
// identity and safe coordinates without serializing its implementation cause.
func GenericInvocationFailureToGenerated(
	failure *models.InvocationFailure,
) *factoryapi.ModelInvocationFailure {
	if failure == nil {
		return nil
	}
	message := failure.Message
	if strings.TrimSpace(message) == "" {
		message = failure.Error()
	}
	projected := &factoryapi.ModelInvocationFailure{
		Class:   factoryapi.ModelInvocationFailureClass(failure.Class),
		Message: message,
	}
	if !failure.Model.IsZero() {
		projected.Model = &factoryapi.ModelReference{NameOrUri: failure.Model.NameOrURI}
	}
	projected.Operation = nonEmptyStringPointer(failure.Operation)
	projected.Slot = nonEmptyStringPointer(failure.Slot)
	projected.Parameter = nonEmptyStringPointer(failure.Parameter)
	projected.Field = nonEmptyStringPointer(failure.Field)
	return projected
}

func genericInferenceInputFromGenerated(
	input factoryapi.ModelInvocationInput,
) (models.InferenceInput, error) {
	carrierCount := 0
	if input.Content != nil {
		carrierCount++
	}
	if input.ContentBase64 != nil {
		carrierCount++
	}
	if input.ArtifactRef != nil {
		carrierCount++
	}
	if carrierCount > 1 {
		return models.InferenceInput{}, newGenericMappingFailure(
			models.InvocationFailureClassInvalidParameter,
			"input must set only one of content, contentBase64, or artifactRef",
		)
	}
	mapped := models.InferenceInput{
		Name:     input.Name,
		Modality: models.Modality(input.Modality),
	}
	if input.ContentType != nil {
		mapped.ContentType = *input.ContentType
	}
	if input.MediaType != nil {
		mapped.MediaType = *input.MediaType
	}
	if input.Content != nil {
		mapped.Content = *input.Content
	}
	if input.ContentBase64 != nil {
		mapped.Content = string(*input.ContentBase64)
	}
	if input.ArtifactRef != nil {
		artifact, err := (models.InferenceArtifactRef{}).Parse(*input.ArtifactRef)
		if err != nil {
			failure := newGenericMappingFailure(
				models.InvocationFailureClassArtifact,
				"input artifact reference is invalid",
			)
			return models.InferenceInput{}, failure
		}
		mapped.Artifact = &artifact
	}
	return mapped, nil
}

func genericInferenceOutputToGenerated(
	output models.InferenceOutput,
) factoryapi.ModelInvocationOutput {
	projected := factoryapi.ModelInvocationOutput{
		Name:     output.Name,
		Modality: factoryapi.ModelInvocationContentType(output.Modality),
	}
	projected.ContentType = nonEmptyStringPointer(output.ContentType)
	projected.MediaType = nonEmptyStringPointer(output.MediaType)
	projected.Content = nonEmptyStringPointer(output.Content)
	if output.Artifact != nil && !output.Artifact.Artifact.IsZero() {
		projected.Artifact = genericInferenceArtifactToGenerated(*output.Artifact)
	}
	return projected
}

func genericInferenceArtifactToGenerated(
	artifact models.InferenceArtifact,
) *factoryapi.ModelInvocationArtifact {
	projected := &factoryapi.ModelInvocationArtifact{
		ArtifactRef: artifact.Artifact.String(),
	}
	projected.Name = nonEmptyStringPointer(artifact.Name)
	projected.MediaType = nonEmptyStringPointer(artifact.MediaType)
	if artifact.SizeBytes >= 0 {
		value := artifact.SizeBytes
		projected.SizeBytes = &value
	}
	if len(artifact.Properties) > 0 {
		properties := make(factoryapi.StringMap, len(artifact.Properties))
		for key, value := range artifact.Properties {
			properties[key] = value
		}
		projected.Properties = &properties
	}
	return projected
}

func cloneGenericJSONValue(value any) any {
	switch typed := value.(type) {
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneGenericJSONValue(item)
		}
		return cloned
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = cloneGenericJSONValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func nonEmptyStringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	cloned := value
	return &cloned
}

func newGenericMappingFailure(class models.InvocationFailureClass, message string) error {
	return &models.InvocationFailure{Class: class, Message: message}
}
