package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedFactoriesAPIExposesDiscoverableEntries proves customers receive
// a non-empty catalog and representative packaged entries contain the fields
// needed to understand and invoke them. Whole-source parity, exact counts, and
// per-entry shape are static publication checks rather than functional cases.
func TestPackagedFactoriesAPIExposesDiscoverableEntries(t *testing.T) {
	catalog, err := discoveredPackagedFactoryCatalogViaHTTP(t)
	if err != nil {
		t.Fatalf("GET /packaged-factories: %v", err)
	}
	if len(catalog.Factories) == 0 {
		t.Fatal("GET /packaged-factories returned no published factories")
	}
	for _, name := range []string{"@you/fix", "@you/ralph"} {
		factory, ok := slices.BinarySearchFunc(catalog.Factories, name, func(factory factoryapi.PackagedFactoryCatalogEntry, target string) int {
			return strings.Compare(factory.Name, target)
		})
		if !ok {
			t.Fatalf("GET /packaged-factories is missing %s", name)
		}
		entry := catalog.Factories[factory]
		if strings.TrimSpace(entry.Description.Value) == "" || len(entry.Examples) == 0 ||
			entry.Name == "" || entry.Project == "" || entry.Slug == "" || len(entry.Json) == 0 || entry.Yaml == "" {
			t.Fatalf("GET /packaged-factories entry %s is not customer-discoverable: %#v", name, entry)
		}
	}
}

type listEntry struct {
	Name             string `json:"name"`
	FactoryDirectory string `json:"factoryDirectory"`
	Description      string `json:"description"`
}

// TestFactoryListProjectsEffectiveCatalogWithoutInitialization proves listing
// reads the effective catalog without mutating Factory installation state.
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
	fixture := sharedCatalogProcess(t)
	fixture.authored.setDelegate(failingDefinitionReader{
		Local:       platformfilesystem.Local{},
		failingPath: filepath.Join(globalRoot, "unreadable", "factory.json"),
	})
	t.Cleanup(func() { fixture.authored.setDelegate(platformfilesystem.Local{}) })
	if err := fixture.process.Execute(inputs.Input); err != nil {
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

// TestFactoryListReportsCatalogDiscoveryFailuresAtomically proves discovery
// failure produces no partial customer catalog result.
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
			fixture := sharedCatalogProcess(t)
			fixture.namedCatalog.setDelegate(failingCatalogFileSystem{
				Local: platformfilesystem.Local{}, failingRoot: failingRoot, err: sourceErr,
			})
			t.Cleanup(func() { fixture.namedCatalog.setDelegate(platformfilesystem.Local{}) })
			err := fixture.process.Execute(inputs.Input)
			if err == nil || !strings.Contains(err.Error(), test.want) || !errors.Is(err, sourceErr) {
				t.Fatalf("Process.Execute(factory list) error = %v, want %q wrapping source failure", err, test.want)
			}
			if inputs.Stdout() != "" {
				t.Fatalf("failed listing output wrote stdout=%q", inputs.Stdout())
			}
			support.RequireSafeCLIDiagnostic(t, inputs.Stderr())
		})
	}
}

// TestFactoryListHonorsPreCanceledContextAtomically proves pre-cancellation
// prevents catalog work and partial customer output.
func TestFactoryListHonorsPreCanceledContextAtomically(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	home := t.TempDir()
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "factory", "list"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = t.TempDir()
	err := sharedCatalogProcess(t).process.Execute(inputs.Input)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Process.Execute(factory list) error = %v, want context canceled", err)
	}
	if inputs.Stdout() != "" || !strings.Contains(inputs.Stderr(), context.Canceled.Error()) {
		t.Fatalf("canceled listing output: stdout=%q stderr=%q", inputs.Stdout(), inputs.Stderr())
	}
}

// sharedPackagedFactoryCatalog caches one local service-mode HTTP catalog
// response for all compatible API-owned catalog observations in this package.
// The server, root process, and temporary directories live entirely inside a
// single fetch; only the immutable decoded response is reused across tests.
var (
	sharedPackagedFactoryCatalogOnce sync.Once
	sharedPackagedFactoryCatalog     factoryapi.PackagedFactoryCatalogResponse
	sharedPackagedFactoryCatalogErr  error
)

func discoveredPackagedFactoryCatalogViaHTTP(t *testing.T) (factoryapi.PackagedFactoryCatalogResponse, error) {
	t.Helper()

	sharedPackagedFactoryCatalogOnce.Do(func() {
		sharedPackagedFactoryCatalog, sharedPackagedFactoryCatalogErr = fetchPackagedFactoryCatalogViaHTTP(t)
	})
	if sharedPackagedFactoryCatalogErr != nil {
		return factoryapi.PackagedFactoryCatalogResponse{}, sharedPackagedFactoryCatalogErr
	}
	return sharedPackagedFactoryCatalog, nil
}

func fetchPackagedFactoryCatalogViaHTTP(t *testing.T) (factoryapi.PackagedFactoryCatalogResponse, error) {
	t.Helper()

	dir := support.ScaffoldFactory(t, packagedFactoryCatalogTestConfig())
	fixture := sharedCatalogProcess(t)
	server := support.NewProcessAPIServer()
	fixture.apiRouter.set(server)
	inputs := support.FakeInputs(context.Background(), []string{
		"you", "run",
		"--continuously",
		"--with-server",
		"--quiet",
		"--dir", dir,
		"--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+t.TempDir(), "USERPROFILE="+t.TempDir())
	inputs.Input.WorkingDirectory = dir
	command := support.StartProcessCommand(t, fixture.process, inputs.Input)
	baseURL, err := server.WaitForBaseURL(15 * time.Second)
	if err != nil {
		return factoryapi.PackagedFactoryCatalogResponse{}, err
	}
	support.WaitForStatus(t, baseURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return strings.TrimSpace(status.RuntimeStatus) != ""
	})

	response, err := http.Get(baseURL + "/packaged-factories")
	if err != nil {
		return factoryapi.PackagedFactoryCatalogResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return factoryapi.PackagedFactoryCatalogResponse{}, fmt.Errorf(
			"GET /packaged-factories status = %d, want %d",
			response.StatusCode,
			http.StatusOK,
		)
	}

	var catalog factoryapi.PackagedFactoryCatalogResponse
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil {
		return factoryapi.PackagedFactoryCatalogResponse{}, err
	}
	for _, factory := range catalog.Factories {
		if strings.TrimSpace(factory.Name) == "" {
			return factoryapi.PackagedFactoryCatalogResponse{}, fmt.Errorf("catalog entry missing name: %#v", factory)
		}
	}
	command.Stop(t)
	return catalog, nil
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

func packagedFactoryCatalogTestConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}
