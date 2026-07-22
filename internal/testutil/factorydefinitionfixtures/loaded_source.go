package factorydefinitionfixtures

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// LoadedSource is a representation-only Factory Definitions root fake. It is
// intentionally map-backed: consumer tests author the exact already-loaded
// value their subject needs and do not invoke Factory Definitions loading or
// composition implementations.
type LoadedSource struct {
	Directory    string
	RuntimeDir   string
	Config       *factorydefinitions.FactoryConfig
	Workers      map[string]*factorydefinitions.FactoryWorkerConfig
	Workstations map[string]*factorydefinitions.FactoryWorkstationConfig
	Replacements []factorydefinitions.PortableBundledFileReplacement
}

var _ factorydefinitions.MutableLoadedFactorySource = (*LoadedSource)(nil)

func newLoadedSourceValue(
	directory string,
	config *factorydefinitions.FactoryConfig,
) *LoadedSource {
	value := &LoadedSource{
		Directory:    directory,
		RuntimeDir:   directory,
		Config:       config,
		Workers:      make(map[string]*factorydefinitions.FactoryWorkerConfig),
		Workstations: make(map[string]*factorydefinitions.FactoryWorkstationConfig),
	}
	if config == nil {
		return value
	}
	for _, worker := range config.Workers {
		clone := factorydefinitions.CloneWorkerConfig(worker)
		value.Workers[clone.Name] = &clone
	}
	for _, workstation := range config.Workstations {
		clone := factorydefinitions.CloneWorkstationConfig(workstation)
		value.Workstations[clone.Name] = &clone
	}
	return value
}

// NewLoadedSourceWithRuntimeDefinitions returns an already-loaded root value
// for consumer tests that separately author effective worker/workstation
// definitions. The error result keeps it assignable to injected loader roles;
// the fixture performs no loading and therefore returns a nil error.
func NewLoadedSource(
	directory string,
	config *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
	replacements []factorydefinitions.PortableBundledFileReplacement,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	value := newLoadedSourceValue(directory, config)
	value.Replacements = append(value.Replacements, replacements...)
	if config == nil || runtimeDefinitions == nil {
		return value, nil
	}
	for _, worker := range config.Workers {
		if effective, ok := runtimeDefinitions.Worker(worker.Name); ok {
			clone := factorydefinitions.CloneWorkerConfig(*effective)
			value.Workers[clone.Name] = &clone
		}
	}
	for _, workstation := range config.Workstations {
		if effective, ok := runtimeDefinitions.Workstation(workstation.Name); ok {
			clone := factorydefinitions.CloneWorkstationConfig(*effective)
			value.Workstations[clone.Name] = &clone
		}
	}
	return value, nil
}

func (s *LoadedSource) FactoryDir() string { return s.Directory }

func (s *LoadedSource) RuntimeBaseDir() string {
	if s.RuntimeDir != "" {
		return s.RuntimeDir
	}
	return s.Directory
}

func (s *LoadedSource) SetRuntimeBaseDir(path string) { s.RuntimeDir = path }

func (s *LoadedSource) FactoryConfig() *factorydefinitions.FactoryConfig { return s.Config }

func (s *LoadedSource) Worker(name string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	worker, ok := s.Workers[name]
	return worker, ok
}

func (s *LoadedSource) Workstation(name string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	workstation, ok := s.Workstations[name]
	return workstation, ok
}

func (s *LoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return append([]factorydefinitions.PortableBundledFileReplacement(nil), s.Replacements...)
}

func (s *LoadedSource) MutateWorkers(mutate func(*factorydefinitions.FactoryWorkerConfig) error) error {
	for name, worker := range s.Workers {
		if err := mutate(worker); err != nil {
			return err
		}
		s.Workers[name] = worker
	}
	return nil
}
