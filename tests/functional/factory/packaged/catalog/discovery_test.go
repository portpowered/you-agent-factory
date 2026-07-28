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
	"testing"

	"github.com/portpowered/infinite-you/internal/migrationledgercheck"
	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/internal/testutil"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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

// TestNewEmbeddedFactoryRequiresFunctionalMatrixEntry proves every embedded
// packaged Factory slug is bound to a declared functional-matrix invocation
// cell in the Wave 2 checklist so new embedded inventory cannot land without
// packaged-factory coverage wiring.
func TestNewEmbeddedFactoryRequiresFunctionalMatrixEntry(t *testing.T) {
	embeddedSlugs, err := embeddedPackagedFactorySlugs()
	if err != nil {
		t.Fatalf("embedded packaged Factory inventory: %v", err)
	}

	repoRoot := testutil.MustRepoRoot(t)
	matrixPaths, err := packagedFactoryInvocationMatrixPaths(repoRoot)
	if err != nil {
		t.Fatalf("functional-matrix checklist: %v", err)
	}

	wantMatrixPaths := make([]string, len(embeddedSlugs))
	for index, slug := range embeddedSlugs {
		wantMatrixPaths[index] = packagedFactoryInvocationMatrixPathForSlug(slug)
	}
	slices.Sort(wantMatrixPaths)

	matrixPathList := make([]string, 0, len(matrixPaths))
	for matrixPath := range matrixPaths {
		matrixPathList = append(matrixPathList, matrixPath)
	}
	slices.Sort(matrixPathList)

	if missing, extra := nameSetDiff(wantMatrixPaths, matrixPathList); len(missing) > 0 || len(extra) > 0 {
		t.Fatalf(
			"functional-matrix drift: missing matrix entries %v, orphan matrix entries %v; embedded slugs=%v matrix=%v",
			missing,
			extra,
			embeddedSlugs,
			matrixPathList,
		)
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
	for _, factory := range catalog.Factories {
		if factory.Name == "" || factory.Project == "" || factory.Slug == "" || len(factory.Json) == 0 || factory.Yaml == "" {
			t.Fatalf("GET /packaged-factories returned incomplete factory: %#v", factory)
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

// TestFactoryListHonorsPreCanceledContextAtomically proves pre-cancellation
// prevents catalog work and partial customer output.
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

func packagedFactoryInvocationMatrixPaths(repoRoot string) (map[string]struct{}, error) {
	checklistPath := filepath.Join(repoRoot, migrationledgercheck.DefaultChecklistPath)
	paths, err := migrationledgercheck.LoadChecklistPaths(checklistPath)
	if err != nil {
		return nil, err
	}
	matrixPaths := make(map[string]struct{})
	for checklistPath := range paths {
		if slug, ok := packagedFactorySlugFromInvocationMatrixPath(checklistPath); ok {
			matrixPaths[packagedFactoryInvocationMatrixPathForSlug(slug)] = struct{}{}
		}
	}
	if len(matrixPaths) == 0 {
		return nil, fmt.Errorf("checklist %s has no packaged Factory invocation matrix cells", migrationledgercheck.DefaultChecklistPath)
	}
	return matrixPaths, nil
}

const packagedFactoryInvocationMatrixPrefix = "tests/functional/factory/packaged/"

func packagedFactoryInvocationMatrixPathForSlug(slug string) string {
	return packagedFactoryInvocationMatrixPrefix + strings.ReplaceAll(slug, "-", "_") + "/invocation_test.go"
}

func packagedFactorySlugFromInvocationMatrixPath(checklistPath string) (string, bool) {
	remainder, ok := strings.CutPrefix(checklistPath, packagedFactoryInvocationMatrixPrefix)
	if !ok {
		return "", false
	}
	subsection, suffix, ok := strings.Cut(remainder, "/")
	if !ok || suffix != "invocation_test.go" || subsection == "" {
		return "", false
	}
	return strings.ReplaceAll(subsection, "_", "-"), true
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

func discoveredPackagedFactoryCatalogViaHTTP(t *testing.T) (factoryapi.PackagedFactoryCatalogResponse, error) {
	t.Helper()

	dir := support.ScaffoldFactory(t, packagedFactoryCatalogTestConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})

	response, err := http.Get(server.URL() + "/packaged-factories")
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
