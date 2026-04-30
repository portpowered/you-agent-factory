package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/internal/testpath"
	"github.com/portpowered/agent-factory/pkg/config"
)

var supportedFactoryConfigLegacyConfigTokens = []string{
	"copy_referenced_scripts",
	"exhaustion_rules",
	"expiry_window",
	"factory_dir",
	"input_types",
	"max_execution_time",
	"max_retries",
	"max_visits",
	"model_provider",
	"on_failure",
	"on_rejection",
	"output_schema",
	"parent_input",
	"prompt_file",
	"prompt_template",
	"resource_usage",
	"resourceUsage",
	"runtime_stop_words",
	"runtimeStopWords",
	"session_id",
	"skip_permissions",
	"source_directory",
	"spawned_by",
	"stop_token",
	"stop_words",
	"trigger_at_start",
	"watch_workstation",
	"work_type",
	"work_types",
	"working_directory",
	"workflow_id",
}

var supportedFactoryConfigLegacyDocTokens = []string{
	"exhaustion_rules",
	"on_failure",
	"on_rejection",
	"max_visits",
	"working_directory",
	"workflow_id",
}

func TestFactoryConfigSupportedSurfaceGuard_SupportedDocsExamplesAndFixturesUseCamelCase(t *testing.T) {
	for _, path := range supportedFactoryConfigDocSurfaces(t) {
		offenses := matchedLegacySurfaceTokens(string(mustReadSupportedSurface(t, path)), supportedFactoryConfigLegacyDocTokens)
		if len(offenses) == 0 {
			continue
		}
		t.Fatalf("%s contains legacy factory-config docs tokens: %s", relativeSupportedSurfacePath(t, path), strings.Join(offenses, ", "))
	}

	for _, path := range supportedFactoryConfigTextSurfaces(t) {
		offenses := matchedLegacySurfaceTokens(string(mustReadSupportedSurface(t, path)), supportedFactoryConfigLegacyConfigTokens)
		if len(offenses) == 0 {
			continue
		}
		t.Fatalf("%s contains legacy factory-config tokens: %s", relativeSupportedSurfacePath(t, path), strings.Join(offenses, ", "))
	}

	for _, path := range supportedFactoryReplayFixtures(t) {
		offenses := matchedLegacySurfaceTokens(string(mustReadReplayFactorySurface(t, path)), supportedFactoryConfigLegacyConfigTokens)
		if len(offenses) == 0 {
			continue
		}
		t.Fatalf("%s RUN_REQUEST.payload.factory contains legacy factory-config tokens: %s", relativeSupportedSurfacePath(t, path), strings.Join(offenses, ", "))
	}
}

func TestFactoryConfigSupportedSurfaceGuard_RegressionDetectsLegacyTokens(t *testing.T) {
	got := matchedLegacySurfaceTokens(`{
  "work_types": [{"name":"story"}],
  "workstations": [{
    "on_failure": {"watch_workstation":"review","work_type":"story","state":"failed"},
    "resource_usage": [{"name":"slot","capacity":1}]
  }]
}`, supportedFactoryConfigLegacyConfigTokens)
	want := []string{"on_failure", "resource_usage", "watch_workstation", "work_type", "work_types"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy token regression guard mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFactoryConfigSupportedSurfaceGuard_DocRegressionDetectsLegacyTokens(t *testing.T) {
	got := matchedLegacySurfaceTokens(`workflow_id, exhaustion_rules, prompt_template, on_failure`, supportedFactoryConfigLegacyDocTokens)
	want := []string{"exhaustion_rules", "on_failure", "workflow_id"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy doc token regression guard mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFactoryConfigSupportedSurfaceGuard_MatchesWholeLegacyTokensOnly(t *testing.T) {
	got := matchedLegacySurfaceTokens(`work_type_name runtime_stop_words trigger_at_start`, supportedFactoryConfigLegacyConfigTokens)
	want := []string{"runtime_stop_words", "trigger_at_start"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy token whole-match guard mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFactoryConfigSupportedSurfaceFixtures_DecodeThroughGeneratedBoundary(t *testing.T) {
	for _, path := range supportedFactoryConfigJSONSurfaces(t) {
		path := path
		t.Run(relativeSupportedSurfacePath(t, path), func(t *testing.T) {
			if _, err := config.GeneratedFactoryFromOpenAPIJSON(mustReadSupportedSurface(t, path)); err != nil {
				t.Fatalf("decode supported factory-config fixture through generated boundary: %v", err)
			}
		})
	}

	for _, path := range supportedFactoryReplayFixtures(t) {
		path := path
		t.Run(relativeSupportedSurfacePath(t, path)+"/run-request-factory", func(t *testing.T) {
			if _, err := config.GeneratedFactoryFromOpenAPIJSON(mustReadReplayFactorySurface(t, path)); err != nil {
				t.Fatalf("decode replay fixture RUN_REQUEST factory through generated boundary: %v", err)
			}
		})
	}
}

func supportedFactoryConfigDocSurfaces(t *testing.T) []string {
	t.Helper()

	root := supportedSurfaceRoot(t)
	paths := []string{
		filepath.Join(repoRoot(t), "factory", "README.md"),
		filepath.Join(root, "README.md"),
		filepath.Join(root, "docs", "README.md"),
		filepath.Join(root, "docs", "authoring-agents-md.md"),
		filepath.Join(root, "docs", "authoring-workflows.md"),
		filepath.Join(root, "docs", "work.md"),
		filepath.Join(root, "docs", "workstations.md"),
	}
	paths = append(paths, collectSupportedSurfaceFiles(t, filepath.Join(root, "examples"), func(path string, entry os.DirEntry) bool {
		if filepath.Base(path) != "README.md" {
			return false
		}
		slash := filepath.ToSlash(path)
		return !strings.Contains(slash, "/workers/") && !strings.Contains(slash, "/workstations/") && !strings.Contains(slash, "/inputs/")
	})...)
	sort.Strings(paths)
	return existingSupportedSurfacePaths(paths)
}

func supportedFactoryConfigTextSurfaces(t *testing.T) []string {
	t.Helper()

	root := supportedSurfaceRoot(t)
	var paths []string
	paths = append(paths, collectSupportedSurfaceFiles(t, filepath.Join(repoRoot(t), "factory"), func(path string, entry os.DirEntry) bool {
		slash := filepath.ToSlash(path)
		if strings.HasPrefix(slash, filepath.ToSlash(filepath.Join(repoRoot(t), "factory", "old"))+"/") {
			return false
		}
		return filepath.Base(path) == "factory.json" || filepath.Base(path) == "AGENTS.md"
	})...)
	paths = append(paths, collectSupportedSurfaceFiles(t, filepath.Join(root, "factory"), func(path string, entry os.DirEntry) bool {
		slash := filepath.ToSlash(path)
		if strings.HasPrefix(slash, filepath.ToSlash(filepath.Join(root, "factory", "old"))+"/") {
			return false
		}
		return filepath.Base(path) == "factory.json" || filepath.Base(path) == "AGENTS.md"
	})...)
	paths = append(paths, collectSupportedSurfaceFiles(t, filepath.Join(root, "examples"), func(path string, entry os.DirEntry) bool {
		return filepath.Base(path) == "factory.json" || filepath.Base(path) == "AGENTS.md"
	})...)
	paths = append(paths, supportedFactoryConfigJSONSurfaces(t)...)
	paths = append(paths, collectSupportedSurfaceFiles(t, filepath.Join(root, "tests", "functional_test", "testdata"), func(path string, entry os.DirEntry) bool {
		return filepath.Base(path) == "AGENTS.md"
	})...)
	sort.Strings(paths)
	return existingSupportedSurfacePaths(dedupSurfacePaths(paths))
}

func supportedFactoryConfigJSONSurfaces(t *testing.T) []string {
	t.Helper()

	root := supportedSurfaceRoot(t)
	var paths []string
	paths = append(paths, collectSupportedSurfaceFiles(t, filepath.Join(repoRoot(t), "factory"), func(path string, entry os.DirEntry) bool {
		slash := filepath.ToSlash(path)
		return filepath.Base(path) == "factory.json" && !strings.HasPrefix(slash, filepath.ToSlash(filepath.Join(repoRoot(t), "factory", "old"))+"/")
	})...)
	paths = append(paths, collectSupportedSurfaceFiles(t, filepath.Join(root, "factory"), func(path string, entry os.DirEntry) bool {
		slash := filepath.ToSlash(path)
		return filepath.Base(path) == "factory.json" && !strings.HasPrefix(slash, filepath.ToSlash(filepath.Join(root, "factory", "old"))+"/")
	})...)
	paths = append(paths, collectSupportedSurfaceFiles(t, filepath.Join(root, "examples"), func(path string, entry os.DirEntry) bool {
		return filepath.Base(path) == "factory.json"
	})...)
	paths = append(paths, collectSupportedSurfaceFiles(t, filepath.Join(root, "tests", "functional_test", "testdata"), func(path string, entry os.DirEntry) bool {
		if filepath.Base(path) == "factory.json" {
			return true
		}
		if filepath.Ext(path) != ".json" || filepath.Base(path) == "adhoc-recording-batch-event-log.json" {
			return false
		}
		return looksLikeStandaloneFactoryConfigFixture(t, path)
	})...)
	sort.Strings(paths)
	return existingSupportedSurfacePaths(dedupSurfacePaths(paths))
}

func supportedFactoryReplayFixtures(t *testing.T) []string {
	t.Helper()

	root := supportedSurfaceRoot(t)
	return existingSupportedSurfacePaths([]string{
		filepath.Join(root, "pkg", "replay", "testdata", "inference-events.replay.json"),
		filepath.Join(root, "tests", "adhoc", "factory-recording-04-11-02.json"),
		filepath.Join(root, "tests", "functional_test", "testdata", "adhoc-recording-batch-event-log.json"),
	})
}

func supportedSurfaceRoot(t *testing.T) string {
	t.Helper()
	return testpath.MustRepoPathFromCaller(t, 0)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return supportedSurfaceRoot(t)
}

func collectSupportedSurfaceFiles(t *testing.T, root string, keep func(string, os.DirEntry) bool) []string {
	t.Helper()

	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if keep(path, entry) {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk supported surfaces under %s: %v", root, err)
	}
	return paths
}

func looksLikeStandaloneFactoryConfigFixture(t *testing.T, path string) bool {
	t.Helper()

	var top map[string]json.RawMessage
	if err := json.Unmarshal(mustReadSupportedSurface(t, path), &top); err != nil {
		t.Fatalf("parse standalone fixture %s: %v", relativeSupportedSurfacePath(t, path), err)
	}
	if _, ok := top["events"]; ok {
		return false
	}
	_, hasWorkstations := top["workstations"]
	_, hasWorkTypes := top["workTypes"]
	_, hasLegacyWorkTypes := top["work_types"]
	return hasWorkstations || hasWorkTypes || hasLegacyWorkTypes
}

func mustReadReplayFactorySurface(t *testing.T, path string) []byte {
	t.Helper()

	var artifact struct {
		Events []struct {
			Type    string `json:"type"`
			Payload struct {
				Factory json.RawMessage `json:"factory"`
			} `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(mustReadSupportedSurface(t, path), &artifact); err != nil {
		t.Fatalf("parse replay fixture %s: %v", relativeSupportedSurfacePath(t, path), err)
	}
	for _, event := range artifact.Events {
		if event.Type == "RUN_REQUEST" && len(event.Payload.Factory) > 0 {
			return event.Payload.Factory
		}
	}
	t.Fatalf("replay fixture %s is missing RUN_REQUEST.payload.factory", relativeSupportedSurfacePath(t, path))
	return nil
}

func mustReadSupportedSurface(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read supported surface %s: %v", relativeSupportedSurfacePath(t, path), err)
	}
	return data
}

func relativeSupportedSurfacePath(t *testing.T, path string) string {
	t.Helper()

	for _, root := range []string{repoRoot(t), supportedSurfaceRoot(t)} {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}
		if rel == "." {
			return filepath.ToSlash(rel)
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func matchedLegacySurfaceTokens(content string, tokens []string) []string {
	matches := make(map[string]struct{})
	for _, token := range tokens {
		if factoryConfigTokenPattern(token).MatchString(content) {
			matches[token] = struct{}{}
		}
	}
	if len(matches) == 0 {
		return nil
	}

	out := make([]string, 0, len(matches))
	for token := range matches {
		out = append(out, token)
	}
	slices.Sort(out)
	return out
}

func dedupSurfacePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func existingSupportedSurfacePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			out = append(out, path)
		}
	}
	return out
}
