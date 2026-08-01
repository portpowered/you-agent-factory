package operatorsettings

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// DefaultsResolver is the exact application operation for resolving operator
// defaults from already-observed process edges and the service-owned file
// configuration.
type DefaultsResolver func(homeDir string, environment Defaults, flags FlagOverrides) (ResolvedDefaults, error)

// DefaultConfigPath is retained as a stateless compatibility helper. Service
// consumers should use Service.DefaultConfigPath so path policy remains behind
// the injected Operator Settings root.
func DefaultConfigPath(homeDir string) string {
	return filepath.Join(strings.TrimSpace(homeDir), ".you-agent-factory", "config.json")
}

// DeriveProviderBackendScopeID is retained as a stateless compatibility helper.
// Service consumers should use Service.DeriveProviderBackendScopeID.
func DeriveProviderBackendScopeID(provider, kind, boundary string) string {
	return fmt.Sprintf(
		"provider-%s-%s-%s",
		sanitizeBackendScopeSegment(provider),
		sanitizeBackendScopeSegment(kind),
		sanitizeBackendScopeSegment(boundary),
	)
}

// LoadFileDefaults is a stateless compatibility helper for callers that still
// own explicit filesystem and decoder ports. Production composition should use
// Service.LoadFileConfig or Service.ResolveFromHomeWithEnvironment.
func LoadFileDefaults(files FileSystem, decode ConfigDecoder, path string) (Defaults, error) {
	config, err := LoadFileConfig(files, decode, path)
	return config.Defaults, err
}

// LoadFileConfig loads one operator configuration without consulting any
// process-global registration. A missing file returns runtime defaults.
func LoadFileConfig(files FileSystem, decode ConfigDecoder, path string) (Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Config{}, fmt.Errorf("operator config path is required")
	}
	if files == nil {
		return Config{}, fmt.Errorf("operator settings filesystem is required")
	}
	data, err := files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{Runtime: defaultRuntimeSettings()}, nil
		}
		return Config{}, fmt.Errorf("read operator config %s: %w", path, err)
	}
	if decode == nil {
		return Config{}, fmt.Errorf("parse operator config %s: global config decoder is required", path)
	}
	config, err := decode(data)
	if err != nil {
		return Config{}, fmt.Errorf("parse operator config %s: %w", path, err)
	}
	return config, nil
}

// Resolve applies file, environment, and flag precedence independently per
// field, canonicalizes supported provider identities, and has no process state.
func Resolve(input ResolveInput, configPath string) (ResolvedDefaults, error) {
	providerRaw, providerSource := winningLayerValue(
		input.File.WorkerModelProvider,
		input.Env.WorkerModelProvider,
		input.Flag.WorkerModelProvider,
	)
	modelRaw, modelSource := winningLayerValue(
		input.File.WorkerModel,
		input.Env.WorkerModel,
		input.Flag.WorkerModel,
	)
	provider, err := resolveWorkerModelProvider(providerRaw, providerSource, input)
	if err != nil {
		return ResolvedDefaults{}, err
	}
	return ResolvedDefaults{
		WorkerModelProvider:       provider,
		WorkerModel:               modelRaw,
		WorkerModelProviderSource: providerSource,
		WorkerModelSource:         modelSource,
		ConfigPath:                configPath,
	}, nil
}

func winningLayerValue(fileValue, environmentValue, flagValue string) (string, Source) {
	switch {
	case strings.TrimSpace(flagValue) != "":
		return strings.TrimSpace(flagValue), SourceFlag
	case strings.TrimSpace(environmentValue) != "":
		return strings.TrimSpace(environmentValue), SourceEnv
	case strings.TrimSpace(fileValue) != "":
		return strings.TrimSpace(fileValue), SourceFile
	default:
		return "", ""
	}
}

func resolveWorkerModelProvider(raw string, winningSource Source, input ResolveInput) (string, error) {
	if raw == "" {
		return "", nil
	}
	canonical, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(raw)
	if !ok {
		return "", unsupportedWorkerModelProviderError(raw)
	}
	if !interfaces.IsSymbolicWorkerModelProviderDefault(canonical) {
		return canonical, nil
	}
	concreteRaw := concreteProviderBelowSource(winningSource, input)
	if concreteRaw == "" {
		return "", errUnresolvedSymbolicDefaultProvider
	}
	concreteCanonical, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(concreteRaw)
	if !ok || interfaces.IsSymbolicWorkerModelProviderDefault(concreteCanonical) {
		return "", unsupportedWorkerModelProviderError(concreteRaw)
	}
	return concreteCanonical, nil
}

func concreteProviderBelowSource(winningSource Source, input ResolveInput) string {
	type layer struct {
		source Source
		value  string
	}
	layers := []layer{
		{source: SourceFile, value: input.File.WorkerModelProvider},
		{source: SourceEnv, value: input.Env.WorkerModelProvider},
		{source: SourceFlag, value: input.Flag.WorkerModelProvider},
	}
	below := make([]layer, 0, 2)
	for _, layer := range layers {
		if layer.source == winningSource {
			break
		}
		below = append(below, layer)
	}
	for index := len(below) - 1; index >= 0; index-- {
		value := strings.TrimSpace(below[index].value)
		if value == "" || interfaces.IsSymbolicWorkerModelProviderDefault(value) {
			continue
		}
		return value
	}
	return ""
}

func unsupportedWorkerModelProviderError(value string) error {
	return fmt.Errorf(
		"unsupported worker model provider %q: %s",
		value,
		interfaces.AcceptedPublicWorkerModelProviderSummary(),
	)
}

var errUnresolvedSymbolicDefaultProvider = errors.New(
	"worker model provider DEFAULT requires a concrete provider from file or environment; " +
		"set defaults.workerModelProvider, YOU_DEFAULT_WORKER_MODEL_PROVIDER, or " +
		"you run --provider to a supported provider",
)
