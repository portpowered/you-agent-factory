package operatorsettings

import (
	"errors"
	"strings"
	"testing"
)

func TestACPAgentProfileNormalizeTrimsAndPreservesOrder(t *testing.T) {
	t.Parallel()

	profile := ACPAgentProfile{
		DefaultTarget:  " factory:@you/review ",
		AllowedTargets: []string{" factory:@you/review ", " factory:@you/factory-builder ", " factory:local/software-auto "},
	}
	got, err := profile.Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	want := ACPAgentProfile{
		DefaultTarget: "factory:@you/review",
		AllowedTargets: []string{
			"factory:@you/review",
			"factory:@you/factory-builder",
			"factory:local/software-auto",
		},
	}
	if got.DefaultTarget != want.DefaultTarget {
		t.Fatalf("DefaultTarget = %q, want %q", got.DefaultTarget, want.DefaultTarget)
	}
	if len(got.AllowedTargets) != len(want.AllowedTargets) {
		t.Fatalf("AllowedTargets = %#v, want %#v", got.AllowedTargets, want.AllowedTargets)
	}
	for i := range want.AllowedTargets {
		if got.AllowedTargets[i] != want.AllowedTargets[i] {
			t.Fatalf("AllowedTargets[%d] = %q, want %q (order must be preserved)", i, got.AllowedTargets[i], want.AllowedTargets[i])
		}
	}
}

func TestACPAgentProfileNormalizeRejectsInvalidShapes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		profile ACPAgentProfile
	}{
		{
			name:    "blank default",
			profile: ACPAgentProfile{DefaultTarget: "   ", AllowedTargets: []string{"factory:@you/factory-builder"}},
		},
		{
			name:    "default outside factory namespace",
			profile: ACPAgentProfile{DefaultTarget: "worker:@you/factory-builder", AllowedTargets: []string{"worker:@you/factory-builder"}},
		},
		{
			name:    "default is bare namespace with no reference",
			profile: ACPAgentProfile{DefaultTarget: "factory:", AllowedTargets: []string{"factory:"}},
		},
		{
			name:    "blank allowlist entry",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/factory-builder", AllowedTargets: []string{"factory:@you/factory-builder", "  "}},
		},
		{
			name:    "allowlist entry outside factory namespace",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/factory-builder", AllowedTargets: []string{"factory:@you/factory-builder", "local/software-auto"}},
		},
		{
			name: "duplicate entries after normalization",
			profile: ACPAgentProfile{
				DefaultTarget:  "factory:@you/factory-builder",
				AllowedTargets: []string{"factory:@you/factory-builder", " factory:@you/factory-builder "},
			},
		},
		{
			name:    "default absent from allowlist",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/review", AllowedTargets: []string{"factory:@you/factory-builder"}},
		},
		{
			name:    "default reference contains internal whitespace",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/bad ref", AllowedTargets: []string{"factory:@you/bad ref"}},
		},
		{
			name:    "allowlist entry contains a control character",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/factory-builder", AllowedTargets: []string{"factory:@you/factory-builder", "factory:@you/bad\nref"}},
		},
		{
			name:    "reference has no path segment after the scope",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you", AllowedTargets: []string{"factory:@you"}},
		},
		{
			name:    "reference has an empty path segment",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/", AllowedTargets: []string{"factory:@you/"}},
		},
		{
			name:    "reference segment has a leading hyphen",
			profile: ACPAgentProfile{DefaultTarget: "factory:@you/-builder", AllowedTargets: []string{"factory:@you/-builder"}},
		},
		{
			name:    "reference uses uppercase characters",
			profile: ACPAgentProfile{DefaultTarget: "factory:@You/Factory-Builder", AllowedTargets: []string{"factory:@You/Factory-Builder"}},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			if _, err := testCase.profile.Normalize(); !errors.Is(err, ErrACPAgentProfileInvalid) {
				t.Fatalf("Normalize() error = %v, want ErrACPAgentProfileInvalid", err)
			}
		})
	}
}

func TestACPAgentProfileCloneDoesNotAliasAllowedTargets(t *testing.T) {
	t.Parallel()

	original := ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder", "factory:@you/review"},
	}
	cloned := original.Clone()
	cloned.AllowedTargets[0] = "factory:mutated/target"

	if original.AllowedTargets[0] != "factory:@you/factory-builder" {
		t.Fatalf("original mutated through clone: %#v", original.AllowedTargets)
	}
}

func TestDefaultACPAgentProfileIsUnrestrictedWithFactoryBuilderDefault(t *testing.T) {
	t.Parallel()

	profile := DefaultACPAgentProfile()
	if profile.DefaultTarget != DefaultACPAgentProfileTarget {
		t.Fatalf("DefaultTarget = %q, want %q", profile.DefaultTarget, DefaultACPAgentProfileTarget)
	}
	// An operator who authored no profile has restricted nothing, so every
	// installed Factory stays selectable and Factory Builder is merely the
	// target that starts current.
	if !profile.IsUnrestricted() {
		t.Fatalf("IsUnrestricted() = false, want true; AllowedTargets = %#v", profile.AllowedTargets)
	}
	if normalized, err := profile.Normalize(); err != nil {
		t.Fatalf("DefaultACPAgentProfile() must already be normalized, got error = %v", err)
	} else if normalized.DefaultTarget != profile.DefaultTarget || !normalized.IsUnrestricted() {
		t.Fatalf("DefaultACPAgentProfile() normalized to a different value: %#v", normalized)
	}
}

func TestACPAgentProfileNormalizeTreatsEmptyAllowlistAsUnrestricted(t *testing.T) {
	t.Parallel()

	// Reaching Normalize with no entries means the operator omitted the
	// restriction: the decode boundary rejects an explicitly authored empty
	// array before it can get here.
	profile := ACPAgentProfile{DefaultTarget: "factory:@you/goal"}
	normalized, err := profile.Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v, want nil", err)
	}
	if !normalized.IsUnrestricted() {
		t.Fatalf("IsUnrestricted() = false, want true; AllowedTargets = %#v", normalized.AllowedTargets)
	}
	if normalized.DefaultTarget != "factory:@you/goal" {
		t.Fatalf("DefaultTarget = %q, want %q", normalized.DefaultTarget, "factory:@you/goal")
	}
}

func TestACPAgentProfileNormalizeKeepsAuthoredRestriction(t *testing.T) {
	t.Parallel()

	profile := ACPAgentProfile{
		DefaultTarget:  "factory:@you/goal",
		AllowedTargets: []string{"factory:@you/goal", "factory:@you/classify"},
	}
	normalized, err := profile.Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v, want nil", err)
	}
	if normalized.IsUnrestricted() {
		t.Fatal("IsUnrestricted() = true, want false for an authored allowlist")
	}
	if len(normalized.AllowedTargets) != 2 ||
		normalized.AllowedTargets[0] != "factory:@you/goal" ||
		normalized.AllowedTargets[1] != "factory:@you/classify" {
		t.Fatalf("AllowedTargets = %#v, want authored order preserved", normalized.AllowedTargets)
	}
}

func TestWorkerSettingsNormalizePreservesAgentProfileWithoutIntegrations(t *testing.T) {
	t.Parallel()

	settings := WorkerSettings{ACP: ACPSettings{
		AgentProfile: &ACPAgentProfile{
			DefaultTarget:  " factory:@you/review ",
			AllowedTargets: []string{" factory:@you/review "},
		},
	}}
	normalized, err := settings.normalize()
	if err != nil {
		t.Fatalf("normalize() error = %v", err)
	}
	if normalized.ACP.Integrations != nil {
		t.Fatalf("Integrations = %#v, want nil", normalized.ACP.Integrations)
	}
	if normalized.ACP.AgentProfile == nil || normalized.ACP.AgentProfile.DefaultTarget != "factory:@you/review" {
		t.Fatalf("AgentProfile = %#v, want normalized factory:@you/review", normalized.ACP.AgentProfile)
	}
}

func TestWorkerSettingsNormalizeRejectsInvalidAgentProfile(t *testing.T) {
	t.Parallel()

	settings := WorkerSettings{ACP: ACPSettings{
		AgentProfile: &ACPAgentProfile{DefaultTarget: "", AllowedTargets: nil},
	}}
	if _, err := settings.normalize(); !errors.Is(err, ErrACPAgentProfileInvalid) {
		t.Fatalf("normalize() error = %v, want ErrACPAgentProfileInvalid", err)
	}
}

func TestConfigDocumentFileConfigDoesNotAliasAgentProfile(t *testing.T) {
	t.Parallel()

	document := ConfigDocument{config: Config{Workers: WorkerSettings{ACP: ACPSettings{
		AgentProfile: &ACPAgentProfile{
			DefaultTarget:  "factory:@you/factory-builder",
			AllowedTargets: []string{"factory:@you/factory-builder"},
		},
	}}}}

	first := document.FileConfig()
	first.Workers.ACP.AgentProfile.DefaultTarget = "factory:mutated"
	first.Workers.ACP.AgentProfile.AllowedTargets[0] = "factory:mutated"

	second := document.FileConfig()
	if second.Workers.ACP.AgentProfile.DefaultTarget != "factory:@you/factory-builder" {
		t.Fatalf("stored DefaultTarget mutated through FileConfig(): %#v", second.Workers.ACP.AgentProfile)
	}
	if second.Workers.ACP.AgentProfile.AllowedTargets[0] != "factory:@you/factory-builder" {
		t.Fatalf("stored AllowedTargets mutated through FileConfig(): %#v", second.Workers.ACP.AgentProfile)
	}
}

func TestDocumentACPSettingsCloneDoesNotAliasAgentProfile(t *testing.T) {
	t.Parallel()

	original := DocumentACPSettings{AgentProfile: &ACPAgentProfile{
		DefaultTarget:  "factory:@you/factory-builder",
		AllowedTargets: []string{"factory:@you/factory-builder"},
	}}
	cloned := original.Clone()
	cloned.AgentProfile.DefaultTarget = "factory:mutated"
	cloned.AgentProfile.AllowedTargets[0] = "factory:mutated"

	if original.AgentProfile.DefaultTarget != "factory:@you/factory-builder" {
		t.Fatalf("original DefaultTarget mutated through Clone(): %#v", original.AgentProfile)
	}
	if original.AgentProfile.AllowedTargets[0] != "factory:@you/factory-builder" {
		t.Fatalf("original AllowedTargets mutated through Clone(): %#v", original.AgentProfile)
	}
}

func TestConfigNormalize_AllowsPartialBuiltInOverlay(t *testing.T) {
	source := " hf://custom/model "
	config, err := (Config{Models: map[string]ModelConfig{
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
	policy := ModelLoadPolicy(" on_demand ")
	config, err := (Config{Models: map[string]ModelConfig{
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
	if entry.LoadPolicy == nil || *entry.LoadPolicy != ModelLoadPolicyOnDemand {
		t.Fatalf("load policy = %#v, want ON_DEMAND", entry.LoadPolicy)
	}
	if len(entry.Operations) != 1 || entry.Operations[0] != ModelOperationASR {
		t.Fatalf("operations = %#v, want [ASR]", entry.Operations)
	}
}

func TestConfigNormalize_ReportsTypedModelConfigurationFailures(t *testing.T) {
	empty := " "
	unsupportedPolicy := ModelLoadPolicy("EAGER")
	tests := []struct {
		name  string
		model string
		entry ModelConfig
		kind  ConfigurationFailureKind
		field string
	}{
		{
			name: "invalid name", model: "bad name", entry: completeModelConfig(),
			kind: ConfigurationFailureKindInvalidModelName, field: "name",
		},
		{
			name: "empty source", model: "new-model", entry: ModelConfigWithSource(&empty),
			kind: ConfigurationFailureKindEmptyField, field: "source",
		},
		{
			name: "unsupported load policy", model: "new-model", entry: ModelConfigWithPolicy(&unsupportedPolicy),
			kind: ConfigurationFailureKindUnsupportedPolicy, field: "loadPolicy",
		},
		{
			name: "malformed operation", model: "new-model", entry: ModelConfigWithOperations("UNKNOWN"),
			kind: ConfigurationFailureKindMalformedOperation, field: "operations",
		},
		{
			name: "incomplete new model", model: "new-model", entry: ModelConfig{},
			kind: ConfigurationFailureKindIncompleteModel, field: "source",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Config{Models: map[string]ModelConfig{
				test.model: test.entry,
			}}).Normalize()
			if err == nil {
				t.Fatal("Normalize() error = nil, want typed configuration failure")
			}
			if !errors.Is(err, ErrConfigurationInvalid) {
				t.Fatalf("Normalize() error = %v, want ErrConfigurationInvalid", err)
			}
			var failure ConfigurationFailure
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
	original, err := (Config{Models: map[string]ModelConfig{
		"new-model": completeModelConfig(),
	}}).Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	cloned := original.Clone()
	clonedEntry := cloned.Models["new-model"]
	*clonedEntry.Source = "changed"
	clonedEntry.Operations[0] = ModelOperationTTS
	cloned.Models["new-model"] = clonedEntry
	delete(cloned.Models, "new-model")

	if _, ok := original.Models["new-model"]; !ok {
		t.Fatal("original models map changed when clone entry was deleted")
	}
	originalEntry := original.Models["new-model"]
	if *originalEntry.Source != "hf://owner/repo" || originalEntry.Operations[0] != ModelOperationASR {
		t.Fatalf("original entry = %#v, want detached clone", originalEntry)
	}
}

func TestDocumentClone_DetachesModelsMap(t *testing.T) {
	source := "hf://owner/repo"
	document := Document{Models: map[string]ModelConfig{
		"new-model": {Source: &source, Operations: []string{ModelOperationASR}},
	}}
	cloned := document.Clone()
	entry := cloned.Models["new-model"]
	*entry.Source = "changed"
	entry.Operations[0] = ModelOperationTTS
	cloned.Models["new-model"] = entry

	original := document.Models["new-model"]
	if *original.Source != "hf://owner/repo" || original.Operations[0] != ModelOperationASR {
		t.Fatalf("original document entry = %#v, want detached clone", original)
	}
}

func completeModelConfig() ModelConfig {
	return ModelConfig{
		Source:     sourcePointer("hf://owner/repo"),
		Backend:    sourcePointer("localai-whisper"),
		LoadPolicy: policyPointer(ModelLoadPolicyOnDemand),
		Operations: []string{ModelOperationASR},
	}
}

func ModelConfigWithSource(source *string) ModelConfig {
	config := completeModelConfig()
	config.Source = source
	return config
}

func ModelConfigWithPolicy(policy *ModelLoadPolicy) ModelConfig {
	config := completeModelConfig()
	config.LoadPolicy = policy
	return config
}

func ModelConfigWithOperations(operations ...string) ModelConfig {
	config := completeModelConfig()
	config.Operations = operations
	return config
}

func sourcePointer(value string) *string { return &value }

func policyPointer(value ModelLoadPolicy) *ModelLoadPolicy {
	return &value
}
