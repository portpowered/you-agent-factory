package defaultscaffold

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestScaffoldInitializerMaterializesDefaultLayoutAndPreservesAuthoredFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, factorydefinitions.FactoryConfigFile)
	const authored = `{"name":"authored"}`
	if err := os.WriteFile(path, []byte(authored), 0o644); err != nil {
		t.Fatalf("write authored file: %v", err)
	}
	output := &bytes.Buffer{}
	initialize, err := NewScaffoldInitializer(platformfilesystem.Local{}, output)
	if err != nil {
		t.Fatalf("NewScaffoldInitializer: %v", err)
	}
	if err := initialize(factorydefinitions.ScaffoldConfig{Dir: dir}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	for _, relativePath := range []string{
		filepath.Join("workers", "processor", "AGENTS.md"),
		filepath.Join("workstations", "process", "AGENTS.md"),
		filepath.Join(
			"inputs",
			factorydefinitions.DefaultFactoryInputType,
			factorydefinitions.DefaultChannelName,
		),
	} {
		if _, err := os.Stat(filepath.Join(dir, relativePath)); err != nil {
			t.Fatalf("scaffold path %q: %v", relativePath, err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read authored file: %v", err)
	}
	if string(got) != authored {
		t.Fatalf("factory.json = %q, want preserved %q", got, authored)
	}
	if !strings.Contains(output.String(), "Initialized default factory directory structure") {
		t.Fatalf("output = %q, want initialization result", output)
	}
}

func TestNewScaffoldInitializerRequiresEffects(t *testing.T) {
	t.Parallel()

	if _, err := NewScaffoldInitializer(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("NewScaffoldInitializer(nil, output) error = nil")
	}
	if _, err := NewScaffoldInitializer(platformfilesystem.Local{}, nil); err == nil {
		t.Fatal("NewScaffoldInitializer(files, nil) error = nil")
	}
}
