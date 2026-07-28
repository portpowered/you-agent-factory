package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/migrationledgercheck"
	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/internal/testutil"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
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
