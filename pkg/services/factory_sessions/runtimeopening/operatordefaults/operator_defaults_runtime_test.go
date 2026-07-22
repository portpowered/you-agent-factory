package operatordefaults_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitionfixtures "github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatordefaultsruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimeopening/operatordefaults"
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

func TestValidateModelWorkerRuntimeProviders_RejectsUnsupportedProvider(t *testing.T) {
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

	err = operatordefaultsruntime.ValidateModelWorkerRuntimeProviders(loaded)
	if err == nil {
		t.Fatal("expected unsupported provider validation error")
	}
	if !strings.Contains(err.Error(), "unsupported model provider") {
		t.Fatalf("error = %q, want unsupported provider guidance", err.Error())
	}
}

func TestValidateModelWorkerRuntimeProviders_AllowsInvocationInterpolationProvider(t *testing.T) {
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

	if err := operatordefaultsruntime.ValidateModelWorkerRuntimeProviders(loaded); err != nil {
		t.Fatalf("ValidateModelWorkerRuntimeProviders: %v", err)
	}
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
