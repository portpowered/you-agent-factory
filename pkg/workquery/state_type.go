package workquery

import factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

// ValidWorkStateType reports whether stateType is an allowed work-list state.type filter.
func ValidWorkStateType(stateType factoryapi.WorkStateType) bool {
	switch stateType {
	case factoryapi.WorkStateTypeINITIAL,
		factoryapi.WorkStateTypePROCESSING,
		factoryapi.WorkStateTypeTERMINAL,
		factoryapi.WorkStateTypeFAILED:
		return true
	default:
		return false
	}
}
