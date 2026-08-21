package operatorsettings

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

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
