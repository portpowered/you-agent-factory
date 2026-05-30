package load_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/load"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestLoadFromFactoryDir_MatchesLoadRuntimeConfigFromFactoryDirForNamedFactory(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")

	factoryDir, err := config.PersistNamedFactory(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	facadeLoaded, err := load.LoadFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("load.LoadFromFactoryDir: %v", err)
	}
	legacyLoaded, err := config.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("config.LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	assertEquivalentLoadedFactoryConfigs(t, facadeLoaded, legacyLoaded)
	if facadeLoaded.FactoryConfig().Name != "alpha" {
		t.Fatalf("name = %q, want alpha", facadeLoaded.FactoryConfig().Name)
	}
}

func TestLoadFromFactoryDir_MatchesLoadRuntimeConfigFromFactoryDirForInlineFactory(t *testing.T) {
	factoryDir := t.TempDir()
	writeRuntimeFactoryJSON(t, factoryDir, map[string]any{
		"name": "inline-load",
		"id":   "inline-load",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-inline",
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
				"type":    "MODEL_WORKSTATION",
				"body":    "Implement {{ .WorkID }}.",
			},
		},
	})

	facadeLoaded, err := load.LoadFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("load.LoadFromFactoryDir: %v", err)
	}
	legacyLoaded, err := config.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("config.LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	assertEquivalentLoadedFactoryConfigs(t, facadeLoaded, legacyLoaded)
}

func TestIsInvalidNamedFactory_DetectsPersistValidationFailure(t *testing.T) {
	rootDir := t.TempDir()

	_, err := config.PersistNamedFactory(rootDir, "broken", []byte(`{"name":"broken"`))
	if err == nil {
		t.Fatal("expected invalid named factory payload to fail")
	}
	if !load.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestLoadRuntimeConfig_PreservesFactoryLayoutNotFoundSemantics(t *testing.T) {
	rootDir := t.TempDir()

	_, err := config.LoadRuntimeConfig(rootDir, nil)
	if err == nil {
		t.Fatal("expected layout-not-found error")
	}
	if !load.IsFactoryLayoutNotFound(err) {
		t.Fatalf("error = %v, want ErrFactoryLayoutNotFound", err)
	}
}

func assertEquivalentLoadedFactoryConfigs(t *testing.T, left, right *config.LoadedFactoryConfig) {
	t.Helper()
	if left == nil || right == nil {
		t.Fatalf("loaded configs must be non-nil: left=%v right=%v", left, right)
	}
	if left.FactoryDir() != right.FactoryDir() {
		t.Fatalf("FactoryDir = %q vs %q", left.FactoryDir(), right.FactoryDir())
	}
	if left.FactoryConfig().Name != right.FactoryConfig().Name {
		t.Fatalf("name = %q vs %q", left.FactoryConfig().Name, right.FactoryConfig().Name)
	}
	if len(left.PortableBundledFileReplacements()) != len(right.PortableBundledFileReplacements()) {
		t.Fatalf("portable replacements = %d vs %d", len(left.PortableBundledFileReplacements()), len(right.PortableBundledFileReplacements()))
	}
	for _, workerName := range []string{"executor"} {
		leftWorker, leftOK := left.Worker(workerName)
		rightWorker, rightOK := right.Worker(workerName)
		if leftOK != rightOK {
			t.Fatalf("worker %q presence = %v vs %v", workerName, leftOK, rightOK)
		}
		if leftOK && leftWorker.Name != rightWorker.Name {
			t.Fatalf("worker %q name = %q vs %q", workerName, leftWorker.Name, rightWorker.Name)
		}
	}
}

func namedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()

	cfg := map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-" + project,
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
				"type":    "MODEL_WORKSTATION",
				"body":    "Implement {{ .WorkID }}.",
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayload): %v", err)
	}
	return data
}

func writeRuntimeFactoryJSON(t *testing.T, factoryDir string, cfg map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
}
