package workstationconfig

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

func TestWorkstationReturnsFalseForNilInputs(t *testing.T) {
	t.Parallel()

	runtimeConfig := runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {Name: "Review", Kind: interfaces.WorkstationTypeLogical, Type: interfaces.WorkstationTypeLogical},
		},
	}

	workstation, ok := Workstation(nil, runtimeConfig)
	if ok || workstation != nil {
		t.Fatalf("Workstation(nil, runtimeConfig) = (%#v, %t), want (nil, false)", workstation, ok)
	}

	workstation, ok = Workstation(&petri.Transition{Name: "review"}, nil)
	if ok || workstation != nil {
		t.Fatalf("Workstation(transition, nil) = (%#v, %t), want (nil, false)", workstation, ok)
	}
}

func TestWorkstationPrefersTransitionName(t *testing.T) {
	t.Parallel()

	runtimeConfig := runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"authored-name": {Name: "Authored Name", Kind: interfaces.WorkstationTypeLogical, Type: interfaces.WorkstationTypeLogical},
			"transition-id": {Name: "Transition ID", Kind: interfaces.WorkstationTypeModel, Type: interfaces.WorkstationTypeModel},
		},
	}

	workstation, ok := Workstation(&petri.Transition{Name: "authored-name", ID: "transition-id"}, runtimeConfig)
	if !ok || workstation == nil {
		t.Fatalf("Workstation(name-hit) = (%#v, %t), want configured workstation", workstation, ok)
	}
	if workstation.Name != "Authored Name" {
		t.Fatalf("Workstation(name-hit).Name = %q, want %q", workstation.Name, "Authored Name")
	}
}

func TestWorkstationFallsBackToIDWhenNameEmpty(t *testing.T) {
	t.Parallel()

	runtimeConfig := runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"transition-id": {Name: "Transition ID", Kind: interfaces.WorkstationTypeModel, Type: interfaces.WorkstationTypeModel},
		},
	}

	workstation, ok := Workstation(&petri.Transition{ID: "transition-id"}, runtimeConfig)
	if !ok || workstation == nil {
		t.Fatalf("Workstation(id-only) = (%#v, %t), want configured workstation", workstation, ok)
	}
	if workstation.Name != "Transition ID" {
		t.Fatalf("Workstation(id-only).Name = %q, want %q", workstation.Name, "Transition ID")
	}
}

func TestWorkstationFallsBackToDistinctIDAfterNameMiss(t *testing.T) {
	t.Parallel()

	runtimeConfig := runtimefixtures.RuntimeWorkstationLookupFixture{
		Workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"transition-id": {
				Name: "Transition ID",
				Kind: interfaces.WorkstationTypeModel,
				Type: interfaces.WorkstationTypeModel,
				Limits: interfaces.WorkstationLimits{
					MaxRetries: 4,
				},
			},
		},
	}

	transition := &petri.Transition{Name: "missing-name", ID: "transition-id"}

	workstation, ok := Workstation(transition, runtimeConfig)
	if !ok || workstation == nil {
		t.Fatalf("Workstation(name-miss-id-hit) = (%#v, %t), want configured workstation", workstation, ok)
	}
	if workstation.Name != "Transition ID" {
		t.Fatalf("Workstation(name-miss-id-hit).Name = %q, want %q", workstation.Name, "Transition ID")
	}
	if got := Kind(transition, runtimeConfig); got != interfaces.WorkstationTypeModel {
		t.Fatalf("Kind(name-miss-id-hit) = %q, want %q", got, interfaces.WorkstationTypeModel)
	}
	if got := MaxRetries(transition, runtimeConfig); got != 4 {
		t.Fatalf("MaxRetries(name-miss-id-hit) = %d, want %d", got, 4)
	}
}

func TestWorkstationKindAndMaxRetriesReturnZeroValuesWhenMissing(t *testing.T) {
	t.Parallel()

	runtimeConfig := runtimefixtures.RuntimeWorkstationLookupFixture{}
	transition := &petri.Transition{Name: "missing-name", ID: "missing-id"}

	workstation, ok := Workstation(transition, runtimeConfig)
	if ok || workstation != nil {
		t.Fatalf("Workstation(missing) = (%#v, %t), want (nil, false)", workstation, ok)
	}
	if got := Kind(transition, runtimeConfig); got != "" {
		t.Fatalf("Kind(missing) = %q, want empty kind", got)
	}
	if got := MaxRetries(transition, runtimeConfig); got != 0 {
		t.Fatalf("MaxRetries(missing) = %d, want 0", got)
	}
}
