package workpropagation

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestModeDefaultsAndTrimsExplicitPolicy(t *testing.T) {
	for _, workstation := range []*factorydefinitions.FactoryWorkstationConfig{nil, {}, {WorkPropagation: &factorydefinitions.WorkPropagationConfig{Mode: "  "}}} {
		if got := (Service{}).Mode(workstation); got != factorydefinitions.WorkPropagationModeOutputAsPayload {
			t.Fatalf("default mode = %q", got)
		}
	}
	if got := (Service{}).Mode(&factorydefinitions.FactoryWorkstationConfig{WorkPropagation: &factorydefinitions.WorkPropagationConfig{Mode: " explicit "}}); got != "explicit" {
		t.Fatalf("explicit mode = %q", got)
	}
}

func TestPolicy_WorkPropagation(t *testing.T) {
	t.Parallel()

	propagation := NewService()
	if got := propagation.Mode(nil); got != factorydefinitions.WorkPropagationModeOutputAsPayload {
		t.Fatalf("Mode(nil) = %q, want output_as_payload default", got)
	}

	workstation := &factorydefinitions.FactoryWorkstationConfig{
		WorkPropagation: &factorydefinitions.WorkPropagationConfig{
			Mode: factorydefinitions.WorkPropagationModePreserveInput,
		},
	}
	if got := propagation.Mode(workstation); got != factorydefinitions.WorkPropagationModePreserveInput {
		t.Fatalf("Mode() = %q, want preserve_input", got)
	}
}
