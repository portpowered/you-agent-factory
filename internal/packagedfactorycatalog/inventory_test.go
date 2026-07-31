package packagedfactorycatalog_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
)

func TestDiscoverReturnsCompleteSortedAuthoredInventory(t *testing.T) {
	t.Parallel()

	inventory, err := packagedfactorycatalog.Discover(
		context.Background(),
		packagedfactories.Source(),
		"factories",
	)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	want := []string{
		"classify", "deep-research", "full-flow", "fusion", "goal", "loop",
		"plan-execute", "plan-parallel", "quorum", "review", "spawn", "subagent",
		"tournament", "tts",
	}
	if len(inventory.Entries) != len(want) {
		t.Fatalf("entries = %d, want %d", len(inventory.Entries), len(want))
	}
	for index, entry := range inventory.Entries {
		if entry.Slug != want[index] {
			t.Fatalf("entry[%d].Slug = %q, want %q", index, entry.Slug, want[index])
		}
		wantExtension := ".yaml"
		if entry.Slug == "deep-research" || entry.Slug == "spawn" || entry.Slug == "tournament" {
			wantExtension = ".js"
		}
		if entry.SourcePath != "factories/"+entry.Slug+"/factory"+wantExtension {
			t.Fatalf("entry[%d].SourcePath = %q", index, entry.SourcePath)
		}
		if entry.Factory.Name != "@you/"+entry.Slug {
			t.Fatalf("entry[%d].Factory.Name = %q", index, entry.Factory.Name)
		}
		if entry.Factory.Project == "" {
			t.Fatalf("entry[%d].Factory.Project is empty", index)
		}
	}
}

func TestDiscoverAcceptsStandaloneJavaScriptRoot(t *testing.T) {
	t.Parallel()
	source := fstest.MapFS{
		"factories/js/factory.js": &fstest.MapFile{Data: []byte(`/* @you-factory-meta
{
  "name":"@you/js",
  "version":1,
  "id":"id-js",
  "description":{"type":"LOCALIZABLE_ASSET","value":"One-file workflow."},
  "argsSchema":{"type":"object","properties":{"request":{"type":"string"}}},
  "defaultPolicy":{"mode":"READ_ONLY","maxAgents":1,"concurrency":1,"maxDepth":1,"maxRetries":0,"allowNetwork":false,"allowConnectors":false,"allowDangerFullAccess":false,"writableRoots":[]}
}
*/
return { request: args.request };`)},
	}
	inventory, err := packagedfactorycatalog.Discover(context.Background(), source, "factories")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	entry := inventory.Entries[0]
	if entry.SourcePath != "factories/js/factory.js" || entry.Factory.Project != "id-js" {
		t.Fatalf("entry = %#v", entry)
	}
	if entry.Factory.Orchestrator == nil || entry.Factory.Orchestrator.JavaScript == nil ||
		entry.Factory.Orchestrator.JavaScript.InlineSource == nil {
		t.Fatalf("JavaScript orchestrator = %#v", entry.Factory.Orchestrator)
	}
}

func TestDiscoverIsIndependentOfMapInsertionOrder(t *testing.T) {
	t.Parallel()

	first := validInventoryFS(
		factoryFile("factories/zeta/factory.json", "@you/zeta", "id-zeta"),
		factoryFile("factories/alpha/factory.json", "@you/alpha", "id-alpha"),
	)
	second := validInventoryFS(
		factoryFile("factories/alpha/factory.json", "@you/alpha", "id-alpha"),
		factoryFile("factories/zeta/factory.json", "@you/zeta", "id-zeta"),
	)

	firstInventory, err := packagedfactorycatalog.Discover(context.Background(), first, "factories")
	if err != nil {
		t.Fatalf("first Discover: %v", err)
	}
	secondInventory, err := packagedfactorycatalog.Discover(context.Background(), second, "factories")
	if err != nil {
		t.Fatalf("second Discover: %v", err)
	}
	for index, want := range []string{"alpha", "zeta"} {
		if firstInventory.Entries[index].Slug != want || secondInventory.Entries[index].Slug != want {
			t.Fatalf(
				"entry[%d] slugs = %q/%q, want %q",
				index,
				firstInventory.Entries[index].Slug,
				secondInventory.Entries[index].Slug,
				want,
			)
		}
	}
}

func TestDiscoverAcceptsCanonicalYAMLRoot(t *testing.T) {
	t.Parallel()

	source := fstest.MapFS{
		"factories/yaml/factory.yaml": &fstest.MapFile{Data: []byte(
			"name: '@you/yaml'\n" +
				"id: id-yaml\n" +
				"workTypes: []\n" +
				"resources: []\n" +
				"workers: []\n" +
				"workstations: []\n",
		)},
	}
	inventory, err := packagedfactorycatalog.Discover(context.Background(), source, "factories")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if got := inventory.Entries[0].Factory.Project; got != "id-yaml" {
		t.Fatalf("Factory.Project = %q, want id-yaml", got)
	}
}

func TestDiscoverRejectsInvalidDirectoryRoots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		source    fstest.MapFS
		fragments []string
	}{
		{
			name: "empty inventory",
			source: fstest.MapFS{
				"factories/.keep": &fstest.MapFile{Data: []byte{}},
			},
			fragments: []string{"factories contains no authored Factory directories"},
		},
		{
			name: "missing root",
			source: fstest.MapFS{
				"factories/missing/prompts/worker.md": &fstest.MapFile{Data: []byte("prompt")},
			},
			fragments: []string{
				"factories/missing has no root Factory document",
				"factories/missing/factory.json",
			},
		},
		{
			name: "ambiguous roots",
			source: validInventoryFS(
				factoryFile("factories/ambiguous/factory.json", "@you/ambiguous", "id-ambiguous"),
				factoryFile("factories/ambiguous/factory.yaml", "@you/ambiguous", "id-ambiguous"),
			),
			fragments: []string{
				"factories/ambiguous has 2 root Factory documents",
				"factories/ambiguous/factory.json",
				"factories/ambiguous/factory.yaml",
			},
		},
		{
			name: "unsupported root",
			source: fstest.MapFS{
				"factories/toml/factory.toml": &fstest.MapFile{Data: []byte("name = '@you/toml'")},
			},
			fragments: []string{
				"unsupported root Factory candidate(s)",
				"factories/toml/factory.toml",
			},
		},
		{
			name: "invalid definition",
			source: fstest.MapFS{
				"factories/invalid/factory.json": &fstest.MapFile{Data: []byte(`{"name":"@you/invalid","id":"id-invalid","unknown":true}`)},
			},
			fragments: []string{
				"factories/invalid/factory.json",
				"decode and map canonical Factory",
				`unknown field "unknown"`,
			},
		},
		{
			name: "invalid canonical topology",
			source: fstest.MapFS{
				"factories/dangling/factory.json": &fstest.MapFile{Data: []byte(
					`{"name":"@you/dangling","id":"id-dangling",` +
						`"workTypes":[{"name":"task","handlingBehavior":["DEFAULT"],"states":[` +
						`{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],` +
						`"resources":[],"workers":[],"workstations":[{"name":"move","type":"LOGICAL_MOVE",` +
						`"inputs":[{"workType":"missing","state":"init"}],` +
						`"outputs":[{"workType":"task","state":"done"}],` +
						`"onFailure":[{"workType":"task","state":"failed"}]}]}`,
				)},
			},
			fragments: []string{
				"factories/dangling/factory.json",
				"canonical Factory validation failed",
				"missing",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := packagedfactorycatalog.Discover(context.Background(), test.source, "factories")
			assertErrorContains(t, err, test.fragments...)
		})
	}
}

func TestDiscoverRejectsMissingAndDuplicateIdentities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		source    fstest.MapFS
		fragments []string
	}{
		{
			name: "missing project",
			source: validInventoryFS(
				factoryFile("factories/alpha/factory.json", "@you/alpha", ""),
			),
			fragments: []string{
				"factories/alpha/factory.json",
				"Factory project/id is empty",
			},
		},
		{
			name: "duplicate name",
			source: validInventoryFS(
				factoryFile("factories/alpha/factory.json", "@you/repeated", "id-alpha"),
				factoryFile("factories/beta/factory.json", "@you/repeated", "id-beta"),
			),
			fragments: []string{
				`duplicate public Factory name "@you/repeated"`,
				"factories/alpha/factory.json",
				"factories/beta/factory.json",
			},
		},
		{
			name: "duplicate project",
			source: validInventoryFS(
				factoryFile("factories/alpha/factory.json", "@you/alpha", "id-repeated"),
				factoryFile("factories/beta/factory.json", "@you/beta", "id-repeated"),
			),
			fragments: []string{
				`duplicate Factory project/id "id-repeated"`,
				"factories/alpha/factory.json",
				"factories/beta/factory.json",
			},
		},
		{
			name: "case folded duplicate slug",
			source: validInventoryFS(
				factoryFile("factories/Alpha/factory.json", "@you/alpha-upper", "id-alpha-upper"),
				factoryFile("factories/alpha/factory.json", "@you/alpha", "id-alpha"),
			),
			fragments: []string{
				`duplicate directory slug "alpha"`,
				"factories/Alpha/factory.json",
				"factories/alpha/factory.json",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := packagedfactorycatalog.Discover(context.Background(), test.source, "factories")
			assertErrorContains(t, err, test.fragments...)
		})
	}
}

func validInventoryFS(files ...mapEntry) fstest.MapFS {
	source := make(fstest.MapFS, len(files))
	for _, file := range files {
		source[file.path] = &fstest.MapFile{Data: file.data}
	}
	return source
}

type mapEntry struct {
	path string
	data []byte
}

func factoryFile(path, name, id string) mapEntry {
	return mapEntry{
		path: path,
		data: []byte(`{"name":"` + name + `","id":"` + id +
			`","workTypes":[],"resources":[],"workers":[],"workstations":[]}`),
	}
}

func assertErrorContains(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want inventory discovery failure")
	}
	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error = %q, want fragment %q", err, fragment)
		}
	}
}
