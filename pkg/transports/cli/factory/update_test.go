package factory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestUpdateFromFile_WritesHumanReadableConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	var out strings.Builder
	if err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: &out,
	}, interfaces.NamedFactoryPersistenceResult{Name: "alpha", FactoryDir: filepath.Join(rootDir, "alpha")}, nil, nil); err != nil {
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
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	var out bytes.Buffer
	if err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		JSON:   true,
		Output: &out,
	}, interfaces.NamedFactoryPersistenceResult{Name: "alpha", FactoryDir: filepath.Join(rootDir, "alpha")}, nil, nil); err != nil {
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

	err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{}, fs.ErrNotExist, nil)

	if err == nil {
		t.Fatal("expected missing factory name to fail")
	}
	if !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf("error = %v, want not-found message", err)
	}
}

func TestUpdateFromFile_RejectsInvalidName(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha"))

	err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "../alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{}, fmt.Errorf("%w: path separators are not allowed", interfaces.ErrInvalidNamedFactoryName), nil)

	if err == nil {
		t.Fatal("expected invalid factory name to fail")
	}
	if !strings.Contains(err.Error(), "path separators") {
		t.Fatalf("error = %v, want invalid-name message", err)
	}
}

func TestUpdateFromFile_RejectsInvalidPayload(t *testing.T) {
	rootDir := t.TempDir()
	from := filepath.Join(rootDir, "broken.json")
	if err := os.WriteFile(from, []byte(`{"id":"broken"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{}, interfaces.ErrInvalidNamedFactory, nil)

	if err == nil {
		t.Fatal("expected invalid factory payload to fail")
	}
	if !strings.Contains(err.Error(), "invalid factory config") {
		t.Fatalf("error = %v, want invalid-config message", err)
	}
}

func TestUpdateFromFile_RejectsInvalidTopologyBeforePersist(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "invalid", saveTestNamedFactoryPayload(t, "invalid"))

	err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{}, &interfaces.BlockingFactoryLoadError{Targets: []interfaces.ValidationTarget{{
		Code:    interfaces.ValidationCodeFactoryPayloadInvalid,
		Message: "Factory topology contains invalid graph references.",
	}}}, nil)

	if err == nil {
		t.Fatal("expected invalid factory topology to fail")
	}
	if !strings.Contains(err.Error(), "invalid factory config") {
		t.Fatalf("error = %v, want invalid-config message", err)
	}

}

func TestUpdateFromFile_ReplacesExistingLayout(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	if err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{Name: "alpha", FactoryDir: filepath.Join(rootDir, "alpha")}, nil, func(request interfaces.NamedFactoryPersistenceRequest) {
		if request.Mode != interfaces.NamedFactoryPersistenceModeReplace || !strings.Contains(string(request.Payload), "execute-alpha-v2") {
			t.Fatalf("request = %#v, want replace payload", request)
		}
	}); err != nil {
		t.Fatalf("UpdateFromFile: %v", err)
	}
}

func TestUpdateFromFile_CurrentPointerUnchangedWhenReplacingCurrent(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "alpha-updated", saveTestNamedFactoryPayload(t, "alpha-v2"))

	if err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "alpha",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{Name: "alpha", FactoryDir: filepath.Join(rootDir, "alpha")}, nil, func(request interfaces.NamedFactoryPersistenceRequest) {
		if request.SetCurrent {
			t.Fatal("replace request SetCurrent = true, want false")
		}
	}); err != nil {
		t.Fatalf("UpdateFromFile: %v", err)
	}

	useNamedFactoryCatalogFake(t, namedFactoryCatalogFake{
		list: func(string) ([]interfaces.NamedFactoryListEntry, error) {
			return []interfaces.NamedFactoryListEntry{{
				Name:       "alpha",
				FactoryDir: filepath.Join(rootDir, "alpha"),
				Current:    true,
			}}, nil
		},
	})
	var out strings.Builder
	if err := testList(ListConfig{Dir: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}
	alphaDir := filepath.Join(rootDir, "alpha")
	wantRow := "alpha\t" + alphaDir + "\tyes\n"
	if !strings.Contains(out.String(), wantRow) {
		t.Fatalf("list output = %q, want current alpha row %q", out.String(), wantRow)
	}
}

func TestUpdateFromFile_ScopedCurrentPointerStillResolvesUpdatedFactoryInExplicitDir(t *testing.T) {
	rootDir := t.TempDir()
	from := writeFactoryConfigFile(t, rootDir, "tts-updated", saveTestNamedFactoryPayload(t, "tts-v2"))
	wantDir := filepath.Join(rootDir, "@you", "tts")

	if err := updateFromFileWithScriptedPersistence(t, UpdateFromFileConfig{
		Name:   "@you/tts",
		From:   from,
		Dir:    rootDir,
		Output: ioDiscard(t),
	}, interfaces.NamedFactoryPersistenceResult{Name: "@you/tts", FactoryDir: wantDir}, nil, func(request interfaces.NamedFactoryPersistenceRequest) {
		if request.RootDir != rootDir || request.Name != "@you/tts" || request.SetCurrent {
			t.Fatalf("request = %#v, want explicit scoped replace", request)
		}
	}); err != nil {
		t.Fatalf("UpdateFromFile(scoped): %v", err)
	}
}
