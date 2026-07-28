package operatorsettings

import (
	"fmt"
	"regexp"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

var acpProviderIdentityPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

const (
	// EnvDefaultWorkerModelProvider is the environment variable for the default
	// worker model provider override.
	EnvDefaultWorkerModelProvider = "YOU_DEFAULT_WORKER_MODEL_PROVIDER"
	// EnvDefaultWorkerModel is the environment variable for the default worker
	// model override.
	EnvDefaultWorkerModel = "YOU_DEFAULT_WORKER_MODEL"

	// DefaultRuntimeArtifactMaxSizeMB is the default rolling-file size.
	DefaultRuntimeArtifactMaxSizeMB = 100
	// DefaultRuntimeArtifactBackups is the default rotated-file count.
	DefaultRuntimeArtifactBackups = 20
	// DefaultRuntimeArtifactMaxAge is the default rotated-file age in days.
	DefaultRuntimeArtifactMaxAge = 30
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
	Runtime        RuntimeSettings
	Workers        WorkerSettings
	WorkerPresets  []WorkerPreset
}

type WorkerSettings struct {
	ACP ACPSettings
}

type ACPSettings struct {
	Integrations []ACPIntegration
}

type ACPIntegration struct {
	ID        string
	Name      string
	Transport string
	Command   string
}

// RuntimeArtifactSettings controls one rolling runtime observability artifact.
// Directory is empty when the runtime-owned root below the operator home is
// selected.
type RuntimeArtifactSettings struct {
	Directory  string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// RuntimeSettings holds the operator-config runtime observability settings.
type RuntimeSettings struct {
	Logging RuntimeArtifactSettings
	Metrics RuntimeArtifactSettings
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

// PrecedenceChain describes the operator default precedence order for diagnostics.
const PrecedenceChain = "file < env < flag"

func defaultRuntimeArtifactSettings() RuntimeArtifactSettings {
	return RuntimeArtifactSettings{
		MaxSizeMB:  DefaultRuntimeArtifactMaxSizeMB,
		MaxBackups: DefaultRuntimeArtifactBackups,
		MaxAgeDays: DefaultRuntimeArtifactMaxAge,
	}
}

func defaultRuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		Logging: defaultRuntimeArtifactSettings(),
		Metrics: defaultRuntimeArtifactSettings(),
	}
}

// Normalize trims decoded values and validates file-only worker presets.
func (cfg Config) Normalize() (Config, error) {
	presets, err := validateWorkerPresets(cfg.WorkerPresets)
	if err != nil {
		return Config{}, err
	}
	runtime, err := cfg.Runtime.normalize()
	if err != nil {
		return Config{}, err
	}
	workers, err := cfg.Workers.normalize()
	if err != nil {
		return Config{}, err
	}
	return Config{
		BackendScopeID: strings.TrimSpace(cfg.BackendScopeID),
		Defaults: Defaults{
			WorkerModelProvider: strings.TrimSpace(cfg.Defaults.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(cfg.Defaults.WorkerModel),
		},
		Runtime:       runtime,
		Workers:       workers,
		WorkerPresets: presets,
	}, nil
}

func (settings WorkerSettings) normalize() (WorkerSettings, error) {
	if settings.ACP.Integrations == nil {
		return WorkerSettings{}, nil
	}
	integrations := make([]ACPIntegration, len(settings.ACP.Integrations))
	ids := make(map[string]struct{}, len(integrations))
	names := make(map[string]struct{}, len(integrations))
	for index, integration := range settings.ACP.Integrations {
		integration = ACPIntegration{
			ID: strings.TrimSpace(integration.ID), Name: strings.TrimSpace(integration.Name),
			Transport: strings.ToLower(strings.TrimSpace(integration.Transport)), Command: strings.TrimSpace(integration.Command),
		}
		if integration.ID == "" || integration.Name == "" || integration.Command == "" {
			return WorkerSettings{}, fmt.Errorf("workers.acp.integrations[%d] requires non-empty id, name, and command", index)
		}
		if err := providers.ID(integration.Name).Validate(); err != nil || !acpProviderIdentityPattern.MatchString(integration.Name) {
			return WorkerSettings{}, fmt.Errorf("workers.acp.integrations[%d].name %q must use canonical lowercase letters, digits, dots, or hyphens", index, integration.Name)
		}
		if integration.Transport != "stdio" {
			return WorkerSettings{}, fmt.Errorf("workers.acp.integrations[%d] %q has unsupported transport %q: accepted value is stdio", index, integration.Name, integration.Transport)
		}
		if _, exists := ids[integration.ID]; exists {
			return WorkerSettings{}, fmt.Errorf("workers.acp.integrations[%d].id %q is duplicated", index, integration.ID)
		}
		if _, exists := names[integration.Name]; exists {
			return WorkerSettings{}, fmt.Errorf("workers.acp.integrations[%d].name %q is duplicated", index, integration.Name)
		}
		ids[integration.ID] = struct{}{}
		names[integration.Name] = struct{}{}
		integrations[index] = integration
	}
	return WorkerSettings{ACP: ACPSettings{Integrations: integrations}}, nil
}

func (settings RuntimeArtifactSettings) normalize(fieldPath string) (RuntimeArtifactSettings, error) {
	settings.Directory = strings.TrimSpace(settings.Directory)
	defaults := defaultRuntimeArtifactSettings()
	if settings.MaxSizeMB == 0 {
		settings.MaxSizeMB = defaults.MaxSizeMB
	} else if settings.MaxSizeMB < 0 {
		return RuntimeArtifactSettings{}, fmt.Errorf("%s.maxSizeMB must be at least 1", fieldPath)
	}
	if settings.MaxBackups == 0 {
		settings.MaxBackups = defaults.MaxBackups
	} else if settings.MaxBackups < 0 {
		return RuntimeArtifactSettings{}, fmt.Errorf("%s.maxBackups must be at least 1", fieldPath)
	}
	if settings.MaxAgeDays == 0 {
		settings.MaxAgeDays = defaults.MaxAgeDays
	} else if settings.MaxAgeDays < 0 {
		return RuntimeArtifactSettings{}, fmt.Errorf("%s.maxAgeDays must be at least 1", fieldPath)
	}
	return settings, nil
}

func (settings RuntimeSettings) normalize() (RuntimeSettings, error) {
	logging, err := settings.Logging.normalize("runtime.logging")
	if err != nil {
		return RuntimeSettings{}, err
	}
	metrics, err := settings.Metrics.normalize("runtime.metrics")
	if err != nil {
		return RuntimeSettings{}, err
	}
	return RuntimeSettings{Logging: logging, Metrics: metrics}, nil
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
		validated[i] = WorkerPreset{
			ID:              id,
			ModelProvider:   provider,
			Model:           strings.TrimSpace(preset.Model),
			ReasoningEffort: effort,
		}
	}
	return validated, nil
}

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
