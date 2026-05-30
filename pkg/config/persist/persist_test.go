package persist_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPersistNamedFactory_MatchesConfigPackage(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")

	facadeDir, err := persist.PersistNamedFactory(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("persist.PersistNamedFactory: %v", err)
	}
	configDir, err := config.PersistNamedFactory(rootDir, "beta", payload)
	if err != nil {
		t.Fatalf("config.PersistNamedFactory: %v", err)
	}

	if facadeDir != filepath.Join(rootDir, "alpha") {
		t.Fatalf("facade factory dir = %q, want %q", facadeDir, filepath.Join(rootDir, "alpha"))
	}
	if configDir != filepath.Join(rootDir, "beta") {
		t.Fatalf("config factory dir = %q, want %q", configDir, filepath.Join(rootDir, "beta"))
	}
}

func TestReplaceNamedFactory_MatchesConfigPackage(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")

	if _, err := persist.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("persist.PersistNamedFactory: %v", err)
	}

	facadeDir, err := persist.ReplaceNamedFactory(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("persist.ReplaceNamedFactory: %v", err)
	}
	configDir, err := config.ReplaceNamedFactory(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("config.ReplaceNamedFactory: %v", err)
	}
	if facadeDir != configDir {
		t.Fatalf("factory dir = %q vs %q", facadeDir, configDir)
	}
}

func TestPersistNamedFactoryWithReport_MatchesConfigPackage(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayloadWithBundledFiles(t, "alpha")

	facadeResult, err := persist.PersistNamedFactoryWithReport(rootDir, "alpha", payload)
	if err != nil {
		t.Fatalf("persist.PersistNamedFactoryWithReport: %v", err)
	}

	rootDir2 := t.TempDir()
	configResult, err := config.PersistNamedFactoryWithReport(rootDir2, "alpha", payload)
	if err != nil {
		t.Fatalf("config.PersistNamedFactoryWithReport: %v", err)
	}

	if facadeResult.FactoryDir != filepath.Join(rootDir, "alpha") {
		t.Fatalf("facade factory dir = %q, want %q", facadeResult.FactoryDir, filepath.Join(rootDir, "alpha"))
	}
	if configResult.FactoryDir != filepath.Join(rootDir2, "alpha") {
		t.Fatalf("config factory dir = %q, want %q", configResult.FactoryDir, filepath.Join(rootDir2, "alpha"))
	}
	if _, err := os.Stat(filepath.Join(facadeResult.FactoryDir, "Makefile")); err != nil {
		t.Fatalf("expected bundled Makefile on disk: %v", err)
	}
}

func TestReadWriteCurrentFactoryPointer_RoundTrip(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")

	if _, err := persist.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("persist.PersistNamedFactory: %v", err)
	}
	if err := persist.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("persist.WriteCurrentFactoryPointer: %v", err)
	}

	got, err := persist.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("persist.ReadCurrentFactoryPointer: %v", err)
	}
	if got != "alpha" {
		t.Fatalf("current factory = %q, want alpha", got)
	}
}

func TestReplaceDefaultFactoryDefinition_StagesAndRestores(t *testing.T) {
	rootDir := t.TempDir()
	initial := namedFactoryPayload(t, "legacy")
	updated := namedFactoryPayload(t, "legacy-v2")

	if err := os.WriteFile(filepath.Join(rootDir, interfaces.FactoryConfigFile), initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	restore, err := persist.ReplaceDefaultFactoryDefinition(rootDir, updated)
	if err != nil {
		t.Fatalf("persist.ReplaceDefaultFactoryDefinition: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if string(got) != string(updated) {
		t.Fatalf("factory.json after replace = %q, want updated payload", string(got))
	}

	restore()

	got, err = os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json) after restore: %v", err)
	}
	if string(got) != string(initial) {
		t.Fatalf("factory.json after restore = %q, want initial payload", string(got))
	}
}

func TestIsInvalidNamedFactory_DetectsPersistValidationFailure(t *testing.T) {
	rootDir := t.TempDir()

	_, err := persist.PersistNamedFactory(rootDir, "broken", []byte(`{"name":"broken"`))
	if err == nil {
		t.Fatal("expected invalid named factory payload to fail")
	}
	if !persist.IsInvalidNamedFactory(err) {
		t.Fatalf("error = %v, want ErrInvalidNamedFactory", err)
	}
}

func TestIsNamedFactoryAlreadyExists_DetectsDuplicatePersist(t *testing.T) {
	rootDir := t.TempDir()
	payload := namedFactoryPayload(t, "alpha")

	if _, err := persist.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("first persist.PersistNamedFactory: %v", err)
	}
	_, err := persist.PersistNamedFactory(rootDir, "alpha", payload)
	if err == nil {
		t.Fatal("expected duplicate named factory to fail")
	}
	if !persist.IsNamedFactoryAlreadyExists(err) {
		t.Fatalf("error = %v, want ErrNamedFactoryAlreadyExists", err)
	}
}

func namedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()

	cfg := map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":    "execute-" + project,
				"worker":  "executor",
				"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
				"type":    "MODEL_WORKSTATION",
				"body":    "Implement {{ .WorkID }}.",
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayload): %v", err)
	}
	return data
}

func namedFactoryPayloadWithBundledFiles(t *testing.T, project string) []byte {
	t.Helper()

	payload := namedFactoryPayload(t, project)
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal(namedFactoryPayload): %v", err)
	}
	decoded["supportingFiles"] = map[string]any{
		"bundledFiles": []map[string]any{
			{
				"type":       "ROOT_HELPER",
				"targetPath": "Makefile",
				"content": map[string]string{
					"encoding": "utf-8",
					"inline":   "test:\n\tgo test ./...\n",
				},
			},
		},
	}
	updated, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal(namedFactoryPayloadWithBundledFiles): %v", err)
	}
	return updated
}

func TestValidateNamedFactoryName_RejectsPathTraversal(t *testing.T) {
	err := persist.ValidateNamedFactoryName("../beta")
	if err == nil {
		t.Fatal("expected invalid named-factory segment to fail")
	}
	if got := err.Error(); !strings.Contains(got, "cannot contain path separators") {
		t.Fatalf("expected path-separator validation error, got %v", err)
	}
}
