package workpropagation

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestModeDefaultsToOutputAsPayload(t *testing.T) {
	t.Parallel()

	service := NewService()
	for _, workstation := range []*factorydefinitions.FactoryWorkstationConfig{
		nil,
		{},
		{WorkPropagation: &factorydefinitions.WorkPropagationConfig{}},
	} {
		if got := service.Mode(workstation); got != factorydefinitions.WorkPropagationModeOutputAsPayload {
			t.Fatalf("Mode() = %q, want %q", got, factorydefinitions.WorkPropagationModeOutputAsPayload)
		}
	}
}

func TestModeReturnsAuthoredMode(t *testing.T) {
	t.Parallel()

	workstation := &factorydefinitions.FactoryWorkstationConfig{
		WorkPropagation: &factorydefinitions.WorkPropagationConfig{
			Mode: factorydefinitions.WorkPropagationModePreserveInput,
		},
	}
	if got := NewService().Mode(workstation); got != factorydefinitions.WorkPropagationModePreserveInput {
		t.Fatalf("Mode() = %q, want %q", got, factorydefinitions.WorkPropagationModePreserveInput)
	}
}
