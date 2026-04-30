package testutil

import "github.com/portpowered/agent-factory/pkg/interfaces"

// RuntimeWorkstationLookupFixture provides a narrow map-backed
// RuntimeWorkstationLookup for tests.
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
	RuntimeWorkstationLookupFixture
	Workers map[string]*interfaces.WorkerConfig
}

var _ interfaces.RuntimeDefinitionLookup = RuntimeDefinitionLookupFixture{}

func (f RuntimeDefinitionLookupFixture) Worker(name string) (*interfaces.WorkerConfig, bool) {
	worker, ok := f.Workers[name]
	return worker, ok
}

// RuntimeConfigLookupFixture provides a narrow map-backed RuntimeConfigLookup
// for tests, with RuntimeBaseDir defaulting to FactoryDir when unset.
type RuntimeConfigLookupFixture struct {
	RuntimeDefinitionLookupFixture
	FactoryPath     string
	RuntimeBasePath string
}

var _ interfaces.RuntimeConfigLookup = RuntimeConfigLookupFixture{}

func (f RuntimeConfigLookupFixture) FactoryDir() string {
	return f.FactoryPath
}

func (f RuntimeConfigLookupFixture) RuntimeBaseDir() string {
	if f.RuntimeBasePath != "" {
		return f.RuntimeBasePath
	}
	return f.FactoryPath
}
