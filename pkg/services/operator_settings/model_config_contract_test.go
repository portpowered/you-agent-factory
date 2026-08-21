package operatorsettings_test

import (
	"errors"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func TestConfigNormalize_AllowsPartialBuiltInOverlay(t *testing.T) {
	source := " hf://custom/model "
	config, err := (operatorsettings.Config{Models: map[string]operatorsettings.ModelConfig{
		" llm ": {Source: &source},
	}}).Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	entry, ok := config.Models["llm"]
	if !ok {
		t.Fatalf("models = %#v, want trimmed built-in key", config.Models)
	}
	if entry.Source == nil || *entry.Source != "hf://custom/model" {
		t.Fatalf("source = %#v, want normalized override", entry.Source)
	}
	if entry.Backend != nil || entry.LoadPolicy != nil || entry.Operations != nil {
		t.Fatalf("partial overlay = %#v, want omitted fields preserved as nil", entry)
	}

	source = "changed after normalize"
	if *entry.Source != "hf://custom/model" {
		t.Fatalf("normalized source = %q, want detached from input pointer", *entry.Source)
	}
}

func TestConfigNormalize_RequiresCompleteNewModelAndCanonicalizesValues(t *testing.T) {
	source := " hf://owner/repo "
	backend := " localai-whisper "
	policy := operatorsettings.ModelLoadPolicy(" on_demand ")
	config, err := (operatorsettings.Config{Models: map[string]operatorsettings.ModelConfig{
		" studio-whisper ": {
			Source: sourcePointer(source), Backend: &backend, LoadPolicy: &policy,
			Operations: []string{" asr "},
		},
	}}).Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}

	entry := config.Models["studio-whisper"]
	if entry.Source == nil || *entry.Source != "hf://owner/repo" || entry.Backend == nil || *entry.Backend != "localai-whisper" {
		t.Fatalf("normalized entry = %#v, want trimmed fields", entry)
	}
	if entry.LoadPolicy == nil || *entry.LoadPolicy != operatorsettings.ModelLoadPolicyOnDemand {
		t.Fatalf("load policy = %#v, want ON_DEMAND", entry.LoadPolicy)
	}
	if len(entry.Operations) != 1 || entry.Operations[0] != operatorsettings.ModelOperationASR {
		t.Fatalf("operations = %#v, want [ASR]", entry.Operations)
	}
}

func TestConfigNormalize_ReportsTypedModelConfigurationFailures(t *testing.T) {
	empty := " "
	unsupportedPolicy := operatorsettings.ModelLoadPolicy("EAGER")
	tests := []struct {
		name  string
		model string
		entry operatorsettings.ModelConfig
		kind  operatorsettings.ConfigurationFailureKind
		field string
	}{
		{
			name: "invalid name", model: "bad name", entry: completeModelConfig(),
			kind: operatorsettings.ConfigurationFailureKindInvalidModelName, field: "name",
		},
		{
			name: "empty source", model: "new-model", entry: ModelConfigWithSource(&empty),
			kind: operatorsettings.ConfigurationFailureKindEmptyField, field: "source",
		},
		{
			name: "unsupported load policy", model: "new-model", entry: ModelConfigWithPolicy(&unsupportedPolicy),
			kind: operatorsettings.ConfigurationFailureKindUnsupportedPolicy, field: "loadPolicy",
		},
		{
			name: "malformed operation", model: "new-model", entry: ModelConfigWithOperations("UNKNOWN"),
			kind: operatorsettings.ConfigurationFailureKindMalformedOperation, field: "operations",
		},
		{
			name: "incomplete new model", model: "new-model", entry: operatorsettings.ModelConfig{},
			kind: operatorsettings.ConfigurationFailureKindIncompleteModel, field: "source",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (operatorsettings.Config{Models: map[string]operatorsettings.ModelConfig{
				test.model: test.entry,
			}}).Normalize()
			if err == nil {
				t.Fatal("Normalize() error = nil, want typed configuration failure")
			}
			if !errors.Is(err, operatorsettings.ErrConfigurationInvalid) {
				t.Fatalf("Normalize() error = %v, want ErrConfigurationInvalid", err)
			}
			var failure operatorsettings.ConfigurationFailure
			if !errors.As(err, &failure) {
				t.Fatalf("Normalize() error = %T %v, want ConfigurationFailure", err, err)
			}
			if failure.Kind != test.kind || failure.Field != test.field || !strings.Contains(err.Error(), `models["`+test.model+`"]`) {
				t.Fatalf("failure = %#v, error = %q, want kind=%q field=%q and model entry", failure, err, test.kind, test.field)
			}
		})
	}
}

func TestConfigClone_DetachesModelsMapEntriesAndOperations(t *testing.T) {
	original, err := (operatorsettings.Config{Models: map[string]operatorsettings.ModelConfig{
		"new-model": completeModelConfig(),
	}}).Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	cloned := original.Clone()
	clonedEntry := cloned.Models["new-model"]
	*clonedEntry.Source = "changed"
	clonedEntry.Operations[0] = operatorsettings.ModelOperationTTS
	cloned.Models["new-model"] = clonedEntry
	delete(cloned.Models, "new-model")

	if _, ok := original.Models["new-model"]; !ok {
		t.Fatal("original models map changed when clone entry was deleted")
	}
	originalEntry := original.Models["new-model"]
	if *originalEntry.Source != "hf://owner/repo" || originalEntry.Operations[0] != operatorsettings.ModelOperationASR {
		t.Fatalf("original entry = %#v, want detached clone", originalEntry)
	}
}

func TestDocumentClone_DetachesModelsMap(t *testing.T) {
	source := "hf://owner/repo"
	document := operatorsettings.Document{Models: map[string]operatorsettings.ModelConfig{
		"new-model": {Source: &source, Operations: []string{operatorsettings.ModelOperationASR}},
	}}
	cloned := document.Clone()
	entry := cloned.Models["new-model"]
	*entry.Source = "changed"
	entry.Operations[0] = operatorsettings.ModelOperationTTS
	cloned.Models["new-model"] = entry

	original := document.Models["new-model"]
	if *original.Source != "hf://owner/repo" || original.Operations[0] != operatorsettings.ModelOperationASR {
		t.Fatalf("original document entry = %#v, want detached clone", original)
	}
}

func completeModelConfig() operatorsettings.ModelConfig {
	return operatorsettings.ModelConfig{
		Source:     sourcePointer("hf://owner/repo"),
		Backend:    sourcePointer("localai-whisper"),
		LoadPolicy: policyPointer(operatorsettings.ModelLoadPolicyOnDemand),
		Operations: []string{operatorsettings.ModelOperationASR},
	}
}

func ModelConfigWithSource(source *string) operatorsettings.ModelConfig {
	config := completeModelConfig()
	config.Source = source
	return config
}

func ModelConfigWithPolicy(policy *operatorsettings.ModelLoadPolicy) operatorsettings.ModelConfig {
	config := completeModelConfig()
	config.LoadPolicy = policy
	return config
}

func ModelConfigWithOperations(operations ...string) operatorsettings.ModelConfig {
	config := completeModelConfig()
	config.Operations = operations
	return config
}

func sourcePointer(value string) *string { return &value }

func policyPointer(value operatorsettings.ModelLoadPolicy) *operatorsettings.ModelLoadPolicy {
	return &value
}
