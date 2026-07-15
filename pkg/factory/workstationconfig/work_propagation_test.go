package workstationconfig

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestWorkPropagationMode_DefaultsToOutputAsPayload(t *testing.T) {
	if got := WorkPropagationMode(nil); got != interfaces.WorkPropagationModeOutputAsPayload {
		t.Fatalf("mode = %q, want %q", got, interfaces.WorkPropagationModeOutputAsPayload)
	}
	if got := WorkPropagationMode(&interfaces.FactoryWorkstationConfig{}); got != interfaces.WorkPropagationModeOutputAsPayload {
		t.Fatalf("mode = %q, want %q", got, interfaces.WorkPropagationModeOutputAsPayload)
	}
}

func TestWorkPropagationMode_ReturnsAuthoredMode(t *testing.T) {
	workstation := &interfaces.FactoryWorkstationConfig{
		WorkPropagation: &interfaces.WorkPropagationConfig{
			Mode: interfaces.WorkPropagationModePreserveInput,
		},
	}
	if got := WorkPropagationMode(workstation); got != interfaces.WorkPropagationModePreserveInput {
		t.Fatalf("mode = %q, want %q", got, interfaces.WorkPropagationModePreserveInput)
	}
}
