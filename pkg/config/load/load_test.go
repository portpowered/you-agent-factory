package load_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/load"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
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

func TestLoadFromCanonicalJSON_MatchesLoadFromFactoryDirForInlineFactory(t *testing.T) {
	factoryDir := t.TempDir()
	inlineCfg := map[string]any{
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
	}
	writeRuntimeFactoryJSON(t, factoryDir, inlineCfg)

	payload, err := json.Marshal(inlineCfg)
	if err != nil {
		t.Fatalf("Marshal(inlineCfg): %v", err)
	}

	dirLoaded, err := load.LoadFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("load.LoadFromFactoryDir: %v", err)
	}
	jsonLoaded, err := load.LoadFromCanonicalJSON(payload, load.LoadOptions{})
	if err != nil {
		t.Fatalf("load.LoadFromCanonicalJSON: %v", err)
	}

	assertEquivalentLoadedFactoryConfigsIgnoreFactoryDir(t, dirLoaded, jsonLoaded)
	if jsonLoaded.FactoryDir() != "" {
		t.Fatalf("FactoryDir = %q, want empty for JSON load", jsonLoaded.FactoryDir())
	}
}

func TestLoadFromCanonicalJSON_RejectsCrossPathInvalidFixture(t *testing.T) {
	_, err := load.LoadFromCanonicalJSON([]byte(factoryvalidation.CrossPathInvalidFactoryJSON), load.LoadOptions{})
	if err == nil {
		t.Fatal("expected cross-path invalid factory to fail load")
	}
	if !load.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestLoadFromFactoryDir_RejectsCrossPathInvalidFixture(t *testing.T) {
	factoryDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		[]byte(factoryvalidation.CrossPathInvalidFactoryJSON),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	_, err := load.LoadFromFactoryDir(factoryDir, nil)
	if err == nil {
		t.Fatal("expected cross-path invalid factory directory to fail load")
	}
	if !load.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestLoadFromCanonicalJSON_RejectsInvalidJSONWithStableMessage(t *testing.T) {
	_, err := load.LoadFromCanonicalJSON([]byte(`{"name":"broken"`), load.LoadOptions{})
	if err == nil {
		t.Fatal("expected invalid JSON to fail load")
	}
	if !load.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
	if !strings.Contains(err.Error(), "parse factory") {
		t.Fatalf("error = %v, want parse failure context", err)
	}
}

func TestLoadFromCanonicalJSON_RejectsMissingFactoryName(t *testing.T) {
	payload := []byte(`{
		"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers":[{"name":"executor"}],
		"workstations":[{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	_, err := load.LoadFromCanonicalJSON(payload, load.LoadOptions{})
	if err == nil {
		t.Fatal("expected missing factory.name to fail load")
	}
	if !strings.Contains(err.Error(), "factory.name is required") {
		t.Fatalf("error = %v, want factory.name boundary message", err)
	}
}

func TestLoadFromCanonicalJSON_AcceptsOpenAPIFixture(t *testing.T) {
	payload := []byte(`{
		"name":"finish-chapter-factory",
		"workTypes": [
			{"name":"chapter","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]},
			{"name":"page","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}
		],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":"CLAUDE","stopToken":"COMPLETE"}],
		"workstations": [{
			"id":"finish-chapter-id",
			"name":"finish-chapter",
			"behavior":"STANDARD",
			"worker":"executor",
			"type":"LOGICAL_MOVE",
			"body":"Finish {{ .WorkID }}.",
			"inputs":[
				{"workType":"chapter","state":"init"},
				{"workType":"page","state":"complete","guards":[{"type":"ALL_CHILDREN_COMPLETE","parentInput":"chapter","spawnedBy":"chapter-parser"}]}
			],
			"outputs":[{"workType":"chapter","state":"complete"}],
			"resources":[{"name":"agent-slot","capacity":2}],
			"guards":[{"type":"VISIT_COUNT","workstation":"review-story","maxVisits":3}],
			"env":{"TEAM":"{{ index .Tags \"team\" }}"}
		}]
	}`)

	loaded, err := load.LoadFromCanonicalJSON(payload, load.LoadOptions{})
	if err != nil {
		t.Fatalf("load.LoadFromCanonicalJSON: %v", err)
	}
	if loaded.FactoryConfig().Name != "finish-chapter-factory" {
		t.Fatalf("name = %q, want finish-chapter-factory", loaded.FactoryConfig().Name)
	}
	workstation, ok := loaded.Workstation("finish-chapter")
	if !ok {
		t.Fatal("expected finish-chapter workstation to be loaded")
	}
	if workstation.Body == "" {
		t.Fatal("expected workstation body from inline JSON load")
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

func assertEquivalentLoadedFactoryConfigsIgnoreFactoryDir(t *testing.T, left, right *config.LoadedFactoryConfig) {
	t.Helper()
	assertEquivalentLoadedFactoryConfigs(t, left, right, true)
}

func assertEquivalentLoadedFactoryConfigs(t *testing.T, left, right *config.LoadedFactoryConfig, skipFactoryDir ...bool) {
	t.Helper()
	if left == nil || right == nil {
		t.Fatalf("loaded configs must be non-nil: left=%v right=%v", left, right)
	}
	ignoreFactoryDir := len(skipFactoryDir) > 0 && skipFactoryDir[0]
	if !ignoreFactoryDir && left.FactoryDir() != right.FactoryDir() {
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
