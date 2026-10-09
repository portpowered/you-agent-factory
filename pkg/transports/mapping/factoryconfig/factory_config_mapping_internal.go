// Factory work-type reverse mapping helpers.
package factoryconfig

import (
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func workTypeHandlingBehaviorInternalFromAPI(behaviors *[]factoryapi.WorkTypeHandlingBehavior) []string {
	if behaviors == nil || len(*behaviors) == 0 {
		return nil
	}
	values := make([]string, 0, len(*behaviors))
	for _, behavior := range *behaviors {
		canonical := interfaces.StrictPublicWorkTypeHandlingBehavior(string(behavior))
		if canonical == "" {
			continue
		}
		values = append(values, canonical)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func workTypesInternalFromAPI(workTypes []factoryapi.WorkType) ([]interfaces.WorkTypeConfig, error) {
	values := make([]interfaces.WorkTypeConfig, len(workTypes))
	for i, workType := range workTypes {
		description, err := nameValueInternalFromAPI(workType.Description, fmt.Sprintf("factory.workTypes[%d].description", i))
		if err != nil {
			return nil, err
		}
		states := make([]interfaces.StateConfig, len(workType.States))
		for si, state := range workType.States {
			states[si] = interfaces.StateConfig{
				ID:   stringValue(state.Id),
				Name: state.Name,
				Type: interfaces.StateType(state.Type),
			}
		}
		values[i] = interfaces.WorkTypeConfig{
			ID:                stringValue(workType.Id),
			Name:              workType.Name,
			Description:       description,
			States:            states,
			HandlingBehavior:  workTypeHandlingBehaviorInternalFromAPI(workType.HandlingBehavior),
			ExpectedArtifacts: expectedArtifactsInternalFromAPI(workType.ExpectedArtifacts),
		}
	}
	return values, nil
}
