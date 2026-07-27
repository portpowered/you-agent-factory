package service_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/internal/service"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

const fixturesRelativeDir = "pkg/services/operator_settings/testdata/fixtures"

func TestLoadDocument_MissingDestinationReturnsEmptyValidDocument(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing-config.json")
	service := newDocumentLoadService(t, map[string][]byte{})

	loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	if loaded.Found {
		t.Fatalf("Found = true, want false for missing destination")
	}
	if loaded.Path != path {
		t.Fatalf("Path = %q, want %q", loaded.Path, path)
	}
	want := operatorsettings.EmptyDocument()
	if !reflect.DeepEqual(loaded.Document, want) {
		t.Fatalf("Document = %#v, want empty valid document %#v", loaded.Document, want)
	}
}

func TestLoadDocument_RequireExistingMissingFailsWithNotFound(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing-config.json")
	service := newDocumentLoadService(t, map[string][]byte{})

	_, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{
		Path:            path,
		RequireExisting: true,
	})
	if !errors.Is(err, operatorsettings.ErrDocumentNotFound) {
		t.Fatalf("LoadDocument() = %v, want ErrDocumentNotFound", err)
	}
}

func TestLoadDocument_MalformedBytesFailClosedWithoutPartialDocument(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		data string
	}{
		{name: "malformed-json", data: `{"defaults":`},
		{name: "trailing-json", data: `{} {}`},
		{name: "unknown-top-level", data: `{"unexpected":true}`},
		{name: "null-document", data: `null`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.json")
			service := newDocumentLoadService(t, map[string][]byte{
				path: []byte(test.data),
			})

			loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
			if !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
				t.Fatalf("LoadDocument() = %v, want ErrDocumentMalformed", err)
			}
			if loaded.Found || loaded.Path != "" || !reflect.DeepEqual(loaded.Document, operatorsettings.Document{}) {
				t.Fatalf("LoadDocument() = %#v, want zero result on malformed load", loaded)
			}
		})
	}
}

func TestLoadDocument_ValidOnDiskDocumentReturnsCompleteValidatedDocument(t *testing.T) {
	t.Parallel()

	path := writeFixtureToTemp(t, "valid/load-defaults.json")
	service := newDocumentLoadService(t, map[string][]byte{
		path: readFixture(t, "valid/load-defaults.json"),
	})

	loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	if !loaded.Found {
		t.Fatal("Found = false, want true for existing valid document")
	}
	if loaded.Path != path {
		t.Fatalf("Path = %q, want %q", loaded.Path, path)
	}
	if loaded.Document.Defaults != (operatorsettings.DocumentDefaults{
		WorkerModelProvider: "claude",
		WorkerModel:         "claude-sonnet",
	}) {
		t.Fatalf("Defaults = %#v, want validated load-defaults values", loaded.Document.Defaults)
	}
	if loaded.Document.Runtime != operatorsettings.EmptyDocument().Runtime {
		t.Fatalf("Runtime = %#v, want production defaults", loaded.Document.Runtime)
	}
}

func TestLoadDocument_ValidDocumentIncludesBackendScopeAndPresets(t *testing.T) {
	t.Parallel()

	path := writeFixtureToTemp(t, "valid/backend-scope-sibling.json")
	service := newDocumentLoadService(t, map[string][]byte{
		path: readFixture(t, "valid/backend-scope-sibling.json"),
	})

	loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		t.Fatalf("LoadDocument() = %v", err)
	}
	if loaded.Document.BackendScopeID != "local-11111111-1111-4111-8111-111111111111" {
		t.Fatalf("BackendScopeID = %q, want persisted scope id", loaded.Document.BackendScopeID)
	}
}

func newDocumentLoadService(t *testing.T, files map[string][]byte) *internalservice.Service {
	t.Helper()

	return internalservice.New(
		&mapFileSystem{files: files},
		func(string, string) (operatorsettings.TemporaryFile, error) {
			t.Fatal("temp-file creation is unexpected during load")
			return nil, errors.New("unexpected temp-file creation")
		},
		globalconfigmapping.Decode,
		globalconfigmapping.Encode,
		func(string) (string, bool) {
			t.Fatal("provider catalog is unexpected during load")
			return "", false
		},
	)
}

type mapFileSystem struct {
	files map[string][]byte
}

func (filesystem *mapFileSystem) ReadFile(path string) ([]byte, error) {
	data, ok := filesystem.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return data, nil
}

func (filesystem *mapFileSystem) MkdirAll(string, fs.FileMode) error {
	panic("unexpected mkdir during load")
}

func (filesystem *mapFileSystem) Remove(string) error {
	panic("unexpected remove during load")
}

func (filesystem *mapFileSystem) Chmod(string, fs.FileMode) error {
	panic("unexpected chmod during load")
}

func (filesystem *mapFileSystem) Rename(string, string) error {
	panic("unexpected rename during load")
}

func readFixture(t *testing.T, relativePath string) []byte {
	t.Helper()

	path := testutil.MustRepoPath(t, filepath.ToSlash(filepath.Join(fixturesRelativeDir, relativePath)))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v", path, err)
	}
	return data
}

func writeFixtureToTemp(t *testing.T, relativePath string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), filepath.Base(relativePath))
	if err := os.WriteFile(path, readFixture(t, relativePath), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) = %v", path, err)
	}
	return path
}

func TestLoadDocument_MalformedFixtureFromInventoryFailsClosed(t *testing.T) {
	t.Parallel()

	path := writeFixtureToTemp(t, "invalid/load-malformed.json")
	service := newDocumentLoadService(t, map[string][]byte{
		path: readFixture(t, "invalid/load-malformed.json"),
	})

	_, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if !errors.Is(err, operatorsettings.ErrDocumentMalformed) {
		t.Fatalf("LoadDocument() = %v, want ErrDocumentMalformed", err)
	}
	if failure, ok := err.(operatorsettings.DocumentFailure); !ok || !strings.Contains(failure.Path, filepath.Base(path)) {
		t.Fatalf("LoadDocument() = %v, want malformed failure naming path", err)
	}
}
