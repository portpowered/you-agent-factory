// Factory work-type and top-level mapping helpers.
package factoryconfig

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func inputTypesAPIFromInternal(inputTypes []interfaces.InputTypeConfig) *[]factoryapi.InputType {
	if len(inputTypes) == 0 {
		return nil
	}
	result := make([]factoryapi.InputType, len(inputTypes))
	for i, inputType := range inputTypes {
		result[i] = factoryapi.InputType{
			Name: inputType.Name,
			Type: publicFactoryInputKindFromInternal(inputType.Type),
		}
	}
	return &result
}

func invocationReturnAPIFromInternal(value *interfaces.InvocationReturnConfig) *factoryapi.InvocationReturn {
	if value == nil {
		return nil
	}
	result := &factoryapi.InvocationReturn{
		Policy: factoryapi.InvocationReturnPolicy(value.Policy),
	}
	if strings.TrimSpace(value.WorkTypeName) != "" {
		result.WorkTypeName = stringPtrIfNotEmpty(value.WorkTypeName)

	}
	if strings.TrimSpace(value.TerminalState) != "" {
		result.TerminalState = stringPtrIfNotEmpty(value.TerminalState)
	}
	if strings.TrimSpace(value.WorkName) != "" {
		result.WorkName = stringPtrIfNotEmpty(value.WorkName)
	}
	return result
}

func workTypeHandlingBehaviorAPIFromInternal(behaviors []string) *[]factoryapi.WorkTypeHandlingBehavior {
	if len(behaviors) == 0 {
		return nil
	}
	values := make([]factoryapi.WorkTypeHandlingBehavior, 0, len(behaviors))
	for _, behavior := range behaviors {
		canonical := interfaces.StrictPublicWorkTypeHandlingBehavior(behavior)
		if canonical == "" {
			continue
		}
		values = append(values, factoryapi.WorkTypeHandlingBehavior(canonical))
	}
	if len(values) == 0 {
		return nil
	}
	return &values
}

func workTypesAPIFromInternal(workTypes []interfaces.WorkTypeConfig) *[]factoryapi.WorkType {
	if len(workTypes) == 0 {
		return nil
	}
	result := make([]factoryapi.WorkType, len(workTypes))
	for i, workType := range workTypes {
		states := make([]factoryapi.WorkState, len(workType.States))
		for stateIndex, state := range workType.States {
			states[stateIndex] = factoryapi.WorkState{
				Id:   stringPtrIfNotEmpty(state.ID),
				Name: state.Name,
				Type: factoryapi.WorkStateType(state.Type),
			}
		}
		result[i] = factoryapi.WorkType{
			Id:                stringPtrIfNotEmpty(workType.ID),
			Name:              workType.Name,
			Description:       NameValueAPIFromInternal(workType.Description),
			States:            states,
			HandlingBehavior:  workTypeHandlingBehaviorAPIFromInternal(workType.HandlingBehavior),
			ExpectedArtifacts: expectedArtifactsAPIFromInternal(workType.ExpectedArtifacts),
		}
	}
	return &result
}

func factoryReferenceName(cfg *interfaces.FactoryConfig) factoryapi.FactoryName {
	if cfg != nil && strings.TrimSpace(cfg.Name) != "" {
		return factoryapi.FactoryName(cfg.Name)
	}
	if cfg != nil && strings.TrimSpace(cfg.Project) != "" {
		return factoryapi.FactoryName(cfg.Project)
	}
	return factoryapi.FactoryName("factory")
}

// FactoryConfigToOpenAPI converts a valid internal factory config into the
// generated OpenAPI model without passing through normalized on-disk JSON.
// It rejects values that cannot be represented by the public contract.
func FactoryConfigToOpenAPI(cfg *interfaces.FactoryConfig) (factoryapi.Factory, error) {
	return factoryAPIFromInternalConfig(cfg)
}

func hybridLogicalTimestampPtr(version *interfaces.FactoryVersion) *factoryapi.HybridLogicalTimestamp {
	if version == nil {
		return nil
	}
	return &factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(version.Logical),
		Physical: version.Physical.UTC(),
	}
}
