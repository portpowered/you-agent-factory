package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
)

type source struct {
	roots    map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate
	packaged []factorydefinitions.EffectiveFactoryCatalogCandidate
}

type baseService struct {
	factorydefinitions.Service
}

func (s source) discovery() factorydefinitions.EffectiveFactoryCatalogDiscovery {
	return factorydefinitions.EffectiveFactoryCatalogDiscovery{
		ListRoot:     s.listRoot,
		ListPackaged: s.listPackaged,
	}
}

func (s source) listRoot(
	_ context.Context,
	root string,
) ([]factorydefinitions.EffectiveFactoryCatalogCandidate, error) {
	return cloneCandidates(s.roots[root]), nil
}

func (s source) listPackaged(
	context.Context,
) ([]factorydefinitions.EffectiveFactoryCatalogCandidate, error) {
	return cloneCandidates(s.packaged), nil
}

func TestCatalogPrecedenceShadowingOrderingAndDetachedDefinitions(t *testing.T) {
	t.Parallel()

	projectRoot := "/project/factories"
	globalRoot := "/global/factories"
	catalog := newCatalog(t, source{
		roots: map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate{
			projectRoot: {
				candidate("zeta", projectRoot, "project-zeta"),
				candidate("shared", projectRoot, "project-shared"),
			},
			globalRoot: {
				candidate("global-only", globalRoot, "global-only"),
				candidate("shared", globalRoot, "global-shared"),
				candidate("@you/goal", globalRoot, "installed-goal"),
			},
		},
		packaged: []factorydefinitions.EffectiveFactoryCatalogCandidate{
			packagedCandidate("@you/review", "packaged-review"),
			packagedCandidate("shared", "packaged-shared"),
			packagedCandidate("@you/goal", "packaged-goal"),
		},
	}.discovery())

	first := list(t, catalog, projectRoot, globalRoot)
	wantNames := []string{"@you/goal", "@you/review", "global-only", "shared", "zeta"}
	if got := names(first.Entries); !slices.Equal(got, wantNames) {
		t.Fatalf("effective names = %v, want %v", got, wantNames)
	}
	assertProject(t, first.Entries, "shared", "project-shared")
	assertProject(t, first.Entries, "@you/goal", "installed-goal")
	assertProject(t, first.Entries, "@you/review", "packaged-review")
	if catalogEntry := entry(t, first.Entries, "@you/review"); catalogEntry.Location != nil {
		t.Fatalf("packaged-only location = %q, want nil", *catalogEntry.Location)
	}

	first.Entries[0].Definition.Project = "mutated"
	first.Entries[0].InvocationSignature.Parameters[0].Name = "mutated"
	*first.Entries[2].Location = "/mutated"
	second := list(t, catalog, projectRoot, globalRoot)
	assertProject(t, second.Entries, "@you/goal", "installed-goal")
	if got := second.Entries[0].InvocationSignature.Parameters[0].Name; got != "prompt" {
		t.Fatalf("detached signature parameter = %q, want prompt", got)
	}
	if got := *entry(t, second.Entries, "global-only").Location; got == "/mutated" {
		t.Fatal("catalog location aliases a prior result")
	}
}

func TestCatalogIncludesEveryPublishedPackagedFactoryWithoutLocation(t *testing.T) {
	t.Parallel()

	published, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("load published packaged definitions: %v", err)
	}
	discovery, err := factoryservice.NewEffectiveCatalogDiscovery(
		rootLister{}.ListNamedFactories,
		definitionFiles{}.ReadFile,
		published.All(),
	)
	if err != nil {
		t.Fatalf("new published effective catalog source: %v", err)
	}
	catalog := newCatalog(t, discovery)

	result := list(t, catalog, "/project", "/global")
	if got, want := names(result.Entries), published.Names(); !slices.Equal(got, want) {
		t.Fatalf("effective packaged names = %v, want published names %v", got, want)
	}
	for _, catalogEntry := range result.Entries {
		if catalogEntry.Location != nil {
			t.Fatalf("%s location = %q, want nil", catalogEntry.Name, *catalogEntry.Location)
		}
		if catalogEntry.Definition == nil {
			t.Fatalf("%s definition is nil", catalogEntry.Name)
		}
	}
}

func TestCatalogCoversEveryPrecedenceCombination(t *testing.T) {
	t.Parallel()

	catalog := newCatalog(t, source{
		roots: map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate{
			"/project": {
				candidate("all-three", "/project", "project"),
				candidate("project-global", "/project", "project"),
				candidate("project-packaged", "/project", "project"),
			},
			"/global": {
				candidate("all-three", "/global", "global"),
				candidate("project-global", "/global", "global"),
				candidate("global-packaged", "/global", "global"),
			},
		},
		packaged: []factorydefinitions.EffectiveFactoryCatalogCandidate{
			packagedCandidate("all-three", "packaged"),
			packagedCandidate("project-packaged", "packaged"),
			packagedCandidate("global-packaged", "packaged"),
		},
	}.discovery())

	result := list(t, catalog, "/project", "/global")
	assertProject(t, result.Entries, "all-three", "project")
	assertProject(t, result.Entries, "project-global", "project")
	assertProject(t, result.Entries, "project-packaged", "project")
	assertProject(t, result.Entries, "global-packaged", "global")
	if len(result.Entries) != 4 {
		t.Fatalf("effective entries = %#v, want four completely shadowed names", result.Entries)
	}
}

func TestAttachPublishesEffectiveCatalogOnRootService(t *testing.T) {
	t.Parallel()

	catalog := newCatalog(t, source{
		roots: map[string][]factorydefinitions.EffectiveFactoryCatalogCandidate{
			"/project": {candidate("project-only", "/project", "project")},
			"/global":  nil,
		},
	}.discovery())
	service, err := factoryservice.AttachEffectiveCatalog(baseService{}, catalog)
	if err != nil {
		t.Fatalf("attach effective catalog: %v", err)
	}

	result, err := service.ListEffectiveFactories(
		t.Context(),
		factorydefinitions.ListEffectiveFactoriesRequest{
			ProjectRoot: "/project",
			GlobalRoot:  "/global",
		},
	)
	if err != nil {
		t.Fatalf("root service ListEffectiveFactories: %v", err)
	}
	if got := names(result.Entries); !slices.Equal(got, []string{"project-only"}) {
		t.Fatalf("root service effective names = %v, want [project-only]", got)
	}
}

func newCatalog(
	t *testing.T,
	discovery factorydefinitions.EffectiveFactoryCatalogDiscovery,
) factorydefinitions.EffectiveFactoryCatalogOperation {
	t.Helper()
	catalog, err := factoryservice.NewEffectiveCatalog(discovery, normalize)
	if err != nil {
		t.Fatalf("new effective catalog: %v", err)
	}
	return catalog
}

func normalize(
	_ context.Context,
	candidate factorydefinitions.EffectiveFactoryCatalogCandidate,
) (*factorydefinitions.FactoryConfig, error) {
	var definition factorydefinitions.FactoryConfig
	if err := json.Unmarshal(candidate.Canonical, &definition); err != nil {
		return nil, err
	}
	return &definition, nil
}

func list(
	t *testing.T,
	catalog factorydefinitions.EffectiveFactoryCatalogOperation,
	projectRoot string,
	globalRoot string,
) factorydefinitions.ListEffectiveFactoriesResult {
	t.Helper()
	result, err := catalog(t.Context(), factorydefinitions.ListEffectiveFactoriesRequest{
		ProjectRoot: projectRoot,
		GlobalRoot:  globalRoot,
	})
	if err != nil {
		t.Fatalf("list effective Factories: %v", err)
	}
	return result
}

func candidate(name, root, project string) factorydefinitions.EffectiveFactoryCatalogCandidate {
	location := fmt.Sprintf("%s/%s", root, name)
	return factorydefinitions.EffectiveFactoryCatalogCandidate{
		Name:      name,
		Location:  &location,
		Canonical: definitionJSON(project),
	}
}

func packagedCandidate(name, project string) factorydefinitions.EffectiveFactoryCatalogCandidate {
	return factorydefinitions.EffectiveFactoryCatalogCandidate{
		Name:      name,
		Canonical: definitionJSON(project),
	}
}

func definitionJSON(project string) []byte {
	return []byte(fmt.Sprintf(
		`{"name":"%s","project":"%s","invocationSignature":{"parameters":[{"name":"prompt","typeHint":"string","binding":{"kind":"positional"}}]},"work_types":[],"resources":[],"workers":[],"workstations":[]}`,
		project,
		project,
	))
}

func cloneCandidates(
	candidates []factorydefinitions.EffectiveFactoryCatalogCandidate,
) []factorydefinitions.EffectiveFactoryCatalogCandidate {
	cloned := make([]factorydefinitions.EffectiveFactoryCatalogCandidate, len(candidates))
	for index, candidate := range candidates {
		candidate.Canonical = append([]byte(nil), candidate.Canonical...)
		if candidate.Location != nil {
			location := *candidate.Location
			candidate.Location = &location
		}
		cloned[index] = candidate
	}
	return cloned
}

func names(entries []factorydefinitions.EffectiveFactoryCatalogEntry) []string {
	result := make([]string, len(entries))
	for index, catalogEntry := range entries {
		result[index] = catalogEntry.Name
	}
	return result
}

func entry(
	t *testing.T,
	entries []factorydefinitions.EffectiveFactoryCatalogEntry,
	name string,
) factorydefinitions.EffectiveFactoryCatalogEntry {
	t.Helper()
	for _, catalogEntry := range entries {
		if catalogEntry.Name == name {
			return catalogEntry
		}
	}
	t.Fatalf("entry %q not found in %#v", name, entries)
	return factorydefinitions.EffectiveFactoryCatalogEntry{}
}

func assertProject(
	t *testing.T,
	entries []factorydefinitions.EffectiveFactoryCatalogEntry,
	name string,
	want string,
) {
	t.Helper()
	if got := entry(t, entries, name).Definition.Project; got != want {
		t.Fatalf("%s project = %q, want %q", name, got, want)
	}
}
