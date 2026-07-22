package scaffold

import (
	"bytes"
	"io/fs"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

func TestNewRequiresScaffoldEffects(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	if _, err := New(nil, output); err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("New(nil, output) error = %v, want required filesystem", err)
	}
	if _, err := New(scaffoldFileSystemStub{}, nil); err == nil || !strings.Contains(err.Error(), "output is required") {
		t.Fatalf("New(files, nil) error = %v, want required output", err)
	}
}

func TestInitializerFailsClosedWhenEffectsAreMissing(t *testing.T) {
	t.Parallel()

	var initializer *Initializer
	if err := initializer.Init(InitConfig{}); err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("nil Initializer.Init error = %v, want required filesystem", err)
	}

	initializer = &Initializer{files: scaffoldFileSystemStub{}}
	if err := initializer.Init(InitConfig{}); err == nil || !strings.Contains(err.Error(), "output is required") {
		t.Fatalf("Initializer without output error = %v, want required output", err)
	}
}

func TestInitializerUsesInjectedDefaultOutput(t *testing.T) {
	t.Parallel()

	output := &bytes.Buffer{}
	initializer, err := New(platformfilesystem.Local{}, output)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := initializer.Init(InitConfig{Dir: t.TempDir()}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !strings.Contains(output.String(), "Initialized default factory directory structure") {
		t.Fatalf("injected default output = %q, want initialization result", output.String())
	}
}

type scaffoldFileSystemStub struct{}

func (scaffoldFileSystemStub) Stat(string) (fs.FileInfo, error)   { return nil, fs.ErrNotExist }
func (scaffoldFileSystemStub) MkdirAll(string, fs.FileMode) error { return nil }
func (scaffoldFileSystemStub) WriteFile(string, []byte, fs.FileMode) error {
	return nil
}
