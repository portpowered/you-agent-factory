package factory_transformation

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

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

func docBundledFileEntry(targetPath, inline string) map[string]any {
	return map[string]any{
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
		"type":       "SCRIPT",
		"targetPath": targetPath,
		"content": map[string]string{
			"encoding": "utf-8",
			"inline":   inline,
		},
	}
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

func assertDocBundledFileInline(t *testing.T, factory factoryapi.Factory, targetPath, wantInline string) {
	t.Helper()
	bundledFile := findBundledFile(t, factory, targetPath)
	if bundledFile == nil {
		t.Fatalf("doc bundled file %q missing from %#v", targetPath, factory.SupportingFiles)
	}
	if bundledFile.Type != factoryapi.BundledFileTypeDOC {
		t.Fatalf("bundled file %q type = %q, want DOC", targetPath, bundledFile.Type)
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
	if bundledFile.Content.Inline != wantInline {
		t.Fatalf("script bundled file %q inline = %q, want %q", targetPath, bundledFile.Content.Inline, wantInline)
	}
}
