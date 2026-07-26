package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type listEntry struct {
	Name             string `json:"name"`
	FactoryDirectory string `json:"factoryDirectory"`
	Description      string `json:"description"`
}

func TestFactoryListProjectsEffectiveCatalogWithoutInitialization(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	projectRoot := filepath.Join(workingDirectory, "factory")
	globalRoot := filepath.Join(home, ".you-agent-factory", "factories")
	writeFactory(t, projectRoot, "local", "local description")
	writeFactory(t, projectRoot, "shared", "project override")
	writeFactory(t, globalRoot, "global", "global description")
	writeFactory(t, globalRoot, "shared", "shadowed global")
	writeFactory(t, globalRoot, "unreadable", "must not be read")
	writePayload(t, globalRoot, "malformed", []byte(`{"secret":"do-not-expose"`))

	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "factory", "list"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := support.BuildProcess(t, serviceedges.Edges{
		FactoryDefinitionAuthoredReaderFileSystem: failingDefinitionReader{
			Local:       platformfilesystem.Local{},
			failingPath: filepath.Join(globalRoot, "unreadable", "factory.json"),
		},
	}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(factory list) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}

	var entries []listEntry
	if err := json.Unmarshal([]byte(inputs.Stdout()), &entries); err != nil {
		t.Fatalf("decode factory list: %v\n%s", err, inputs.Stdout())
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name
	}
	if !slices.IsSorted(names) || slices.Contains(names, "malformed") ||
		slices.Contains(names, "unreadable") ||
		strings.Count(strings.Join(names, "\n"), "shared") != 1 {
		t.Fatalf("effective names = %v", names)
	}
	assertEntry(t, entries, "local", filepath.Join(projectRoot, "local"), "local description")
	assertEntry(t, entries, "global", filepath.Join(globalRoot, "global"), "global description")
	assertEntry(t, entries, "shared", filepath.Join(projectRoot, "shared"), "project override")
	if !slices.ContainsFunc(entries, func(entry listEntry) bool {
		return strings.HasPrefix(entry.Name, "@you/") && entry.FactoryDirectory == "-"
	}) {
		t.Fatalf("entries = %#v, want an unmaterialized packaged Factory", entries)
	}
	if diagnostics := inputs.Stderr(); !strings.Contains(diagnostics, "malformed") ||
		!strings.Contains(diagnostics, "unreadable") ||
		strings.Contains(diagnostics, "do-not-expose") {
		t.Fatalf("diagnostics = %q", diagnostics)
	}
	if _, err := os.Stat(filepath.Join(home, ".you-agent-factory", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("factory list initialized customer home: %v", err)
	}
}

type failingDefinitionReader struct {
	platformfilesystem.Local
	failingPath string
}

func (f failingDefinitionReader) ReadFile(path string) ([]byte, error) {
	if path == f.failingPath {
		return nil, errors.New("definition unavailable")
	}
	return f.Local.ReadFile(path)
}

type failingCatalogFileSystem struct {
	platformfilesystem.Local
	failingRoot string
	err         error
}

func (f failingCatalogFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	if path == f.failingRoot {
		return nil, f.err
	}
	return f.Local.ReadDir(path)
}

func TestFactoryListReportsCatalogDiscoveryFailuresAtomically(t *testing.T) {
	sourceErr := errors.New("catalog unavailable")
	for _, test := range []struct {
		name        string
		failingTier string
		want        string
	}{
		{name: "project", failingTier: "project", want: "discover project-local Factories"},
		{name: "global", failingTier: "global", want: "discover global Factories"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			workingDirectory := t.TempDir()
			projectRoot := filepath.Join(workingDirectory, "factory")
			globalRoot := filepath.Join(home, ".you-agent-factory", "factories")
			if err := os.MkdirAll(projectRoot, 0o755); err != nil {
				t.Fatalf("create project root: %v", err)
			}
			if err := os.MkdirAll(globalRoot, 0o755); err != nil {
				t.Fatalf("create global root: %v", err)
			}
			failingRoot := projectRoot
			if test.failingTier == "global" {
				failingRoot = globalRoot
			}
			inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "factory", "list"})
			inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.Input.WorkingDirectory = workingDirectory
			err := support.BuildProcess(t, serviceedges.Edges{
				FactoryDefinitionNamedFactoryCatalogFileSystem: failingCatalogFileSystem{
					Local: platformfilesystem.Local{}, failingRoot: failingRoot, err: sourceErr,
				},
			}).Execute(inputs.Input)
			if err == nil || !strings.Contains(err.Error(), test.want) || !errors.Is(err, sourceErr) {
				t.Fatalf("Process.Execute(factory list) error = %v, want %q wrapping source failure", err, test.want)
			}
			if inputs.Stdout() != "" || !strings.Contains(inputs.Stderr(), test.want) {
				t.Fatalf("failed listing output: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
			}
		})
	}
}

func TestFactoryListHonorsPreCanceledContextAtomically(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	home := t.TempDir()
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "factory", "list"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = t.TempDir()
	err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Process.Execute(factory list) error = %v, want context canceled", err)
	}
	if inputs.Stdout() != "" || !strings.Contains(inputs.Stderr(), context.Canceled.Error()) {
		t.Fatalf("canceled listing output: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
}

func writeFactory(t *testing.T, root, name, description string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"name": name,
		"id":   name,
		"description": map[string]any{
			"type": "LOCALIZABLE_ASSET", "value": description,
		},
		"workTypes": []any{}, "resources": []any{}, "workers": []any{}, "workstations": []any{},
	})
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	writePayload(t, root, name, payload)
}

func writePayload(t *testing.T, root, name string, payload []byte) {
	t.Helper()
	factoryDirectory := filepath.Join(root, name)
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDirectory, "factory.json"), payload, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func assertEntry(t *testing.T, entries []listEntry, name, directory, description string) {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == name {
			if entry.FactoryDirectory != directory || entry.Description != description {
				t.Fatalf("entry %s = %#v", name, entry)
			}
			return
		}
	}
	t.Fatalf("entry %s missing from %#v", name, entries)
}
