package validation

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type managedRuntimeDependencySpec struct {
	backend string
}

var managedRuntimeDependencySpecs = map[string]managedRuntimeDependencySpec{
	canonicalManagedRuntimeIdentity("OMNIVOICE_Q4_K_M"): {backend: "LLAMACPP"},
}

var supportedManagedRuntimeLoadPolicies = map[string]struct{}{
	"ON_DEMAND": {},
	"EAGER":     {},
}

func canonicalManagedRuntimeIdentity(model string) string {
	return strings.ToUpper(strings.TrimSpace(model))
}

func canonicalManagedRuntimeBackend(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func isKnownManagedRuntimeIdentity(model string) bool {
	_, ok := managedRuntimeDependencySpecs[canonicalManagedRuntimeIdentity(model)]
	return ok
}

func requiredBackendForManagedRuntime(model string) (string, bool) {
	spec, ok := managedRuntimeDependencySpecs[canonicalManagedRuntimeIdentity(model)]
	if !ok {
		return "", false
	}
	return spec.backend, true
}

// ManagedRuntimeDependencyTargets validates managed-runtime dependency declarations
// shared by packaged and authored factories.
func ManagedRuntimeDependencyTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}

	resourceByName := make(map[string]interfaces.ResourceConfig, len(cfg.Resources))
	var targets []Target
	for i, resource := range cfg.Resources {
		if strings.TrimSpace(resource.Name) == "" {
			continue
		}
		resourceByName[resource.Name] = resource
		targets = append(targets, managedRuntimeResourceTargets(i, resource)...)
	}
	for workerIndex, worker := range cfg.Workers {
		targets = append(targets, managedRuntimeWorkerTargets(workerIndex, worker, resourceByName)...)
	}
	return targets
}

func managedRuntimeResourceTargets(index int, resource interfaces.ResourceConfig) []Target {
	if strings.TrimSpace(resource.Type) != interfaces.ResourceTypeModel {
		return nil
	}
	basePath := fmt.Sprintf("resources[%d](%s)", index, resource.Name)
	modelIdentity := strings.TrimSpace(resource.Model)
	if modelIdentity == "" {
		return nil
	}
	if !isKnownManagedRuntimeIdentity(modelIdentity) {
		return []Target{managedRuntimeResourceTarget(
			CodeManagedRuntimeUnsupportedIdentity,
			resource.Name,
			basePath+".model",
			fmt.Sprintf("managed runtime identity %q is not supported in this environment", modelIdentity),
		)}
	}
	if requiredBackend, ok := requiredBackendForManagedRuntime(modelIdentity); ok {
		if canonicalManagedRuntimeBackend(resource.Backend) != requiredBackend {
			return []Target{managedRuntimeResourceTarget(
				CodeManagedRuntimeInvalidBackend,
				resource.Name,
				basePath+".backend",
				fmt.Sprintf(
					"managed runtime %q requires backend %q, got %q",
					modelIdentity,
					requiredBackend,
					strings.TrimSpace(resource.Backend),
				),
			)}
		}
	}
	if loadPolicy := strings.ToUpper(strings.TrimSpace(resource.LoadPolicy)); loadPolicy != "" {
		if _, ok := supportedManagedRuntimeLoadPolicies[loadPolicy]; !ok {
			return []Target{managedRuntimeResourceTarget(
				CodeManagedRuntimeInvalidLoadPolicy,
				resource.Name,
				basePath+".loadPolicy",
				fmt.Sprintf("managed runtime loadPolicy must be ON_DEMAND or EAGER, got %q", resource.LoadPolicy),
			)}
		}
	}
	return nil
}

func managedRuntimeWorkerTargets(
	workerIndex int,
	worker interfaces.WorkerConfig,
	resourceByName map[string]interfaces.ResourceConfig,
) []Target {
	if strings.TrimSpace(worker.Type) != interfaces.WorkerTypeModel {
		return nil
	}
	if strings.TrimSpace(worker.ModelLocality) != interfaces.ModelLocalityLocal {
		return nil
	}

	basePath := fmt.Sprintf("workers[%d](%s)", workerIndex, worker.Name)
	modelIdentity := strings.TrimSpace(worker.Model)
	if modelIdentity == "" {
		return []Target{managedRuntimeWorkerTarget(
			CodeManagedRuntimeWorkerMissingModel,
			worker.Name,
			basePath+".model",
			"LOCAL model workers require a managed runtime identity in model",
		)}
	}

	var matchedResource *interfaces.ResourceConfig
	for _, requirement := range worker.Resources {
		resource, ok := resourceByName[strings.TrimSpace(requirement.Name)]
		if !ok || strings.TrimSpace(resource.Type) != interfaces.ResourceTypeModel {
			continue
		}
		if canonicalManagedRuntimeIdentity(resource.Model) != canonicalManagedRuntimeIdentity(modelIdentity) {
			continue
		}
		copied := resource
		matchedResource = &copied
		break
	}
	if matchedResource == nil {
		return []Target{managedRuntimeWorkerTarget(
			CodeManagedRuntimeWorkerMissingDep,
			worker.Name,
			basePath+".resources",
			fmt.Sprintf(
				"LOCAL model worker %q requires a top-level MODEL resource for managed runtime %q",
				worker.Name,
				modelIdentity,
			),
		)}
	}
	if canonicalManagedRuntimeIdentity(matchedResource.Model) != canonicalManagedRuntimeIdentity(modelIdentity) {
		return []Target{managedRuntimeWorkerTarget(
			CodeManagedRuntimeWorkerModelMismatch,
			worker.Name,
			basePath+".model",
			fmt.Sprintf("worker model %q must match managed runtime resource model %q", modelIdentity, matchedResource.Model),
		)}
	}
	return nil
}

func managedRuntimeResourceTarget(code, resourceName, fieldPath, message string) Target {
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeResource,
			ID:       resourceName,
			Location: SubjectLocationDefinition,
		},
		Path: fmt.Sprintf("%s.%s", validationRoot, fieldPath),
	}
}

func managedRuntimeWorkerTarget(code, workerName, fieldPath, message string) Target {
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeWorker,
			ID:       workerName,
			Location: SubjectLocationReference,
		},
		Path: fmt.Sprintf("%s.%s", validationRoot, fieldPath),
	}
}
