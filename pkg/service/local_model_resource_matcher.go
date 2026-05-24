package service

import "github.com/portpowered/infinite-you/pkg/interfaces"

type localModelResourceMatch struct {
	requirement interfaces.ResourceConfig
	resource    interfaces.ResourceConfig
	key         string
}

func eligibleLocalModelResourceMatches(factoryCfg *interfaces.FactoryConfig, workerDef *interfaces.WorkerConfig) []localModelResourceMatch {
	if factoryCfg == nil || workerDef == nil || workerDef.ModelLocality != interfaces.ModelLocalityLocal {
		return nil
	}
	if len(workerDef.Resources) == 0 {
		return nil
	}

	resourcesByName := make(map[string]interfaces.ResourceConfig, len(factoryCfg.Resources))
	for _, resource := range factoryCfg.Resources {
		resourcesByName[resource.Name] = resource
	}

	var matches []localModelResourceMatch
	for _, requirement := range workerDef.Resources {
		resource, ok := resourcesByName[requirement.Name]
		if !ok || !isProcessScopedLocalModelResource(resource) {
			continue
		}
		if canonicalModelName(resource.Model) != canonicalModelName(workerDef.Model) {
			continue
		}
		key := localModelResourceKey(resource)
		if key == "" {
			continue
		}
		matches = append(matches, localModelResourceMatch{
			requirement: requirement,
			resource:    resource,
			key:         key,
		})
	}
	return matches
}
