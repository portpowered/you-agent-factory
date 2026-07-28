package definitions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	importExportNamedMakefileBody     = "test:\n\tgo test ./...\n"
	importExportNamedPortableDocBody  = "# Portable factory\n"
	importExportNamedPortableScriptBody = "Write-Output 'portable script'\n"
)

func startImportExportNamedFactoryServer(t *testing.T, rootDir string) (*support.FunctionalAPIServer, *support.RecordingCommandRunner) {
	t.Helper()

	runner := support.NewRecordingCommandRunner("runtime must not execute during import_export named-factory API proofs")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                rootDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	return server, runner
}

func saveCurrentFactoryDefinition(t *testing.T, serverURL, body string) factoryapi.Factory {
	t.Helper()

	resp := saveCurrentFactoryDefinitionExpectStatus(t, serverURL, body, http.StatusOK)
	var saved factoryapi.Factory
	decodeJSONResponse(t, resp, &saved, "decode current factory save response")
	return saved
}

func createNamedFactoryFromBody(t *testing.T, serverURL, body string) factoryapi.Factory {
	t.Helper()

	resp := createNamedFactoryExpectStatus(t, serverURL, body, http.StatusOK)
	var created factoryapi.Factory
	decodeJSONResponse(t, resp, &created, "decode upsert named factory response")
	return created
}

func upsertNamedFactoryFromBody(t *testing.T, serverURL, factoryBody string) factoryapi.Factory {
	t.Helper()

	requestBody := fmt.Sprintf(`{"mode":"UPSERT_NAMED_AND_ACTIVATE","factory":%s}`, factoryBody)
	resp := putFactoryForSessionRequestExpectStatus(
		t,
		serverURL,
		"/factory-sessions/~default/factory",
		requestBody,
		http.StatusOK,
	)
	var created factoryapi.Factory
	decodeJSONResponse(t, resp, &created, "decode upsert named factory response")
	return created
}

func putFactoryForSessionRequestExpectStatusWithClient(
	t *testing.T,
	client *http.Client,
	serverURL,
	path string,
	body string,
	wantStatus int,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPut, serverURL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new factory session save request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("PUT %s status = %d, want %d: %s", path, resp.StatusCode, wantStatus, payload)
	}
	return resp
}

func submitWorkAndExpectStatus(t *testing.T, serverURL, workType, title string, wantStatus int) *http.Response {
	t.Helper()

	resp, err := http.Post(
		support.DefaultSessionWorkURL(serverURL, "/work"),
		"application/json",
		bytes.NewBufferString(`{"name":"import-export-named-factory-submit","workTypeName":"`+workType+`","payload":{"title":"`+title+`"}}`),
	)
	if err != nil {
		t.Fatalf("POST /work %s: %v", workType, err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work %s status = %d, want %d", workType, resp.StatusCode, wantStatus)
	}
	return resp
}

func requireFactoryChangeAfter(t *testing.T, before []factoryapi.FactoryEvent, after []factoryapi.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()

	minSequence := -1
	for _, event := range before {
		if event.Context.Sequence > minSequence {
			minSequence = event.Context.Sequence
		}
	}
	for _, event := range after {
		if event.Context.Sequence > minSequence && event.Type == factoryapi.FactoryEventTypeFactoryChange {
			return event
		}
	}
	t.Fatalf("factory-change event not found after save; before=%d after=%d", len(before), len(after))
	return factoryapi.FactoryEvent{}
}

func functionalNamedFactoryBody(name, workType string) string {
	return functionalNamedFactoryPayloadJSON(name, workType)
}

func functionalNamedFactoryBodyWithBundledFiles(name, workType string) string {
	return `{
		"name":"` + name + `",
		"id":"` + name + `",
		"workTypes":[{"name":"` + workType + `","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"done"}]}],
		"supportingFiles":{"bundledFiles":[
			{"type":"ROOT_HELPER","targetPath":"Makefile","content":{"encoding":"utf-8","inline":"test:\n\tgo test ./...\n"}},
			{"type":"DOC","targetPath":"factory/docs/README.md","content":{"encoding":"utf-8","inline":"# Portable factory\n"}},
			{"type":"SCRIPT","targetPath":"factory/scripts/execute-story.ps1","content":{"encoding":"utf-8","inline":"Write-Output 'portable script'\n"}}
		]}
	}`
}

func functionalNamedFactoryBodyWithPortableLayout(name, workType string) string {
	return currentFactorySaveDocumentWithPortableLayout(nil, name, workType, nil)
}

func currentFactorySaveDocumentWithPortableLayout(t *testing.T, name, workType string, version any) string {
	if t != nil {
		t.Helper()
	}
	document := map[string]any{
		"name": name,
		"id":   name,
		"layout": map[string]any{
			"schemaVersion": 1,
			"nodes": []map[string]any{{
				"id":       "workstation:plan-task",
				"position": map[string]any{"x": 144, "y": 288},
				"size":     map[string]any{"width": 320, "height": 180},
				"locked":   true,
			}},
			"edges": []map[string]any{{
				"id":        "workstation-output:workstation:plan-task->work-state:" + workType + ":done",
				"waypoints": []map[string]any{{"x": 200, "y": 300}},
			}},
			"groups": []map[string]any{{
				"id":      "group-1",
				"label":   "Planning",
				"nodeIds": []string{"workstation:plan-task"},
				"bounds":  map[string]any{"x": 100, "y": 220, "width": 420, "height": 240},
			}},
			"viewport":    map[string]any{"x": 40, "y": 60, "zoom": 0.85},
			"preferences": map[string]any{"direction": "RIGHT"},
		},
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":             "planner",
			"type":             "MODEL_WORKER",
			"modelProvider":    "CLAUDE",
			"executorProvider": "SCRIPT_WRAP",
			"model":            "claude-sonnet-4-20250514",
			"body":             "You are the planner.",
		}},
		"workstations": []map[string]any{{
			"name":     "plan-task",
			"behavior": "STANDARD",
			"type":     "MODEL_WORKSTATION",
			"worker":   "planner",
			"body":     "Plan the work.",
			"inputs":   []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":  []map[string]string{{"workType": workType, "state": "done"}},
		}},
	}
	if version != nil {
		document["version"] = version
	}
	body, err := json.Marshal(document)
	if err != nil {
		if t != nil {
			t.Fatalf("marshal portable layout factory document: %v", err)
		}
		panic(err)
	}
	return string(body)
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

func assertNamedFactoryPortableLayoutResponse(t *testing.T, layout *factoryapi.FactoryLayout, wantNodeID, wantWorkType string) {
	t.Helper()

	if layout == nil {
		t.Fatal("expected named-factory response layout")
	}
	if layout.SchemaVersion != 1 {
		t.Fatalf("layout schemaVersion = %d, want 1", layout.SchemaVersion)
	}
	if layout.Nodes == nil || len(*layout.Nodes) != 1 || (*layout.Nodes)[0].Id != wantNodeID {
		t.Fatalf("layout nodes = %#v, want %s", layout.Nodes, wantNodeID)
	}
	wantEdgeID := "workstation-output:workstation:plan-task->work-state:" + wantWorkType + ":done"
	if layout.Edges == nil || len(*layout.Edges) != 1 || (*layout.Edges)[0].Id != wantEdgeID {
		t.Fatalf("layout edges = %#v, want %s", layout.Edges, wantEdgeID)
	}
	if layout.Groups == nil || len(*layout.Groups) != 1 || (*layout.Groups)[0].Id != "group-1" {
		t.Fatalf("layout groups = %#v, want group-1", layout.Groups)
	}
	if layout.Viewport == nil || math.Abs(float64(layout.Viewport.Zoom)-0.85) > 1e-6 {
		t.Fatalf("layout viewport = %#v, want zoom 0.85", layout.Viewport)
	}
	if layout.Preferences == nil || layout.Preferences.Direction == nil || *layout.Preferences.Direction != factoryapi.RIGHT {
		t.Fatalf("layout preferences = %#v, want RIGHT", layout.Preferences)
	}
}

func assertPortableLayoutPayload(t *testing.T, value any) {
	t.Helper()

	layout, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("persisted layout = %#v, want object", value)
	}
	if got := layout["schemaVersion"]; got != float64(1) {
		t.Fatalf("persisted layout schemaVersion = %#v, want 1", got)
	}
	nodes, ok := layout["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("persisted layout nodes = %#v, want one node", layout["nodes"])
	}
	node, ok := nodes[0].(map[string]any)
	if !ok || node["id"] != "workstation:plan-task" {
		t.Fatalf("persisted layout node = %#v, want workstation:plan-task", nodes[0])
	}
	edges, ok := layout["edges"].([]any)
	if !ok || len(edges) != 1 {
		t.Fatalf("persisted layout edges = %#v, want one edge", layout["edges"])
	}
	groups, ok := layout["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("persisted layout groups = %#v, want one group", layout["groups"])
	}
	viewport, ok := layout["viewport"].(map[string]any)
	if !ok || viewport["zoom"] != 0.85 {
		t.Fatalf("persisted layout viewport = %#v, want zoom 0.85", layout["viewport"])
	}
	preferences, ok := layout["preferences"].(map[string]any)
	if !ok || preferences["direction"] != "RIGHT" {
		t.Fatalf("persisted layout preferences = %#v, want RIGHT", layout["preferences"])
	}
}

func assertBundledFilesWithoutInlineScriptsAndDocs(t *testing.T, namedFactory factoryapi.Factory, contextLabel string) {
	t.Helper()

	if namedFactory.SupportingFiles == nil || namedFactory.SupportingFiles.BundledFiles == nil {
		t.Fatalf("%s supportingFiles = %#v, want bundled files", contextLabel, namedFactory.SupportingFiles)
	}
	bundledFiles := *namedFactory.SupportingFiles.BundledFiles
	if len(bundledFiles) != 3 {
		t.Fatalf("%s bundled files = %#v, want 3 entries", contextLabel, bundledFiles)
	}
	assertBundledFileEntry(t, bundledFiles[0], factoryapi.BundledFileTypeROOTHELPER, "Makefile", importExportNamedMakefileBody, contextLabel)
	assertBundledFileEntryWithoutInline(t, bundledFiles[1], factoryapi.BundledFileTypeDOC, "factory/docs/README.md", contextLabel)
	assertBundledFileEntryWithoutInline(t, bundledFiles[2], factoryapi.BundledFileTypeSCRIPT, "factory/scripts/execute-story.ps1", contextLabel)
}

func assertBundledFilesWithInlineScriptsAndDocs(t *testing.T, namedFactory factoryapi.Factory, contextLabel string) {
	t.Helper()

	if namedFactory.SupportingFiles == nil || namedFactory.SupportingFiles.BundledFiles == nil {
		t.Fatalf("%s supportingFiles = %#v, want bundled files", contextLabel, namedFactory.SupportingFiles)
	}
	bundledFiles := *namedFactory.SupportingFiles.BundledFiles
	if len(bundledFiles) != 3 {
		t.Fatalf("%s bundled files = %#v, want 3 entries", contextLabel, bundledFiles)
	}
	assertBundledFileEntry(t, bundledFiles[0], factoryapi.BundledFileTypeROOTHELPER, "Makefile", importExportNamedMakefileBody, contextLabel)
	assertBundledFileEntry(t, bundledFiles[1], factoryapi.BundledFileTypeDOC, "factory/docs/README.md", importExportNamedPortableDocBody, contextLabel)
	assertBundledFileEntry(t, bundledFiles[2], factoryapi.BundledFileTypeSCRIPT, "factory/scripts/execute-story.ps1", importExportNamedPortableScriptBody, contextLabel)
}

func assertBundledFileEntry(
	t *testing.T,
	bundledFile factoryapi.BundledFile,
	wantType factoryapi.BundledFileType,
	wantPath string,
	wantInline string,
	contextLabel string,
) {
	t.Helper()

	if bundledFile.Type != wantType || bundledFile.TargetPath != wantPath {
		t.Fatalf("%s bundled file = %#v, want type %s path %s", contextLabel, bundledFile, wantType, wantPath)
	}
	if bundledFile.Content.Encoding != "utf-8" || bundledFile.Content.Inline != wantInline {
		t.Fatalf("%s bundled file content = %#v, want inline %q", contextLabel, bundledFile.Content, wantInline)
	}
}

func assertBundledFileEntryWithoutInline(
	t *testing.T,
	bundledFile factoryapi.BundledFile,
	wantType factoryapi.BundledFileType,
	wantPath string,
	contextLabel string,
) {
	t.Helper()

	if bundledFile.Type != wantType || bundledFile.TargetPath != wantPath {
		t.Fatalf("%s bundled file = %#v, want type %s path %s", contextLabel, bundledFile, wantType, wantPath)
	}
	if bundledFile.Content.Encoding != "utf-8" || bundledFile.Content.Inline != "" {
		t.Fatalf("%s bundled file content = %#v, want utf-8 without inline content", contextLabel, bundledFile.Content)
	}
}

func assertPortableFile(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}

func assertPersistedFactoryJSONStripsInlineBundledContent(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	payload := string(data)
	for _, forbidden := range []string{
		importExportNamedPortableDocBody,
		importExportNamedPortableScriptBody,
	} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Fatalf("persisted factory payload %s still contains inline portable content %q: %s", path, forbidden, payload)
		}
	}
}

func assertFunctionalSplitLayoutAtRoot(t *testing.T, rootDir, project string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the planner.") {
		t.Fatalf("factory.json should omit inlined planner body after split save, got %s", factoryJSON)
	}

	agentsPath := filepath.Join(rootDir, interfaces.WorkersDir, "planner", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("expected planner AGENTS.md at %s: %v", agentsPath, err)
	}

	workstationPath := filepath.Join(rootDir, interfaces.WorkstationsDir, "plan-task", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(workstationPath); err != nil {
		t.Fatalf("expected plan-task AGENTS.md at %s: %v", workstationPath, err)
	}

	loaded, err := support.LoadedCurrentFactory(t, rootDir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.Id == nil || *loaded.Id != project {
		t.Fatalf("factory id = %v, want %q", loaded.Id, project)
	}
}

func assertImportEquivalentSplitMaterialization(t *testing.T, factoryRootDir, wantProject string) {
	t.Helper()

	assertFunctionalSplitLayoutAtRoot(t, factoryRootDir, wantProject)
	assertPortableFile(t, filepath.Join(factoryRootDir, "Makefile"), importExportNamedMakefileBody)
	assertPortableFile(t, filepath.Join(factoryRootDir, "docs", "README.md"), importExportNamedPortableDocBody)
	assertPortableFile(
		t,
		filepath.Join(factoryRootDir, "scripts", "execute-story.ps1"),
		importExportNamedPortableScriptBody,
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

func assertImportExportNamedFactoryRunnerIdle(t *testing.T, runner *support.RecordingCommandRunner) {
	t.Helper()
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during named-factory API proofs", runner.CallCount())
	}
}

func freshNamedFactoryVersion(created factoryapi.Factory) factoryapi.HybridLogicalTimestamp {
	return factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
}
