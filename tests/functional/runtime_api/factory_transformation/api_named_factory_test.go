package factory_transformation

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFactoryTransformation_CreateNamedFactoryReadbackAndWorkSurface(t *testing.T) {
	support.SkipLongFunctional(t, "slow named-factory API sweep")
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)

	created := createNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))
	assertFactoryWorkType(t, created, "beta-task", "created factory")
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	assertCurrentFactoryPointer(t, rootDir, "beta")

	current := getCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
	assertFactoryWorkType(t, current, "beta-task", "current factory readback")

	betaResp := submitWorkAndExpectStatus(t, server.URL(), "beta-task", "beta", http.StatusCreated)
	var betaSubmit factoryapi.SubmitWorkResponse
	decodeJSONResponse(t, betaResp, &betaSubmit, "decode beta-task submit response")
	if betaSubmit.TraceId == "" {
		t.Fatal("expected non-empty trace ID for activated beta-task submission")
	}

	legacyResp := submitWorkAndExpectStatus(t, server.URL(), "alpha-task", "alpha", http.StatusBadRequest)
	var legacyErr factoryapi.ErrorResponse
	decodeJSONResponse(t, legacyResp, &legacyErr, "decode alpha-task error response")
	if legacyErr.Code != factoryapi.BADREQUEST {
		t.Fatalf("alpha-task error code = %q, want BAD_REQUEST", legacyErr.Code)
	}
}

func TestFactoryTransformation_NamedFactoryPortableFilesReadBackThroughCanonicalContract(t *testing.T) {
	support.SkipLongFunctional(t, "slow named-factory API sweep")
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)

	created := createNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBodyWithBundledFiles("beta", "beta-task"))
	assertBundledFilesWithoutInlineScriptsAndDocs(t, created, "created response")

	current := getCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
	assertFactoryWorkType(t, current, "beta-task", "current factory readback")
	assertBundledFilesWithInlineScriptsAndDocs(t, current, "current response")

	importedDir := filepath.Join(rootDir, "beta")
	assertPortableFile(t, filepath.Join(importedDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableFile(t, filepath.Join(importedDir, "docs", "README.md"), "# Portable factory\n")
	assertPortableFile(t, filepath.Join(importedDir, "scripts", "execute-story.ps1"), "Write-Output 'portable script'\n")
	assertPersistedFactoryJSONStripsInlineBundledContent(t, filepath.Join(importedDir, interfaces.FactoryConfigFile))
}

func TestFactoryTransformation_CreateNamedFactoryEmitsCanonicalFactoryChangeEvent(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
	initialEvents := server.GetFactoryEvents(t)

	createNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))

	change := requireFactoryChangeAfter(t, initialEvents, server.GetFactoryEvents(t))
	if change.Context.Tick <= latestEventTick(initialEvents) {
		t.Fatalf("factory-change tick = %d, want > initial event tick", change.Context.Tick)
	}

	payload, err := change.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode factory-change payload: %v", err)
	}
	if payload.Factory.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("factory-change payload name = %q, want beta", payload.Factory.Name)
	}
	assertFactoryWorkType(t, payload.Factory, "beta-task", "factory-change payload")
}

func createNamedFactoryFromBody(t *testing.T, serverURL, body string) factoryapi.Factory {
	t.Helper()
	resp, err := http.Post(serverURL+"/factories", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /factories: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("POST /factories status = %d, want 201", resp.StatusCode)
	}
	var created factoryapi.Factory
	decodeJSONResponse(t, resp, &created, "decode create factory response")
	return created
}

func assertCurrentFactoryPointer(t *testing.T, rootDir, want string) {
	t.Helper()
	got, err := config.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if got != want {
		t.Fatalf("current factory pointer = %q, want %q", got, want)
	}
}

func assertFactoryWorkType(t *testing.T, factory factoryapi.Factory, want string, contextLabel string) {
	t.Helper()
	if factory.WorkTypes == nil || len(*factory.WorkTypes) != 1 || (*factory.WorkTypes)[0].Name != want {
		t.Fatalf("%s work types = %#v, want %s", contextLabel, factory.WorkTypes, want)
	}
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

func assertBundledFilesWithoutInlineScriptsAndDocs(t *testing.T, namedFactory factoryapi.Factory, contextLabel string) {
	t.Helper()

	if namedFactory.SupportingFiles == nil || namedFactory.SupportingFiles.BundledFiles == nil {
		t.Fatalf("%s supportingFiles = %#v, want bundled files", contextLabel, namedFactory.SupportingFiles)
	}
	bundledFiles := *namedFactory.SupportingFiles.BundledFiles
	if len(bundledFiles) != 3 {
		t.Fatalf("%s bundled files = %#v, want 3 entries", contextLabel, bundledFiles)
	}
	assertBundledFileEntry(t, bundledFiles[0], factoryapi.BundledFileTypeROOTHELPER, "Makefile", "test:\n\tgo test ./...\n", contextLabel)
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
	assertBundledFileEntry(t, bundledFiles[0], factoryapi.BundledFileTypeROOTHELPER, "Makefile", "test:\n\tgo test ./...\n", contextLabel)
	assertBundledFileEntry(t, bundledFiles[1], factoryapi.BundledFileTypeDOC, "factory/docs/README.md", "# Portable factory\n", contextLabel)
	assertBundledFileEntry(t, bundledFiles[2], factoryapi.BundledFileTypeSCRIPT, "factory/scripts/execute-story.ps1", "Write-Output 'portable script'\n", contextLabel)
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
		"# Portable factory",
		"Write-Output 'portable script'",
	} {
		if bytes.Contains(data, []byte(forbidden)) {
			t.Fatalf("persisted factory payload %s still contains inline portable content %q: %s", path, forbidden, payload)
		}
	}
}

func latestEventTick(events []factoryapi.FactoryEvent) int {
	latest := -1
	for _, event := range events {
		if event.Context.Tick > latest {
			latest = event.Context.Tick
		}
	}
	return latest
}
