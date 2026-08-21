package operatorsettings

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

var acpProviderIdentityPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

var priceTableDecimalPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

// ErrPriceTableInvalid reports an authored price table that cannot be safely
// used for valuation.
var ErrPriceTableInvalid = errors.New("operator price table is invalid")

const (
	// PriceTableCurrencyUSD is the only currency supported by the operator
	// price table. Rates and calculated amounts are expressed in USD.
	PriceTableCurrencyUSD = "USD"

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
	PriceTable     PriceTable
	Runtime        RuntimeSettings
	Workers        WorkerSettings
	WorkerPresets  []WorkerPreset
	Models         map[string]ModelConfig
}

// PriceTable is the operator-owned, exact-rate table used by the Costs
// service. A zero value is normalized to an empty USD table.
type PriceTable struct {
	Currency string
	Models   []PriceTableModel
}

// PriceTableModel is one exact provider/model price entry. Optional subclass
// rates use pointers so an explicit "0" remains distinct from an omitted
// rate.
type PriceTableModel struct {
	Provider                        string
	Model                           string
	InputPerMillionTokens           string
	OutputPerMillionTokens          string
	CachedInputPerMillionTokens     *string
	ReasoningOutputPerMillionTokens *string
}

// DefaultPriceTable returns the detached default-empty USD table.
func DefaultPriceTable() PriceTable {
	return PriceTable{Currency: PriceTableCurrencyUSD, Models: []PriceTableModel{}}
}

// Clone returns a detached price table whose model slice and optional rates do
// not alias the receiver.
func (table PriceTable) Clone() PriceTable {
	cloned := table
	if table.Models == nil {
		return cloned
	}
	cloned.Models = make([]PriceTableModel, len(table.Models))
	for index, model := range table.Models {
		cloned.Models[index] = model.Clone()
	}
	return cloned
}

// Clone returns a detached model-price entry.
func (model PriceTableModel) Clone() PriceTableModel {
	cloned := model
	cloned.CachedInputPerMillionTokens = cloneOptionalString(model.CachedInputPerMillionTokens)
	cloned.ReasoningOutputPerMillionTokens = cloneOptionalString(model.ReasoningOutputPerMillionTokens)
	return cloned
}

// Normalize trims identities, canonicalizes provider aliases, validates exact
// non-negative decimal rates, and rejects duplicate provider/model keys.
func (table PriceTable) Normalize() (PriceTable, error) {
	currency := table.Currency
	if currency == "" {
		currency = PriceTableCurrencyUSD
	}
	if currency != PriceTableCurrencyUSD {
		return PriceTable{}, fmt.Errorf("%w: currency %q is unsupported; only %s is accepted", ErrPriceTableInvalid, table.Currency, PriceTableCurrencyUSD)
	}

	normalized := PriceTable{Currency: currency}
	if table.Models == nil {
		normalized.Models = []PriceTableModel{}
	} else {
		normalized.Models = make([]PriceTableModel, len(table.Models))
	}
	seen := make(map[string]struct{}, len(table.Models))
	for index, model := range table.Models {
		normalizedModel, err := normalizePriceTableModel(index, model)
		if err != nil {
			return PriceTable{}, err
		}
		key := normalizedModel.Provider + "\x00" + normalizedModel.Model
		if _, exists := seen[key]; exists {
			return PriceTable{}, fmt.Errorf("%w: priceTable.models[%d] duplicates provider/model %q/%q", ErrPriceTableInvalid, index, normalizedModel.Provider, normalizedModel.Model)
		}
		seen[key] = struct{}{}
		normalized.Models[index] = normalizedModel
	}
	return normalized, nil
}

func normalizePriceTableModel(index int, model PriceTableModel) (PriceTableModel, error) {
	providerInput := strings.TrimSpace(model.Provider)
	provider, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(providerInput)
	if providerInput == "" || !ok || interfaces.IsSymbolicWorkerModelProviderDefault(provider) {
		return PriceTableModel{}, fmt.Errorf("%w: priceTable.models[%d].provider %q must be a concrete canonical provider identity", ErrPriceTableInvalid, index, model.Provider)
	}
	modelName := strings.TrimSpace(model.Model)
	if modelName == "" {
		return PriceTableModel{}, fmt.Errorf("%w: priceTable.models[%d].model must be non-empty", ErrPriceTableInvalid, index)
	}
	inputRate, err := normalizePriceRate(fmt.Sprintf("priceTable.models[%d].inputPerMillionTokens", index), model.InputPerMillionTokens, true)
	if err != nil {
		return PriceTableModel{}, err
	}
	outputRate, err := normalizePriceRate(fmt.Sprintf("priceTable.models[%d].outputPerMillionTokens", index), model.OutputPerMillionTokens, true)
	if err != nil {
		return PriceTableModel{}, err
	}
	normalized := PriceTableModel{
		Provider:               provider,
		Model:                  modelName,
		InputPerMillionTokens:  inputRate,
		OutputPerMillionTokens: outputRate,
	}
	if model.CachedInputPerMillionTokens != nil {
		rate, err := normalizePriceRate(fmt.Sprintf("priceTable.models[%d].cachedInputPerMillionTokens", index), *model.CachedInputPerMillionTokens, false)
		if err != nil {
			return PriceTableModel{}, err
		}
		normalized.CachedInputPerMillionTokens = &rate
	}
	if model.ReasoningOutputPerMillionTokens != nil {
		rate, err := normalizePriceRate(fmt.Sprintf("priceTable.models[%d].reasoningOutputPerMillionTokens", index), *model.ReasoningOutputPerMillionTokens, false)
		if err != nil {
			return PriceTableModel{}, err
		}
		normalized.ReasoningOutputPerMillionTokens = &rate
	}
	return normalized, nil
}

func normalizePriceRate(fieldPath, value string, required bool) (string, error) {
	if value == "" {
		if required {
			return "", fmt.Errorf("%w: %s is required", ErrPriceTableInvalid, fieldPath)
		}
		return "", fmt.Errorf("%w: %s must be a non-negative decimal string when supplied", ErrPriceTableInvalid, fieldPath)
	}
	if !priceTableDecimalPattern.MatchString(value) {
		return "", fmt.Errorf("%w: %s %q must be a non-negative decimal string", ErrPriceTableInvalid, fieldPath, value)
	}
	return value, nil
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

type WorkerSettings struct {
	ACP ACPSettings
}

type ACPSettings struct {
	Integrations []ACPIntegration
	AgentProfile *ACPAgentProfile
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
	models, err := normalizeModelConfigs(cfg.Models)
	if err != nil {
		return Config{}, err
	}
	priceTable, err := cfg.PriceTable.Normalize()
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
		PriceTable:     priceTable,
		Defaults: Defaults{
			WorkerModelProvider: strings.TrimSpace(cfg.Defaults.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(cfg.Defaults.WorkerModel),
		},
		Runtime:       runtime,
		Workers:       workers,
		WorkerPresets: presets,
		Models:        models,
	}, nil
}

func (settings WorkerSettings) normalize() (WorkerSettings, error) {
	normalized := WorkerSettings{}
	if settings.ACP.Integrations != nil {
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
		normalized.ACP.Integrations = integrations
	}
	if settings.ACP.AgentProfile != nil {
		profile, err := settings.ACP.AgentProfile.Normalize()
		if err != nil {
			return WorkerSettings{}, err
		}
		normalized.ACP.AgentProfile = &profile
	}
	return normalized, nil
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
			return nil, fmt.Errorf("workerPresets[%d] %q has unsupported reasoningEffort %q: accepted values are minimal, low, medium, high, xhigh, max", i, id, preset.ReasoningEffort)
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

// ErrConfigurationInvalid classifies a malformed operator configuration
// value. It is separate from document/storage failures so callers can report
// the authored model entry and field without exposing persistence details.
var ErrConfigurationInvalid = errors.New("operator configuration is invalid")

// ConfigurationFailureKind identifies the actionable part of a malformed
// model configuration entry.
type ConfigurationFailureKind string

const (
	ConfigurationFailureKindInvalidModelName   ConfigurationFailureKind = "invalid_model_name"
	ConfigurationFailureKindEmptyField         ConfigurationFailureKind = "empty_field"
	ConfigurationFailureKindUnsupportedPolicy  ConfigurationFailureKind = "unsupported_load_policy"
	ConfigurationFailureKindMalformedOperation ConfigurationFailureKind = "malformed_operation"
	ConfigurationFailureKindIncompleteModel    ConfigurationFailureKind = "incomplete_model"
)

// ConfigurationFailure retains the model entry, field, and normalized reason
// for one invalid operator model configuration. It deliberately contains no
// resolver, cache, backend, or filesystem state.
type ConfigurationFailure struct {
	Kind      ConfigurationFailureKind
	ModelName string
	Field     string
	Message   string
}

func (failure ConfigurationFailure) Error() string {
	location := fmt.Sprintf("models[%q]", strings.TrimSpace(failure.ModelName))
	if field := strings.TrimSpace(failure.Field); field != "" {
		location += "." + field
	}
	message := strings.TrimSpace(failure.Message)
	if message == "" {
		return fmt.Sprintf("%s: %s", ErrConfigurationInvalid, location)
	}
	return fmt.Sprintf("%s: %s: %s", ErrConfigurationInvalid, location, message)
}

func (failure ConfigurationFailure) Unwrap() error {
	return ErrConfigurationInvalid
}

// ModelLoadPolicy is the operator-facing load policy for one model entry.
// Resolution and activation remain outside this configuration contract.
type ModelLoadPolicy = string

const (
	// ModelLoadPolicyOnDemand leaves the model unloaded until a later
	// invocation stage needs it.
	ModelLoadPolicyOnDemand ModelLoadPolicy = "ON_DEMAND"
)

// LoadPolicy is retained as a concise alias for callers that already use the
// Models vocabulary at the Operator Settings boundary.
type LoadPolicy = ModelLoadPolicy

const LoadPolicyOnDemand = ModelLoadPolicyOnDemand

const (
	ModelOperationOMNI  = "OMNI"
	ModelOperationEMBED = "EMBED"
	ModelOperationTTS   = "TTS"
	ModelOperationASR   = "ASR"
)

var modelNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ModelConfig is one optional operator overlay. A nil field means that the
// built-in value, when one exists, remains unchanged. A non-nil field is an
// authored override and is therefore validated even when its value is empty.
type ModelConfig struct {
	Source     *string
	Backend    *string
	LoadPolicy *ModelLoadPolicy
	Operations []string
}

// ModelConfiguration is the descriptive alias used by configuration
// consumers.
type ModelConfiguration = ModelConfig

// Clone returns a detached operator configuration. It is intentionally a
// value operation: cloning does not resolve a model or perform any side
// effect.
func (cfg Config) Clone() Config {
	cloned := cfg
	if cfg.WorkerPresets != nil {
		cloned.WorkerPresets = make([]WorkerPreset, len(cfg.WorkerPresets))
		copy(cloned.WorkerPresets, cfg.WorkerPresets)
	}
	if cfg.Workers.ACP.Integrations != nil {
		cloned.Workers.ACP.Integrations = make([]ACPIntegration, len(cfg.Workers.ACP.Integrations))
		copy(cloned.Workers.ACP.Integrations, cfg.Workers.ACP.Integrations)
	}
	cloned.Workers.ACP.AgentProfile = cloneACPAgentProfilePointer(cfg.Workers.ACP.AgentProfile)
	cloned.PriceTable = cfg.PriceTable.Clone()
	cloned.Models = cloneModelConfigs(cfg.Models)
	return cloned
}

// Clone returns a detached model overlay, including its optional fields and
// operation-name collection.
func (config ModelConfig) Clone() ModelConfig {
	cloned := config
	cloned.Source = cloneModelConfigString(config.Source)
	cloned.Backend = cloneModelConfigString(config.Backend)
	if config.LoadPolicy != nil {
		policy := *config.LoadPolicy
		cloned.LoadPolicy = &policy
	}
	if config.Operations != nil {
		cloned.Operations = make([]string, len(config.Operations))
		copy(cloned.Operations, config.Operations)
	}
	return cloned
}

func cloneModelConfigString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneModelConfigs(values map[string]ModelConfig) map[string]ModelConfig {
	if values == nil {
		return nil
	}
	cloned := make(map[string]ModelConfig, len(values))
	for name, config := range values {
		cloned[name] = config.Clone()
	}
	return cloned
}

func normalizeModelConfigs(values map[string]ModelConfig) (map[string]ModelConfig, error) {
	if values == nil {
		return nil, nil
	}
	normalized := make(map[string]ModelConfig, len(values))
	names := make([]string, 0, len(values))
	for rawName := range values {
		names = append(names, rawName)
	}
	sort.Strings(names)
	for _, rawName := range names {
		config := values[rawName]
		name := strings.TrimSpace(rawName)
		if !modelNamePattern.MatchString(name) {
			return nil, ConfigurationFailure{
				Kind:      ConfigurationFailureKindInvalidModelName,
				ModelName: rawName,
				Field:     "name",
				Message:   "must be non-empty and contain only letters, digits, dots, hyphens, or underscores",
			}
		}
		if _, exists := normalized[name]; exists {
			return nil, ConfigurationFailure{
				Kind:      ConfigurationFailureKindInvalidModelName,
				ModelName: name,
				Field:     "name",
				Message:   "duplicates another model name after trimming",
			}
		}
		entry, err := config.normalize(name)
		if err != nil {
			return nil, err
		}
		normalized[name] = entry
	}
	return normalized, nil
}

func (config ModelConfig) normalize(name string) (ModelConfig, error) {
	normalized := ModelConfig{}
	var err error
	if normalized.Source, err = normalizeModelConfigString(name, "source", config.Source); err != nil {
		return ModelConfig{}, err
	}
	if normalized.Backend, err = normalizeModelConfigString(name, "backend", config.Backend); err != nil {
		return ModelConfig{}, err
	}
	if normalized.LoadPolicy, err = normalizeModelConfigLoadPolicy(name, config.LoadPolicy); err != nil {
		return ModelConfig{}, err
	}
	if normalized.Operations, err = normalizeModelConfigOperations(name, config.Operations); err != nil {
		return ModelConfig{}, err
	}
	if isBuiltInModelName(name) {
		return normalized, nil
	}
	for _, required := range []struct {
		field   string
		present bool
	}{
		{field: "source", present: normalized.Source != nil},
		{field: "backend", present: normalized.Backend != nil},
		{field: "loadPolicy", present: normalized.LoadPolicy != nil},
		{field: "operations", present: normalized.Operations != nil},
	} {
		if !required.present {
			return ModelConfig{}, ConfigurationFailure{
				Kind:      ConfigurationFailureKindIncompleteModel,
				ModelName: name,
				Field:     required.field,
				Message:   "is required for a new model entry; provide source, backend, loadPolicy, and operations",
			}
		}
	}
	return normalized, nil
}

func normalizeModelConfigString(name, field string, value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, ConfigurationFailure{
			Kind:      ConfigurationFailureKindEmptyField,
			ModelName: name,
			Field:     field,
			Message:   "must be omitted or non-empty",
		}
	}
	return &trimmed, nil
}

func normalizeModelConfigLoadPolicy(name string, value *ModelLoadPolicy) (*ModelLoadPolicy, error) {
	if value == nil {
		return nil, nil
	}
	canonical := ModelLoadPolicy(strings.ToUpper(strings.TrimSpace(string(*value))))
	if canonical == "" {
		return nil, ConfigurationFailure{
			Kind:      ConfigurationFailureKindEmptyField,
			ModelName: name,
			Field:     "loadPolicy",
			Message:   "must be omitted or non-empty",
		}
	}
	if canonical != ModelLoadPolicyOnDemand {
		return nil, ConfigurationFailure{
			Kind:      ConfigurationFailureKindUnsupportedPolicy,
			ModelName: name,
			Field:     "loadPolicy",
			Message:   fmt.Sprintf("unsupported value %q; accepted value is %s", canonical, ModelLoadPolicyOnDemand),
		}
	}
	return &canonical, nil
}

func normalizeModelConfigOperations(name string, values []string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	if len(values) == 0 {
		return nil, ConfigurationFailure{
			Kind:      ConfigurationFailureKindMalformedOperation,
			ModelName: name,
			Field:     "operations",
			Message:   "must be omitted or contain at least one generic operation",
		}
	}
	normalized := make([]string, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		canonical := strings.ToUpper(strings.TrimSpace(value))
		if !isGenericModelOperation(canonical) {
			return nil, ConfigurationFailure{
				Kind:      ConfigurationFailureKindMalformedOperation,
				ModelName: name,
				Field:     "operations",
				Message:   fmt.Sprintf("entry %d has unsupported operation %q; accepted values are OMNI, EMBED, TTS, and ASR", index, value),
			}
		}
		if _, exists := seen[canonical]; exists {
			return nil, ConfigurationFailure{
				Kind:      ConfigurationFailureKindMalformedOperation,
				ModelName: name,
				Field:     "operations",
				Message:   fmt.Sprintf("entry %d duplicates operation %q", index, canonical),
			}
		}
		seen[canonical] = struct{}{}
		normalized[index] = canonical
	}
	return normalized, nil
}

func isGenericModelOperation(value string) bool {
	switch value {
	case ModelOperationOMNI, ModelOperationEMBED, ModelOperationTTS, ModelOperationASR:
		return true
	default:
		return false
	}
}

func isBuiltInModelName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "llm", "asr", "tts", "embed":
		return true
	default:
		return false
	}
}
