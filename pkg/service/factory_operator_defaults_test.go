package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"go.uber.org/zap"
)

func TestBuildFactoryService_AppliesOperatorDefaultsToOmittedModelWorkerFields(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "executor",
			"type": "MODEL_WORKER",
			"body": "You are the executor.",
		}},
		"workstations": []map[string]any{{
			"name":    "execute-task",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":    "Implement {{ .WorkID }}.",
		}},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir: dir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider: "CODEX",
			WorkerModel:         "gpt-5-codex",
		},
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runtimeCfg := svc.currentRuntimeConfig()
	if runtimeCfg == nil {
		t.Fatal("expected current runtime config")
	}
	worker, ok := runtimeCfg.Worker("executor")
	if !ok {
		t.Fatal("expected executor worker")
	}
	if worker.ModelProvider != string(interfaces.ModelProviderCodex) {
		t.Fatalf("modelProvider = %q, want %q", worker.ModelProvider, interfaces.ModelProviderCodex)
	}
	if worker.Model != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", worker.Model)
	}
}

func TestBuildFactoryService_PreservesAuthoredModelWorkerFieldsOverOperatorDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "executor",
			"type":          "MODEL_WORKER",
			"modelProvider": "CLAUDE",
			"model":         "claude-sonnet-4-20250514",
			"body":          "You are the executor.",
		}},
		"workstations": []map[string]any{{
			"name":    "execute-task",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":    "Implement {{ .WorkID }}.",
		}},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir: dir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider: "CODEX",
			WorkerModel:         "gpt-5-codex",
		},
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	worker, ok := svc.currentRuntimeConfig().Worker("executor")
	if !ok {
		t.Fatal("expected executor worker")
	}
	if worker.ModelProvider != string(interfaces.ModelProviderClaude) {
		t.Fatalf("modelProvider = %q, want %q", worker.ModelProvider, interfaces.ModelProviderClaude)
	}
	if worker.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("model = %q, want authored model", worker.Model)
	}
}

func TestBuildReplacementFactoryRuntime_AppliesOperatorDefaults(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir: rootDir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider: "CODEX",
			WorkerModel:         "gpt-5-codex",
		},
		RuntimeMode:                             interfaces.RuntimeModeService,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	alphaWorker, ok := svc.currentRuntimeConfig().Worker("executor")
	if !ok {
		t.Fatal("expected alpha executor worker")
	}
	if alphaWorker.ModelProvider != string(interfaces.ModelProviderCodex) {
		t.Fatalf("alpha modelProvider = %q, want codex", alphaWorker.ModelProvider)
	}

	replacement, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, defaultFactorySessionID)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if replacement.dir != betaDir {
		t.Fatalf("replacement dir = %q, want %q", replacement.dir, betaDir)
	}
	betaWorker, ok := replacement.runtimeCfg.Worker("executor")
	if !ok {
		t.Fatal("expected beta executor worker")
	}
	if betaWorker.ModelProvider != string(interfaces.ModelProviderCodex) {
		t.Fatalf("beta modelProvider = %q, want codex", betaWorker.ModelProvider)
	}
	if betaWorker.Model != "gpt-5-codex" {
		t.Fatalf("beta model = %q, want gpt-5-codex", betaWorker.Model)
	}
	_ = alphaDir
}

func TestGeneratedFactoryFromRuntimeConfig_CapturesOperatorDefaultedModelWorkerFields(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "executor",
			"type": "MODEL_WORKER",
			"body": "You are the executor.",
		}},
		"workstations": []map[string]any{{
			"name":    "execute-task",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":    "Implement {{ .WorkID }}.",
		}},
	})

	loaded, err := config.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if err := config.ApplyOperatorDefaultsToLoadedConfig(loaded, operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CODEX",
		WorkerModel:         "gpt-5-codex",
	}); err != nil {
		t.Fatalf("ApplyOperatorDefaultsToLoadedConfig: %v", err)
	}

	generated, err := replay.GeneratedFactoryFromRuntimeConfig(dir, loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("generated workers = %#v, want one worker", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.ModelProvider == nil || string(*worker.ModelProvider) != "CODEX" {
		t.Fatalf("generated modelProvider = %#v, want CODEX", worker.ModelProvider)
	}
	if worker.Model == nil || *worker.Model != "gpt-5-codex" {
		t.Fatalf("generated model = %#v, want gpt-5-codex", worker.Model)
	}
}
