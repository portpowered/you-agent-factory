package internal

import (
	"fmt"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"strings"
)

// RuntimeSnapshotMaterializer reconstructs scoped definition data using fixed
// loaded-source behavior, without rereading authored files.
type RuntimeSnapshotMaterializer struct {
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory
}

func NewRuntimeSnapshotMaterializer(newLoadedFactory factorydefinitions.LoadedFactorySourceFactory) *RuntimeSnapshotMaterializer {
	return &RuntimeSnapshotMaterializer{newLoadedFactory: newLoadedFactory}
}

type invocationSensitiveLoadedFactory struct {
	factorydefinitions.MutableLoadedFactorySource
	pointers         []string
	promptProvenance []factorydefinitions.RuntimePromptProvenance
}

func (source *invocationSensitiveLoadedFactory) WorkerPromptSource(
	name string,
) (factorydefinitions.PromptSource, bool) {
	if source == nil || source.MutableLoadedFactorySource == nil {
		return factorydefinitions.PromptSource{}, false
	}
	lookup, ok := source.MutableLoadedFactorySource.(factorydefinitions.RuntimePromptSourceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.PromptSource{}, false
	}
	return lookup.WorkerPromptSource(name)
}

func (source *invocationSensitiveLoadedFactory) WorkstationPromptSource(
	name string,
) (factorydefinitions.PromptSource, bool) {
	if source == nil || source.MutableLoadedFactorySource == nil {
		return factorydefinitions.PromptSource{}, false
	}
	lookup, ok := source.MutableLoadedFactorySource.(factorydefinitions.RuntimePromptSourceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.PromptSource{}, false
	}
	return lookup.WorkstationPromptSource(name)
}

func (source *invocationSensitiveLoadedFactory) InvocationSensitiveJSONPointers() []string {
	if source == nil {
		return nil
	}
	return append([]string(nil), source.pointers...)
}

func (source *invocationSensitiveLoadedFactory) WorkerPromptProvenance(
	name string,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	return lookupRuntimePromptProvenance(source.MutableLoadedFactorySource, source.promptProvenance, name, true)
}

func (source *invocationSensitiveLoadedFactory) WorkstationPromptProvenance(
	name string,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	return lookupRuntimePromptProvenance(source.MutableLoadedFactorySource, source.promptProvenance, name, false)
}

type invocationSensitiveSpanLoadedFactory struct {
	factorydefinitions.MutableLoadedFactorySource
	spans            []factorydefinitions.InvocationSensitiveJSONSpan
	promptProvenance []factorydefinitions.RuntimePromptProvenance
}

func (source *invocationSensitiveSpanLoadedFactory) WorkerPromptSource(
	name string,
) (factorydefinitions.PromptSource, bool) {
	if source == nil || source.MutableLoadedFactorySource == nil {
		return factorydefinitions.PromptSource{}, false
	}
	lookup, ok := source.MutableLoadedFactorySource.(factorydefinitions.RuntimePromptSourceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.PromptSource{}, false
	}
	return lookup.WorkerPromptSource(name)
}

func (source *invocationSensitiveSpanLoadedFactory) WorkstationPromptSource(
	name string,
) (factorydefinitions.PromptSource, bool) {
	if source == nil || source.MutableLoadedFactorySource == nil {
		return factorydefinitions.PromptSource{}, false
	}
	lookup, ok := source.MutableLoadedFactorySource.(factorydefinitions.RuntimePromptSourceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.PromptSource{}, false
	}
	return lookup.WorkstationPromptSource(name)
}

func (source *invocationSensitiveSpanLoadedFactory) InvocationSensitiveJSONSpans() []factorydefinitions.InvocationSensitiveJSONSpan {
	if source == nil {
		return nil
	}
	return append([]factorydefinitions.InvocationSensitiveJSONSpan(nil), source.spans...)
}

func (source *invocationSensitiveSpanLoadedFactory) WorkerPromptProvenance(
	name string,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	return lookupRuntimePromptProvenance(source.MutableLoadedFactorySource, source.promptProvenance, name, true)
}

func (source *invocationSensitiveSpanLoadedFactory) WorkstationPromptProvenance(
	name string,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	return lookupRuntimePromptProvenance(source.MutableLoadedFactorySource, source.promptProvenance, name, false)
}

func lookupRuntimePromptProvenance(
	source factorydefinitions.MutableLoadedFactorySource,
	provenance []factorydefinitions.RuntimePromptProvenance,
	name string,
	worker bool,
) (factorydefinitions.RuntimePromptProvenance, bool) {
	for _, candidate := range provenance {
		if candidate.Name == name {
			return candidate, true
		}
	}
	lookup, ok := source.(factorydefinitions.RuntimePromptProvenanceLookup)
	if !ok || lookup == nil {
		return factorydefinitions.RuntimePromptProvenance{}, false
	}
	if worker {
		return lookup.WorkerPromptProvenance(name)
	}
	return lookup.WorkstationPromptProvenance(name)
}

func (m *RuntimeSnapshotMaterializer) Materialize(
	resolved *factorydefinitions.RuntimeSnapshot,
	executionBaseDir string,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved Factory Definition snapshot is required")
	}
	if m.newLoadedFactory == nil {
		return nil, fmt.Errorf("factory definitions loaded-source factory is required")
	}
	snapshot, err := resolved.Clone()
	if err != nil {
		return nil, fmt.Errorf("clone resolved Factory Definition snapshot: %w", err)
	}
	if strings.TrimSpace(snapshot.FactoryDir) == "" {
		return nil, fmt.Errorf("resolved Factory Definition snapshot directory is required")
	}
	if strings.TrimSpace(snapshot.EffectiveFactory.Name) == "" {
		return nil, fmt.Errorf("resolved Factory Definition snapshot Factory name is required")
	}
	lookup := newRuntimeSnapshotLookup(&snapshot)
	config := snapshot.EffectiveFactory
	attachRuntimeSnapshotPromptSources(&config, snapshot.PromptSources)
	loaded, err := m.newLoadedFactory(
		snapshot.FactoryDir,
		&config,
		lookup,
		snapshot.BundledFiles,
	)
	if err != nil {
		return nil, fmt.Errorf("build loaded Factory from resolved snapshot: %w", err)
	}
	if loaded == nil {
		return nil, fmt.Errorf("factory definitions loaded-source factory returned no source")
	}
	baseDir := strings.TrimSpace(executionBaseDir)
	if baseDir == "" {
		baseDir = snapshot.RuntimeBaseDir
	}
	loaded.SetRuntimeBaseDir(baseDir)
	if len(snapshot.InvocationSensitiveJSONSpans) > 0 {
		loaded = &invocationSensitiveSpanLoadedFactory{
			MutableLoadedFactorySource: loaded,
			spans:                      append([]factorydefinitions.InvocationSensitiveJSONSpan(nil), snapshot.InvocationSensitiveJSONSpans...),
			promptProvenance:           append([]factorydefinitions.RuntimePromptProvenance(nil), snapshot.PromptProvenance...),
		}
	} else if len(snapshot.InvocationSensitiveJSONPointers) > 0 || len(snapshot.PromptProvenance) > 0 {
		loaded = &invocationSensitiveLoadedFactory{
			MutableLoadedFactorySource: loaded,
			pointers:                   append([]string(nil), snapshot.InvocationSensitiveJSONPointers...),
			promptProvenance:           append([]factorydefinitions.RuntimePromptProvenance(nil), snapshot.PromptProvenance...),
		}
	}
	return loaded, nil
}

type runtimeSnapshotLookup struct {
	workers      map[string]*factorydefinitions.FactoryWorkerConfig
	workstations map[string]*factorydefinitions.FactoryWorkstationConfig
}

func newRuntimeSnapshotLookup(
	snapshot *factorydefinitions.RuntimeSnapshot,
) factorydefinitions.RuntimeDefinitionLookup {
	lookup := &runtimeSnapshotLookup{
		workers:      make(map[string]*factorydefinitions.FactoryWorkerConfig, len(snapshot.Workers)),
		workstations: make(map[string]*factorydefinitions.FactoryWorkstationConfig, len(snapshot.Workstations)),
	}
	for _, worker := range snapshot.Workers {
		cloned := factorydefinitions.CloneWorkerConfig(worker)
		lookup.workers[cloned.Name] = &cloned
	}
	for _, workstation := range snapshot.Workstations {
		cloned := factorydefinitions.CloneWorkstationConfig(workstation)
		lookup.workstations[cloned.Name] = &cloned
	}
	return lookup
}

func (lookup *runtimeSnapshotLookup) Worker(
	name string,
) (*factorydefinitions.FactoryWorkerConfig, bool) {
	if lookup == nil {
		return nil, false
	}
	worker, ok := lookup.workers[name]
	return worker, ok
}

func (lookup *runtimeSnapshotLookup) Workstation(
	name string,
) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	if lookup == nil {
		return nil, false
	}
	workstation, ok := lookup.workstations[name]
	return workstation, ok
}

func attachRuntimeSnapshotPromptSources(
	config *factorydefinitions.FactoryConfig,
	sources []factorydefinitions.RuntimePromptSource,
) {
	if config == nil {
		return
	}
	for _, source := range sources {
		if strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Path) == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(source.Role)) {
		case "worker":
			for index := range config.Workers {
				if config.Workers[index].Name == source.Name {
					config.Workers[index].PromptSourcePath = source.Path
					break
				}
			}
		case "workstation":
			for index := range config.Workstations {
				if config.Workstations[index].Name == source.Name {
					config.Workstations[index].PromptSourcePath = source.Path
					config.Workstations[index].PromptSourceIsTemplate = source.IsTemplate
					break
				}
			}
		}
	}
}
