package loadedsource

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/runtimeconfig"
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
	workerPromptSources         map[string]factorydefinitions.PromptSource
	workstationPromptSources    map[string]factorydefinitions.PromptSource
	portableBundledReplacements []factorydefinitions.PortableBundledFileReplacement
	activation                  *factorydefinitions.FactoryActivationProvenance
	authoredSourceComparison    factorydefinitions.AuthoredSourceComparison
	loadedFactoryVersion        *factorydefinitions.FactoryVersion
}

var _ factorydefinitions.RuntimeConfigLookup = (*Source)(nil)
var _ factorydefinitions.RuntimeFactoryConfigLookup = (*Source)(nil)
var _ factorydefinitions.RuntimePromptSourceLookup = (*Source)(nil)
var _ factorydefinitions.MutableLoadedFactorySource = (*Source)(nil)
var _ factorydefinitions.LoadedFactoryActivationSource = (*Source)(nil)
var _ factorydefinitions.LoadedFactoryAuthoredSourceComparator = (*Source)(nil)
var _ factorydefinitions.LoadedFactoryVersionSource = (*Source)(nil)
var _ factorydefinitions.LoadedFactorySourceMetadataSetter = (*Source)(nil)

// New constructs an effective loaded source from an authored Factory
// Definition and optional runtime definitions.
func New(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
	portableBundledReplacements []factorydefinitions.PortableBundledFileReplacement,
	activationID func() string,
) (*Source, error) {
	if factoryConfig == nil {
		return &Source{
			factoryDir:                  factoryDir,
			portableBundledReplacements: cloneReplacements(portableBundledReplacements),
		}, nil
	}
	if activationID == nil {
		return nil, fmt.Errorf("Factory activation ID generator is required")
	}
	effectiveFactory, err := runtimeconfig.Merge(factoryConfig, runtimeDefinitions)
	if err != nil {
		return nil, err
	}
	if effectiveFactory == nil {
		return nil, fmt.Errorf("runtime Factory Definition merger returned nil")
	}
	loadedSourceDigest, err := LoadedSourceDigest(effectiveFactory)
	if err != nil {
		return nil, fmt.Errorf("compute loaded Factory source digest: %w", err)
	}

	loaded := &Source{
		factoryDir:                  factoryDir,
		factory:                     effectiveFactory,
		workers:                     make(map[string]*factorydefinitions.FactoryWorkerConfig, len(effectiveFactory.Workers)),
		workstations:                make(map[string]*factorydefinitions.FactoryWorkstationConfig, len(effectiveFactory.Workstations)),
		workerPromptSources:         make(map[string]factorydefinitions.PromptSource),
		workstationPromptSources:    make(map[string]factorydefinitions.PromptSource),
		portableBundledReplacements: cloneReplacements(portableBundledReplacements),
		activation: &factorydefinitions.FactoryActivationProvenance{
			ActivationID:       activationID(),
			LoadedSourceDigest: loadedSourceDigest,
			State:              factorydefinitions.FactoryActivationStateNotActivated,
		},
		loadedFactoryVersion: cloneFactoryVersion(effectiveFactory.Version),
	}
	for index := range effectiveFactory.Workers {
		worker := &effectiveFactory.Workers[index]
		if worker.PromptSourcePath != "" {
			loaded.workerPromptSources[worker.Name] = factorydefinitions.PromptSource{
				Path: worker.PromptSourcePath,
			}
			worker.PromptSourcePath = ""
		}
	}
	for index := range effectiveFactory.Workstations {
		workstation := &effectiveFactory.Workstations[index]
		if workstation.PromptSourcePath != "" {
			loaded.workstationPromptSources[workstation.Name] = factorydefinitions.PromptSource{
				Path:       workstation.PromptSourcePath,
				IsTemplate: workstation.PromptSourceIsTemplate,
			}
			workstation.PromptSourcePath = ""
			workstation.PromptSourceIsTemplate = false
		}
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

// LoadedSourceDigest returns the deterministic, secret-safe identity of the
// effective Factory and its resolved runtime instruction content. Runtime-only
// source paths are excluded so moving a loaded source does not change its
// content identity.
func LoadedSourceDigest(factoryConfig *factorydefinitions.FactoryConfig) (string, error) {
	if factoryConfig == nil {
		return "", fmt.Errorf("factory config is required")
	}
	detached, err := factorydefinitions.CloneFactoryConfig(factoryConfig)
	if err != nil {
		return "", err
	}
	clearRuntimeSourcePaths(detached)
	encoded, err := json.Marshal(detached)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// EffectiveLoadedSourceDigest applies the same runtime-definition merge used
// by Source and computes the resulting immutable-source identity.
func EffectiveLoadedSourceDigest(
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeDefinitions factorydefinitions.RuntimeDefinitionLookup,
) (string, error) {
	effective, err := runtimeconfig.Merge(factoryConfig, runtimeDefinitions)
	if err != nil {
		return "", err
	}
	return LoadedSourceDigest(effective)
}

func clearRuntimeSourcePaths(factoryConfig *factorydefinitions.FactoryConfig) {
	if factoryConfig == nil {
		return
	}
	for index := range factoryConfig.Workers {
		factoryConfig.Workers[index].PromptSourcePath = ""
	}
	for index := range factoryConfig.Workstations {
		factoryConfig.Workstations[index].PromptSourcePath = ""
		factoryConfig.Workstations[index].PromptSourceIsTemplate = false
	}
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

// FactoryActivationProvenance returns a detached identity for the immutable
// source accepted by this loaded runtime.
func (s *Source) FactoryActivationProvenance() *factorydefinitions.FactoryActivationProvenance {
	if s == nil || s.activation == nil {
		return nil
	}
	provenance := *s.activation
	return &provenance
}

// CompareAuthoredSource returns the safe authored-source comparison state. A
// source without a loader-installed comparison is intentionally unproven.
func (s *Source) CompareAuthoredSource() (factorydefinitions.FactoryActivationState, error) {
	if s == nil || s.authoredSourceComparison == nil {
		return factorydefinitions.FactoryActivationStateNotActivated, nil
	}
	state, err := s.authoredSourceComparison()
	if err != nil {
		return "", err
	}
	switch state {
	case factorydefinitions.FactoryActivationStateActive,
		factorydefinitions.FactoryActivationStateAuthoredChanged,
		factorydefinitions.FactoryActivationStateNotActivated,
		factorydefinitions.FactoryActivationStateAuthoredSourceUnavailable:
		return state, nil
	default:
		return "", fmt.Errorf("unknown Factory activation state %q", state)
	}
}

// SetAuthoredSourceComparison attaches the read-only comparison selected by
// the Definitions loader before this source is published to a runtime.
func (s *Source) SetAuthoredSourceComparison(comparison factorydefinitions.AuthoredSourceComparison) {
	if s == nil {
		return
	}
	s.authoredSourceComparison = comparison
}

// LoadedFactoryVersion returns the version observed when this source was
// loaded, detached from the source's mutable configuration.
func (s *Source) LoadedFactoryVersion() *factorydefinitions.FactoryVersion {
	if s == nil {
		return nil
	}
	return cloneFactoryVersion(s.loadedFactoryVersion)
}

// SetLoadedFactoryVersion records the version observed during source loading.
func (s *Source) SetLoadedFactoryVersion(version *factorydefinitions.FactoryVersion) {
	if s == nil {
		return
	}
	s.loadedFactoryVersion = cloneFactoryVersion(version)
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

func (s *Source) WorkerPromptSource(name string) (factorydefinitions.PromptSource, bool) {
	if s == nil {
		return factorydefinitions.PromptSource{}, false
	}
	source, ok := s.workerPromptSources[name]
	return source, ok
}

func (s *Source) WorkstationPromptSource(name string) (factorydefinitions.PromptSource, bool) {
	if s == nil {
		return factorydefinitions.PromptSource{}, false
	}
	source, ok := s.workstationPromptSources[name]
	return source, ok
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

func cloneFactoryVersion(version *factorydefinitions.FactoryVersion) *factorydefinitions.FactoryVersion {
	if version == nil {
		return nil
	}
	cloned := *version
	return &cloned
}
