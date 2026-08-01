package loadedsource

import (
	"fmt"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/runtimeconfig"
)

// Source is the effective Factory Definition retained by a live runtime.
// Construction belongs below the service root; consumers use
// factorydefinitions.MutableLoadedFactorySource.
type Source struct {
	factoryDir                  string
	runtimeBaseDir              string
	factory                     *factorydefinitions.FactoryConfig
	workers                     map[string]*factorydefinitions.FactoryWorkerConfig
	workstations                map[string]*factorydefinitions.FactoryWorkstationConfig
	portableBundledReplacements []factorydefinitions.PortableBundledFileReplacement
}

var _ factorydefinitions.RuntimeConfigLookup = (*Source)(nil)
var _ factorydefinitions.RuntimeFactoryConfigLookup = (*Source)(nil)
var _ factorydefinitions.MutableLoadedFactorySource = (*Source)(nil)

// New constructs an effective loaded source from an authored Factory
// Definition and optional runtime definitions.
func New(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
	portableBundledReplacements []factorydefinitions.PortableBundledFileReplacement,
) (*Source, error) {
	if factoryConfig == nil {
		return &Source{
			factoryDir:                  factoryDir,
			portableBundledReplacements: cloneReplacements(portableBundledReplacements),
		}, nil
	}
	effectiveFactory, err := runtimeconfig.Merge(factoryConfig, runtimeDefinitions)
	if err != nil {
		return nil, err
	}
	if effectiveFactory == nil {
		return nil, fmt.Errorf("runtime Factory Definition merger returned nil")
	}

	loaded := &Source{
		factoryDir:                  factoryDir,
		factory:                     effectiveFactory,
		workers:                     make(map[string]*factorydefinitions.FactoryWorkerConfig, len(effectiveFactory.Workers)),
		workstations:                make(map[string]*factorydefinitions.FactoryWorkstationConfig, len(effectiveFactory.Workstations)),
		portableBundledReplacements: cloneReplacements(portableBundledReplacements),
	}
	for index := range effectiveFactory.Workers {
		worker := factorydefinitions.CloneWorkerConfig(effectiveFactory.Workers[index])
		loaded.workers[worker.Name] = &worker
	}
	for index := range effectiveFactory.Workstations {
		workstation := factorydefinitions.CloneWorkstationConfig(effectiveFactory.Workstations[index])
		loaded.workstations[workstation.Name] = &workstation
	}
	return loaded, nil
}

func (s *Source) FactoryDir() string {
	if s == nil {
		return ""
	}
	return s.factoryDir
}

func (s *Source) RuntimeBaseDir() string {
	if s == nil {
		return ""
	}
	if s.runtimeBaseDir != "" {
		return s.runtimeBaseDir
	}
	return s.factoryDir
}

func (s *Source) SetRuntimeBaseDir(dir string) {
	if s == nil {
		return
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		s.runtimeBaseDir = ""
		return
	}
	s.runtimeBaseDir = filepath.Clean(dir)
}

func (s *Source) FactoryConfig() *factorydefinitions.FactoryConfig {
	if s == nil {
		return nil
	}
	return s.factory
}

func (s *Source) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	if s == nil {
		return nil
	}
	return cloneReplacements(s.portableBundledReplacements)
}

func (s *Source) Worker(name string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	if s == nil {
		return nil, false
	}
	worker, ok := s.workers[name]
	return worker, ok
}

func (s *Source) Workstation(name string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if s == nil {
		return nil, false
	}
	workstation, ok := s.workstations[name]
	return workstation, ok
}

func (s *Source) MutateWorkers(
	mutate func(*factorydefinitions.FactoryWorkerConfig) error,
) error {
	if s == nil || s.factory == nil {
		return nil
	}
	if mutate == nil {
		return fmt.Errorf("worker mutator is required")
	}
	for index := range s.factory.Workers {
		if err := mutate(&s.factory.Workers[index]); err != nil {
			return err
		}
	}
	for name, worker := range s.workers {
		if worker == nil {
			continue
		}
		if err := mutate(worker); err != nil {
			return fmt.Errorf("worker %q: %w", name, err)
		}
	}
	return nil
}

func cloneReplacements(
	replacements []factorydefinitions.PortableBundledFileReplacement,
) []factorydefinitions.PortableBundledFileReplacement {
	return append([]factorydefinitions.PortableBundledFileReplacement(nil), replacements...)
}
