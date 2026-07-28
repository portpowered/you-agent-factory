package runtimeconfig_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/runtimeconfig"
)

func TestTransitionalExport_MergeDelegatesToCompilationOwner(t *testing.T) {
	t.Parallel()

	merged, err := runtimeconfig.Merge(
		&factorydefinitions.FactoryConfig{
			Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker"}},
		},
		emptyLookup{},
	)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if merged == nil || merged.Workers[0].Name != "worker" {
		t.Fatalf("Merge = %#v, want worker topology", merged)
	}
}

type emptyLookup struct{}

func (emptyLookup) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}
func (emptyLookup) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
