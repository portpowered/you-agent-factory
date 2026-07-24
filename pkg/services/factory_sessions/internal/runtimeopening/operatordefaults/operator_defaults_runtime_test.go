package operatordefaults_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatordefaultsruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening/operatordefaults"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

func TestApplyOperatorDefaultsToLoadedConfig_FillsOmittedModelWorkerFields(t *testing.T) {
	loaded := newOperatorDefaultsRuntimeFixture(t, map[string]any{
		"workers": []map[string]any{{
			"name": "executor",
			"type": "MODEL_WORKER",
			"body": "You are the executor.",
		}},
	})

	if err := operatordefaultsruntime.ApplyToLoadedConfig(loaded, operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CODEX",
		WorkerModel:         "gpt-5-codex",
	}); err != nil {
		t.Fatalf("ApplyOperatorDefaultsToLoadedConfig: %v", err)
	}

	worker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected executor worker")
	}
	if worker.ModelProvider != string(modelprovider.ProviderCodex) {
		t.Fatalf("modelProvider = %q, want %q", worker.ModelProvider, modelprovider.ProviderCodex)
	}
	if worker.Model != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", worker.Model)
	}
}

func TestApplyOperatorDefaultsToLoadedConfig_PreservesAuthoredModelWorkerFields(t *testing.T) {
	loaded := newOperatorDefaultsRuntimeFixture(t, map[string]any{
		"workers": []map[string]any{{
			"name":          "executor",
			"type":          "MODEL_WORKER",
			"modelProvider": "CLAUDE",
			"model":         "claude-sonnet-4-20250514",
			"body":          "You are the executor.",
		}},
	})

	if err := operatordefaultsruntime.ApplyToLoadedConfig(loaded, operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CODEX",
		WorkerModel:         "gpt-5-codex",
	}); err != nil {
		t.Fatalf("ApplyOperatorDefaultsToLoadedConfig: %v", err)
	}

	worker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("expected executor worker")
	}
	if worker.ModelProvider != string(modelprovider.ProviderClaude) {
		t.Fatalf("modelProvider = %q, want %q", worker.ModelProvider, modelprovider.ProviderClaude)
	}
	if worker.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("model = %q, want authored model", worker.Model)
	}
}

func TestApplyOperatorDefaultsToLoadedConfig_SkipsScriptAndHostedWorkers(t *testing.T) {
	factoryDir := t.TempDir()
	loaded, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{
			{Name: "script-worker", Type: interfaces.WorkerTypeScript, Body: "Run scripts."},
			{Name: "hosted-worker", Type: interfaces.WorkerTypeHosted, Provider: interfaces.HostedWorkerProviderLinear, Body: "Poll linear."},
		},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	if err = operatordefaultsruntime.ApplyToLoadedConfig(loaded, operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CODEX",
		WorkerModel:         "gpt-5-codex",
	}); err != nil {
		t.Fatalf("ApplyOperatorDefaultsToLoadedConfig: %v", err)
	}

	for _, name := range []string{"script-worker", "hosted-worker"} {
		worker, ok := loaded.Worker(name)
		if !ok {
			t.Fatalf("expected worker %q", name)
		}
		if worker.ModelProvider != "" {
			t.Fatalf("worker %q modelProvider = %q, want empty", name, worker.ModelProvider)
		}
		if worker.Model != "" {
			t.Fatalf("worker %q model = %q, want empty", name, worker.Model)
		}
	}
}

func TestResolveConcreteProviderSelectionsRejectsUnknownProviderAtWorkerField(t *testing.T) {
	factoryDir := t.TempDir()
	loaded, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{{
			Name:          "executor",
			Type:          interfaces.WorkerTypeModel,
			ModelProvider: "not-a-provider",
			Body:          "You are the executor.",
		}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	err = operatordefaultsruntime.ResolveConcreteProviderSelections(
		loaded,
		providerResolverStub{errors: map[string]error{
			"not-a-provider": errors.New(`provider "not-a-provider" is unknown`),
		}}.CanonicalIdentity,
	)
	if err == nil {
		t.Fatal("expected unknown provider validation error")
	}
	if !strings.Contains(err.Error(), "workers[0].modelProvider") ||
		!strings.Contains(err.Error(), `provider "not-a-provider" is unknown`) {
		t.Fatalf("error = %q, want field-local unknown-provider diagnostic", err.Error())
	}
}

func TestResolveConcreteProviderSelectionsCanonicalizesWorkersAndGuards(t *testing.T) {
	factoryDir := t.TempDir()
	loaded, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{{
			Name:          "executor",
			Type:          interfaces.WorkerTypeModel,
			ModelProvider: "customer",
			Body:          "You are the executor.",
		}},
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			ModelProvider: "agent",
		}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	resolver := providerResolverStub{canonical: map[string]string{
		"customer": "customer.provider",
		"agent":    "cursor",
	}}
	if err := operatordefaultsruntime.ResolveConcreteProviderSelections(
		loaded,
		resolver.CanonicalIdentity,
	); err != nil {
		t.Fatalf("ResolveConcreteProviderSelections: %v", err)
	}
	worker, ok := loaded.Worker("executor")
	if !ok || worker.ModelProvider != "customer.provider" {
		t.Fatalf("worker = %#v, want canonical extension provider", worker)
	}
	if got := loaded.FactoryConfig().Guards[0].ModelProvider; got != "cursor" {
		t.Fatalf("guard modelProvider = %q, want canonical cursor identity", got)
	}
}

func TestResolveConcreteProviderSelectionsRejectsNonSelectableGuardAtGuardField(t *testing.T) {
	factoryDir := t.TempDir()
	loaded, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, &interfaces.FactoryConfig{
		Guards: []interfaces.FactoryGuardConfig{{
			Type:          interfaces.GuardTypeInferenceThrottle,
			ModelProvider: "agy",
		}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	err = operatordefaultsruntime.ResolveConcreteProviderSelections(
		loaded,
		providerResolverStub{errors: map[string]error{
			"agy": errors.New(`provider "agy" is not selectable (catalog-only)`),
		}}.CanonicalIdentity,
	)
	if err == nil ||
		!strings.Contains(err.Error(), "guards[0].modelProvider") ||
		!strings.Contains(err.Error(), `provider "agy" is not selectable (catalog-only)`) {
		t.Fatalf("error = %v, want field-local catalog-only diagnostic", err)
	}
}

func TestResolveConcreteProviderSelectionsAllowsInvocationInterpolationProvider(t *testing.T) {
	factoryDir := t.TempDir()
	loaded, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, &interfaces.FactoryConfig{
		InvocationSignature: &interfaces.InvocationSignatureConfig{
			Parameters: []interfaces.InvocationParameterConfig{{
				Name: "firstProvider",
			}},
		},
		Workers: []interfaces.FactoryWorkerConfig{{
			Name:          "executor",
			Type:          interfaces.WorkerTypeModel,
			ModelProvider: "${firstProvider}",
			Body:          "You are the executor.",
		}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	resolver := &recordingProviderResolver{}
	if err := operatordefaultsruntime.ResolveConcreteProviderSelections(
		loaded,
		resolver.CanonicalIdentity,
	); err != nil {
		t.Fatalf("ResolveConcreteProviderSelections: %v", err)
	}
	if len(resolver.identities) != 0 {
		t.Fatalf("registry lookups = %v, want none for unresolved invocation provider", resolver.identities)
	}
}

func TestApplyOperatorDefaultsToLoadedConfigPreservesExtensionProvider(t *testing.T) {
	loaded := newOperatorDefaultsRuntimeFixture(t, map[string]any{
		"workers": []map[string]any{{
			"name": "executor",
			"type": "MODEL_WORKER",
			"body": "You are the executor.",
		}},
	})

	if err := operatordefaultsruntime.ApplyToLoadedConfig(loaded, operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "customer.provider",
	}); err != nil {
		t.Fatalf("ApplyToLoadedConfig: %v", err)
	}
	worker, ok := loaded.Worker("executor")
	if !ok || worker.ModelProvider != "customer.provider" {
		t.Fatalf("worker = %#v, want preserved extension default", worker)
	}
}

type providerResolverStub struct {
	canonical map[string]string
	errors    map[string]error
}

func (r providerResolverStub) CanonicalIdentity(identity string) (string, error) {
	if err := r.errors[identity]; err != nil {
		return "", err
	}
	if canonical := r.canonical[identity]; canonical != "" {
		return canonical, nil
	}
	return identity, nil
}

type recordingProviderResolver struct {
	identities []string
}

func (r *recordingProviderResolver) CanonicalIdentity(identity string) (string, error) {
	r.identities = append(r.identities, identity)
	return identity, nil
}

func newOperatorDefaultsRuntimeFixture(t *testing.T, factory map[string]any) interfaces.MutableLoadedFactorySource {
	t.Helper()

	factoryDir := t.TempDir()
	payload, err := json.Marshal(mergeOperatorDefaultsFactoryFixture(factory))
	if err != nil {
		t.Fatalf("Marshal(factory): %v", err)
	}
	config, err := factorymapping.NewFactoryConfigMapper().Expand(payload)
	if err != nil {
		t.Fatalf("Expand(factory): %v", err)
	}
	loaded, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, config, nil, nil)
	if err != nil {
		t.Fatalf("NewLoadedSource: %v", err)
	}
	return loaded
}

func mergeOperatorDefaultsFactoryFixture(factory map[string]any) map[string]any {
	base := map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workstations": []map[string]any{{
			"name":    "execute-task",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
			"type":    "MODEL_WORKSTATION",
			"body":    "Implement {{ .WorkID }}.",
		}},
	}
	for key, value := range factory {
		base[key] = value
	}
	return base
}

func writeOperatorDefaultsFactoryJSON(t *testing.T, factoryDir string, factory map[string]any) {
	t.Helper()

	data, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("Marshal(factory): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
}
