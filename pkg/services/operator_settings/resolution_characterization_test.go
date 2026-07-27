package operatorsettings_test

import (
	"errors"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// resolutionPeerFake implements effective resolution using only Operator Settings
// root contracts and plain in-memory facts. It does not import filesystem, codec,
// CLI, UI, Wire, or Initializer packages.
type resolutionPeerFake struct{}

func newResolutionPeerFake() *resolutionPeerFake {
	return &resolutionPeerFake{}
}

func (fake *resolutionPeerFake) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	if err := request.Validate(); err != nil {
		return operatorsettings.ResolveEffectiveResult{}, err
	}

	providerRaw, providerSource := fake.winningLayerValue(
		request.DocumentBaseline.WorkerModelProvider,
		request.EnvironmentOverrides.WorkerModelProvider,
		request.InvocationOverrides.WorkerModelProvider,
	)
	modelRaw, modelSource := fake.winningLayerValue(
		request.DocumentBaseline.WorkerModel,
		request.EnvironmentOverrides.WorkerModel,
		request.InvocationOverrides.WorkerModel,
	)

	resolvedProvider, err := fake.resolveWorkerModelProvider(
		providerRaw,
		providerSource,
		request,
	)
	if err != nil {
		return operatorsettings.ResolveEffectiveResult{}, err
	}

	return operatorsettings.ResolveEffectiveResult{
		Selection: operatorsettings.EffectiveSelection{
			WorkerModelProvider:       resolvedProvider,
			WorkerModel:               strings.TrimSpace(modelRaw),
			WorkerModelProviderSource: providerSource,
			WorkerModelSource:         modelSource,
			ConfigPath:                strings.TrimSpace(request.ConfigPath),
		},
	}, nil
}

func (fake *resolutionPeerFake) winningLayerValue(
	baselineValue, envValue, flagValue string,
) (string, operatorsettings.EffectiveLayerSource) {
	switch {
	case strings.TrimSpace(flagValue) != "":
		return strings.TrimSpace(flagValue), operatorsettings.EffectiveLayerSourceFlag
	case strings.TrimSpace(envValue) != "":
		return strings.TrimSpace(envValue), operatorsettings.EffectiveLayerSourceEnv
	case strings.TrimSpace(baselineValue) != "":
		return strings.TrimSpace(baselineValue), operatorsettings.EffectiveLayerSourceFile
	default:
		return "", ""
	}
}

func (fake *resolutionPeerFake) resolveWorkerModelProvider(
	raw string,
	winningSource operatorsettings.EffectiveLayerSource,
	request operatorsettings.ResolveEffectiveRequest,
) (string, error) {
	if raw == "" {
		return "", nil
	}

	canonical, ok := fake.canonicalizeProvider(raw)
	if !ok {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindUnsupportedOverride,
			Message: raw,
			Field:   "workerModelProvider",
		}
	}
	if canonical != "DEFAULT" {
		return canonical, nil
	}

	concreteRaw := fake.concreteProviderBelowSource(winningSource, request)
	if concreteRaw == "" {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindInvalidInput,
			Message: "symbolic DEFAULT requires a concrete provider from file or environment",
			Field:   "workerModelProvider",
		}
	}
	concreteCanonical, ok := fake.canonicalizeProvider(concreteRaw)
	if !ok || concreteCanonical == "DEFAULT" {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindInvalidInput,
			Message: "symbolic DEFAULT requires a concrete provider from file or environment",
			Field:   "workerModelProvider",
		}
	}
	return concreteCanonical, nil
}

func (fake *resolutionPeerFake) concreteProviderBelowSource(
	winningSource operatorsettings.EffectiveLayerSource,
	request operatorsettings.ResolveEffectiveRequest,
) string {
	type layer struct {
		source operatorsettings.EffectiveLayerSource
		value  string
	}
	layers := []layer{
		{source: operatorsettings.EffectiveLayerSourceFile, value: request.DocumentBaseline.WorkerModelProvider},
		{source: operatorsettings.EffectiveLayerSourceEnv, value: request.EnvironmentOverrides.WorkerModelProvider},
		{source: operatorsettings.EffectiveLayerSourceFlag, value: request.InvocationOverrides.WorkerModelProvider},
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
		if value == "" || strings.EqualFold(value, "DEFAULT") {
			continue
		}
		return value
	}
	return ""
}

func (fake *resolutionPeerFake) canonicalizeProvider(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	switch strings.ToLower(value) {
	case "", "default":
		return "DEFAULT", true
	case "codex", "openai":
		return "CODEX", true
	case "claude", "anthropic":
		return "CLAUDE", true
	case "gemini":
		return "GEMINI", true
	default:
		if strings.Contains(value, ".") {
			return value, true
		}
		return "", false
	}
}

func TestResolutionContract_Characterization_EffectiveSelectionSuccess(t *testing.T) {
	t.Parallel()

	fake := newResolutionPeerFake()
	configPath := "/home/operator/.you-agent-factory/config.json"

	resolved, err := fake.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "claude",
			WorkerModel:         "file-model",
		},
		EnvironmentOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "codex",
			WorkerModel:         "env-model",
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "gemini",
			WorkerModel:         "flag-model",
		},
		ConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}

	selection := resolved.Selection
	if selection.WorkerModelProvider != "GEMINI" {
		t.Fatalf("provider = %q, want GEMINI", selection.WorkerModelProvider)
	}
	if selection.WorkerModel != "flag-model" {
		t.Fatalf("model = %q, want flag-model", selection.WorkerModel)
	}
	if selection.WorkerModelProviderSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("provider source = %q, want flag", selection.WorkerModelProviderSource)
	}
	if selection.WorkerModelSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("model source = %q, want flag", selection.WorkerModelSource)
	}
	if selection.ConfigPath != configPath {
		t.Fatalf("config path = %q, want %q", selection.ConfigPath, configPath)
	}

	cloned := selection.Clone()
	cloned.WorkerModel = "mutated"
	if selection.WorkerModel == "mutated" {
		t.Fatalf("Clone() did not detach model: %#v", selection)
	}
}

func TestResolutionContract_Characterization_SymbolicDefaultResolvesThroughFileBaseline(t *testing.T) {
	t.Parallel()

	fake := newResolutionPeerFake()
	resolved, err := fake.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: "codex",
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "DEFAULT",
		},
		ConfigPath: "/tmp/config.json",
	})
	if err != nil {
		t.Fatalf("ResolveEffective() = %v", err)
	}
	if resolved.Selection.WorkerModelProvider != "CODEX" {
		t.Fatalf("provider = %q, want CODEX", resolved.Selection.WorkerModelProvider)
	}
	if resolved.Selection.WorkerModelProviderSource != operatorsettings.EffectiveLayerSourceFlag {
		t.Fatalf("provider source = %q, want flag", resolved.Selection.WorkerModelProviderSource)
	}
}

func TestResolutionContract_Characterization_TypedFailures(t *testing.T) {
	t.Parallel()

	fake := newResolutionPeerFake()

	_, err := fake.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "unsupported-provider",
		},
		ConfigPath: "/tmp/config.json",
	})
	if !errors.Is(err, operatorsettings.ErrResolutionUnsupportedOverride) {
		t.Fatalf("unsupported override error = %v, want ErrResolutionUnsupportedOverride", err)
	}
	var unsupported operatorsettings.ResolutionFailure
	if !errors.As(err, &unsupported) ||
		unsupported.Kind != operatorsettings.ResolutionFailureKindUnsupportedOverride {
		t.Fatalf("unsupported override failure = %#v", err)
	}

	_, err = fake.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: "DEFAULT",
		},
		ConfigPath: "/tmp/config.json",
	})
	if !errors.Is(err, operatorsettings.ErrResolutionInvalidInput) {
		t.Fatalf("unresolved DEFAULT error = %v, want ErrResolutionInvalidInput", err)
	}

	expected := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5",
	}
	stale := operatorsettings.DocumentDefaults{
		WorkerModelProvider: "claude",
		WorkerModel:         "gpt-5",
	}
	_, err = fake.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline:         stale,
		ExpectedDocumentBaseline: &expected,
		ConfigPath:               "/tmp/config.json",
	})
	if !errors.Is(err, operatorsettings.ErrResolutionConflict) {
		t.Fatalf("baseline conflict error = %v, want ErrResolutionConflict", err)
	}
}
