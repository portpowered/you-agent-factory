package runtimetests

import (
	"encoding/json"
	. "github.com/portpowered/infinite-you/pkg/config"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPersistNamedFactory_WritesCanonicalNamedLayout(t *testing.T) {
	rootDir := t.TempDir()

	factoryDir, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha"))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	wantDir := filepath.Join(rootDir, "alpha")
	assertPersistedNamedFactoryLayout(t, factoryDir, wantDir)
	assertPersistedNamedFactoryPayload(t, factoryDir)
	assertPersistedNamedFactoryAgents(t, factoryDir)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(persisted named factory): %v", err)
	}
	if loaded.FactoryConfig().Project != "alpha" {
		t.Fatalf("project = %q, want alpha", loaded.FactoryConfig().Project)
	}
}

func TestPersistNamedFactory_PreservesVersionMetadataAcrossLoadRoundTrip(t *testing.T) {
	rootDir := t.TempDir()
	versionTime := time.Date(2026, 5, 23, 11, 45, 0, 0, time.UTC)

	factoryDir, err := PersistNamedFactory(rootDir, "alpha", withNamedFactoryPayloadVersion(t, namedFactoryPayload(t, "alpha"), 17, versionTime))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.FactoryConfig().Version == nil {
		t.Fatal("expected persisted factory version metadata")
	}
	if loaded.FactoryConfig().Version.Logical != 17 || !loaded.FactoryConfig().Version.Physical.Equal(versionTime) {
		t.Fatalf("loaded version = %#v, want logical=17 physical=%s", loaded.FactoryConfig().Version, versionTime)
	}

	factoryJSON, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(factoryJSON, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	version, ok := payload["version"].(map[string]any)
	if !ok {
		t.Fatalf("persisted version payload = %#v, want object", payload["version"])
	}
	if got := version["logical"]; got != "17" {
		t.Fatalf("persisted logical version = %#v, want 17", got)
	}
	if got := version["physical"]; got != versionTime.Format(time.RFC3339Nano) {
		t.Fatalf("persisted physical version = %#v, want %q", got, versionTime.Format(time.RFC3339Nano))
	}
}

func TestPersistNamedFactory_StripsSupportedBundledFileInlineContentFromFactoryJSON(t *testing.T) {
	rootDir := t.TempDir()

	factoryDir, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayloadWithBundledFiles(t, "alpha"))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	assertRuntimeFactoryFileContent(t, filepath.Join(factoryDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertRuntimeFactoryFileContent(t, filepath.Join(factoryDir, "docs", "README.md"), "# Portable factory\n")
	assertRuntimeFactoryFileContent(t, filepath.Join(factoryDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertRuntimeFactoryFileContent(t, filepath.Join(factoryDir, "inputs", "task", "default", "starter.md"), "starter work\n")
	assertRuntimeFactoryFileMode(t, filepath.Join(factoryDir, "scripts", "execute-story.ps1"), 0o755)

	factoryJSON, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(factoryJSON, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	resourceManifest, ok := payload["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected supportingFiles object, got %#v", payload["supportingFiles"])
	}
	bundledFiles, ok := resourceManifest["bundledFiles"].([]any)
	if !ok || len(bundledFiles) != 4 {
		t.Fatalf("expected four bundled files, got %#v", resourceManifest["bundledFiles"])
	}
	assertPersistedBundledFileEntries(t, bundledFiles)

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(persisted thin bundled-file layout): %v", err)
	}
	if loaded.FactoryConfig().ResourceManifest == nil || len(loaded.FactoryConfig().ResourceManifest.BundledFiles) != 4 {
		t.Fatalf("loaded bundled files = %#v, want 4 supported bundled files", loaded.FactoryConfig().ResourceManifest)
	}
}

func TestPersistNamedFactory_PreservesPortableLayoutAcrossNamedFactoryLoadFlattenAndExpand(t *testing.T) {
	rootDir := t.TempDir()

	factoryDir, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayloadWithLayout(t, "alpha"))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(persisted layout factory): %v", err)
	}
	assertLoadedPortableLayoutConfig(t, loaded.FactoryConfig())

	flattened, err := FlattenFactoryConfig(factoryDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(named layout): %v", err)
	}
	assertPortableLayoutJSON(t, flattened)

	canonicalDir := t.TempDir()
	canonicalPath := filepath.Join(canonicalDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(canonicalPath, flattened, 0o644); err != nil {
		t.Fatalf("WriteFile(flattened factory.json): %v", err)
	}

	expandedDir, err := ExpandFactoryConfigLayout(canonicalPath)
	if err != nil {
		t.Fatalf("ExpandFactoryConfigLayout(flattened layout): %v", err)
	}
	expanded, err := LoadRuntimeConfig(expandedDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded layout factory): %v", err)
	}
	assertLoadedPortableLayoutConfig(t, expanded.FactoryConfig())
}

func TestPersistNamedFactory_RejectsDuplicateNames(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("first PersistNamedFactory: %v", err)
	}
	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha-second")); err == nil {
		t.Fatal("expected duplicate named factory to fail")
	} else if !strings.Contains(err.Error(), `factory "alpha" already exists`) {
		t.Fatalf("expected duplicate-name error, got %v", err)
	}
}

func TestPersistNamedFactory_RejectsInvalidNames(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "../alpha", namedFactoryPayload(t, "alpha")); err == nil {
		t.Fatal("expected invalid factory name to fail")
	} else if !strings.Contains(err.Error(), `factory name "../alpha" cannot contain path separators`) {
		t.Fatalf("expected invalid-name error, got %v", err)
	}
}

func TestPersistNamedFactory_RejectsInvalidPayloadWithoutChangingCurrentFactory(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	invalidPayload, err := json.Marshal(map[string]any{
		"id": "broken",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{"name": "executor"},
		},
		"workstations": []map[string]any{
			{
				"name":      "execute-broken",
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "failed"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
		"exhaustionRules": []map[string]any{
			{
				"name":             "broken-loop-cap",
				"watchWorkstation": "execute-broken",
				"maxVisits":        3,
				"source":           map[string]string{"workType": "task", "state": "init"},
				"target":           map[string]string{"workType": "task", "state": "failed"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(invalid named factory payload): %v", err)
	}

	if _, err := PersistNamedFactory(rootDir, "broken", invalidPayload); err == nil {
		t.Fatal("expected invalid named factory payload to fail")
	} else if got := err.Error(); !containsAll(got, generatedFactoryBoundaryErrorPrefix, "exhaustion_rules is retired") {
		t.Fatalf("expected generated-boundary validation error, got %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootDir, "broken")); !os.IsNotExist(err) {
		t.Fatalf("expected rejected factory directory to be absent, got stat err=%v", err)
	}

	loaded, err := LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(current after invalid persist): %v", err)
	}
	if loaded.FactoryDir() != filepath.Join(rootDir, "alpha") {
		t.Fatalf("FactoryDir after invalid persist = %q, want %q", loaded.FactoryDir(), filepath.Join(rootDir, "alpha"))
	}
	if loaded.FactoryConfig().Project != "alpha" {
		t.Fatalf("project after invalid persist = %q, want alpha", loaded.FactoryConfig().Project)
	}
}

func TestLoadRuntimeConfig_UsesCurrentFactoryPointerFromNamedLayout(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := PersistNamedFactory(rootDir, "beta", namedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "beta"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	loaded, err := LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(named root): %v", err)
	}
	if loaded.FactoryDir() != filepath.Join(rootDir, "beta") {
		t.Fatalf("FactoryDir = %q, want %q", loaded.FactoryDir(), filepath.Join(rootDir, "beta"))
	}
	if loaded.FactoryConfig().Project != "beta" {
		t.Fatalf("project = %q, want beta", loaded.FactoryConfig().Project)
	}
}

func TestLoadRuntimeConfig_CurrentFactoryPointerOverridesLegacyRootFactory(t *testing.T) {
	rootDir := t.TempDir()

	writeRuntimeFactoryJSON(t, rootDir, map[string]any{
		"name": "legacy-root",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"name":    "execute-story",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
			"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
		}},
	})
	writeRuntimeWorkerAgentsMD(t, rootDir, "executor", `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
---
Legacy root worker.
`)
	writeRuntimeWorkstationAgentsMD(t, rootDir, "execute-story", `---
type: MODEL_WORKSTATION
worker: executor
---
Legacy root workstation.
`)

	if _, err := PersistNamedFactory(rootDir, "beta", namedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "beta"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(beta): %v", err)
	}

	loaded, err := LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(named root): %v", err)
	}
	if loaded.FactoryDir() != filepath.Join(rootDir, "beta") {
		t.Fatalf("FactoryDir = %q, want %q", loaded.FactoryDir(), filepath.Join(rootDir, "beta"))
	}
	if loaded.FactoryConfig().Project != "beta" {
		t.Fatalf("project = %q, want beta", loaded.FactoryConfig().Project)
	}
}

func TestLoadRuntimeConfig_InvalidCurrentFactoryPointerReturnsStructuredError(t *testing.T) {
	rootDir := t.TempDir()

	writeRuntimeFactoryJSON(t, rootDir, map[string]any{
		"name": "legacy-root",
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{"name": "executor"}},
		"workstations": []map[string]any{{
			"name":    "execute-story",
			"worker":  "executor",
			"inputs":  []map[string]string{{"workType": "story", "state": "init"}},
			"outputs": []map[string]string{{"workType": "story", "state": "complete"}},
		}},
	})

	if err := os.WriteFile(filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile), []byte("../beta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(current pointer): %v", err)
	}

	_, err := LoadRuntimeConfig(rootDir, nil)
	if err == nil {
		t.Fatal("expected malformed current-factory pointer to fail")
	}
	if got := err.Error(); !containsAll(got, "read current factory pointer", "cannot contain path separators") {
		t.Fatalf("expected structured current-pointer error, got %v", err)
	}
}

func TestWriteCurrentFactoryPointer_RejectsMissingPersistedFactory(t *testing.T) {
	rootDir := t.TempDir()

	err := WriteCurrentFactoryPointer(rootDir, "missing")
	if err == nil {
		t.Fatal("expected missing persisted factory to be rejected")
	}
	if got := err.Error(); !containsAll(got, `set current factory "missing"`, "find factory config") {
		t.Fatalf("expected missing-factory error context, got %v", err)
	}
}

func TestValidateNamedFactoryName_RejectsPathTraversal(t *testing.T) {
	err := ValidateNamedFactoryName("../beta")
	if err == nil {
		t.Fatal("expected invalid named-factory segment to fail")
	}
	if got := err.Error(); !containsAll(got, `factory name "../beta"`, "cannot contain path separators") {
		t.Fatalf("expected path-separator validation error, got %v", err)
	}
}

func TestResolveNamedFactoryDir_RejectsDirectoryWithoutFactoryConfig(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootDir, "beta"), 0o755); err != nil {
		t.Fatalf("MkdirAll(beta): %v", err)
	}

	_, err := ResolveNamedFactoryDir(rootDir, "beta")
	if err == nil {
		t.Fatal("expected missing named factory config to fail")
	}
	if got := err.Error(); !containsAll(got, `resolve factory "beta"`, "find factory config") {
		t.Fatalf("expected missing-config resolution error, got %v", err)
	}
}

func withNamedFactoryPayloadVersion(t *testing.T, payload []byte, logical int64, physical time.Time) []byte {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal(namedFactoryPayload): %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  logical,
		"physical": physical.UTC().Format(time.RFC3339Nano),
	}
	updated, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayload with version): %v", err)
	}
	return updated
}

func namedFactoryPayloadWithLayout(t *testing.T, project string) []byte {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(namedFactoryPayload(t, project), &payload); err != nil {
		t.Fatalf("Unmarshal(namedFactoryPayload): %v", err)
	}
	payload["layout"] = map[string]any{
		"schemaVersion": 1,
		"nodes": []map[string]any{{
			"id": "workstation:execute-alpha",
			"position": map[string]any{
				"x": 128,
				"y": 256,
			},
			"size": map[string]any{
				"width":  320,
				"height": 180,
			},
			"locked": true,
		}},
		"edges": []map[string]any{{
			"id": "output:workstation:execute-alpha->work-type:task",
			"waypoints": []map[string]any{{
				"x": 180,
				"y": 220,
			}},
		}},
		"groups": []map[string]any{{
			"id":      "group-1",
			"label":   "Main lane",
			"nodeIds": []string{"workstation:execute-alpha"},
			"bounds": map[string]any{
				"x":      100,
				"y":      200,
				"width":  400,
				"height": 240,
			},
		}},
		"viewport": map[string]any{
			"x":    40,
			"y":    60,
			"zoom": 0.9,
		},
		"preferences": map[string]any{
			"direction": "RIGHT",
		},
	}
	updated, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayload with layout): %v", err)
	}
	return updated
}

func assertLoadedPortableLayoutConfig(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()

	if cfg == nil || cfg.Layout == nil {
		t.Fatalf("expected portable layout on loaded factory config, got %#v", cfg)
	}
	if cfg.Layout.SchemaVersion != 1 {
		t.Fatalf("layout schemaVersion = %d, want 1", cfg.Layout.SchemaVersion)
	}
	if len(cfg.Layout.Nodes) != 1 || cfg.Layout.Nodes[0].ID != "workstation:execute-alpha" {
		t.Fatalf("layout nodes = %#v, want execute-alpha node", cfg.Layout.Nodes)
	}
	if cfg.Layout.Nodes[0].Position.X != 128 || cfg.Layout.Nodes[0].Position.Y != 256 {
		t.Fatalf("layout node position = %#v, want x=128 y=256", cfg.Layout.Nodes[0].Position)
	}
	if cfg.Layout.Viewport == nil || math.Abs(cfg.Layout.Viewport.Zoom-0.9) > 1e-6 {
		t.Fatalf("layout viewport = %#v, want zoom 0.9", cfg.Layout.Viewport)
	}
	if cfg.Layout.Preferences == nil || cfg.Layout.Preferences.Direction != "RIGHT" {
		t.Fatalf("layout preferences = %#v, want RIGHT direction", cfg.Layout.Preferences)
	}
}

func assertPortableLayoutJSON(t *testing.T, payload []byte) {
	t.Helper()

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal(flattened payload): %v", err)
	}
	layout, ok := decoded["layout"].(map[string]any)
	if !ok {
		t.Fatalf("flattened layout = %#v, want object", decoded["layout"])
	}
	if got := layout["schemaVersion"]; got != float64(1) {
		t.Fatalf("flattened layout schemaVersion = %#v, want 1", got)
	}
	nodes, ok := layout["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("flattened layout nodes = %#v, want one node", layout["nodes"])
	}
	node, ok := nodes[0].(map[string]any)
	if !ok || node["id"] != "workstation:execute-alpha" {
		t.Fatalf("flattened layout node = %#v, want workstation:execute-alpha", nodes[0])
	}
	viewport, ok := layout["viewport"].(map[string]any)
	if !ok || viewport["zoom"] != 0.9 {
		t.Fatalf("flattened layout viewport = %#v, want zoom 0.9", layout["viewport"])
	}
}

func assertPersistedNamedFactoryLayout(t *testing.T, factoryDir, wantDir string) {
	t.Helper()

	if factoryDir != wantDir {
		t.Fatalf("factory dir = %q, want %q", factoryDir, wantDir)
	}
	for _, path := range []string{
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		filepath.Join(factoryDir, interfaces.InputsDir),
		filepath.Join(factoryDir, interfaces.InputsDir, "task", interfaces.DefaultChannelName),
		filepath.Join(factoryDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName),
		filepath.Join(factoryDir, interfaces.WorkstationsDir, "execute-alpha", interfaces.FactoryAgentsFileName),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected persisted named-factory path %s: %v", path, err)
		}
	}
}

func assertPersistedNamedFactoryPayload(t *testing.T, factoryDir string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(factoryJSON, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	workerPayloads, ok := payload["workers"].([]any)
	if !ok || len(workerPayloads) != 1 {
		t.Fatalf("expected one persisted worker payload, got %#v", payload["workers"])
	}
	workerPayload, ok := workerPayloads[0].(map[string]any)
	if !ok {
		t.Fatalf("expected persisted worker payload object, got %#v", workerPayloads[0])
	}
	if _, ok := workerPayload["body"]; ok {
		t.Fatalf("expected persisted worker payload to omit inline body, got %#v", workerPayload)
	}
	workstationPayloads, ok := payload["workstations"].([]any)
	if !ok || len(workstationPayloads) != 1 {
		t.Fatalf("expected one persisted workstation payload, got %#v", payload["workstations"])
	}
	workstationPayload, ok := workstationPayloads[0].(map[string]any)
	if !ok {
		t.Fatalf("expected persisted workstation payload object, got %#v", workstationPayloads[0])
	}
	if _, ok := workstationPayload["body"]; ok {
		t.Fatalf("expected persisted workstation payload to omit inline body, got %#v", workstationPayload)
	}
}

func assertPersistedNamedFactoryAgents(t *testing.T, factoryDir string) {
	t.Helper()

	workerAgents, err := os.ReadFile(filepath.Join(factoryDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName))
	if err != nil {
		t.Fatalf("ReadFile(worker AGENTS.md): %v", err)
	}
	if got := string(workerAgents); got != "You are the executor.\n" {
		t.Fatalf("persisted worker AGENTS.md = %q, want body-only worker content", got)
	}
	workstationAgents, err := os.ReadFile(filepath.Join(factoryDir, interfaces.WorkstationsDir, "execute-alpha", interfaces.FactoryAgentsFileName))
	if err != nil {
		t.Fatalf("ReadFile(workstation AGENTS.md): %v", err)
	}
	if got := string(workstationAgents); got != "Implement {{ .WorkID }}.\n" {
		t.Fatalf("persisted workstation AGENTS.md = %q, want body-only workstation content", got)
	}
}

func assertPersistedBundledFileEntries(t *testing.T, bundledFiles []any) {
	t.Helper()

	for _, entry := range bundledFiles {
		bundledFile, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file object, got %#v", entry)
		}
		content, ok := bundledFile["content"].(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file content object, got %#v", bundledFile["content"])
		}
		assertPersistedBundledFileContent(t, bundledFile["targetPath"], content)
	}
}

func assertPersistedBundledFileContent(t *testing.T, targetPathValue any, content map[string]any) {
	t.Helper()

	targetPath, _ := targetPathValue.(string)
	switch targetPath {
	case "Makefile":
		if got := content["inline"]; got != "test:\n\tgo test ./...\n" {
			t.Fatalf("expected persisted root helper inline content to stay inlined, got %#v", content)
		}
		if got := content["encoding"]; got != "utf-8" {
			t.Fatalf("expected persisted root helper encoding to stay canonical, got %#v", content)
		}
	case "factory/docs/README.md", "factory/scripts/execute-story.ps1", "factory/inputs/task/default/starter.md":
		if _, ok := content["inline"]; ok {
			t.Fatalf("expected persisted bundled file inline content to be omitted, got %#v", content)
		}
		if got := content["encoding"]; got != "utf-8" {
			t.Fatalf("expected persisted bundled file encoding to stay canonical, got %#v", content)
		}
	default:
		t.Fatalf("unexpected persisted bundled file targetPath = %#v", targetPath)
	}
}
