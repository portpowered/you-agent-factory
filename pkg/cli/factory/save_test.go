package factory

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
)

func TestSaveFromFile_WritesHumanReadableConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	var out strings.Builder
	if err := SaveFromFile(SaveFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		Output: &out,
	}); err != nil {
		t.Fatalf("SaveFromFile: %v", err)
	}

	wantDir := filepath.Join(rootDir, "gamma")
	want := "Saved factory gamma\nDirectory: " + wantDir + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestSaveFromFile_JSONEmitsStructuredConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	var out bytes.Buffer
	if err := SaveFromFile(SaveFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		JSON:   true,
		Output: &out,
	}); err != nil {
		t.Fatalf("SaveFromFile: %v", err)
	}

	var result SaveFromFileResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result.Name != "gamma" || result.FactoryDir != filepath.Join(rootDir, "gamma") {
		t.Fatalf("result = %#v, want gamma factory directory", result)
	}
}

func TestSaveFromFile_RejectsDuplicateName(t *testing.T) {
	rootDir := t.TempDir()
	payload := saveTestNamedFactoryPayload(t, "alpha")
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", payload); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "alpha-copy", payload)

	err := SaveFromFile(SaveFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected duplicate factory name to fail")
	}
	if !strings.Contains(err.Error(), "factory already exists") {
		t.Fatalf("error = %v, want duplicate-name message", err)
	}
}

func TestSaveFromFile_RejectsInvalidName(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha"))

	err := SaveFromFile(SaveFromFileConfig{
		Name:   "../alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected invalid factory name to fail")
	}
	if !strings.Contains(err.Error(), "path separators") {
		t.Fatalf("error = %v, want invalid-name message", err)
	}
}

func TestSaveFromFile_RejectsInvalidPayload(t *testing.T) {
	rootDir := t.TempDir()
	from := filepath.Join(rootDir, "broken.json")
	if err := os.WriteFile(from, []byte(`{"id":"broken"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := SaveFromFile(SaveFromFileConfig{
		Name:   "broken",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected invalid factory payload to fail")
	}
	if !strings.Contains(err.Error(), "invalid factory config") {
		t.Fatalf("error = %v, want invalid-config message", err)
	}
}

func TestSaveFromFile_RejectsInvalidTopologyBeforePersist(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "invalid", []byte(factoryvalidation.CrossPathInvalidFactoryJSON))

	err := SaveFromFile(SaveFromFileConfig{
		Name:   "invalid",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected invalid factory topology to fail")
	}
	if !strings.Contains(err.Error(), "invalid factory config") {
		t.Fatalf("error = %v, want invalid-config message", err)
	}
	if _, statErr := os.Stat(filepath.Join(rootDir, "invalid")); !os.IsNotExist(statErr) {
		t.Fatalf("named factory directory should not be created, stat err = %v", statErr)
	}
}

func TestSaveFromFile_SetCurrentUpdatesPointer(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	if err := SaveFromFile(SaveFromFileConfig{
		Name:       "gamma",
		From:       from,
		Dir:        rootDir,
		SetCurrent: true,
		Output:     ioDiscard(t),
	}); err != nil {
		t.Fatalf("SaveFromFile: %v", err)
	}

	current, err := factoryconfig.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if current != "gamma" {
		t.Fatalf("current = %q, want gamma", current)
	}
}

func TestSaveFromFile_OmitsSetCurrentLeavesPointerUnchanged(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "beta", saveTestNamedFactoryPayload(t, "beta"))

	if err := SaveFromFile(SaveFromFileConfig{
		Name:   "beta",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("SaveFromFile: %v", err)
	}

	current, err := factoryconfig.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if current != "alpha" {
		t.Fatalf("current = %q, want alpha", current)
	}
}

func TestSaveFromFile_ListIncludesSavedFactory(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "gamma", saveTestNamedFactoryPayload(t, "gamma"))

	if err := SaveFromFile(SaveFromFileConfig{
		Name:   "gamma",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("SaveFromFile: %v", err)
	}

	var out strings.Builder
	if err := List(ListConfig{Dir: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(out.String(), "gamma\t"+filepath.Join(rootDir, "gamma")) {
		t.Fatalf("list output = %q, want saved gamma factory row", out.String())
	}
}

func writeFactoryConfigFile(t *testing.T, dir, stem string, payload []byte) string {
	t.Helper()

	path := filepath.Join(dir, stem+".json")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func saveTestNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	return listTestNamedFactoryPayload(t, project)
}

func ioDiscard(t *testing.T) *strings.Builder {
	t.Helper()
	return &strings.Builder{}
}
