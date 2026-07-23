// Package operatorsettings owns operator-level defaults, worker presets, and
// deterministic file/environment/flag precedence.
package operatorsettings

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	// EnvDefaultWorkerModelProvider is the environment variable for the default
	// worker model provider override.
	EnvDefaultWorkerModelProvider = "YOU_DEFAULT_WORKER_MODEL_PROVIDER"
	// EnvDefaultWorkerModel is the environment variable for the default worker
	// model override.
	EnvDefaultWorkerModel = "YOU_DEFAULT_WORKER_MODEL"
)

// Source identifies which precedence layer supplied an effective default value.
type Source string

const (
	SourceFile Source = "file"
	SourceEnv  Source = "env"
	SourceFlag Source = "flag"
)

// Defaults holds raw operator default values before precedence resolution.
type Defaults struct {
	WorkerModelProvider string
	WorkerModel         string
}

// WorkerPreset is a reusable, file-only child-worker configuration.
type WorkerPreset struct {
	ID              string `json:"id"`
	ModelProvider   string `json:"modelProvider"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
}

// Config holds normalized operator settings after the serialized global-config
// contract has been decoded at the transport boundary.
type Config struct {
	BackendScopeID string
	Defaults       Defaults
	WorkerPresets  []WorkerPreset
}

// ResolvedDefaults holds effective operator defaults after precedence and
// symbolic DEFAULT resolution.
type ResolvedDefaults struct {
	WorkerModelProvider       string
	WorkerModel               string
	WorkerModelProviderSource Source
	WorkerModelSource         Source
	ConfigPath                string
}

// ResolveInput supplies operator default layers in precedence order: file, env,
// then CLI flags.
type ResolveInput struct {
	File Defaults
	Env  Defaults
	Flag Defaults
}

// FlagOverrides carries CLI flag values for operator defaults.
type FlagOverrides struct {
	WorkerModelProvider string
	WorkerModel         string
}

// DefaultConfigPath returns the default operator config file path for homeDir.
func DefaultConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".you-agent-factory", "config.json")
}

// LoadFileDefaults reads operator defaults from path. A missing file returns
// empty defaults without error. Malformed JSON fails with an error naming path.
func LoadFileDefaults(files FileSystem, decode ConfigDecoder, path string) (Defaults, error) {
	cfg, err := LoadFileConfig(files, decode, path)
	return cfg.Defaults, err
}

// LoadFileConfig reads and validates the operator-owned configuration. A
// missing file returns an empty configuration without error.
func LoadFileConfig(files FileSystem, decode ConfigDecoder, path string) (Config, error) {
	data, err := files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read operator config %s: %w", path, err)
	}
	if decode == nil {
		return Config{}, fmt.Errorf("parse operator config %s: global config decoder is required", path)
	}
	cfg, err := decode(data)
	if err != nil {
		return Config{}, fmt.Errorf("parse operator config %s: %w", path, err)
	}
	return cfg, nil
}

// Normalize trims decoded values and validates file-only worker presets.
func (cfg Config) Normalize() (Config, error) {
	presets, err := validateWorkerPresets(cfg.WorkerPresets)
	if err != nil {
		return Config{}, err
	}
	return Config{BackendScopeID: strings.TrimSpace(cfg.BackendScopeID), Defaults: Defaults{
		WorkerModelProvider: strings.TrimSpace(cfg.Defaults.WorkerModelProvider),
		WorkerModel:         strings.TrimSpace(cfg.Defaults.WorkerModel),
	}, WorkerPresets: presets}, nil
}

func validateWorkerPresets(presets []WorkerPreset) ([]WorkerPreset, error) {
	validated := make([]WorkerPreset, len(presets))
	seen := make(map[string]struct{}, len(presets))
	for i, preset := range presets {
		id := strings.TrimSpace(preset.ID)
		if id == "" {
			return nil, fmt.Errorf("workerPresets[%d].id %q must be non-empty", i, preset.ID)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("workerPresets[%d].id %q is duplicated", i, id)
		}
		seen[id] = struct{}{}
		provider, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(preset.ModelProvider)
		if strings.TrimSpace(preset.ModelProvider) == "" || !ok || interfaces.IsSymbolicWorkerModelProviderDefault(provider) {
			return nil, fmt.Errorf("workerPresets[%d] %q has unsupported modelProvider %q: %s", i, id, preset.ModelProvider, interfaces.AcceptedPublicWorkerModelProviderSummary())
		}
		effort, ok := interfaces.CanonicalizeReasoningEffort(preset.ReasoningEffort)
		if !ok {
			return nil, fmt.Errorf("workerPresets[%d] %q has unsupported reasoningEffort %q: accepted values are minimal, low, medium, high", i, id, preset.ReasoningEffort)
		}
		validated[i] = WorkerPreset{ID: id, ModelProvider: provider, Model: strings.TrimSpace(preset.Model), ReasoningEffort: effort}
	}
	return validated, nil
}

// Resolve applies file, environment, and flag precedence independently per
// field, resolves symbolic DEFAULT providers, and validates supported providers.
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

	resolvedProvider, err := resolveWorkerModelProvider(providerRaw, providerSource, input)
	if err != nil {
		return ResolvedDefaults{}, err
	}

	return ResolvedDefaults{
		WorkerModelProvider:       resolvedProvider,
		WorkerModel:               modelRaw,
		WorkerModelProviderSource: providerSource,
		WorkerModelSource:         modelSource,
		ConfigPath:                configPath,
	}, nil
}

func winningLayerValue(fileValue, envValue, flagValue string) (string, Source) {
	switch {
	case strings.TrimSpace(flagValue) != "":
		return strings.TrimSpace(flagValue), SourceFlag
	case strings.TrimSpace(envValue) != "":
		return strings.TrimSpace(envValue), SourceEnv
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
	if !ok {
		return "", unsupportedWorkerModelProviderError(concreteRaw)
	}
	if interfaces.IsSymbolicWorkerModelProviderDefault(concreteCanonical) {
		return "", errUnresolvedSymbolicDefaultProvider
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
		"--default-worker-model-provider to a supported provider",
)

// PrecedenceChain describes the operator default precedence order for diagnostics.
const PrecedenceChain = "file < env < flag"

// DiagnosticsLine returns a redacted verbose diagnostic line for resolved defaults.
func (r ResolvedDefaults) DiagnosticsLine() string {
	return fmt.Sprintf(
		"operatorDefaults precedence=%s provider=%s providerSource=%s model=%s modelSource=%s configPath=%q",
		PrecedenceChain,
		diagnosticsDefaultValue(r.WorkerModelProvider, r.WorkerModelProviderSource),
		diagnosticsSourceLabel(r.WorkerModelProviderSource),
		diagnosticsDefaultValue(r.WorkerModel, r.WorkerModelSource),
		diagnosticsSourceLabel(r.WorkerModelSource),
		r.ConfigPath,
	)
}

func diagnosticsSourceLabel(source Source) string {
	if source == "" {
		return "unset"
	}
	return string(source)
}

func diagnosticsDefaultValue(value string, source Source) string {
	if source == "" || strings.TrimSpace(value) == "" {
		return "unset"
	}
	return value
}
