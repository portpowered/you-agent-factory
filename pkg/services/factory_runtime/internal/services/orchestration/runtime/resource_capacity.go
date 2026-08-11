package runtime

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

var (
	_ factory.ResourceCapacityService         = (*factoryImpl)(nil)
	_ factory.AdmittedResourceCapacityService = (*factoryImpl)(nil)
	_ factory.ResourceCapacityAdmission       = (*factoryImpl)(nil)
)

func (f *factoryImpl) AcquireResourceCapacityAdmission(ctx context.Context) (func(), error) {
	if f == nil || f.engine == nil {
		return nil, fmt.Errorf("Factory Runtime resource admission is unavailable")
	}
	return f.engine.AcquireResourceCapacityAdmission(ctx)
}

func (f *factoryImpl) PreviewResourceCapacity(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.PreviewResourceCapacity(ctx, request)
	if err != nil {
		return result, err
	}
	return f.attachResourceCapacitySnapshot(result)
}

func (f *factoryImpl) PreviewResourceCapacityAdmitted(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.PreviewResourceCapacityAdmitted(ctx, request)
	if err != nil {
		return result, err
	}
	return f.attachResourceCapacitySnapshot(result)
}

func (f *factoryImpl) SetResourceCapacity(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.SetResourceCapacity(ctx, request)
	if err != nil {
		return result, err
	}
	result, err = f.attachResourceCapacitySnapshot(result)
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
	result, err = f.attachResourceCapacitySnapshot(result)
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

func (f *factoryImpl) attachResourceCapacitySnapshot(result factory.ResourceCapacityResult) (factory.ResourceCapacityResult, error) {
	config, err := f.effectiveFactoryConfigForCapacity()
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
	result.Factory = snapshot
	return result, nil
}

func (f *factoryImpl) effectiveFactoryConfigForCapacity() (*interfaces.FactoryConfig, error) {
	if f == nil {
		return nil, fmt.Errorf("Factory Runtime is unavailable")
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
