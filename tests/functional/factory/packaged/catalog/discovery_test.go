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

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedFactoryCatalogListsEveryEmbeddedFactory proves runtime packaged
// Factory catalog discovery through GET /packaged-factories returns every
// Factory in the embedded authored inventory and does not invent names absent
// from that inventory.
func TestPackagedFactoryCatalogListsEveryEmbeddedFactory(t *testing.T) {

	embeddedNames, err := embeddedPackagedFactoryNames()
	if err != nil {
		t.Fatalf("embedded packaged Factory inventory: %v", err)
	}

	discoveredNames, err := discoveredPackagedFactoryNamesViaHTTP(t)
	if err != nil {
		t.Fatalf("runtime packaged Factory catalog discovery: %v", err)
	}

	if missing, extra := nameSetDiff(embeddedNames, discoveredNames); len(missing) > 0 || len(extra) > 0 {
		t.Fatalf(
			"catalog discovery drift: missing from discovery %v, invented by discovery %v; embedded=%v discovered=%v",
			missing,
			extra,
			embeddedNames,
			discoveredNames,
		)
	}

	failCatalogForcedUnwindAfterAssertion(t)
}

// TestPackagedFactoryCatalogHasUniqueStableNames proves runtime packaged
// Factory catalog discovery through GET /packaged-factories publishes each
// Factory under a unique, stable customer-facing identity: lexical name order,
// no duplicate names/slugs/projects, and consistent @you/<slug> name binding
// suitable for matrix targeting.
func TestPackagedFactoryCatalogHasUniqueStableNames(t *testing.T) {
	catalog, err := discoveredPackagedFactoryCatalogViaHTTP(t)
	if err != nil {
		t.Fatalf("runtime packaged Factory catalog discovery: %v", err)
	}
	if len(catalog.Factories) == 0 {
		t.Fatal("catalog discovery returned no published factories")
	}

	embeddedSlugByName, err := embeddedPackagedFactorySlugByName()
	if err != nil {
		t.Fatalf("embedded packaged Factory inventory: %v", err)
	}

	names := make([]string, len(catalog.Factories))
	seenNames := make(map[string]struct{}, len(catalog.Factories))
	seenSlugs := make(map[string]struct{}, len(catalog.Factories))
	seenProjects := make(map[string]struct{}, len(catalog.Factories))
	for index, factory := range catalog.Factories {
		names[index] = factory.Name
		if factory.Name == "" || factory.Slug == "" || factory.Project == "" {
			t.Fatalf("catalog entry missing stable identity fields: %#v", factory)
		}
		expectedName := "@you/" + factory.Slug
		if factory.Name != expectedName {
			t.Fatalf("name/slug binding drift: name=%q slug=%q want name %q", factory.Name, factory.Slug, expectedName)
		}
		if embeddedSlug, ok := embeddedSlugByName[factory.Name]; !ok {
			t.Fatalf("catalog name %q absent from embedded inventory", factory.Name)
		} else if embeddedSlug != factory.Slug {
			t.Fatalf(
				"catalog slug drift for %q: discovered=%q embedded=%q",
				factory.Name,
				factory.Slug,
				embeddedSlug,
			)
		}
		if _, duplicate := seenNames[factory.Name]; duplicate {
			t.Fatalf("duplicate catalog name %q", factory.Name)
		}
		seenNames[factory.Name] = struct{}{}
		if _, duplicate := seenSlugs[factory.Slug]; duplicate {
			t.Fatalf("duplicate catalog slug %q", factory.Slug)
		}
		seenSlugs[factory.Slug] = struct{}{}
		if _, duplicate := seenProjects[factory.Project]; duplicate {
			t.Fatalf("duplicate catalog project %q", factory.Project)
		}
		seenProjects[factory.Project] = struct{}{}
	}
	if !slices.IsSorted(names) {
		t.Fatalf("catalog names not in stable lexical order: %v", names)
	}
}

// TestNewEmbeddedFactoryRequiresFunctionalMatrixEntry proves runtime packaged
// Factory catalog discovery through GET /packaged-factories publishes every
// embedded inventory slug as an invocation-targetable Factory with stable
// builtin project identity and a non-empty executable work graph suitable for
// functional-matrix binding.
func TestNewEmbeddedFactoryRequiresFunctionalMatrixEntry(t *testing.T) {
	embeddedSlugs, err := embeddedPackagedFactorySlugs()
	if err != nil {
		t.Fatalf("embedded packaged Factory inventory: %v", err)
	}

	catalog, err := discoveredPackagedFactoryCatalogViaHTTP(t)
	if err != nil {
		t.Fatalf("runtime packaged Factory catalog discovery: %v", err)
	}

	catalogBySlug := make(map[string]factoryapi.PackagedFactoryCatalogEntry, len(catalog.Factories))
	for _, factory := range catalog.Factories {
		catalogBySlug[factory.Slug] = factory
	}

	for _, slug := range embeddedSlugs {
		factory, ok := catalogBySlug[slug]
		if !ok {
			t.Fatalf("embedded slug %q missing from runtime catalog discovery", slug)
		}
		wantProject := builtinProjectForPackagedFactorySlug(slug)
		if factory.Project != wantProject {
			t.Fatalf(
				"matrix project binding drift for slug %q: discovered=%q want=%q",
				slug,
				factory.Project,
				wantProject,
			)
		}
		if !publishedPackagedFactoryHasInvocationMatrixBinding(factory) {
			t.Fatalf(
				"catalog entry for slug %q is not invocation-matrix ready: %#v",
				slug,
				factory,
			)
		}
	}
}

// TestPackagedFactoriesAPI_ReturnsPublishedCatalog proves the HTTP API exposes
// the published packaged Factory catalog with complete customer-facing fields.
func TestPackagedFactoriesAPI_ReturnsPublishedCatalog(t *testing.T) {
	catalog, err := discoveredPackagedFactoryCatalogViaHTTP(t)
	if err != nil {
		t.Fatalf("GET /packaged-factories: %v", err)
	}
	if len(catalog.Factories) == 0 {
		t.Fatal("GET /packaged-factories returned no published factories")
	}
	if len(catalog.Factories) != 19 {
		t.Fatalf("GET /packaged-factories count = %d, want exact published catalog count 19", len(catalog.Factories))
	}
	for _, name := range []string{"@you/fix", "@you/ralph"} {
		factory, ok := slices.BinarySearchFunc(catalog.Factories, name, func(factory factoryapi.PackagedFactoryCatalogEntry, target string) int {
			return strings.Compare(factory.Name, target)
		})
		if !ok {
			t.Fatalf("GET /packaged-factories is missing %s", name)
		}
		entry := catalog.Factories[factory]
		if strings.TrimSpace(entry.Description.Value) == "" || len(entry.Examples) == 0 || !publishedPackagedFactoryHasInvocationMatrixBinding(entry) {
			t.Fatalf("GET /packaged-factories entry %s is not customer-discoverable: %#v", name, entry)
		}
	}
	for _, factory := range catalog.Factories {
		if factory.Name == "" || factory.Project == "" || factory.Slug == "" || len(factory.Json) == 0 || factory.Yaml == "" {
			t.Fatalf("GET /packaged-factories returned incomplete factory: %#v", factory)
		}
		if strings.TrimSpace(factory.Description.Value) == "" || len(factory.Examples) == 0 {
			t.Fatalf("GET /packaged-factories returned undiscoverable factory metadata: %#v", factory)
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

func embeddedPackagedFactoryNames() ([]string, error) {
	inventory, err := packagedfactorycatalog.Discover(
		context.Background(),
		packagedfactories.Source(),
		"factories",
	)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(inventory.Entries))
	for index, entry := range inventory.Entries {
		names[index] = entry.Factory.Name
	}
	slices.Sort(names)
	return names, nil
}

func embeddedPackagedFactorySlugs() ([]string, error) {
	inventory, err := packagedfactorycatalog.Discover(
		context.Background(),
		packagedfactories.Source(),
		"factories",
	)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, len(inventory.Entries))
	for index, entry := range inventory.Entries {
		slugs[index] = entry.Slug
	}
	slices.Sort(slugs)
	return slugs, nil
}

func embeddedPackagedFactorySlugByName() (map[string]string, error) {
	inventory, err := packagedfactorycatalog.Discover(
		context.Background(),
		packagedfactories.Source(),
		"factories",
	)
	if err != nil {
		return nil, err
	}
	slugByName := make(map[string]string, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		slugByName[entry.Factory.Name] = entry.Slug
	}
	return slugByName, nil
}

func builtinProjectForPackagedFactorySlug(slug string) string {
	return "builtin-" + slug
}

func publishedPackagedFactoryHasInvocationMatrixBinding(factory factoryapi.PackagedFactoryCatalogEntry) bool {
	if factory.Json == nil {
		return false
	}
	if invocationSignature, hasSignature := factory.Json["invocationSignature"].(map[string]any); hasSignature {
		if parameters, ok := invocationSignature["parameters"].([]any); ok && len(parameters) > 0 {
			return true
		}
	}
	if invocationReturn, hasReturn := factory.Json["invocationReturn"].(map[string]any); hasReturn {
		if policy, ok := invocationReturn["policy"].(string); ok && strings.TrimSpace(policy) != "" {
			return true
		}
	}
	if orchestrator, hasOrchestrator := factory.Json["orchestrator"].(map[string]any); hasOrchestrator && len(orchestrator) > 0 {
		return true
	}
	workTypes, workTypesOK := factory.Json["workTypes"].([]any)
	workstations, workstationsOK := factory.Json["workstations"].([]any)
	return workTypesOK && workstationsOK && len(workTypes) > 0 && len(workstations) > 0
}

func discoveredPackagedFactoryNamesViaHTTP(t *testing.T) ([]string, error) {
	t.Helper()

	catalog, err := discoveredPackagedFactoryCatalogViaHTTP(t)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(catalog.Factories))
	for index, factory := range catalog.Factories {
		names[index] = factory.Name
	}
	slices.Sort(names)
	return names, nil
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

func nameSetDiff(want, got []string) (missing, extra []string) {
	wantSet := make(map[string]struct{}, len(want))
	for _, name := range want {
		wantSet[name] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(got))
	for _, name := range got {
		gotSet[name] = struct{}{}
		if _, ok := wantSet[name]; !ok {
			extra = append(extra, name)
		}
	}
	for _, name := range want {
		if _, ok := gotSet[name]; !ok {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	return missing, extra
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
