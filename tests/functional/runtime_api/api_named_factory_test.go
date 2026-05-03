package runtime_api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
)

func TestNamedFactoryAPI_PersistsActivatesAndSwitchesWorkSurface(t *testing.T) {
	support.SkipLongFunctional(t, "slow named-factory API sweep")
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFunctionalServerWithConfig(t, rootDir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.Logger = zap.NewNop()
	})

	created := createNamedFactoryFromBody(t, server.URL(), "beta", "beta-task", functionalNamedFactoryBody("beta", "beta-task"))
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	assertNamedFactoryCurrentPointer(t, rootDir, "beta")

	current := getNamedFactoryCurrent(t, server.URL())
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}

	betaResp := submitWorkAndExpectStatus(t, server.URL(), "beta-task", "beta", http.StatusCreated)
	var betaSubmit factoryapi.SubmitWorkResponse
	decodeNamedFactoryJSONResponse(t, betaResp, &betaSubmit, "decode beta-task submit response")
	if betaSubmit.TraceId == "" {
		t.Fatal("expected non-empty trace ID for activated beta-task submission")
	}

	legacyResp := submitWorkAndExpectStatus(t, server.URL(), "alpha-task", "alpha", http.StatusBadRequest)
	var legacyErr factoryapi.ErrorResponse
	decodeNamedFactoryJSONResponse(t, legacyResp, &legacyErr, "decode alpha-task error response")
	if legacyErr.Code != factoryapi.BADREQUEST {
		t.Fatalf("alpha-task error code = %q, want BAD_REQUEST", legacyErr.Code)
	}
}

func TestNamedFactoryAPI_RoundTripsPortableBundledFilesThroughCanonicalFactoryContract(t *testing.T) {
	support.SkipLongFunctional(t, "slow named-factory API sweep")
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFunctionalServerWithConfig(t, rootDir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.Logger = zap.NewNop()
	})

	created := createNamedFactoryFromBody(t, server.URL(), "beta", "beta-task", functionalNamedFactoryBodyWithBundledFiles("beta", "beta-task"))
	assertFunctionalNamedFactoryBundledFiles(t, created, "created response")

	current := getNamedFactoryCurrent(t, server.URL())
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
	assertFunctionalNamedFactoryBundledFiles(t, current, "current response")

	importedDir := filepath.Join(rootDir, "beta")
	assertFunctionalPortableFile(t, filepath.Join(importedDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertFunctionalPortableFile(t, filepath.Join(importedDir, "docs", "README.md"), "# Portable factory\n")
	assertFunctionalPortableFile(t, filepath.Join(importedDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertFunctionalPersistedFactoryJSONStripsInlineBundledContent(t, filepath.Join(importedDir, interfaces.FactoryConfigFile))
}

func seedNamedFactoryRoot(t *testing.T, rootDir, name, workType string) {
	t.Helper()
	if _, err := config.PersistNamedFactory(rootDir, name, functionalNamedFactoryPayloadWithWorkType(t, name, workType)); err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, name); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(%s): %v", name, err)
	}
}

func createNamedFactoryFromBody(t *testing.T, serverURL, name, workType, body string) factoryapi.Factory {
	t.Helper()
	resp, err := http.Post(serverURL+"/factory", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /factory: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("POST /factory status = %d, want 201", resp.StatusCode)
	}
	var created factoryapi.Factory
	decodeNamedFactoryJSONResponse(t, resp, &created, "decode create factory response")
	return created
}

func getNamedFactoryCurrent(t *testing.T, serverURL string) factoryapi.Factory {
	t.Helper()
	resp, err := http.Get(serverURL + "/factory/~current")
	if err != nil {
		t.Fatalf("GET /factory/~current: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET /factory/~current status = %d, want 200", resp.StatusCode)
	}
	var current factoryapi.Factory
	decodeNamedFactoryJSONResponse(t, resp, &current, "decode current factory response")
	return current
}

func submitWorkAndExpectStatus(t *testing.T, serverURL, workType, title string, wantStatus int) *http.Response {
	t.Helper()
	resp, err := http.Post(serverURL+"/work", "application/json", bytes.NewBufferString(`{"workTypeName":"`+workType+`","payload":{"title":"`+title+`"}}`))
	if err != nil {
		t.Fatalf("POST /work %s: %v", workType, err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work %s status = %d, want %d", workType, resp.StatusCode, wantStatus)
	}
	return resp
}

func decodeNamedFactoryJSONResponse(t *testing.T, resp *http.Response, target any, message string) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func assertNamedFactoryCurrentPointer(t *testing.T, rootDir, want string) {
	t.Helper()
	got, err := config.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if got != want {
		t.Fatalf("current factory pointer = %q, want %q", got, want)
	}
}

func functionalNamedFactoryPayloadWithWorkType(t *testing.T, name, workType string) []byte {
	t.Helper()
	return []byte(functionalNamedFactoryPayloadJSON(name, workType))
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

func functionalNamedFactoryPayloadJSON(name, workType string) string {
	return `{
		"name":"` + name + `",
		"id":"` + name + `",
		"workTypes":[{"name":"` + workType + `","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514"}],
		"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"done"}]}]
	}`
}

func assertFunctionalNamedFactoryBundledFiles(t *testing.T, namedFactory factoryapi.Factory, contextLabel string) {
	t.Helper()

	if namedFactory.SupportingFiles == nil || namedFactory.SupportingFiles.BundledFiles == nil {
		t.Fatalf("%s supportingFiles = %#v, want bundled files", contextLabel, namedFactory.SupportingFiles)
	}
	bundledFiles := *namedFactory.SupportingFiles.BundledFiles
	if len(bundledFiles) != 3 {
		t.Fatalf("%s bundled files = %#v, want 3 entries", contextLabel, bundledFiles)
	}
	assertFunctionalBundledFileEntry(t, bundledFiles[0], factoryapi.ROOTHELPER, "Makefile", "test:\n\tgo test ./...\n", contextLabel)
	assertFunctionalBundledFileEntry(t, bundledFiles[1], factoryapi.DOC, "factory/docs/README.md", "# Portable factory\n", contextLabel)
	assertFunctionalBundledFileEntry(t, bundledFiles[2], factoryapi.SCRIPT, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n", contextLabel)
}

func assertFunctionalBundledFileEntry(
	t *testing.T,
	bundledFile factoryapi.BundledFile,
	wantType factoryapi.BundledFileType,
	wantPath, wantInline, contextLabel string,
) {
	t.Helper()

	if bundledFile.Type != wantType {
		t.Fatalf("%s bundled file type = %q, want %q", contextLabel, bundledFile.Type, wantType)
	}
	if bundledFile.TargetPath != wantPath {
		t.Fatalf("%s bundled file targetPath = %q, want %q", contextLabel, bundledFile.TargetPath, wantPath)
	}
	if bundledFile.Content.Encoding != factoryapi.Utf8 {
		t.Fatalf("%s bundled file encoding = %q, want %q", contextLabel, bundledFile.Content.Encoding, factoryapi.Utf8)
	}
	if bundledFile.Content.Inline != wantInline {
		t.Fatalf("%s bundled file inline = %q, want %q", contextLabel, bundledFile.Content.Inline, wantInline)
	}
}

func assertFunctionalPortableFile(t *testing.T, path, want string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}

func assertFunctionalPersistedFactoryJSONStripsInlineBundledContent(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal(%s): %v", path, err)
	}

	supportingFiles, ok := payload["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("persisted supportingFiles = %#v, want object", payload["supportingFiles"])
	}
	bundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok || len(bundledFiles) != 3 {
		t.Fatalf("persisted bundledFiles = %#v, want 3 entries", supportingFiles["bundledFiles"])
	}
	for _, entry := range bundledFiles {
		bundledFile, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("persisted bundled file = %#v, want object", entry)
		}
		content, ok := bundledFile["content"].(map[string]any)
		if !ok {
			t.Fatalf("persisted bundled content = %#v, want object", bundledFile["content"])
		}
		if got := content["inline"]; got != "" {
			t.Fatalf("persisted bundled inline = %#v, want empty string", got)
		}
		if got := content["encoding"]; got != "" {
			t.Fatalf("persisted bundled encoding = %#v, want empty string", got)
		}
	}
}
