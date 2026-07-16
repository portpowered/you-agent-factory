package persist_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/config/persist"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

func TestPersistNamedFactory_MatchesConfigPackage(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")

	facadeDir, err := persist.PersistNamedFactory(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("persist.PersistNamedFactory: %v", err)
	}
	configDir, err := config.PersistNamedFactory(rootDir, "beta", payload)
	if err != nil {
		t.Fatalf("config.PersistNamedFactory: %v", err)
	}

	if facadeDir != filepath.Join(rootDir, "alpha") {
		t.Fatalf("facade factory dir = %q, want %q", facadeDir, filepath.Join(rootDir, "alpha"))
	}
	if configDir != filepath.Join(rootDir, "beta") {
		t.Fatalf("config factory dir = %q, want %q", configDir, filepath.Join(rootDir, "beta"))
	}
}

func TestReplaceNamedFactory_MatchesConfigPackage(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")

	if _, err := persist.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("persist.PersistNamedFactory: %v", err)
	}

	facadeDir, err := persist.ReplaceNamedFactory(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("persist.ReplaceNamedFactory: %v", err)
	}
	configDir, err := config.ReplaceNamedFactory(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("config.ReplaceNamedFactory: %v", err)
	}
	if facadeDir != configDir {
		t.Fatalf("factory dir = %q vs %q", facadeDir, configDir)
	}
}

func TestPersistNamedFactoryWithReport_MatchesConfigPackage(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayloadWithBundledFiles(t, "alpha")

	facadeResult, err := persist.PersistNamedFactoryWithReport(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("persist.PersistNamedFactoryWithReport: %v", err)
	}

	rootDir2 := t.TempDir()
	configResult, err := config.PersistNamedFactoryWithReport(rootDir2, "alpha", payload)
	if err != nil {
		t.Fatalf("config.PersistNamedFactoryWithReport: %v", err)
	}

	if facadeResult.FactoryDir != filepath.Join(rootDir, "alpha") {
		t.Fatalf("facade factory dir = %q, want %q", facadeResult.FactoryDir, filepath.Join(rootDir, "alpha"))
	}
	if configResult.FactoryDir != filepath.Join(rootDir2, "alpha") {
		t.Fatalf("config factory dir = %q, want %q", configResult.FactoryDir, filepath.Join(rootDir2, "alpha"))
	}
	if _, err := os.Stat(filepath.Join(facadeResult.FactoryDir, "Makefile")); err != nil {
		t.Fatalf("expected bundled Makefile on disk: %v", err)
	}
}

func TestReplaceNamedFactoryWithReport_MatchesConfigPackage(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayloadWithBundledFiles(t, "alpha")

	if _, err := persist.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("persist.PersistNamedFactory: %v", err)
	}

	facadeResult, err := persist.ReplaceNamedFactoryWithReport(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("persist.ReplaceNamedFactoryWithReport: %v", err)
	}

	rootDir2 := t.TempDir()
	if _, err := config.PersistNamedFactory(rootDir2, "alpha", payload); err != nil {
		t.Fatalf("config.PersistNamedFactory: %v", err)
	}
	configResult, err := config.ReplaceNamedFactoryWithReport(rootDir2, "alpha", payload)
	if err != nil {
		t.Fatalf("config.ReplaceNamedFactoryWithReport: %v", err)
	}

	if facadeResult.FactoryDir != filepath.Join(rootDir, "alpha") {
		t.Fatalf("facade factory dir = %q, want %q", facadeResult.FactoryDir, filepath.Join(rootDir, "alpha"))
	}
	if configResult.FactoryDir != filepath.Join(rootDir2, "alpha") {
		t.Fatalf("config factory dir = %q, want %q", configResult.FactoryDir, filepath.Join(rootDir2, "alpha"))
	}
	if _, err := os.Stat(filepath.Join(facadeResult.FactoryDir, "Makefile")); err != nil {
		t.Fatalf("expected bundled Makefile on disk: %v", err)
	}
}

func TestReadWriteCurrentFactoryPointer_RoundTrip(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")

	if _, err := persist.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("persist.PersistNamedFactory: %v", err)
	}
	if err := persist.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("persist.WriteCurrentFactoryPointer: %v", err)
	}

	got, err := persist.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("persist.ReadCurrentFactoryPointer: %v", err)
	}
	if got != "alpha" {
		t.Fatalf("current factory = %q, want alpha", got)
	}
}

func TestNamedFactoryHelpers_MatchConfigPackage(t *testing.T) {
	assertNamedFactoryPathSegmentsHelpersMatchConfig(t)
	assertNamedFactorySegmentHelpersMatchConfig(t)
	assertNamedFactoryRootHelpersMatchConfig(t)
}

func assertNamedFactoryPathSegmentsHelpersMatchConfig(t *testing.T) {
	t.Helper()

	facadeSegments, err := persist.NamedFactoryPathSegments("@you/goal")
	if err != nil {
		t.Fatalf("persist.NamedFactoryPathSegments: %v", err)
	}
	configSegments, err := config.NamedFactoryPathSegments("@you/goal")
	if err != nil {
		t.Fatalf("config.NamedFactoryPathSegments: %v", err)
	}
	if len(facadeSegments) != len(configSegments) {
		t.Fatalf("path segments = %#v vs %#v", facadeSegments, configSegments)
	}
	for i := range facadeSegments {
		if facadeSegments[i] != configSegments[i] {
			t.Fatalf("path segment[%d] = %q vs %q", i, facadeSegments[i], configSegments[i])
		}
	}

	facadeRoundTrip, err := persist.NamedFactoryNameFromPathSegments(facadeSegments)
	if err != nil {
		t.Fatalf("persist.NamedFactoryNameFromPathSegments: %v", err)
	}
	configRoundTrip, err := config.NamedFactoryNameFromPathSegments(configSegments)
	if err != nil {
		t.Fatalf("config.NamedFactoryNameFromPathSegments: %v", err)
	}
	if facadeRoundTrip != configRoundTrip || facadeRoundTrip != "@you/goal" {
		t.Fatalf("round-trip name = %q vs %q, want @you/goal", facadeRoundTrip, configRoundTrip)
	}
}

func assertNamedFactorySegmentHelpersMatchConfig(t *testing.T) {
	t.Helper()

	rootDir := t.TempDir()
	facadeDir, err := persist.MapNamedFactoryDir(rootDir, "@you/goal")
	if err != nil {
		t.Fatalf("persist.MapNamedFactoryDir: %v", err)
	}
	configDir, err := config.MapNamedFactoryDir(rootDir, "@you/goal")
	if err != nil {
		t.Fatalf("config.MapNamedFactoryDir: %v", err)
	}
	if facadeDir != configDir {
		t.Fatalf("mapped dir = %q vs %q", facadeDir, configDir)
	}
}

func assertNamedFactoryRootHelpersMatchConfig(t *testing.T) {
	t.Helper()

	homeDir := t.TempDir()
	facadeGlobalRoot, err := persist.GlobalNamedFactoryRootForHome(homeDir)
	if err != nil {
		t.Fatalf("persist.GlobalNamedFactoryRootForHome: %v", err)
	}
	configGlobalRoot, err := config.GlobalNamedFactoryRootForHome(homeDir)
	if err != nil {
		t.Fatalf("config.GlobalNamedFactoryRootForHome: %v", err)
	}
	if facadeGlobalRoot != configGlobalRoot {
		t.Fatalf("global root = %q vs %q", facadeGlobalRoot, configGlobalRoot)
	}
	if want := defaultpaths.NamedFactoriesRoot(homeDir); configGlobalRoot != want {
		t.Fatalf("config global root = %q, want defaultpaths.NamedFactoriesRoot = %q", configGlobalRoot, want)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	facadeDefaultGlobalRoot, err := persist.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		t.Fatalf("persist.DefaultGlobalNamedFactoryRoot: %v", err)
	}
	configDefaultGlobalRoot, err := config.DefaultGlobalNamedFactoryRoot()
	if err != nil {
		t.Fatalf("config.DefaultGlobalNamedFactoryRoot: %v", err)
	}
	if facadeDefaultGlobalRoot != configDefaultGlobalRoot {
		t.Fatalf("default global root = %q vs %q", facadeDefaultGlobalRoot, configDefaultGlobalRoot)
	}
	if want := defaultpaths.NamedFactoriesRoot(homeDir); configDefaultGlobalRoot != want {
		t.Fatalf("default global root = %q, want defaultpaths.NamedFactoriesRoot = %q", configDefaultGlobalRoot, want)
	}

	cwd := filepath.Join("repo", "app")
	facadeProjectRoot, err := persist.DefaultProjectNamedFactoryRoot(cwd)
	if err != nil {
		t.Fatalf("persist.DefaultProjectNamedFactoryRoot: %v", err)
	}
	configProjectRoot, err := config.DefaultProjectNamedFactoryRoot(cwd)
	if err != nil {
		t.Fatalf("config.DefaultProjectNamedFactoryRoot: %v", err)
	}
	if facadeProjectRoot != configProjectRoot {
		t.Fatalf("project root = %q vs %q", facadeProjectRoot, configProjectRoot)
	}
}

func TestReplaceFactoryLayoutAtDir_MatchesConfigPackage(t *testing.T) {
	targetDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	facadeRestore, err := persist.ReplaceFactoryLayoutAtDir(
		targetDir,
		payload,
		persist.DefaultFactoryLayoutReplaceOptions(targetDir),
	)
	if err != nil {
		t.Fatalf("persist.ReplaceFactoryLayoutAtDir: %v", err)
	}
	if facadeRestore == nil {
		t.Fatal("expected restore callback from persist facade")
	}

	rootDir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir2, interfaces.FactoryConfigFile), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	configRestore, err := config.ReplaceFactoryLayoutAtDir(
		rootDir2,
		payload,
		config.DefaultFactoryLayoutReplaceOptions(rootDir2),
	)
	if err != nil {
		t.Fatalf("config.ReplaceFactoryLayoutAtDir: %v", err)
	}
	if configRestore == nil {
		t.Fatal("expected restore callback from config package")
	}

	agentsPath := filepath.Join(targetDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("expected split worker file from facade path: %v", err)
	}
	agentsPath = filepath.Join(rootDir2, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("expected split worker file from config path: %v", err)
	}
}

func TestReplaceFactorySplitLayout_MatchesConfigPackage(t *testing.T) {
	targetDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")
	if err := os.WriteFile(filepath.Join(targetDir, interfaces.FactoryConfigFile), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	facadeResult, err := persist.ReplaceFactorySplitLayout(targetDir, payload)
	if err != nil {
		t.Fatalf("persist.ReplaceFactorySplitLayout: %v", err)
	}
	if facadeResult == nil || facadeResult.Restore == nil {
		t.Fatal("expected restore callback from persist facade")
	}

	rootDir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir2, interfaces.FactoryConfigFile), payload, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	configResult, err := config.ReplaceFactorySplitLayout(rootDir2, payload)
	if err != nil {
		t.Fatalf("config.ReplaceFactorySplitLayout: %v", err)
	}
	if configResult == nil || configResult.Restore == nil {
		t.Fatal("expected restore callback from config package")
	}

	agentsPath := filepath.Join(targetDir, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("expected split worker file from facade path: %v", err)
	}
	agentsPath = filepath.Join(rootDir2, interfaces.WorkersDir, "executor", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("expected split worker file from config path: %v", err)
	}
}

func TestIsInvalidNamedFactory_DetectsPersistValidationFailure(t *testing.T) {
	rootDir := t.TempDir()

	_, err := persist.PersistNamedFactory(rootDir, "broken", []byte(`{"name":"broken"`))
	if err == nil {
		t.Fatal("expected invalid named factory payload to fail")
	}
	if !persist.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestIsInvalidNamedFactoryName_DetectsNameValidationFailure(t *testing.T) {
	_, err := config.ResolveNamedFactoryDir(t.TempDir(), "@you")
	if err == nil {
		t.Fatal("expected invalid named factory name to fail")
	}
	if !persist.IsInvalidNamedFactoryName(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactoryName", err)
	}
}

func TestIsNamedFactoryNotFound_DetectsResolveMiss(t *testing.T) {
	_, err := config.ResolveNamedFactoryDir(t.TempDir(), "missing")
	if err == nil {
		t.Fatal("expected missing named factory to fail")
	}
	if !persist.IsNamedFactoryNotFound(err) {
		t.Fatalf("error = %v, want ErrNamedFactoryNotFound", err)
	}
}

func TestIsNamedFactoryAlreadyExists_DetectsDuplicatePersist(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")

	if _, err := persist.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("first persist.PersistNamedFactory: %v", err)
	}
	_, err := persist.PersistNamedFactory(rootDir, "alpha", payload)
	if err == nil {
		t.Fatal("expected duplicate named factory to fail")
	}
	if !persist.IsNamedFactoryAlreadyExists(err) {
		t.Fatalf("error = %v, want ErrNamedFactoryAlreadyExists", err)
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

func namedFactoryPayloadWithBundledFiles(t *testing.T, project string) []byte {
	t.Helper()

	payload := namedFactoryPayload(t, project)
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal(namedFactoryPayload): %v", err)
	}
	decoded["supportingFiles"] = map[string]any{
		"bundledFiles": []map[string]any{
			{
				"type":       "ROOT_HELPER",
				"targetPath": "Makefile",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "test:\n\tgo test ./...\n",
				},
			},
		},
	}
	updated, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayloadWithBundledFiles): %v", err)
	}
	return updated
}

func TestValidateNamedFactoryName_RejectsPathTraversal(t *testing.T) {
	err := persist.ValidateNamedFactoryName("../beta")
	if err == nil {
		t.Fatal("expected invalid named-factory segment to fail")
	}
	if got := err.Error(); !strings.Contains(got, "cannot contain path separators") {
		t.Fatalf("expected path-separator validation error, got %v", err)
	}
}
