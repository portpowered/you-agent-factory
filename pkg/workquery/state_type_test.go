package workquery

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestValidWorkStateType_AllowedValues(t *testing.T) {
	t.Parallel()

	allowed := []factoryapi.WorkStateType{
		factoryapi.WorkStateTypeINITIAL,
		factoryapi.WorkStateTypePROCESSING,
		factoryapi.WorkStateTypeTERMINAL,
		factoryapi.WorkStateTypeFAILED,
	}
	for _, stateType := range allowed {
		t.Run(string(stateType), func(t *testing.T) {
			t.Parallel()
			if !ValidWorkStateType(stateType) {
				t.Fatalf("ValidWorkStateType(%q) = false, want true", stateType)
			}
		})
	}
}

func TestValidWorkStateType_RejectedValue(t *testing.T) {
	t.Parallel()

	if ValidWorkStateType(factoryapi.WorkStateType("UNKNOWN")) {
		t.Fatal("ValidWorkStateType(UNKNOWN) = true, want false")
	}
}
