package service

import (
	"fmt"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

type supervisedIdentity struct {
	Name    string
	Backend string
}

type hostFailureClass string

const (
	hostFailureClassNone           hostFailureClass = ""
	hostFailureClassLoadingTimeout hostFailureClass = "loading_timeout"
	hostFailureClassProcessCrash   hostFailureClass = "process_crash"
	hostFailureClassCancelled      hostFailureClass = "cancelled"
)

func canonicalModelKey(modelName string) string {
	return strings.ToUpper(strings.TrimSpace(modelName))
}

func runtimeSlotKey(scope models.RuntimeScopeRef, modelName string) string {
	return scope.String() + "|" + canonicalModelKey(modelName)
}

func requiresSupervisedBackend(backend string) bool {
	return strings.ToUpper(strings.TrimSpace(backend)) == "LLAMACPP"
}

func localWorkerForModel(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (*models.RuntimeWorker, error) {
	if runtimeCfg == nil {
		return nil, fmt.Errorf("runtime config is not available")
	}
	target := canonicalModelKey(modelName)
	for _, worker := range runtimeCfg.Workers {
		if canonicalModelKey(worker.Model) != target {
			continue
		}
		if strings.TrimSpace(worker.ModelLocality) != models.RuntimeModelLocalityLocal {
			continue
		}
		copied := worker
		return &copied, nil
	}
	return nil, fmt.Errorf("local model worker not found for %q", modelName)
}

func modelBackend(runtimeCfg *models.RuntimeConfig, modelName string) string {
	resource := modelScopedResource(runtimeCfg, modelName)
	if resource == nil {
		return ""
	}
	return strings.TrimSpace(resource.Backend)
}

func modelScopedResource(factoryCfg *models.RuntimeConfig, modelName string) *models.RuntimeResource {
	if factoryCfg == nil {
		return nil
	}
	key := localmodels.CanonicalModelName(modelName)
	for _, resource := range factoryCfg.Resources {
		if strings.TrimSpace(resource.Type) != models.RuntimeResourceTypeModel {
			continue
		}
		if localmodels.CanonicalModelName(resource.Model) != key {
			continue
		}
		copied := resource
		return &copied
	}
	return nil
}

func supervisedIdentityForModel(runtimeCfg *models.RuntimeConfig, modelName string) supervisedIdentity {
	return supervisedIdentity{
		Name:    strings.TrimSpace(modelName),
		Backend: modelBackend(runtimeCfg, modelName),
	}
}

func cacheInspectionFromAssets(inspection scopedassets.RuntimeCacheInspection) cacheInspection {
	return cacheInspection{
		Supported:          inspection.Supported,
		Installed:          inspection.Installed,
		Revision:           inspection.Revision,
		CachePath:          inspection.CachePath,
		InstalledFileCount: inspection.InstalledFileCount,
		MissingAssets:      append([]string(nil), inspection.MissingAssets...),
		PartialArtifacts:   inspection.PartialArtifacts,
	}
}

type cacheInspection struct {
	Supported          bool
	Installed          bool
	Revision           string
	CachePath          string
	InstalledFileCount int
	MissingAssets      []string
	PartialArtifacts   bool
}

func defaultServerStartBuilder(
	identity supervisedIdentity,
	inspection cacheInspection,
	worker *models.RuntimeWorker,
) (models.HostProcessStartSpec, error) {
	if worker == nil {
		return models.HostProcessStartSpec{}, fmt.Errorf(
			"local model worker is required for supervised backend %q",
			identity.Backend,
		)
	}
	command := strings.TrimSpace(worker.Command)
	if command == "" {
		command = localmodels.DefaultOmniVoiceCommand
	}
	healthEndpoint, args, err := supervisedHealthEndpointAndArgs(worker.Args)
	if err != nil {
		return models.HostProcessStartSpec{}, err
	}
	if strings.TrimSpace(inspection.CachePath) == "" {
		return models.HostProcessStartSpec{}, fmt.Errorf(
			"%w: cache path is required for supervised runtime %q",
			models.ErrHostMissingAssets,
			identity.Name,
		)
	}
	args = append([]string{"serve"}, args...)
	args = append(args, "--cache-path", inspection.CachePath)
	return models.HostProcessStartSpec{
		Command:        command,
		Args:           args,
		HealthEndpoint: healthEndpoint,
	}, nil
}

func supervisedHealthEndpointAndArgs(workerArgs []string) (string, []string, error) {
	args := append([]string(nil), workerArgs...)
	for i := 0; i < len(args); i++ {
		if args[i] != supervisedHealthEndpointFlag {
			continue
		}
		if i+1 >= len(args) {
			return "", nil, fmt.Errorf("flag %q requires a value", supervisedHealthEndpointFlag)
		}
		endpoint := strings.TrimSpace(args[i+1])
		remaining := append(append([]string(nil), args[:i]...), args[i+2:]...)
		if endpoint == "" {
			return "", nil, fmt.Errorf("flag %q requires a non-empty value", supervisedHealthEndpointFlag)
		}
		return endpoint, remaining, nil
	}
	return "", args, fmt.Errorf(
		"supervised llama.cpp runtime requires worker arg %q",
		supervisedHealthEndpointFlag,
	)
}

func workerDeclaresSupervisedHealthEndpoint(worker *models.RuntimeWorker) bool {
	if worker == nil {
		return false
	}
	_, _, err := supervisedHealthEndpointAndArgs(worker.Args)
	return err == nil
}

func cancelHostError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", models.ErrHostCancelled, err)
}
