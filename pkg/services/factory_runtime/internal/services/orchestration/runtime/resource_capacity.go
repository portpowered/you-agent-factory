package runtime

import (
	"context"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

var (
	_ factory.ResourceCapacityService         = (*factoryImpl)(nil)
	_ factory.AdmittedResourceCapacityService = (*factoryImpl)(nil)
	_ factory.ResourceCapacityAdmission       = (*factoryImpl)(nil)
	_ factory.ResourceCapacityLeaseAdmission  = (*factoryImpl)(nil)
	_ factory.ResourceCapacityRevisionService = (*factoryImpl)(nil)
)

func (f *factoryImpl) AcquireResourceCapacityAdmission(ctx context.Context) (func(), error) {
	if f == nil || f.engine == nil {
		return nil, fmt.Errorf("Factory Runtime resource admission is unavailable")
	}
	return f.engine.AcquireResourceCapacityAdmission(ctx)
}

func (f *factoryImpl) AcquireResourceCapacityLease(
	ctx context.Context,
	request factory.ResourceCapacityLeaseRequest,
) (*factory.ResourceCapacityLease, error) {
	if f == nil || f.engine == nil {
		return nil, fmt.Errorf("Factory Runtime resource lease admission is unavailable")
	}
	return f.engine.AcquireResourceCapacityLease(ctx, request)
}

func (f *factoryImpl) CurrentFactoryRevision() int {
	if f == nil || f.engine == nil {
		return 0
	}
	return f.engine.CurrentFactoryRevision()
}

func (f *factoryImpl) SetFactoryRevision(revision int) {
	if f == nil || f.engine == nil {
		return
	}
	f.engine.SetFactoryRevision(revision)
}

func (f *factoryImpl) PreviewResourceCapacity(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.PreviewResourceCapacity(ctx, request)
	if err != nil {
		return result, err
	}
	return f.attachResourceCapacitySnapshot(result, false)
}

func (f *factoryImpl) PreviewResourceCapacityAdmitted(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.PreviewResourceCapacityAdmitted(ctx, request)
	if err != nil {
		return result, err
	}
	return f.attachResourceCapacitySnapshot(result, false)
}

func (f *factoryImpl) SetResourceCapacity(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.SetResourceCapacity(ctx, request)
	if err != nil {
		return result, err
	}
	result, err = f.attachResourceCapacitySnapshot(result, result.Outcome == factory.ResourceCapacityOutcomeApplied)
	if err != nil {
		return result, err
	}
	f.wakeAfterResourceCapacityChange(result)
	return result, nil
}

func (f *factoryImpl) SetResourceCapacityAdmitted(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.SetResourceCapacityAdmitted(ctx, request)
	if err != nil {
		return result, err
	}
	result, err = f.attachResourceCapacitySnapshot(result, result.Outcome == factory.ResourceCapacityOutcomeApplied)
	if err != nil {
		return result, err
	}
	f.wakeAfterResourceCapacityChange(result)
	return result, nil
}

func (f *factoryImpl) wakeAfterResourceCapacityChange(result factory.ResourceCapacityResult) {
	if f == nil || f.engine == nil || result.Outcome != factory.ResourceCapacityOutcomeApplied {
		return
	}
	f.engine.WakeForResourceCapacity()
}

func (f *factoryImpl) attachResourceCapacitySnapshot(result factory.ResourceCapacityResult, commit bool) (factory.ResourceCapacityResult, error) {
	if f == nil {
		return result, fmt.Errorf("Factory Runtime is unavailable")
	}
	f.capacitySnapshotMu.Lock()
	defer f.capacitySnapshotMu.Unlock()
	config, err := f.effectiveFactoryConfigForCapacityLocked()
	if err != nil {
		return result, err
	}
	for index := range config.Resources {
		if interfaces.CanonicalFactoryGraphResourceID(config.Resources[index]) != result.ResourceID {
			continue
		}
		config.Resources[index].Capacity = result.EffectiveCapacity
		if config.Resources[index].Name == "" {
			config.Resources[index].Name = result.ResourceName
		}
		break
	}
	if !hasResourceConfig(config, result.ResourceID) {
		config.Resources = append(config.Resources, interfaces.ResourceConfig{
			ID: result.ResourceID, Name: result.ResourceName, Capacity: result.EffectiveCapacity,
		})
	}
	snapshot, err := interfaces.NewFactorySnapshot(config)
	if err != nil {
		return result, fmt.Errorf("capture effective resource capacity Factory: %w", err)
	}
	if commit && result.Outcome == factory.ResourceCapacityOutcomeApplied {
		f.effectiveFactoryConfig = config
	}
	result.Factory = snapshot
	return result, nil
}

func (f *factoryImpl) effectiveFactoryConfigForCapacityLocked() (*interfaces.FactoryConfig, error) {
	if f.effectiveFactoryConfig != nil {
		cloned, err := interfaces.CloneFactoryConfig(f.effectiveFactoryConfig)
		if err != nil {
			return nil, fmt.Errorf("clone effective Factory: %w", err)
		}
		return cloned, nil
	}
	var source *interfaces.FactoryConfig
	if f.cfg != nil {
		source = factoryConfigFromFactoryConfig(f.cfg)
	}
	if source != nil {
		cloned, err := interfaces.CloneFactoryConfig(source)
		if err != nil {
			return nil, fmt.Errorf("clone effective Factory: %w", err)
		}
		return cloned, nil
	}
	name := ""
	if f.topology != nil {
		name = f.topology.ID
	}
	return &interfaces.FactoryConfig{Name: name}, nil
}

func hasResourceConfig(config *interfaces.FactoryConfig, resourceID string) bool {
	if config == nil {
		return false
	}
	for _, resource := range config.Resources {
		if interfaces.CanonicalFactoryGraphResourceID(resource) == resourceID {
			return true
		}
	}
	return false
}

// BindModelsRuntimeScope attaches the opened Models capability to this
// session's managed-model dispatches. The scope is a runtime-owned binding;
// Workers still owns inference selection and execution through Execute.
func (f *factoryImpl) BindModelsRuntimeScope(scope modelprovider.RuntimeScopeRef) error {
	if f == nil || f.cfg == nil {
		return fmt.Errorf("Factory Runtime is unavailable")
	}
	if scope.IsZero() {
		return modelprovider.ErrRuntimeScopeInvalid
	}
	f.cfg.modelRuntimeScope = scope
	return nil
}

// modelRuntimeInputForSelection projects the session-owned Models scope into
// the detached Workers request selected by Factory Runtime. Runtime does not
// invoke Models or choose a backend here; it only carries the opened scope and
// authored worker/resource facts to Workers, whose inference runner owns the
// local-vs-provider decision.
func modelRuntimeInputForSelection(
	cfg *runtimeConfig,
	selection runtimeExecutionSelection,
) *workers.ModelRuntimeInput {
	if cfg == nil || cfg.modelRuntimeScope.IsZero() ||
		!strings.EqualFold(
			strings.TrimSpace(selection.modelLocality),
			modelprovider.RuntimeModelLocalityLocal,
		) || strings.TrimSpace(selection.model) == "" {
		return nil
	}

	worker := modelprovider.LocalWorker{
		Name:          strings.TrimSpace(selection.workerName),
		Type:          strings.TrimSpace(selection.workerType),
		Model:         strings.TrimSpace(selection.model),
		ModelLocality: strings.TrimSpace(selection.modelLocality),
	}
	var resources []modelprovider.LocalResource
	if lookup, ok := runtimeDefinitionLookup(cfg); ok {
		if definition, found := lookup.Worker(worker.Name); found && definition != nil {
			worker.Resources = localResourcesFromFactory(definition.Resources)
		}
		if factoryLookup, found := lookup.(interfaces.RuntimeFactoryConfigLookup); found {
			if factoryConfig := factoryLookup.FactoryConfig(); factoryConfig != nil {
				resources = localResourcesFromFactory(factoryConfig.Resources)
			}
		}
	}

	return &workers.ModelRuntimeInput{
		Scope:     cfg.modelRuntimeScope,
		Worker:    worker,
		Resources: resources,
	}
}

func localResourcesFromFactory(
	resources []interfaces.ResourceConfig,
) []modelprovider.LocalResource {
	if len(resources) == 0 {
		return nil
	}
	projected := make([]modelprovider.LocalResource, len(resources))
	for index, resource := range resources {
		projected[index] = modelprovider.LocalResource{
			ID:         resource.ID,
			Name:       resource.Name,
			Type:       resource.Type,
			Capacity:   resource.Capacity,
			Model:      resource.Model,
			Backend:    resource.Backend,
			LoadPolicy: resource.LoadPolicy,
			Provider:   resource.Provider,
		}
	}
	return projected
}
