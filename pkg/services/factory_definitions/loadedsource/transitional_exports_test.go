package loadedsource_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/loadedsource"
)

func TestTransitionalExport_NewDelegatesToCompilationOwner(t *testing.T) {
	t.Parallel()

	source, err := loadedsource.New(
		"factory",
		&factorydefinitions.FactoryConfig{
			Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker"}},
		},
		emptyLookup{},
		nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if source.FactoryDir() != "factory" {
		t.Fatalf("FactoryDir = %q, want factory", source.FactoryDir())
	}
}

type emptyLookup struct{}

func (emptyLookup) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}
func (emptyLookup) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
