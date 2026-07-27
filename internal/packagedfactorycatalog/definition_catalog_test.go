package packagedfactorycatalog_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

var publishedCatalogNames = []string{
	"@you/deep-research",
	"@you/fusion",
	"@you/goal",
	"@you/quorum",
	"@you/review",
	"@you/subagent",
	"@you/tts",
}

func TestLoadPublishedDefinitionCatalogReturnsExactDetachedGeneratedDefinitions(t *testing.T) {
	t.Parallel()

	catalog, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		t.Fatalf("LoadPublishedDefinitionCatalog: %v", err)
	}
	if got := catalog.Names(); !reflect.DeepEqual(got, publishedCatalogNames) {
		t.Fatalf("Names() = %v, want %v", got, publishedCatalogNames)
	}

	manifest := readFixtureManifest(t, publishedFixture(t))
	entries := make(map[string]packagedfactorycatalog.ManifestEntry, len(manifest.Factories))
	for _, entry := range manifest.Factories {
		entries[entry.PublicName] = entry
	}
	definitions := catalog.All()
	if len(definitions) != len(publishedCatalogNames) {
		t.Fatalf("All() returned %d definitions, want %d", len(definitions), len(publishedCatalogNames))
	}
	for _, definition := range definitions {
		entry := entries[definition.Name]
		want, err := fs.ReadFile(packagedfactories.Published(), entry.JSON.Locator)
		if err != nil {
			t.Fatalf("read published %s: %v", entry.JSON.Locator, err)
		}
		if definition.Project != entry.Project || !reflect.DeepEqual(definition.JSON, want) {
			t.Fatalf("definition %q does not preserve manifest identity and exact generated JSON bytes", definition.Name)
		}
		wantYAML, err := fs.ReadFile(packagedfactories.Published(), entry.YAML.Locator)
		if err != nil {
			t.Fatalf("read published %s: %v", entry.YAML.Locator, err)
		}
		if !reflect.DeepEqual(definition.YAML, wantYAML) ||
			!reflect.DeepEqual(definition.Formats, []factorydefinitions.PackagedFactoryFormat{
				factorydefinitions.PackagedFactoryFormatJSON,
				factorydefinitions.PackagedFactoryFormatYAML,
			}) {
			t.Fatalf("definition %q does not preserve published format metadata", definition.Name)
		}
	}

	definitions[0].JSON[0] = 'x'
	definitions[0].YAML[0] = 'x'
	definitions[0].Formats[0] = "changed"
	again := catalog.All()
	if again[0].JSON[0] == 'x' || again[0].YAML[0] == 'x' ||
		again[0].Formats[0] != "JSON" {
		t.Fatal("All() returned shared mutable definition bytes")
	}
	lookup, ok := catalog.Lookup("@you/goal")
	if !ok {
		t.Fatal("Lookup(@you/goal) did not find a definition")
	}
	lookup.JSON[0] = 'x'
	secondLookup, ok := catalog.Lookup("@you/goal")
	if !ok || secondLookup.JSON[0] == 'x' {
		t.Fatal("Lookup() did not return detached definition bytes")
	}
	if _, ok := catalog.Lookup("@you/missing"); ok {
		t.Fatal("Lookup(@you/missing) unexpectedly found a definition")
	}
}

func TestLoadDefinitionCatalogRejectsManifestAndSchemaFailuresWithoutPartialCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(t *testing.T, fixture fstest.MapFS)
		wantErrors []string
	}{
		{
			name: "missing manifest",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				delete(fixture, packagedfactorycatalog.CatalogManifestPath)
			},
			wantErrors: []string{"read manifest", "manifest.json"},
		},
		{
			name: "malformed manifest",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				writeFixture(fixture, packagedfactorycatalog.CatalogManifestPath, []byte(`{`))
			},
			wantErrors: []string{"decode manifest", "unexpected EOF"},
		},
		{
			name: "unknown manifest field",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				payload := fixture[packagedfactorycatalog.CatalogManifestPath].Data
				payload = []byte(strings.Replace(string(payload), `"formatVersion": "1"`, `"unknown": true, "formatVersion": "1"`, 1))
				writeFixture(fixture, packagedfactorycatalog.CatalogManifestPath, payload)
			},
			wantErrors: []string{"decode manifest", "unknown field"},
		},
		{
			name: "unsupported format",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					manifest.FormatVersion = "2"
				})
			},
			wantErrors: []string{"formatVersion", "unsupported", `"2"`},
		},
		{
			name: "unsupported manifest schema identity",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					manifest.FactorySchema = "https://example.com/factory.schema.json"
				})
			},
			wantErrors: []string{"factorySchema", "unsupported"},
		},
		{
			name: "unsupported embedded schema identity",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				var schema map[string]any
				decodeJSON(t, fixture["schemas/factory.schema.json"].Data, &schema)
				schema["$id"] = "https://example.com/factory.schema.json"
				writeJSONFixture(t, fixture, "schemas/factory.schema.json", schema)
			},
			wantErrors: []string{"Factory schema $id", "unsupported"},
		},
		{
			name: "empty inventory",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					manifest.Factories = nil
				})
			},
			wantErrors: []string{"contains no Factories"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := publishedFixture(t)
			test.mutate(t, fixture)
			catalog, err := packagedfactorycatalog.LoadDefinitionCatalog(fixture)
			assertCatalogError(t, catalog.Names(), err, test.wantErrors...)
		})
	}

	catalog, err := packagedfactorycatalog.LoadDefinitionCatalog(nil)
	assertCatalogError(t, catalog.Names(), err, "source filesystem is required")
}

func TestLoadDefinitionCatalogRejectsUnsafeMissingDuplicateAndMismatchedEntries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(t *testing.T, fixture fstest.MapFS)
		wantErrors []string
	}{
		{
			name: "unsafe locator",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					manifest.Factories[0].JSON.Locator = "../factory.json"
				})
			},
			wantErrors: []string{"@you/deep-research", "JSON", "unsafe package-public locator"},
		},
		{
			name: "locator does not match slug",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					manifest.Factories[0].JSON.Locator = manifest.Factories[1].JSON.Locator
				})
			},
			wantErrors: []string{"does not resolve", "deep-research"},
		},
		{
			name: "missing artifact",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				manifest := readFixtureManifest(t, fixture)
				delete(fixture, manifest.Factories[0].YAML.Locator)
			},
			wantErrors: []string{"@you/deep-research", "YAML", "read", "file does not exist"},
		},
		{
			name: "duplicate public name",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					manifest.Factories[1] = manifest.Factories[0]
				})
			},
			wantErrors: []string{"duplicate public name", "@you/deep-research"},
		},
		{
			name: "duplicate project",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					manifest.Factories[1].Project = manifest.Factories[0].Project
				})
			},
			wantErrors: []string{"duplicate project", "builtin-deep-research"},
		},
		{
			name: "case-insensitive duplicate slug",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					duplicate := manifest.Factories[0]
					duplicate.PublicName = "@you/Deep-Research"
					duplicate.Project = "builtin-deep-research-copy"
					duplicate.Slug = "Deep-Research"
					duplicate.JSON.Locator = "generated/factories/Deep-Research/factory.json"
					duplicate.YAML.Locator = "generated/factories/Deep-Research/factory.yaml"
					manifest.Factories[1] = duplicate
				})
			},
			wantErrors: []string{"duplicate slug", "Deep-Research"},
		},
		{
			name: "public name disagrees with slug",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					manifest.Factories[0].PublicName = "@you/other"
				})
			},
			wantErrors: []string{"public name", "does not agree", "deep-research"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := publishedFixture(t)
			test.mutate(t, fixture)
			catalog, err := packagedfactorycatalog.LoadDefinitionCatalog(fixture)
			assertCatalogError(t, catalog.Names(), err, test.wantErrors...)
		})
	}
}

func TestLoadDefinitionCatalogRejectsIntegrityDecodeIdentityAndValidationFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(t *testing.T, fixture fstest.MapFS)
		wantErrors []string
	}{
		{
			name: "invalid hash declaration",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				mutateManifest(t, fixture, func(manifest *packagedfactorycatalog.Manifest) {
					manifest.Factories[0].JSON.SHA256 = "NOT-A-HASH"
				})
			},
			wantErrors: []string{"invalid SHA-256", "64 lowercase hexadecimal"},
		},
		{
			name: "hash mismatch",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				manifest := readFixtureManifest(t, fixture)
				artifact := manifest.Factories[0].JSON.Locator
				writeFixture(fixture, artifact, append(fixture[artifact].Data, '\n'))
			},
			wantErrors: []string{"SHA-256 mismatch", "manifest=", "actual="},
		},
		{
			name: "JSON decode failure with valid hash",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				replaceArtifactAndHash(t, fixture, 0, "JSON", []byte(`{`))
			},
			wantErrors: []string{"JSON artifact", "decode JSON boundary"},
		},
		{
			name: "YAML decode failure with valid hash",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				replaceArtifactAndHash(t, fixture, 0, "YAML", []byte("value: [\n"))
			},
			wantErrors: []string{"YAML artifact", "decode"},
		},
		{
			name: "decoded identity mismatch",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				manifest := readFixtureManifest(t, fixture)
				entry := manifest.Factories[0]
				var document map[string]any
				decodeJSON(t, fixture[entry.JSON.Locator].Data, &document)
				document["name"] = "other"
				payload := marshalJSON(t, document)
				replaceArtifactAndHash(t, fixture, 0, "JSON", payload)
			},
			wantErrors: []string{"decoded identity", `name="other"`, `slug="deep-research"`},
		},
		{
			name: "Factory Definitions validation failure",
			mutate: func(t *testing.T, fixture fstest.MapFS) {
				manifest := readFixtureManifest(t, fixture)
				entry := manifest.Factories[2]
				var document map[string]any
				decodeJSON(t, fixture[entry.JSON.Locator].Data, &document)
				invocationReturn := document["invocationReturn"].(map[string]any)
				invocationReturn["terminalState"] = "missing-terminal"
				payload := marshalJSON(t, document)
				replaceArtifactAndHash(t, fixture, 2, "JSON", payload)
			},
			wantErrors: []string{"Factory Definitions validation", "missing-terminal"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := publishedFixture(t)
			test.mutate(t, fixture)
			catalog, err := packagedfactorycatalog.LoadDefinitionCatalog(fixture)
			assertCatalogError(t, catalog.Names(), err, test.wantErrors...)
		})
	}
}

func publishedFixture(t *testing.T) fstest.MapFS {
	t.Helper()
	fixture := fstest.MapFS{}
	err := fs.WalkDir(packagedfactories.Published(), ".", func(target string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		payload, err := fs.ReadFile(packagedfactories.Published(), target)
		if err != nil {
			return err
		}
		writeFixture(fixture, target, payload)
		return nil
	})
	if err != nil {
		t.Fatalf("copy published fixture: %v", err)
	}
	return fixture
}

func mutateManifest(
	t *testing.T,
	fixture fstest.MapFS,
	mutate func(*packagedfactorycatalog.Manifest),
) {
	t.Helper()
	manifest := readFixtureManifest(t, fixture)
	mutate(&manifest)
	writeJSONFixture(t, fixture, packagedfactorycatalog.CatalogManifestPath, manifest)
}

func readFixtureManifest(t *testing.T, fixture fstest.MapFS) packagedfactorycatalog.Manifest {
	t.Helper()
	var manifest packagedfactorycatalog.Manifest
	decodeJSON(t, fixture[packagedfactorycatalog.CatalogManifestPath].Data, &manifest)
	return manifest
}

func replaceArtifactAndHash(
	t *testing.T,
	fixture fstest.MapFS,
	entryIndex int,
	format string,
	payload []byte,
) {
	t.Helper()
	manifest := readFixtureManifest(t, fixture)
	artifact := &manifest.Factories[entryIndex].JSON
	if format == "YAML" {
		artifact = &manifest.Factories[entryIndex].YAML
	}
	writeFixture(fixture, artifact.Locator, payload)
	sum := sha256.Sum256(payload)
	artifact.SHA256 = hex.EncodeToString(sum[:])
	writeJSONFixture(t, fixture, packagedfactorycatalog.CatalogManifestPath, manifest)
}

func decodeJSON(t *testing.T, payload []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(payload, target); err != nil {
		t.Fatalf("decode test fixture: %v", err)
	}
}

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal test fixture: %v", err)
	}
	return payload
}

func writeJSONFixture(t *testing.T, fixture fstest.MapFS, target string, value any) {
	t.Helper()
	writeFixture(fixture, target, marshalJSON(t, value))
}

func writeFixture(fixture fstest.MapFS, target string, payload []byte) {
	fixture[target] = &fstest.MapFile{Data: append([]byte(nil), payload...), Mode: 0o444}
}

func assertCatalogError(t *testing.T, names []string, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("LoadDefinitionCatalog unexpectedly succeeded")
	}
	if len(names) != 0 {
		t.Fatalf("failed catalog exposed partial names: %v", names)
	}
	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not contain %q", err, fragment)
		}
	}
}
