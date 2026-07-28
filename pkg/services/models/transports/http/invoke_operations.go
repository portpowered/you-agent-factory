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
	workerinferencemapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerinference"
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
	authored := workerinferencemapping.OperationBindingsFromGenerated(bindings)
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
