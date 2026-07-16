package projections

import (
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func placeIDsFromGeneratedIOs(values []factoryapi.WorkstationIO) []string {
	if len(values) == 0 {
		return nil
	}
	ids := make([]string, 0, len(values))
	for _, value := range values {
		ids = append(ids, placeIDFromGeneratedIO(value))
	}
	return ids
}

func placeIDsFromGeneratedIOsPtr(values *[]factoryapi.WorkstationIO) []string {
	if values == nil {
		return nil
	}
	return placeIDsFromGeneratedIOs(*values)
}

func placeIDFromGeneratedIO(value factoryapi.WorkstationIO) string {
	return generatedPlaceID(value.WorkType, value.State)
}

func generatedPlaceID(workTypeID string, stateValue string) string {
	if workTypeID == "" || stateValue == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", workTypeID, stateValue)
}

func firstString(values *[]string) string {
	for _, value := range sliceValue(values) {
		if value != "" {
			return value
		}
	}
	return ""
}

func stringMapFromGenerated(values *factoryapi.StringMap) map[string]string {
	if values == nil {
		return nil
	}
	return cloneStringMap(map[string]string(*values))
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func generatedWorkStateName(value *factoryapi.WorkState) string {
	if value == nil {
		return ""
	}
	return value.Name
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func enumStringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func workstationKindString(value *factoryapi.WorkstationKind) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func intPtrValue(value *int) *int {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func sliceValue[T any](values *[]T) []T {
	if values == nil {
		return nil
	}
	return *values
}
