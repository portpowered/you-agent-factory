package factory_transformation

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCurrentFactoryPUT_NonDefaultSessionImportIsolatesDefaultFactoryAndMaterializesBundledFiles(t *testing.T) {
	rootDir := t.TempDir()
	seedDocumentNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	createNamedFactoryFixture(
		t,
		rootDir,
		"beta",
		functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task"),
	)

	server := startDocumentTransformationServer(t, rootDir, "alpha")
	betaServer := startDocumentTransformationServer(t, rootDir, "beta")

	defaultBefore := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	alphaConfigPath := filepath.Join(rootDir, "alpha", interfaces.FactoryConfigFile)
	alphaConfigBefore, err := os.ReadFile(alphaConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", alphaConfigPath, err)
	}

	sessionCurrent := getCurrentFactoryForSession(t, betaServer.URL(), betaServer.SessionID())
	if sessionCurrent.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("session current factory name = %q, want beta", sessionCurrent.Name)
	}
	importBody := nonDefaultSessionImportBodyWithBundledFiles(t, sessionCurrent, "imported-task")

	saved := saveCurrentFactoryForSession(t, betaServer.URL(), betaServer.SessionID(), importBody)
	if saved.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("saved session import factory name = %q, want beta", saved.Name)
	}
	assertFactoryWorkType(t, saved, "imported-task", "saved non-default session import")

	reloaded := getCurrentFactoryForSession(t, betaServer.URL(), betaServer.SessionID())
	if reloaded.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("reloaded session factory name = %q, want beta", reloaded.Name)
	}
	assertFactoryWorkType(t, reloaded, "imported-task", "session GET after non-default import")

	defaultAfter := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	if defaultAfter.Name != defaultBefore.Name {
		t.Fatalf("default current factory name = %q, want %q", defaultAfter.Name, defaultBefore.Name)
	}
	assertFactoryWorkType(t, defaultAfter, "alpha-task", "default session after non-default import")
	alphaConfigAfter, err := os.ReadFile(alphaConfigPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", alphaConfigPath, err)
	}
	if !bytes.Equal(alphaConfigBefore, alphaConfigAfter) {
		t.Fatalf("alpha on-disk factory config changed after non-default session import")
	}

	betaFactoryDir := filepath.Join(rootDir, "beta")
	assertPortableFile(t, filepath.Join(betaFactoryDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableFile(t, filepath.Join(betaFactoryDir, "docs", "README.md"), "# Session import factory\n")
	assertPortableFile(
		t,
		filepath.Join(betaFactoryDir, "scripts", "session-import.py"),
		"print('session import script')\n",
	)
	assertPersistedFactoryJSONStripsInlineBundledContent(t, filepath.Join(betaFactoryDir, interfaces.FactoryConfigFile))
	persistedBetaConfig, err := os.ReadFile(filepath.Join(betaFactoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", filepath.Join(betaFactoryDir, interfaces.FactoryConfigFile), err)
	}
	for _, forbidden := range []string{"# Session import factory", "print('session import script')"} {
		if bytes.Contains(persistedBetaConfig, []byte(forbidden)) {
			t.Fatalf("persisted beta factory still contains inline portable content %q", forbidden)
		}
	}

	alphaMakefile := filepath.Join(rootDir, "alpha", "Makefile")
	if _, err := os.Stat(alphaMakefile); err == nil {
		t.Fatalf("default session factory directory %q should not contain imported Makefile", alphaMakefile)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(%s): %v", alphaMakefile, err)
	}

	submitWorkForSessionAndExpectStatus(t, betaServer.URL(), betaServer.SessionID(), "imported-task", "session-import-submit", http.StatusCreated)
	submitWorkForSessionAndExpectStatus(t, server.URL(), server.SessionID(), "alpha-task", "default-still-alpha-after-import", http.StatusCreated)
}

func nonDefaultSessionImportBodyWithBundledFiles(
	t *testing.T,
	sessionCurrent factoryapi.Factory,
	workType string,
) string {
	t.Helper()
	if sessionCurrent.Version == nil {
		t.Fatal("session current factory version = nil, want version metadata for import")
	}

	body, err := json.Marshal(sessionCurrent)
	if err != nil {
		t.Fatalf("marshal session current factory document: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode session current factory document: %v", err)
	}

	document["version"] = versionDocument(advancedFactoryVersion(t, sessionCurrent.Version))
	document["workTypes"] = []map[string]any{{
		"name": workType,
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "done", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}}
	document["workers"] = []map[string]any{{
		"body":             "Plan imported work.",
		"executorProvider": "SCRIPT_WRAP",
		"model":            "claude-sonnet-4-20250514",
		"modelProvider":    "CLAUDE",
		"name":             "planner",
		"type":             "MODEL_WORKER",
	}}
	document["workstations"] = []map[string]any{{
		"behavior": "STANDARD",
		"body":     "Plan the imported work.",
		"inputs":   []map[string]string{{"state": "init", "workType": workType}},
		"name":     "plan-task",
		"outputs":  []map[string]string{{"state": "done", "workType": workType}},
		"type":     "MODEL_WORKSTATION",
		"worker":   "planner",
	}}
	document["supportingFiles"] = map[string]any{
		"bundledFiles": []map[string]any{
			{
				"type":       "ROOT_HELPER",
				"targetPath": "Makefile",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "test:\n\tgo test ./...\n",
				},
			},
			{
				"type":       "DOC",
				"targetPath": "factory/docs/README.md",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "# Session import factory\n",
				},
			},
			{
				"type":       "SCRIPT",
				"targetPath": "factory/scripts/session-import.py",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "print('session import script')\n",
				},
			},
		},
	}

	body, err = json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal non-default session import document: %v", err)
	}
	return string(body)
}
