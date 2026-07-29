package settingsresolution

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func defaultRuntimeSettings() operatorsettings.RuntimeSettings {
	return operatorsettings.RuntimeSettings{
		Logging: operatorsettings.RuntimeArtifactSettings{
			MaxSizeMB:  operatorsettings.DefaultRuntimeArtifactMaxSizeMB,
			MaxBackups: operatorsettings.DefaultRuntimeArtifactBackups,
			MaxAgeDays: operatorsettings.DefaultRuntimeArtifactMaxAge,
		},
		Metrics: operatorsettings.RuntimeArtifactSettings{
			MaxSizeMB:  operatorsettings.DefaultRuntimeArtifactMaxSizeMB,
			MaxBackups: operatorsettings.DefaultRuntimeArtifactBackups,
			MaxAgeDays: operatorsettings.DefaultRuntimeArtifactMaxAge,
		},
	}
}

// DefaultConfigPath returns the default operator config file path for homeDir.
func DefaultConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".you-agent-factory", "config.json")
}

// LoadFileDefaults reads operator defaults from path. A missing file returns
// empty defaults without error. Malformed JSON fails with an error naming path.
func LoadFileDefaults(
	files operatorsettings.FileSystem,
	decode operatorsettings.ConfigDecoder,
	path string,
) (operatorsettings.Defaults, error) {
	cfg, err := LoadFileConfig(files, decode, path)
	return cfg.Defaults, err
}

// LoadFileConfig reads and validates the operator-owned configuration. A
// missing file returns an empty configuration without error.
func LoadFileConfig(
	files operatorsettings.FileSystem,
	decode operatorsettings.ConfigDecoder,
	path string,
) (operatorsettings.Config, error) {
	data, err := files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return operatorsettings.Config{Runtime: defaultRuntimeSettings()}, nil
		}
		return operatorsettings.Config{}, fmt.Errorf("read operator config %s: %w", path, err)
	}
	if decode == nil {
		return operatorsettings.Config{}, fmt.Errorf("parse operator config %s: global config decoder is required", path)
	}
	cfg, err := decode(data)
	if err != nil {
		return operatorsettings.Config{}, fmt.Errorf("parse operator config %s: %w", path, err)
	}
	return cfg, nil
}

// Resolve applies file, environment, and flag precedence independently per
// field, resolves symbolic DEFAULT providers, and validates supported providers.
func Resolve(input operatorsettings.ResolveInput, configPath string) (operatorsettings.ResolvedDefaults, error) {
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

	resolvedProvider, err := resolveWorkerModelProvider(providerRaw, providerSource, input)
	if err != nil {
		return operatorsettings.ResolvedDefaults{}, err
	}

	return operatorsettings.ResolvedDefaults{
		WorkerModelProvider:       resolvedProvider,
		WorkerModel:               modelRaw,
		WorkerModelProviderSource: providerSource,
		WorkerModelSource:         modelSource,
		ConfigPath:                configPath,
	}, nil
}

func winningLayerValue(fileValue, envValue, flagValue string) (string, operatorsettings.Source) {
	switch {
	case strings.TrimSpace(flagValue) != "":
		return strings.TrimSpace(flagValue), operatorsettings.SourceFlag
	case strings.TrimSpace(envValue) != "":
		return strings.TrimSpace(envValue), operatorsettings.SourceEnv
	case strings.TrimSpace(fileValue) != "":
		return strings.TrimSpace(fileValue), operatorsettings.SourceFile
	default:
		return "", ""
	}
}

func resolveWorkerModelProvider(
	raw string,
	winningSource operatorsettings.Source,
	input operatorsettings.ResolveInput,
) (string, error) {
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
	if !ok {
		return "", unsupportedWorkerModelProviderError(concreteRaw)
	}
	if interfaces.IsSymbolicWorkerModelProviderDefault(concreteCanonical) {
		return "", errUnresolvedSymbolicDefaultProvider
	}
	return concreteCanonical, nil
}

func concreteProviderBelowSource(winningSource operatorsettings.Source, input operatorsettings.ResolveInput) string {
	type layer struct {
		source operatorsettings.Source
		value  string
	}
	layers := []layer{
		{source: operatorsettings.SourceFile, value: input.File.WorkerModelProvider},
		{source: operatorsettings.SourceEnv, value: input.Env.WorkerModelProvider},
		{source: operatorsettings.SourceFlag, value: input.Flag.WorkerModelProvider},
	}

	below := make([]layer, 0, 2)
	for _, layer := range layers {
		if layer.source == winningSource {
			break
		}
		below = append(below, layer)
	}
	for i := len(below) - 1; i >= 0; i-- {
		value := strings.TrimSpace(below[i].value)
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
