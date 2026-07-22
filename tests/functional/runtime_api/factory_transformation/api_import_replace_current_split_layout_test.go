package factory_transformation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestFactoryTransformation_ReplaceCurrentImportMatchesCreateNamedSplitLayout(t *testing.T) {
	replaceRoot := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(replaceRoot, interfaces.FactoryConfigFile),
		functionalNamedFactoryPayloadWithWorkType(t, "root-runtime", "root-task"),
		0o644,
	); err != nil {
		t.Fatalf("write default factory config: %v", err)
	}

	namedRoot := t.TempDir()
	seedNamedFactoryRoot(t, namedRoot, "alpha", "alpha-task")

	replaceServer := startFactoryTransformationServer(t, replaceRoot)
	namedServer := startFactoryTransformationServer(t, namedRoot)

	replaceCurrent := getCurrentFactory(t, replaceServer.URL())
	versionJSON, err := json.Marshal(versionDocument(advancedFactoryVersion(t, replaceCurrent.Version)))
	if err != nil {
		t.Fatalf("marshal replace-current version: %v", err)
	}
	importBody := functionalImportEquivalentBundledDocument(
		"UNDEFINED",
		"root-runtime",
		"imported-task",
		string(versionJSON),
	)
	saveCurrentFactoryDefinition(t, replaceServer.URL(), importBody)

	createNamedFactoryFromBody(
		t,
		namedServer.URL(),
		functionalImportEquivalentBundledDocument("imported", "imported", "imported-task", ""),
	)

	assertImportEquivalentSplitMaterialization(t, replaceRoot, "root-runtime")
	assertImportEquivalentSplitMaterialization(t, filepath.Join(namedRoot, "imported"), "imported")

	replacePaths, err := splitLayoutMaterializationPaths(replaceRoot)
	if err != nil {
		t.Fatalf("split layout paths (replace-current): %v", err)
	}
	namedPaths, err := splitLayoutMaterializationPaths(filepath.Join(namedRoot, "imported"))
	if err != nil {
		t.Fatalf("split layout paths (create-named): %v", err)
	}
	if strings.Join(replacePaths, "\n") != strings.Join(namedPaths, "\n") {
		t.Fatalf(
			"replace-current split layout paths differ from create-named\nreplace-current:\n%s\ncreate-named:\n%s",
			strings.Join(replacePaths, "\n"),
			strings.Join(namedPaths, "\n"),
		)
	}
}

func functionalImportEquivalentBundledDocument(name, id, workType, versionJSON string) string {
	versionField := ""
	if strings.TrimSpace(versionJSON) != "" {
		versionField = `"version":` + versionJSON + `,`
	}
	return `{
		"name":"` + name + `",
		"id":"` + id + `",
		` + versionField + `
		"workTypes":[{"name":"` + workType + `","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514","body":"You are the planner."}],
		"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","body":"Plan imported work.","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"done"}]}],
		"supportingFiles":{"bundledFiles":[
			{"type":"ROOT_HELPER","targetPath":"Makefile","content":{"encoding":"utf-8","inline":"test:\n\tgo test ./...\n"}},
			{"type":"DOC","targetPath":"factory/docs/README.md","content":{"encoding":"utf-8","inline":"# Portable factory\n"}},
			{"type":"SCRIPT","targetPath":"factory/scripts/execute-story.ps1","content":{"encoding":"utf-8","inline":"Write-Output 'portable script'\n"}}
		]}
	}`
}

func assertImportEquivalentSplitMaterialization(t *testing.T, factoryRootDir, wantProject string) {
	t.Helper()

	assertFunctionalSplitLayoutAtRoot(t, factoryRootDir, wantProject)
	assertPortableFile(t, filepath.Join(factoryRootDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableFile(t, filepath.Join(factoryRootDir, "docs", "README.md"), "# Portable factory\n")
	assertPortableFile(
		t,
		filepath.Join(factoryRootDir, "scripts", "execute-story.ps1"),
		"Write-Output 'portable script'\n",
	)
	assertPersistedFactoryJSONStripsInlineBundledContent(t, filepath.Join(factoryRootDir, interfaces.FactoryConfigFile))

	factoryJSON, err := os.ReadFile(filepath.Join(factoryRootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "Plan imported work.") {
		t.Fatalf("factory.json should omit inlined workstation body, got %s", factoryJSON)
	}

	workstationAgents, err := os.ReadFile(
		filepath.Join(factoryRootDir, interfaces.WorkstationsDir, "plan-task", interfaces.FactoryAgentsFileName),
	)
	if err != nil {
		t.Fatalf("ReadFile(plan-task AGENTS.md): %v", err)
	}
	if !strings.Contains(string(workstationAgents), "Plan imported work.") {
		t.Fatalf("plan-task AGENTS.md = %q, want imported workstation body", workstationAgents)
	}
}

func splitLayoutMaterializationPaths(factoryRootDir string) ([]string, error) {
	var paths []string

	for _, top := range []string{interfaces.WorkersDir, interfaces.WorkstationsDir} {
		topPath := filepath.Join(factoryRootDir, top)
		entries, err := os.ReadDir(topPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			rel := filepath.Join(top, entry.Name(), interfaces.FactoryAgentsFileName)
			if _, err := os.Stat(filepath.Join(factoryRootDir, rel)); err == nil {
				paths = append(paths, rel)
			}
		}
	}

	for _, rel := range []string{
		"Makefile",
		filepath.Join("docs", "README.md"),
		filepath.Join("scripts", "execute-story.ps1"),
		interfaces.FactoryConfigFile,
	} {
		if _, err := os.Stat(filepath.Join(factoryRootDir, rel)); err == nil {
			paths = append(paths, rel)
		}
	}

	sort.Strings(paths)
	return paths, nil
}
