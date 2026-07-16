package factory_transformation

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCurrentFactoryEvents_InitialStructureIncludesBundledFileContent(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.FactoryConfigFile),
		currentFactoryEventDocumentWithBundledFiles(
			t,
			"root-runtime",
			"story",
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "# Overview\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
		0o644,
	); err != nil {
		t.Fatalf("write factory config with bundled files: %v", err)
	}

	server := startFactoryTransformationServer(t, rootDir)
	payload := requireInitialStructurePayload(t, server.GetFactoryEvents(t))
	assertDocBundledFileInline(t, payload.Factory, "factory/docs/overview.md", "# Overview\n")
	assertScriptBundledFileInline(t, payload.Factory, "factory/scripts/setup-workspace.py", "print('setup')\n")
}

func TestCurrentFactoryPUT_DocsCreateEditRenameDeleteRoundTrip(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())

	created := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			current,
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "# Overview\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)
	assertDocBundledFileInline(t, created, "factory/docs/overview.md", "# Overview\n")
	assertScriptBundledFileInline(t, created, "factory/scripts/setup-workspace.py", "print('setup')\n")

	alphaDir := filepath.Join(rootDir, "alpha")
	assertPortableFile(t, filepath.Join(alphaDir, "docs", "overview.md"), "# Overview\n")
	assertPortableFile(t, filepath.Join(alphaDir, "scripts", "setup-workspace.py"), "print('setup')\n")

	afterCreate := getCurrentFactory(t, server.URL())
	edited := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			afterCreate,
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "# Overview updated\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)
	assertDocBundledFileInline(t, edited, "factory/docs/overview.md", "# Overview updated\n")
	assertPortableFile(t, filepath.Join(alphaDir, "docs", "overview.md"), "# Overview updated\n")

	afterEdit := getCurrentFactory(t, server.URL())
	renamed := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			afterEdit,
			[]map[string]any{
				docBundledFileEntry("factory/docs/guide.md", "# Overview updated\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)
	assertDocBundledFileInline(t, renamed, "factory/docs/guide.md", "# Overview updated\n")
	if _, err := os.Stat(filepath.Join(alphaDir, "docs", "overview.md")); !os.IsNotExist(err) {
		t.Fatalf("renamed-away doc stat error = %v, want not exist", err)
	}
	assertPortableFile(t, filepath.Join(alphaDir, "docs", "guide.md"), "# Overview updated\n")

	afterRename := getCurrentFactory(t, server.URL())
	deleted := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			afterRename,
			[]map[string]any{
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)
	if findBundledFile(t, deleted, "factory/docs/guide.md") != nil {
		t.Fatalf("deleted doc still present in save response: %#v", deleted.SupportingFiles)
	}
	if _, err := os.Stat(filepath.Join(alphaDir, "docs", "guide.md")); !os.IsNotExist(err) {
		t.Fatalf("deleted doc stat error = %v, want not exist", err)
	}

	reloaded := getCurrentFactory(t, server.URL())
	if findBundledFile(t, reloaded, "factory/docs/guide.md") != nil {
		t.Fatalf("deleted doc still present after reload: %#v", reloaded.SupportingFiles)
	}
	assertScriptBundledFileInline(t, reloaded, "factory/scripts/setup-workspace.py", "print('setup')\n")
}

func TestCurrentFactoryPUT_DocsSaveEmitsFactoryChangeWithBundledFilesAndVersion(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
	initialEvents := server.GetFactoryEvents(t)
	current := getCurrentFactory(t, server.URL())

	saved := saveCurrentFactoryDefinition(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			current,
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "# Overview\n"),
				scriptBundledFileEntry("factory/scripts/setup-workspace.py", "print('setup')\n"),
			},
		),
	)

	change := requireFactoryChangeAfter(t, initialEvents, server.GetFactoryEvents(t))
	payload, err := change.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode factory-change payload: %v", err)
	}
	assertFactoryChangeVersion(t, payload.Factory, saved)
	assertDocBundledFileInline(t, payload.Factory, "factory/docs/overview.md", "# Overview\n")
	assertScriptBundledFileInline(t, payload.Factory, "factory/scripts/setup-workspace.py", "print('setup')\n")
}

func TestCurrentFactoryPUT_RejectsInvalidDocTargets(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())

	cases := []struct {
		name       string
		targetPath string
	}{
		{name: "outside docs root", targetPath: "factory/scripts/readme.md"},
		{name: "non canonical path", targetPath: "factory/docs/./notes.md"},
		{name: "escaping path", targetPath: "factory/docs/../secrets.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := saveCurrentFactoryDefinitionExpectStatus(
				t,
				server.URL(),
				currentFactoryDocumentWithBundledDocs(
					t,
					current,
					[]map[string]any{
						docBundledFileEntry(tc.targetPath, "invalid target\n"),
					},
				),
				http.StatusBadRequest,
			)
			resp.Body.Close()
		})
	}
}

func TestCurrentFactoryPUT_RejectsDuplicateDocTargetPaths(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())

	resp := saveCurrentFactoryDefinitionExpectStatus(
		t,
		server.URL(),
		currentFactoryDocumentWithBundledDocs(
			t,
			current,
			[]map[string]any{
				docBundledFileEntry("factory/docs/overview.md", "first\n"),
				docBundledFileEntry("factory/docs/overview.md", "second\n"),
			},
		),
		http.StatusBadRequest,
	)
	resp.Body.Close()
}

func TestCurrentFactoryPUT_ShellEscapedBundledInlineReplayReturnsPayloadInvalid(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startFactoryTransformationServer(t, rootDir)
	current := getCurrentFactory(t, server.URL())
	if current.Version == nil {
		t.Fatal("current factory version = nil, want version metadata for save")
	}

	// This mirrors a common replay mistake from copy-as-curl artifacts: the
	// bundled file inline content still contains shell-style \' escaping instead
	// of valid JSON string content, so the request fails at payload decoding
	// before factory validation runs.
	body := `{
		"mode":"REPLACE_CURRENT",
		"factory":{
			"name":"alpha",
			"version":{"physical":"` + current.Version.Physical.UTC().Add(time.Nanosecond).Format(time.RFC3339Nano) + `","logical":"` + strconv.FormatInt(current.Version.Logical.Int64()+1, 10) + `"},
			"workTypes":[{"name":"alpha-task","states":[
				{"name":"init","type":"INITIAL"},
				{"name":"complete","type":"TERMINAL"},
				{"name":"failed","type":"FAILED"}
			]}],
			"workers":[{"name":"planner","type":"MODEL_WORKER","modelProvider":"CLAUDE","executorProvider":"SCRIPT_WRAP","model":"claude-sonnet-4-20250514","body":"Plan work."}],
			"workstations":[{"name":"plan-task","behavior":"STANDARD","type":"MODEL_WORKSTATION","worker":"planner","inputs":[{"workType":"alpha-task","state":"init"}],"outputs":[{"workType":"alpha-task","state":"complete"}]}],
			"supportingFiles":{"bundledFiles":[
				{"type":"SCRIPT","targetPath":"factory/scripts/setup-workspace.py","content":{"encoding":"utf-8","inline":"print(\'setup\')\n"}}
			]}
		}
	}`

	req, err := http.NewRequest(http.MethodPut, server.URL()+"/factory-sessions/~default/factory", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new malformed current factory save request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /factory-sessions/~default/factory: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("PUT /factory-sessions/~default/factory status = %d, want 400", resp.StatusCode)
	}

	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode malformed bundled inline save response")
	if errResp.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("error code = %q, want BAD_REQUEST", errResp.Code)
	}
	if errResp.Targets == nil || !hasValidationTargetCode(*errResp.Targets, "factory.payload.invalid") {
		t.Fatalf("error targets = %#v, want factory.payload.invalid decode target", errResp.Targets)
	}
}

func currentFactoryDocumentWithBundledDocs(t *testing.T, current factoryapi.Factory, bundledFiles []map[string]any) string {
	t.Helper()

	body, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("marshal current factory document: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode current factory document: %v", err)
	}
	document["version"] = versionDocument(advancedFactoryVersion(t, current.Version))
	document["supportingFiles"] = map[string]any{
		"bundledFiles": bundledFiles,
	}
	body, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal current factory document with bundled docs: %v", err)
	}
	return string(body)
}

func currentFactoryDocumentWithBundledDocsAndLayout(
	t *testing.T,
	current factoryapi.Factory,
	bundledFiles []map[string]any,
	layout map[string]any,
) string {
	t.Helper()

	body, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("marshal current factory document: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode current factory document: %v", err)
	}
	document["version"] = versionDocument(advancedFactoryVersion(t, current.Version))
	document["supportingFiles"] = map[string]any{
		"bundledFiles": bundledFiles,
	}
	document["layout"] = layout
	body, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal current factory document with bundled docs and layout: %v", err)
	}
	return string(body)
}

func docBundledFileEntry(targetPath, inline string) map[string]any {
	return map[string]any{
		"id":         targetPath,
		"type":       "DOC",
		"targetPath": targetPath,
		"content": map[string]string{
			"encoding": "utf-8",
			"inline":   inline,
		},
	}
}

func scriptBundledFileEntry(targetPath, inline string) map[string]any {
	return map[string]any{
		"id":         targetPath,
		"type":       "SCRIPT",
		"targetPath": targetPath,
		"content": map[string]string{
			"encoding": "utf-8",
			"inline":   inline,
		},
	}
}

func currentFactoryEventDocumentWithBundledFiles(
	t *testing.T,
	name string,
	workType string,
	bundledFiles []map[string]any,
) []byte {
	t.Helper()

	var document map[string]any
	if err := json.Unmarshal([]byte(functionalNamedFactoryPayloadJSON(name, workType)), &document); err != nil {
		t.Fatalf("decode factory event bundled-file document: %v", err)
	}
	document["supportingFiles"] = map[string]any{
		"bundledFiles": bundledFiles,
	}
	body, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal factory event bundled-file document: %v", err)
	}
	return body
}

func findBundledFile(t *testing.T, factory factoryapi.Factory, targetPath string) *factoryapi.BundledFile {
	t.Helper()
	if factory.SupportingFiles == nil || factory.SupportingFiles.BundledFiles == nil {
		return nil
	}
	for _, bundledFile := range *factory.SupportingFiles.BundledFiles {
		if bundledFile.TargetPath == targetPath {
			copied := bundledFile
			return &copied
		}
	}
	return nil
}

func assertFactoryChangeVersion(t *testing.T, eventFactory factoryapi.Factory, saved factoryapi.Factory) {
	t.Helper()
	if saved.Version == nil {
		t.Fatal("saved factory version = nil, want version metadata")
	}
	if eventFactory.Version == nil {
		t.Fatal("factory-change payload version = nil, want saved version metadata")
	}
	if eventFactory.Version.Logical != saved.Version.Logical || !eventFactory.Version.Physical.Equal(saved.Version.Physical) {
		t.Fatalf("factory-change payload version = %#v, want saved version %#v", eventFactory.Version, saved.Version)
	}
}

func assertDocBundledFileInline(t *testing.T, factory factoryapi.Factory, targetPath, wantInline string) {
	t.Helper()
	bundledFile := findBundledFile(t, factory, targetPath)
	if bundledFile == nil {
		t.Fatalf("doc bundled file %q missing from %#v", targetPath, factory.SupportingFiles)
	}
	if bundledFile.Type != factoryapi.BundledFileTypeDOC {
		t.Fatalf("bundled file %q type = %q, want DOC", targetPath, bundledFile.Type)
	}
	if bundledFile.Id == nil || *bundledFile.Id != targetPath {
		t.Fatalf("doc bundled file %q id = %#v, want %q", targetPath, bundledFile.Id, targetPath)
	}
	if bundledFile.Content.Inline != wantInline {
		t.Fatalf("doc bundled file %q inline = %q, want %q", targetPath, bundledFile.Content.Inline, wantInline)
	}
}

func assertScriptBundledFileInline(t *testing.T, factory factoryapi.Factory, targetPath, wantInline string) {
	t.Helper()
	bundledFile := findBundledFile(t, factory, targetPath)
	if bundledFile == nil {
		t.Fatalf("script bundled file %q missing from %#v", targetPath, factory.SupportingFiles)
	}
	if bundledFile.Type != factoryapi.BundledFileTypeSCRIPT {
		t.Fatalf("bundled file %q type = %q, want SCRIPT", targetPath, bundledFile.Type)
	}
	if bundledFile.Id == nil || *bundledFile.Id != targetPath {
		t.Fatalf("script bundled file %q id = %#v, want %q", targetPath, bundledFile.Id, targetPath)
	}
	if bundledFile.Content.Inline != wantInline {
		t.Fatalf("script bundled file %q inline = %q, want %q", targetPath, bundledFile.Content.Inline, wantInline)
	}
}
