package service

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

type supervisedIdentity struct {
	Name       string
	Backend    string
	LoadPolicy string
	Revision   string
}

type hostFailureClass string

const (
	hostFailureClassNone                hostFailureClass = ""
	hostFailureClassLoadingTimeout      hostFailureClass = "loading_timeout"
	hostFailureClassProcessCrash        hostFailureClass = "process_crash"
	hostFailureClassCancelled           hostFailureClass = "cancelled"
	hostFailureClassProtocol            hostFailureClass = "protocol_incompatible"
	hostFailureClassUnsupportedPlatform hostFailureClass = "unsupported_platform"
)

func canonicalModelKey(modelName string) string {
	return strings.ToUpper(strings.TrimSpace(modelName))
}

func runtimeSlotKey(scope models.RuntimeScopeRef, modelName string) string {
	return scope.String() + "|" + canonicalModelKey(modelName)
}

func requiresSupervisedBackend(backend string) bool {
	return models.IsManagedRuntimeBackend(backend)
}

func requiresRuntimeHostBackend(backend string) bool {
	return requiresSupervisedBackend(backend) || requiresPinnedGRPCBackend(backend)
}

func requiresPinnedGRPCBackend(backend string) bool {
	canonical := strings.ToLower(strings.TrimSpace(backend))
	return strings.HasPrefix(canonical, "localai-") ||
		canonical == "localai" || canonical == "localai_grpc" || canonical == "localai-grpc"
}

func localWorkerForModel(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (*models.RuntimeWorker, error) {
	target := canonicalModelKey(modelName)
	if runtimeCfg != nil {
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
	}
	if definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(modelName); ok {
		return &models.RuntimeWorker{
			Name:          definition.Name,
			Type:          models.RuntimeWorkerTypeInference,
			Model:         definition.Name,
			ModelLocality: models.RuntimeModelLocalityLocal,
		}, nil
	}
	if runtimeCfg == nil {
		return nil, fmt.Errorf("runtime config is not available")
	}
	return nil, fmt.Errorf("local model worker not found for %q", modelName)
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

func supervisedIdentityForModel(
	runtimeCfg *models.RuntimeConfig,
	overlays map[string]models.ModelOverlay,
	modelName string,
) supervisedIdentity {
	identity := supervisedIdentity{Name: strings.TrimSpace(modelName)}
	if definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(modelName); ok {
		identity.Backend = strings.TrimSpace(definition.Backend)
		identity.LoadPolicy = string(definition.LoadPolicy)
	}
	if resource := modelScopedResource(runtimeCfg, modelName); resource != nil {
		if backend := strings.TrimSpace(resource.Backend); backend != "" {
			identity.Backend = backend
		}
		if loadPolicy := strings.ToUpper(strings.TrimSpace(resource.LoadPolicy)); loadPolicy != "" {
			identity.LoadPolicy = loadPolicy
		}
	}
	if overlay, ok := modelOverlay(overlays, modelName); ok {
		if overlay.Backend != nil {
			identity.Backend = strings.TrimSpace(*overlay.Backend)
		}
		if overlay.LoadPolicy != nil {
			identity.LoadPolicy = strings.ToUpper(strings.TrimSpace(string(*overlay.LoadPolicy)))
		}
	}
	return supervisedIdentity{
		Name:       identity.Name,
		Backend:    identity.Backend,
		LoadPolicy: identity.LoadPolicy,
		Revision:   identity.Revision,
	}
}

func modelOverlay(
	overlays map[string]models.ModelOverlay,
	modelName string,
) (models.ModelOverlay, bool) {
	canonical := strings.ToLower(strings.TrimSpace(modelName))
	matching := make([]string, 0, 1)
	for name := range overlays {
		if strings.ToLower(strings.TrimSpace(name)) == canonical {
			matching = append(matching, name)
		}
	}
	if len(matching) == 0 {
		return models.ModelOverlay{}, false
	}
	sort.Strings(matching)
	return overlays[matching[0]].Clone(), true
}

func cacheInspectionFromAssets(inspection scopedassets.RuntimeCacheInspection) cacheInspection {
	return cacheInspection{
		Supported:             inspection.Supported,
		Installed:             inspection.Installed,
		Revision:              inspection.Revision,
		CachePath:             inspection.CachePath,
		InstalledFileCount:    inspection.InstalledFileCount,
		MissingAssets:         append([]string(nil), inspection.MissingAssets...),
		PartialArtifacts:      inspection.PartialArtifacts,
		BackendRequired:       inspection.BackendRequired,
		BackendCachePath:      inspection.BackendCachePath,
		BackendRevision:       inspection.BackendRevision,
		BackendInstalledFiles: inspection.BackendInstalledFiles,
		BackendFiles:          append([]string(nil), inspection.BackendFiles...),
		ObservedArtifacts:     append([]models.AssetArtifact(nil), inspection.ObservedArtifacts...),
	}
}

type cacheInspection struct {
	Supported             bool
	Installed             bool
	Revision              string
	CachePath             string
	InstalledFileCount    int
	MissingAssets         []string
	PartialArtifacts      bool
	BackendRequired       bool
	BackendCachePath      string
	BackendRevision       string
	BackendInstalledFiles int
	BackendFiles          []string
	ObservedArtifacts     []models.AssetArtifact
}

func defaultServerStartBuilder(
	identity supervisedIdentity,
	inspection cacheInspection,
	worker *models.RuntimeWorker,
) (modelseffects.HostProcessStartSpec, error) {
	if worker == nil {
		return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
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
		return modelseffects.HostProcessStartSpec{}, err
	}
	if strings.TrimSpace(inspection.CachePath) == "" {
		return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
			"%w: cache path is required for supervised runtime %q",
			models.ErrHostMissingAssets,
			identity.Name,
		)
	}
	args = append([]string{"serve"}, args...)
	args = append(args, "--cache-path", inspection.CachePath)
	if inspection.BackendRequired {
		if strings.TrimSpace(inspection.BackendCachePath) == "" {
			return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
				"%w: pinned backend assets are not installed for runtime %q",
				models.ErrHostMissingAssets,
				identity.Name,
			)
		}
		args = append(args, "--backend-cache-path", inspection.BackendCachePath)
	}
	return modelseffects.HostProcessStartSpec{
		Command:        command,
		Args:           args,
		HealthEndpoint: healthEndpoint,
	}, nil
}

func defaultGRPCServerStartBuilder(
	identity supervisedIdentity,
	inspection cacheInspection,
	worker *models.RuntimeWorker,
) (modelseffects.HostProcessStartSpec, error) {
	if worker == nil {
		return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
			"local model worker is required for supervised backend %q",
			identity.Backend,
		)
	}
	command := strings.TrimSpace(worker.Command)
	if command == "" {
		if !inspection.BackendRequired || len(inspection.BackendFiles) == 0 {
			return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
				"%w: managed backend executable is not installed for model %q",
				models.ErrHostMissingAssets, identity.Name,
			)
		}
		modelPath, err := firstModelArtifactPath(inspection)
		if err != nil {
			return modelseffects.HostProcessStartSpec{}, err
		}
		return modelseffects.HostProcessStartSpec{
			Backend:      identity.Backend,
			ModelPath:    modelPath,
			BackendFiles: append([]string(nil), inspection.BackendFiles...),
		}, nil
	}
	endpoint, args, err := supervisedGRPCEndpointAndArgs(worker.Args)
	if err != nil {
		return modelseffects.HostProcessStartSpec{}, err
	}
	if strings.TrimSpace(inspection.CachePath) == "" {
		return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
			"%w: cache path is required for supervised runtime %q",
			models.ErrHostMissingAssets,
			identity.Name,
		)
	}
	args = append([]string{"serve"}, args...)
	args = append(args, "--cache-path", inspection.CachePath)
	if inspection.BackendRequired {
		if strings.TrimSpace(inspection.BackendCachePath) == "" {
			return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
				"%w: pinned backend assets are not installed for runtime %q",
				models.ErrHostMissingAssets,
				identity.Name,
			)
		}
		args = append(args, "--backend-cache-path", inspection.BackendCachePath)
	}
	return modelseffects.HostProcessStartSpec{
		Command:        command,
		Args:           args,
		HealthEndpoint: endpoint,
	}, nil
}

func firstModelArtifactPath(inspection cacheInspection) (string, error) {
	cachePath := strings.TrimSpace(inspection.CachePath)
	if cachePath == "" {
		return "", fmt.Errorf(
			"%w: model cache path is required for supervised runtime",
			models.ErrHostMissingAssets,
		)
	}
	for _, artifact := range inspection.ObservedArtifacts {
		name := filepath.ToSlash(strings.TrimSpace(artifact.Name))
		if name == "" || name == "." || name == ".." || filepath.IsAbs(name) {
			continue
		}
		return filepath.Join(cachePath, filepath.FromSlash(name)), nil
	}
	return "", fmt.Errorf(
		"%w: model artifact is not installed for supervised runtime",
		models.ErrHostMissingAssets,
	)
}

func supervisedGRPCEndpointAndArgs(workerArgs []string) (string, []string, error) {
	args := append([]string(nil), workerArgs...)
	for _, flag := range []string{"--grpc-endpoint", "--grpc-address", "--endpoint"} {
		for index := 0; index < len(args); index++ {
			if args[index] != flag {
				continue
			}
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
				return "", nil, fmt.Errorf("flag %q requires a non-empty value", flag)
			}
			endpoint := strings.TrimSpace(args[index+1])
			remaining := append(append([]string(nil), args[:index]...), args[index+2:]...)
			return endpoint, remaining, nil
		}
	}
	return "", nil, fmt.Errorf(
		"managed LocalAI backend requires one of %q, %q, or %q",
		"--grpc-endpoint", "--grpc-address", "--endpoint",
	)
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

func sameBackend(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func typedHostReadinessFailure(
	identity supervisedIdentity,
	class hostFailureClass,
	cause error,
) error {
	readiness := models.ReadinessStateFailed
	lifecycle := models.LifecycleStateLoaded
	if class == hostFailureClassLoadingTimeout {
		readiness = models.ReadinessStateLoading
		lifecycle = models.LifecycleStateLoading
	}
	return &models.HostReadinessError{
		Snapshot: models.HostReadinessSnapshot{
			Identity: models.HostIdentity{
				Name:       identity.Name,
				Backend:    identity.Backend,
				LoadPolicy: identity.LoadPolicy,
			},
			ReadinessState: readiness,
			LifecycleState: lifecycle,
			FailureClass:   publicHostFailureClass(class),
		},
		Cause: cause,
	}
}

func publicHostFailureClass(class hostFailureClass) models.HostFailureClass {
	switch class {
	case hostFailureClassLoadingTimeout:
		return models.HostFailureClassLoadingTimeout
	case hostFailureClassProcessCrash:
		return models.HostFailureClassProcessCrash
	case hostFailureClassCancelled:
		return models.HostFailureClassCancelled
	case hostFailureClassProtocol:
		return models.HostFailureClassProtocol
	case hostFailureClassUnsupportedPlatform:
		return models.HostFailureClassUnsupportedPlatform
	default:
		return models.HostFailureClassNone
	}
}

func cancelHostError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", models.ErrHostCancelled, err)
}
