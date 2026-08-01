package service

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func loadFileConfig(
	files operatorsettings.FileSystem,
	decode operatorsettings.ConfigDecoder,
	path string,
) (operatorsettings.Config, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return operatorsettings.Config{}, fmt.Errorf("operator config path is required")
	}
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
	config, err := decode(data)
	if err != nil {
		return operatorsettings.Config{}, fmt.Errorf("parse operator config %s: %w", path, err)
	}
	return config, nil
}

func defaultRuntimeSettings() operatorsettings.RuntimeSettings {
	defaults := operatorsettings.RuntimeArtifactSettings{
		MaxSizeMB:  operatorsettings.DefaultRuntimeArtifactMaxSizeMB,
		MaxBackups: operatorsettings.DefaultRuntimeArtifactBackups,
		MaxAgeDays: operatorsettings.DefaultRuntimeArtifactMaxAge,
	}
	return operatorsettings.RuntimeSettings{Logging: defaults, Metrics: defaults}
}

func resolveDefaults(
	input operatorsettings.ResolveInput,
	configPath string,
) (operatorsettings.ResolvedDefaults, error) {
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
	if !ok || interfaces.IsSymbolicWorkerModelProviderDefault(concreteCanonical) {
		return "", unsupportedWorkerModelProviderError(concreteRaw)
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

func deriveProviderBackendScopeID(provider, kind, boundary string) string {
	return fmt.Sprintf(
		"provider-%s-%s-%s",
		sanitizeBackendScopeSegment(provider),
		sanitizeBackendScopeSegment(kind),
		sanitizeBackendScopeSegment(boundary),
	)
}

func sanitizeBackendScopeSegment(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", "|", "-")
	return replacer.Replace(trimmed)
}

func defaultConfigPath(homeDir string) string {
	return filepath.Join(strings.TrimSpace(homeDir), ".you-agent-factory", "config.json")
}
