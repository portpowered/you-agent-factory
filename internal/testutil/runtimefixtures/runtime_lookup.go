package runtimefixtures

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	// RuntimeWorkstationLookupFixture provides a narrow map-backed
	// RuntimeWorkstationLookup for tests.
)

type RuntimeWorkstationLookupFixture struct {
	Workstations map[string]*interfaces.FactoryWorkstationConfig
}

var _ interfaces.RuntimeWorkstationLookup = RuntimeWorkstationLookupFixture{}

func (f RuntimeWorkstationLookupFixture) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := f.Workstations[name]
	return workstation, ok
}

// RuntimeDefinitionLookupFixture provides a narrow map-backed
// RuntimeDefinitionLookup for tests.
type RuntimeDefinitionLookupFixture struct {
	Workstations map[string]*interfaces.FactoryWorkstationConfig
	Workers      map[string]*interfaces.FactoryWorkerConfig
	Factory      *interfaces.FactoryConfig
}

var _ interfaces.RuntimeDefinitionLookup = RuntimeDefinitionLookupFixture{}
var _ interfaces.RuntimeFactoryConfigLookup = RuntimeDefinitionLookupFixture{}

func (f RuntimeDefinitionLookupFixture) Worker(name string) (*interfaces.FactoryWorkerConfig, bool) {
	worker, ok := f.Workers[name]
	return worker, ok
}

func (f RuntimeDefinitionLookupFixture) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := f.Workstations[name]
	return workstation, ok
}

func (f RuntimeDefinitionLookupFixture) FactoryConfig() *interfaces.FactoryConfig {
	return f.Factory
}

// RuntimeConfigLookupFixture provides a narrow map-backed RuntimeConfigLookup
// for tests, with RuntimeBaseDir defaulting to FactoryDir when unset.
type RuntimeConfigLookupFixture struct {
	Workstations     map[string]*interfaces.FactoryWorkstationConfig
	WorkstationsByID map[string]*interfaces.FactoryWorkstationConfig
	Workers          map[string]*interfaces.FactoryWorkerConfig
	Factory          *interfaces.FactoryConfig
	FactoryPath      string
	RuntimeBasePath  string
}

var _ interfaces.RuntimeConfigLookup = RuntimeConfigLookupFixture{}
var _ interfaces.RuntimeFactoryConfigLookup = RuntimeConfigLookupFixture{}
var _ interfaces.ReplayRuntimeConfig = RuntimeConfigLookupFixture{}

func (f RuntimeConfigLookupFixture) FactoryDir() string {
	return f.FactoryPath
}

func (f RuntimeConfigLookupFixture) Worker(name string) (*interfaces.FactoryWorkerConfig, bool) {
	worker, ok := f.Workers[name]
	return worker, ok
}

func (f RuntimeConfigLookupFixture) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := f.Workstations[name]
	return workstation, ok
}

func (f RuntimeConfigLookupFixture) WorkstationByID(id string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := f.WorkstationsByID[id]
	return workstation, ok
}

func (f RuntimeConfigLookupFixture) RuntimeBaseDir() string {
	if f.RuntimeBasePath != "" {
		return f.RuntimeBasePath
	}
	return f.FactoryPath
}

func (f RuntimeConfigLookupFixture) FactoryConfig() *interfaces.FactoryConfig {
	return f.Factory
}

// ReplayRuntimeConfigValue indexes an already-decoded Factory Definition root
// value for tests that inject replay lookups at a consumer boundary. It does
// not reproduce snapshot decoding or replay policy; those remain covered by
// the Factory Definitions owner.
func ReplayRuntimeConfigValue(
	factory *interfaces.FactoryConfig,
	factoryDir string,
) RuntimeConfigLookupFixture {
	fixture := RuntimeConfigLookupFixture{
		Factory:          factory,
		FactoryPath:      factoryDir,
		Workers:          make(map[string]*interfaces.FactoryWorkerConfig),
		Workstations:     make(map[string]*interfaces.FactoryWorkstationConfig),
		WorkstationsByID: make(map[string]*interfaces.FactoryWorkstationConfig),
	}
	if factory == nil {
		return fixture
	}
	for _, worker := range factory.Workers {
		value := interfaces.CloneWorkerConfig(worker)
		fixture.Workers[value.Name] = &value
	}
	for _, workstation := range factory.Workstations {
		value := interfaces.CloneWorkstationConfig(workstation)
		fixture.Workstations[value.Name] = &value
		if value.ID != "" {
			fixture.WorkstationsByID[value.ID] = &value
		}
	}
	return fixture
}
