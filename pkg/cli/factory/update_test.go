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
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestUpdateFromFile_WritesHumanReadableConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	var out strings.Builder
	if err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: &out,
	}); err != nil {
		t.Fatalf("UpdateFromFile: %v", err)
	}

	wantDir := filepath.Join(rootDir, "alpha")
	want := "Updated factory alpha\nDirectory: " + wantDir + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestUpdateFromFile_JSONEmitsStructuredConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	var out bytes.Buffer
	if err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		JSON:   true,
		Output: &out,
	}); err != nil {
		t.Fatalf("UpdateFromFile: %v", err)
	}

	var result UpdateFromFileResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result.Name != "alpha" || result.FactoryDir != filepath.Join(rootDir, "alpha") {
		t.Fatalf("result = %#v, want alpha factory directory", result)
	}
}

func TestUpdateFromFile_RejectsMissingName(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha"))

	err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected missing factory name to fail")
	}
	if !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf("error = %v, want not-found message", err)
	}
}

func TestUpdateFromFile_RejectsInvalidName(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha"))

	err := UpdateFromFile(UpdateFromFileConfig{
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

func TestUpdateFromFile_RejectsInvalidPayload(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := filepath.Join(rootDir, "broken.json")
	if err := os.WriteFile(from, []byte(`{"id":"broken"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
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

func TestUpdateFromFile_RejectsInvalidTopologyBeforePersist(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "invalid", []byte(factoryvalidation.CrossPathInvalidFactoryJSON))

	err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
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

	configPath := filepath.Join(rootDir, "alpha", interfaces.FactoryConfigFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}
	if !strings.Contains(string(data), "execute-alpha") {
		t.Fatalf("factory config = %q, want original alpha workstation body preserved", string(data))
	}
}

func TestUpdateFromFile_ReplacesExistingLayout(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	if err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("UpdateFromFile: %v", err)
	}

	configPath := filepath.Join(rootDir, "alpha", interfaces.FactoryConfigFile)
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", configPath, err)
	}
	if !strings.Contains(string(data), "execute-alpha-v2") {
		t.Fatalf("factory config = %q, want updated workstation body", string(data))
	}
}

func TestUpdateFromFile_CurrentPointerUnchangedWhenReplacingCurrent(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	if err := UpdateFromFile(UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("UpdateFromFile: %v", err)
	}

	current, err := factoryconfig.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if current != "alpha" {
		t.Fatalf("current = %q, want alpha", current)
	}

	var out strings.Builder
	if err := List(ListConfig{Dir: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}
	alphaDir := filepath.Join(rootDir, "alpha")
	wantRow := "alpha\t" + alphaDir + "\tyes\n"
	if !strings.Contains(out.String(), wantRow) {
		t.Fatalf("list output = %q, want current alpha row %q", out.String(), wantRow)
	}
}
