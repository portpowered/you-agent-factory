package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

const (
	builtInLLMModelArtifactName       = "gemma-4-E4B-it-Q4_K_M.gguf"
	builtInLLMModelArtifactBytes      = int64(4977171584)
	builtInLLMModelArtifactSHA256     = "85a896a047553e842f25297ee5b031d64ff30147d9c4af17b1e4b394cd1fab87"
	builtInLLMProjectorArtifactName   = "mmproj-F16.gguf"
	builtInLLMProjectorArtifactBytes  = int64(990372672)
	builtInLLMProjectorArtifactSHA256 = "ddf46c21d7078e95338cfc22306b19b276a29a5ad089023449dd54d4b6170a51"
)

type supervisedIdentity struct {
	Name       string
	Backend    string
	LoadPolicy string
	Source     string
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
		identity.Source = strings.TrimSpace(definition.Source)
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
		if overlay.Source != nil {
			identity.Source = strings.TrimSpace(*overlay.Source)
		}
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
		Source:     identity.Source,
		Revision:   identity.Revision,
	}
}

func identityWithResolvedHostConfiguration(
	identity supervisedIdentity,
	configuration modelseffects.ResolvedHostConfiguration,
	inspection scopedassets.RuntimeCacheInspection,
) supervisedIdentity {
	if name := strings.TrimSpace(configuration.ModelName); name != "" {
		identity.Name = name
	}
	if backend := strings.TrimSpace(configuration.Backend); backend != "" {
		identity.Backend = backend
	}
	if source := strings.TrimSpace(configuration.Source.NameOrURI); source != "" {
		identity.Source = source
	}
	if revision := strings.TrimSpace(configuration.Revision); revision != "" {
		identity.Revision = revision
	} else if revision := strings.TrimSpace(inspection.Revision); revision != "" {
		identity.Revision = revision
	}
	return identity
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
		ExpectedArtifacts:     append([]models.AssetRequirement(nil), inspection.ExpectedArtifacts...),
		IntegrityVerified:     inspection.IntegrityVerified,
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
	ExpectedArtifacts     []models.AssetRequirement
	IntegrityVerified     bool
	BackendRequired       bool
	BackendCachePath      string
	BackendRevision       string
	BackendInstalledFiles int
	BackendFiles          []string
	ObservedArtifacts     []models.AssetArtifact
}

func resolvedHostConfigurationFromInspection(
	scope models.RuntimeScopeRef,
	identity supervisedIdentity,
	inspection cacheInspection,
	platform models.AssetHostPlatform,
	resolveSymlinks modelseffects.HostResolveSymlinks,
) (modelseffects.ResolvedHostConfiguration, error) {
	configuration := modelseffects.ResolvedHostConfiguration{
		Scope:            scope,
		ModelName:        strings.TrimSpace(identity.Name),
		Source:           models.ModelReference{NameOrURI: strings.TrimSpace(identity.Source)},
		Revision:         strings.TrimSpace(identity.Revision),
		Backend:          strings.TrimSpace(identity.Backend),
		Platform:         platform,
		ProtocolVersion:  modelseffects.PinnedHostProtocolVersion,
		ModelCachePath:   strings.TrimSpace(inspection.CachePath),
		BackendCachePath: strings.TrimSpace(inspection.BackendCachePath),
		BackendFiles:     append([]string(nil), inspection.BackendFiles...),
	}
	if configuration.Revision == "" {
		configuration.Revision = strings.TrimSpace(inspection.Revision)
	}
	if len(inspection.ObservedArtifacts) == 0 {
		return configuration, nil
	}

	var (
		modelFiles []string
		err        error
	)
	if isBuiltInLLMIdentity(identity) {
		_, _, modelFiles, err = builtInLLMArtifactPaths(inspection, resolveSymlinks)
	} else {
		modelFiles, err = modelArtifactPaths(inspection, resolveSymlinks)
	}
	if err != nil {
		return modelseffects.ResolvedHostConfiguration{}, err
	}
	configuration.ModelFiles = append([]string(nil), modelFiles...)
	configuration.ModelPath, configuration.MMProjPath = hostModelPaths(modelFiles)
	return configuration, nil
}

func validateResolvedHostConfigurationPaths(
	configuration modelseffects.ResolvedHostConfiguration,
	identity supervisedIdentity,
	inspection cacheInspection,
	platform models.AssetHostPlatform,
	resolveSymlinks modelseffects.HostResolveSymlinks,
) error {
	if !hasResolvedHostModelPathFacts(configuration) && len(inspection.ObservedArtifacts) == 0 {
		return nil
	}
	validated, err := resolvedHostConfigurationFromInspection(
		configuration.Scope,
		identity,
		inspection,
		platform,
		resolveSymlinks,
	)
	if err != nil {
		return err
	}
	if !sameHostPath(configuration.ModelCachePath, validated.ModelCachePath) ||
		!sameHostPath(configuration.ModelPath, validated.ModelPath) ||
		!sameHostPath(configuration.MMProjPath, validated.MMProjPath) ||
		!sameHostPathSlice(configuration.ModelFiles, validated.ModelFiles) {
		return invalidModelArtifactLayout()
	}
	return nil
}

func hasResolvedHostModelPathFacts(configuration modelseffects.ResolvedHostConfiguration) bool {
	return strings.TrimSpace(configuration.ModelCachePath) != "" ||
		strings.TrimSpace(configuration.ModelPath) != "" ||
		strings.TrimSpace(configuration.MMProjPath) != "" ||
		len(configuration.ModelFiles) > 0
}

func sameHostPath(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return left == right
	}
	leftPath := filepath.FromSlash(left)
	rightPath := filepath.FromSlash(right)
	return leftPath == rightPath &&
		leftPath == filepath.Clean(leftPath) &&
		rightPath == filepath.Clean(rightPath)
}

func sameHostPathSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !sameHostPath(left[index], right[index]) {
			return false
		}
	}
	return true
}

func defaultServerStartBuilder(
	configuration modelseffects.ResolvedHostConfiguration,
	worker *models.RuntimeWorker,
) (modelseffects.HostProcessStartSpec, error) {
	if worker == nil {
		return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
			"local model worker is required for supervised backend %q",
			configuration.Backend,
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
	if strings.TrimSpace(configuration.ModelCachePath) == "" {
		return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
			"%w: cache path is required for supervised runtime %q",
			models.ErrHostMissingAssets,
			configuration.ModelName,
		)
	}
	args = append([]string{"serve"}, args...)
	args = append(args, "--cache-path", configuration.ModelCachePath)
	if configuration.BackendCachePath != "" || len(configuration.BackendFiles) > 0 || configuration.BackendArtifact.Name != "" {
		if strings.TrimSpace(configuration.BackendCachePath) == "" {
			return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
				"%w: pinned backend assets are not installed for runtime %q",
				models.ErrHostMissingAssets,
				configuration.ModelName,
			)
		}
		args = append(args, "--backend-cache-path", configuration.BackendCachePath)
	}
	return modelseffects.HostProcessStartSpec{
		Command:        command,
		Args:           args,
		HealthEndpoint: healthEndpoint,
		Configuration:  configuration.Clone(),
	}, nil
}

func defaultGRPCServerStartBuilderWithSymlinkResolver(
	configuration modelseffects.ResolvedHostConfiguration,
	worker *models.RuntimeWorker,
) (modelseffects.HostProcessStartSpec, error) {
	if worker == nil {
		return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
			"local model worker is required for supervised backend %q",
			configuration.Backend,
		)
	}
	command := strings.TrimSpace(worker.Command)
	if command == "" {
		if len(configuration.BackendFiles) == 0 {
			return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
				"%w: managed backend executable is not installed for model %q",
				models.ErrHostMissingAssets, configuration.ModelName,
			)
		}
		return modelseffects.HostProcessStartSpec{
			Configuration: configuration.Clone(),
		}, nil
	}
	endpoint, args, err := supervisedGRPCEndpointAndArgs(worker.Args)
	if err != nil {
		return modelseffects.HostProcessStartSpec{}, err
	}
	if strings.TrimSpace(configuration.ModelCachePath) == "" {
		return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
			"%w: cache path is required for supervised runtime %q",
			models.ErrHostMissingAssets,
			configuration.ModelName,
		)
	}
	args = append([]string{"serve"}, args...)
	args = append(args, "--cache-path", configuration.ModelCachePath)
	if configuration.BackendCachePath != "" || len(configuration.BackendFiles) > 0 || configuration.BackendArtifact.Name != "" {
		if strings.TrimSpace(configuration.BackendCachePath) == "" {
			return modelseffects.HostProcessStartSpec{}, fmt.Errorf(
				"%w: pinned backend assets are not installed for runtime %q",
				models.ErrHostMissingAssets,
				configuration.ModelName,
			)
		}
		args = append(args, "--backend-cache-path", configuration.BackendCachePath)
	}
	return modelseffects.HostProcessStartSpec{
		Command:        command,
		Args:           args,
		HealthEndpoint: endpoint,
		Configuration:  configuration.Clone(),
	}, nil
}

func hostModelPaths(files []string) (string, string) {
	var modelPath, mmProjPath string
	for _, raw := range files {
		file := strings.TrimSpace(raw)
		if file == "" {
			continue
		}
		base := strings.ToLower(filepath.Base(filepath.Clean(file)))
		if strings.Contains(base, "mmproj") {
			if mmProjPath == "" {
				mmProjPath = file
			}
			continue
		}
		if modelPath == "" && !strings.Contains(base, "tokenizer") && !strings.HasPrefix(base, "voice") {
			modelPath = file
		}
	}
	if modelPath == "" {
		for _, raw := range files {
			file := strings.TrimSpace(raw)
			base := strings.ToLower(filepath.Base(filepath.Clean(file)))
			if file == "" || strings.Contains(base, "mmproj") {
				continue
			}
			modelPath = file
			break
		}
	}
	return modelPath, mmProjPath
}

func isBuiltInLLMIdentity(identity supervisedIdentity) bool {
	if !strings.EqualFold(strings.TrimSpace(identity.Name), models.BuiltInModelNameLLM) {
		return false
	}
	definition, ok := (models.BuiltInCatalog{}).ModelDefinitionFor(models.BuiltInModelNameLLM)
	return ok && strings.TrimSpace(identity.Source) == strings.TrimSpace(definition.Source)
}

func builtInLLMArtifactPaths(
	inspection cacheInspection,
	resolveSymlinks modelseffects.HostResolveSymlinks,
) (string, string, []string, error) {
	paths, err := modelArtifactPaths(inspection, resolveSymlinks)
	if err != nil {
		return "", "", nil, err
	}
	wanted := builtInLLMArtifactRequirements()
	if !inspection.IntegrityVerified {
		return "", "", nil, invalidModelArtifactLayout()
	}
	if len(inspection.ExpectedArtifacts) != len(wanted) || len(inspection.ObservedArtifacts) != len(wanted) || len(paths) != len(wanted) {
		return "", "", nil, invalidModelArtifactLayout()
	}
	if !validBuiltInLLMRequirements(inspection.ExpectedArtifacts, wanted) {
		return "", "", nil, invalidModelArtifactLayout()
	}
	modelPath, mmProjPath, valid := builtInLLMObservedPaths(inspection.ObservedArtifacts, paths, wanted)
	if !valid {
		return "", "", nil, invalidModelArtifactLayout()
	}
	return modelPath, mmProjPath, []string{modelPath, mmProjPath}, nil
}

func builtInLLMArtifactRequirements() map[string]models.AssetRequirement {
	return map[string]models.AssetRequirement{
		builtInLLMModelArtifactName: {
			Name: builtInLLMModelArtifactName, Bytes: builtInLLMModelArtifactBytes,
			SHA256: builtInLLMModelArtifactSHA256,
		},
		builtInLLMProjectorArtifactName: {
			Name: builtInLLMProjectorArtifactName, Bytes: builtInLLMProjectorArtifactBytes,
			SHA256: builtInLLMProjectorArtifactSHA256,
		},
	}
}

func validBuiltInLLMRequirements(
	requirements []models.AssetRequirement,
	wanted map[string]models.AssetRequirement,
) bool {
	seen := make(map[string]struct{}, len(requirements))
	for _, requirement := range requirements {
		if !matchesBuiltInLLMRequirement(requirement.Name, requirement.Bytes, requirement.SHA256, wanted) {
			return false
		}
		if _, duplicate := seen[requirement.Name]; duplicate {
			return false
		}
		seen[requirement.Name] = struct{}{}
	}
	return len(seen) == len(wanted)
}

func matchesBuiltInLLMRequirement(
	name string,
	bytes int64,
	sha256 string,
	wanted map[string]models.AssetRequirement,
) bool {
	trimmedName := strings.TrimSpace(name)
	want, ok := wanted[trimmedName]
	if !ok {
		return false
	}
	if trimmedName != name {
		return false
	}
	if bytes != want.Bytes {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(sha256), want.SHA256)
}

func builtInLLMObservedPaths(
	artifacts []models.AssetArtifact,
	paths []string,
	wanted map[string]models.AssetRequirement,
) (string, string, bool) {
	seen := make(map[string]struct{}, len(artifacts))
	var modelPath, mmProjPath string
	for index, artifact := range artifacts {
		if !matchesBuiltInLLMRequirement(artifact.Name, artifact.Bytes, artifact.SHA256, wanted) {
			return "", "", false
		}
		if _, duplicate := seen[artifact.Name]; duplicate {
			return "", "", false
		}
		seen[artifact.Name] = struct{}{}
		switch artifact.Name {
		case builtInLLMModelArtifactName:
			modelPath = paths[index]
		case builtInLLMProjectorArtifactName:
			mmProjPath = paths[index]
		}
	}
	if len(seen) != len(wanted) {
		return "", "", false
	}
	return modelPath, mmProjPath, true
}

func modelArtifactPaths(
	inspection cacheInspection,
	resolveSymlinks modelseffects.HostResolveSymlinks,
) ([]string, error) {
	cachePath := strings.TrimSpace(inspection.CachePath)
	if cachePath == "" {
		return nil, fmt.Errorf(
			"%w: model cache path is required for supervised runtime",
			models.ErrHostMissingAssets,
		)
	}
	root := filepath.Clean(filepath.FromSlash(cachePath))
	paths := make([]string, 0, len(inspection.ObservedArtifacts))
	seen := make(map[string]struct{}, len(inspection.ObservedArtifacts))
	for _, artifact := range inspection.ObservedArtifacts {
		name := strings.TrimSpace(artifact.Name)
		normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
		if name == "" || name == "." || name == ".." ||
			name != normalized || strings.Contains(name, "\\") ||
			filepath.IsAbs(name) || filepath.IsAbs(filepath.FromSlash(name)) ||
			filepath.VolumeName(name) != "" {
			return nil, invalidModelArtifactLayout()
		}
		candidate := filepath.Clean(filepath.Join(root, filepath.FromSlash(name)))
		if !pathWithinRoot(root, candidate) || !resolvedPathWithinRoot(resolveSymlinks, root, candidate) {
			return nil, invalidModelArtifactLayout()
		}
		if _, duplicate := seen[candidate]; duplicate {
			return nil, invalidModelArtifactLayout()
		}
		seen[candidate] = struct{}{}
		paths = append(paths, candidate)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf(
			"%w: model artifact is not installed for supervised runtime",
			models.ErrHostMissingAssets,
		)
	}
	return paths, nil
}

func invalidModelArtifactLayout() error {
	return fmt.Errorf(
		"%w: verified model layout contains invalid artifact metadata",
		models.ErrHostMissingAssets,
	)
}

func pathWithinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func resolvedPathWithinRoot(
	resolveSymlinks modelseffects.HostResolveSymlinks,
	root, candidate string,
) bool {
	if resolveSymlinks == nil {
		return true
	}
	resolvedRoot, rootErr := resolveSymlinks(root)
	if rootErr != nil {
		if os.IsNotExist(rootErr) {
			return true
		}
		return false
	}
	for current := candidate; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(current); err == nil {
			resolvedCandidate, candidateErr := resolveSymlinks(current)
			if candidateErr != nil {
				return false
			}
			return pathWithinRoot(resolvedRoot, resolvedCandidate)
		} else if !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
	}
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
		Cause: modelseffects.WrapRuntimeFailure(
			runtimeStageForHostFailure(class),
			cause,
		),
	}
}

func runtimeStageForHostFailure(class hostFailureClass) modelseffects.RuntimeStage {
	switch class {
	case hostFailureClassProtocol:
		return modelseffects.RuntimeStageProtocolLoad
	case hostFailureClassProcessCrash, hostFailureClassUnsupportedPlatform:
		return modelseffects.RuntimeStageBackendStart
	default:
		return modelseffects.RuntimeStageBackendStart
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
