package bootstrap_portability

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
)

func TestExportImportSmoke_ExportedFactoryCanBeReimportedThroughCustomerPath(t *testing.T) {
	fixture := newExportImportFixture(t)
	harness := newExportImportSmokeHarness(fixture)

	result := harness.Run(t)

	result.AssertAPIContractSuccess(t, fixture)
	result.AssertDashboardActivationSuccess(t, fixture)

	importedResp := submitWorkAndExpectStatus(
		t,
		result.Server.URL(),
		fixture.Expected.WorkTypeName,
		"reimported-service-simple",
		http.StatusCreated,
	)
	var importedSubmit factoryapi.SubmitWorkResponse
	decodeJSONResponse(t, importedResp, &importedSubmit, "decode reimported work submit response")
	if importedSubmit.TraceId == "" {
		t.Fatal("active-factory drift: imported factory should accept work through POST /work")
	}

	legacyResp := submitWorkAndExpectStatus(
		t,
		result.Server.URL(),
		"legacy-"+fixture.Expected.WorkTypeName,
		"legacy",
		http.StatusBadRequest,
	)
	var legacyErr factoryapi.ErrorResponse
	decodeJSONResponse(t, legacyResp, &legacyErr, "decode legacy work type error response")
	if legacyErr.Code != factoryapi.BADREQUEST {
		t.Fatalf("active-factory drift: legacy work type error code = %q, want BAD_REQUEST", legacyErr.Code)
	}
}

func TestExportImportSmoke_ImportedFactoryPersistsThinSplitRuntimeLayout(t *testing.T) {
	fixture := newExportImportFixture(t)
	harness := newExportImportSmokeHarness(fixture)

	result := harness.Run(t)

	assertImportedFactoryLayoutOmitsInlineRuntimeBodies(t, result.ImportedDir)
	assertImportedPortableBundledFilesPersistThinAndMaterializeOnDisk(t, result.ImportedDir)
	assertImportedWorkerBodiesPersistOnlyInAgentsFiles(t, result.ImportedDir, valueOrEmpty(result.ImportedFactory.Workers))
	assertImportedWorkstationBodiesPersistOnlyInAgentsFiles(t, result.ImportedDir, valueOrEmpty(result.ImportedFactory.Workstations))
	assertImportedFactoryRuntimeReloadPreservesBodies(t, result.ImportedDir, valueOrEmpty(result.ImportedFactory.Workers), valueOrEmpty(result.ImportedFactory.Workstations))
}

func submitWorkAndExpectStatus(
	t *testing.T,
	serverURL, workTypeName, title string,
	wantStatus int,
) *http.Response {
	t.Helper()

	request := factoryapi.SubmitWorkRequest{
		WorkTypeName: workTypeName,
		Payload:      []byte(`{"title":"` + title + `"}`),
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}
	resp, err := http.Post(serverURL+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work status = %d, want %d", resp.StatusCode, wantStatus)
	}
	return resp
}

func assertImportedFactoryLayoutOmitsInlineRuntimeBodies(t *testing.T, factoryDir string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(factoryDir, "factory.json"))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}

	for _, workerEntry := range requireObjectSlice(t, payload["workers"], "workers") {
		if _, ok := workerEntry["body"]; ok {
			t.Fatalf("imported factory.json worker should omit inline body: %#v", workerEntry)
		}
	}
	for _, workstationEntry := range requireObjectSlice(t, payload["workstations"], "workstations") {
		if _, ok := workstationEntry["body"]; ok {
			t.Fatalf("imported factory.json workstation should omit inline body: %#v", workstationEntry)
		}
	}
}

func assertImportedWorkerBodiesPersistOnlyInAgentsFiles(t *testing.T, factoryDir string, workers []factoryapi.Worker) {
	t.Helper()

	for _, worker := range workers {
		if worker.Body == nil {
			t.Fatalf("expected imported worker %q to expose a runtime body", worker.Name)
		}
		agentsPath := filepath.Join(factoryDir, "workers", worker.Name, "AGENTS.md")
		contents, err := os.ReadFile(agentsPath)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", agentsPath, err)
		}
		got := string(contents)
		if got != *worker.Body+"\n" {
			t.Fatalf("imported worker AGENTS.md for %q = %q, want body-only %q", worker.Name, got, *worker.Body+"\n")
		}
		if strings.HasPrefix(got, "---") {
			t.Fatalf("imported worker AGENTS.md for %q should be body-only, got frontmatter:\n%s", worker.Name, got)
		}
	}
}

func assertImportedWorkstationBodiesPersistOnlyInAgentsFiles(t *testing.T, factoryDir string, workstations []factoryapi.Workstation) {
	t.Helper()

	for _, workstation := range workstations {
		if workstation.Body == nil {
			t.Fatalf("expected imported workstation %q to expose a runtime body", workstation.Name)
		}
		agentsPath := filepath.Join(factoryDir, "workstations", workstation.Name, "AGENTS.md")
		contents, err := os.ReadFile(agentsPath)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", agentsPath, err)
		}
		got := string(contents)
		if got != *workstation.Body+"\n" {
			t.Fatalf("imported workstation AGENTS.md for %q = %q, want body-only %q", workstation.Name, got, *workstation.Body+"\n")
		}
		if strings.HasPrefix(got, "---") {
			t.Fatalf("imported workstation AGENTS.md for %q should be body-only, got frontmatter:\n%s", workstation.Name, got)
		}
	}
}

func assertImportedFactoryRuntimeReloadPreservesBodies(
	t *testing.T,
	factoryDir string,
	workers []factoryapi.Worker,
	workstations []factoryapi.Workstation,
) {
	t.Helper()

	loaded, err := config.LoadRuntimeConfig(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(%s): %v", factoryDir, err)
	}

	for _, worker := range workers {
		runtimeWorker, ok := loaded.Worker(worker.Name)
		if !ok {
			t.Fatalf("expected imported runtime worker %q to load", worker.Name)
		}
		if worker.Body == nil || runtimeWorker.Body != *worker.Body {
			t.Fatalf("runtime worker %q body = %q, want %q", worker.Name, runtimeWorker.Body, stringPtrValue(worker.Body))
		}
	}
	for _, workstation := range workstations {
		runtimeWorkstation, ok := loaded.Workstation(workstation.Name)
		if !ok {
			t.Fatalf("expected imported runtime workstation %q to load", workstation.Name)
		}
		if workstation.Body == nil || runtimeWorkstation.Body != *workstation.Body {
			t.Fatalf("runtime workstation %q body = %q, want %q", workstation.Name, runtimeWorkstation.Body, stringPtrValue(workstation.Body))
		}
		if runtimeWorkstation.PromptTemplate != stringPtrValue(workstation.Body) {
			t.Fatalf("runtime workstation %q prompt template = %q, want %q", workstation.Name, runtimeWorkstation.PromptTemplate, stringPtrValue(workstation.Body))
		}
	}
}

func assertImportedPortableBundledFilesPersistThinAndMaterializeOnDisk(t *testing.T, factoryDir string) {
	t.Helper()

	assertImportedPortableFile(t, filepath.Join(factoryDir, "Makefile"), exportImportPortableMakefileBody)
	assertImportedPortableFile(t, filepath.Join(factoryDir, "docs", "README.md"), exportImportPortableDocBody)
	assertImportedPortableFile(t, filepath.Join(factoryDir, "scripts", "execute-story.ps1"), exportImportPortableScriptBody)

	data, err := os.ReadFile(filepath.Join(factoryDir, "factory.json"))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}

	supportingFiles, ok := payload["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected supportingFiles object, got %#v", payload["supportingFiles"])
	}
	bundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok || len(bundledFiles) != 3 {
		t.Fatalf("expected 3 persisted bundled files, got %#v", supportingFiles["bundledFiles"])
	}

	for _, entry := range bundledFiles {
		bundledFile, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file object, got %#v", entry)
		}
		content, ok := bundledFile["content"].(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file content object, got %#v", bundledFile["content"])
		}

		targetPath, _ := bundledFile["targetPath"].(string)
		switch targetPath {
		case exportImportPortableMakefilePath:
			if got := content["inline"]; got != exportImportPortableMakefileBody {
				t.Fatalf("persisted root helper inline = %#v, want %q", got, exportImportPortableMakefileBody)
			}
		case exportImportPortableDocPath, exportImportPortableScriptPath:
			if _, ok := content["inline"]; ok {
				t.Fatalf("persisted bundled inline for %q should be omitted, got %#v", targetPath, content["inline"])
			}
		default:
			t.Fatalf("unexpected persisted bundled targetPath = %#v", targetPath)
		}
	}
}

func assertImportedPortableFile(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}

func requireObjectSlice(t *testing.T, value any, field string) []map[string]any {
	t.Helper()

	entries, ok := value.([]any)
	if !ok {
		t.Fatalf("expected %s to be an array, got %#v", field, value)
	}
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		obj, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected %s entry to be an object, got %#v", field, entry)
		}
		out = append(out, obj)
	}
	return out
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
