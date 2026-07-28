package definitions

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryTransformation_CreateNamedFactoryReadbackAndWorkSurface(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server, runner := startImportExportNamedFactoryServer(t, rootDir)
	defer server.Stop(t)

	created := createNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))
	assertFactoryWorkType(t, created, "beta-task", "created factory")
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}

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
	if legacyErr.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("alpha-task error code = %q, want BAD_REQUEST", legacyErr.Code)
	}

	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1 for accepted beta-task submission", runner.CallCount())
	}
}

func TestFactoryTransformation_NamedFactoryPortableFilesReadBackThroughCanonicalContract(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server, runner := startImportExportNamedFactoryServer(t, rootDir)
	defer server.Stop(t)

	created := createNamedFactoryFromBody(
		t,
		server.URL(),
		functionalNamedFactoryBodyWithBundledFiles("beta", "beta-task"),
	)
	assertBundledFilesWithoutInlineScriptsAndDocs(t, created, "created response")

	current := getCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
	assertFactoryWorkType(t, current, "beta-task", "current factory readback")
	assertBundledFilesWithInlineScriptsAndDocs(t, current, "current response")

	importedDir := filepath.Join(rootDir, "beta")
	assertPortableFile(t, filepath.Join(importedDir, "Makefile"), importExportNamedMakefileBody)
	assertPortableFile(t, filepath.Join(importedDir, "docs", "README.md"), importExportNamedPortableDocBody)
	assertPortableFile(t, filepath.Join(importedDir, "scripts", "execute-story.ps1"), importExportNamedPortableScriptBody)
	assertPersistedFactoryJSONStripsInlineBundledContent(t, filepath.Join(importedDir, interfaces.FactoryConfigFile))

	assertImportExportNamedFactoryRunnerIdle(t, runner)
}

func TestFactoryTransformation_CreateNamedFactoryEmitsCanonicalFactoryChangeEvent(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server, runner := startImportExportNamedFactoryServer(t, rootDir)
	defer server.Stop(t)
	initialEvents := server.GetFactoryEvents(t)

	createNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))

	change := requireFactoryChangeAfter(t, initialEvents, server.GetFactoryEvents(t))
	payload, err := change.Payload.AsFactoryChangeEventPayload()
	if err != nil {
		t.Fatalf("decode factory-change payload: %v", err)
	}
	if payload.Factory.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("factory-change payload name = %q, want beta", payload.Factory.Name)
	}
	assertFactoryWorkType(t, payload.Factory, "beta-task", "factory-change payload")

	assertImportExportNamedFactoryRunnerIdle(t, runner)
}

func TestFactoryTransformation_CreateNamedFactoryPreservesPortableLayoutThroughActivationAndReadback(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server, runner := startImportExportNamedFactoryServer(t, rootDir)
	defer server.Stop(t)

	created := createNamedFactoryFromBody(
		t,
		server.URL(),
		functionalNamedFactoryBodyWithPortableLayout("beta", "beta-task"),
	)
	assertNamedFactoryPortableLayoutResponse(t, created.Layout, "workstation:plan-task", "beta-task")

	current := getCurrentFactory(t, server.URL())
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
	assertNamedFactoryPortableLayoutResponse(t, current.Layout, "workstation:plan-task", "beta-task")

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, "beta", interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(beta/factory.json): %v", err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(factoryJSON, &persisted); err != nil {
		t.Fatalf("Unmarshal(beta/factory.json): %v", err)
	}
	assertPortableLayoutPayload(t, persisted["layout"])

	submitWorkAndExpectStatus(t, server.URL(), "beta-task", "layout-named-factory", http.StatusCreated)
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1 for accepted layout-named-factory submission", runner.CallCount())
	}
}

func TestFactoryTransformation_UpsertNamedFactoryReplacePreservesPortableLayout(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server, runner := startImportExportNamedFactoryServer(t, rootDir)
	defer server.Stop(t)

	created := createNamedFactoryFromBody(
		t,
		server.URL(),
		functionalNamedFactoryBodyWithPortableLayout("beta", "beta-task"),
	)
	if created.Version == nil {
		t.Fatal("created factory version = nil, want version metadata")
	}

	freshVersion := freshNamedFactoryVersion(created)
	replaced := upsertNamedFactoryFromBody(
		t,
		server.URL(),
		currentFactorySaveDocumentWithPortableLayout(t, "beta", "beta-task", versionDocument(freshVersion)),
	)
	assertNamedFactoryPortableLayoutResponse(t, replaced.Layout, "workstation:plan-task", "beta-task")

	current := getCurrentFactory(t, server.URL())
	assertNamedFactoryPortableLayoutResponse(t, current.Layout, "workstation:plan-task", "beta-task")

	assertImportExportNamedFactoryRunnerIdle(t, runner)
}

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

	replaceServer, replaceRunner := startImportExportNamedFactoryServer(t, replaceRoot)
	defer replaceServer.Stop(t)
	namedServer, namedRunner := startImportExportNamedFactoryServer(t, namedRoot)
	defer namedServer.Stop(t)

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

	assertImportExportNamedFactoryRunnerIdle(t, replaceRunner)
	assertImportExportNamedFactoryRunnerIdle(t, namedRunner)
}
