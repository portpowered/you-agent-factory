package runtimeopening

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type runtimeConfigLookupStub struct {
	factoryDir string
	baseDir    string
	cfg        *factorydefinitions.FactoryConfig
}

func (s runtimeConfigLookupStub) FactoryConfig() *factorydefinitions.FactoryConfig { return s.cfg }
func (s runtimeConfigLookupStub) FactoryDir() string                               { return s.factoryDir }
func (s runtimeConfigLookupStub) RuntimeBaseDir() string                           { return s.baseDir }
func (s runtimeConfigLookupStub) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}
func (s runtimeConfigLookupStub) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

func TestProjectModelsRuntimeConfigReturnsNilWithoutFactoryConfig(t *testing.T) {
	t.Parallel()

	if got := ProjectModelsRuntimeConfig(nil); got != nil {
		t.Fatalf("ProjectModelsRuntimeConfig(nil) = %#v, want nil", got)
	}
	if got := ProjectModelsRuntimeConfig(runtimeConfigLookupStub{}); got != nil {
		t.Fatalf("ProjectModelsRuntimeConfig(empty lookup) = %#v, want nil", got)
	}
}

func TestProjectModelsRuntimeConfigProjectsWorkersResourcesAndOperations(t *testing.T) {
	t.Parallel()

	source := runtimeConfigLookupStub{
		factoryDir: "/factory",
		baseDir:    "/runtime",
		cfg: &factorydefinitions.FactoryConfig{
			Workers: []factorydefinitions.FactoryWorkerConfig{
				{
					Name:          "worker-a",
					Type:          "model",
					Model:         "fixture-model",
					ModelProvider: "mock",
					ModelLocality: "local",
					Command:       "echo",
					Args:          []string{"hello"},
					Operations: []factorydefinitions.ModelOperation{
						{
							Name: "invoke",
							Inputs: []factorydefinitions.ModelOperationSlot{
								{Name: "text", ContentTypes: []string{"text/plain"}, Required: true},
							},
							Outputs: []factorydefinitions.ModelOperationSlot{
								{Name: "result", ContentTypes: []string{"text/plain"}},
							},
						},
					},
					Resources: []factorydefinitions.ResourceConfig{
						{ID: "gpu", Name: "GPU", Type: "device", Capacity: 1},
					},
				},
			},
			Resources: []factorydefinitions.ResourceConfig{
				{ID: "shared", Name: "Shared", Type: "pool", Capacity: 2, Provider: "mock"},
			},
		},
	}

	projected := ProjectModelsRuntimeConfig(source)
	if projected == nil {
		t.Fatal("ProjectModelsRuntimeConfig() = nil, want projected runtime config")
	}
	if projected.FactoryDirectory != "/factory" || projected.BaseDirectory != "/runtime" {
		t.Fatalf(
			"projected directories = (%q, %q), want (/factory, /runtime)",
			projected.FactoryDirectory,
			projected.BaseDirectory,
		)
	}
	if len(projected.Workers) != 1 || projected.Workers[0].Name != "worker-a" {
		t.Fatalf("projected workers = %#v, want one worker-a entry", projected.Workers)
	}
	if len(projected.Workers[0].Operations) != 1 || projected.Workers[0].Operations[0].Name != "invoke" {
		t.Fatalf("projected operations = %#v, want invoke operation", projected.Workers[0].Operations)
	}
	if len(projected.Workers[0].Resources) != 1 || projected.Workers[0].Resources[0].ID != "gpu" {
		t.Fatalf("projected worker resources = %#v, want gpu resource", projected.Workers[0].Resources)
	}
	if len(projected.Resources) != 1 || projected.Resources[0].ID != "shared" {
		t.Fatalf("projected resources = %#v, want shared resource", projected.Resources)
	}
}
