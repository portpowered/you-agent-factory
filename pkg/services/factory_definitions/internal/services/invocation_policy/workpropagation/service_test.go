package workpropagation

import (
	"testing"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestModeDefaultsAndTrimsExplicitPolicy(t *testing.T) {
	for _, workstation := range []*definitions.FactoryWorkstationConfig{nil, {}, {WorkPropagation: &definitions.WorkPropagationConfig{Mode: "  "}}} {
		if got := (Service{}).Mode(workstation); got != definitions.WorkPropagationModeOutputAsPayload {
			t.Fatalf("default mode = %q", got)
		}
	}
	if got := (Service{}).Mode(&definitions.FactoryWorkstationConfig{WorkPropagation: &definitions.WorkPropagationConfig{Mode: " explicit "}}); got != "explicit" {
		t.Fatalf("explicit mode = %q", got)
	}
}
