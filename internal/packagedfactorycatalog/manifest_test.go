package packagedfactorycatalog_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestGenerateManifestProjectsCompleteSortedCatalogWithExactIntegrity(t *testing.T) {
	t.Parallel()

	result, err := packagedfactorycatalog.GenerateManifest(
		context.Background(),
		packagedfactories.Source(),
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	if result.Manifest.FormatVersion != packagedfactorycatalog.ManifestFormatVersion {
		t.Fatalf("formatVersion = %q", result.Manifest.FormatVersion)
	}
	const schemaIdentity = "https://schemas.portpowered.com/you/config/factory.schema.json"
	if result.Manifest.FactorySchema != schemaIdentity {
		t.Fatalf("factorySchema = %q, want %q", result.Manifest.FactorySchema, schemaIdentity)
	}
	if len(result.Manifest.Factories) != 7 {
		t.Fatalf("factories = %d, want 7", len(result.Manifest.Factories))
	}

	names := make([]string, 0, len(result.Manifest.Factories))
	artifacts, err := packagedfactorycatalog.GenerateArtifacts(
		context.Background(),
		packagedfactories.Source(),
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("GenerateArtifacts: %v", err)
	}
	artifactsBySlug := make(map[string]packagedfactorycatalog.ArtifactPair, len(artifacts))
	for _, artifact := range artifacts {
		artifactsBySlug[artifact.Slug] = artifact
	}
	for _, entry := range result.Manifest.Factories {
		names = append(names, entry.PublicName)
		artifact := artifactsBySlug[entry.Slug]
		assertManifestIntegrity(t, entry.JSON, artifact.JSON)
		assertManifestIntegrity(t, entry.YAML, artifact.YAML)
		if entry.JSON.Locator != "generated/factories/"+entry.Slug+"/factory.json" ||
			entry.YAML.Locator != "generated/factories/"+entry.Slug+"/factory.yaml" {
			t.Fatalf("%s locators = %q, %q", entry.Slug, entry.JSON.Locator, entry.YAML.Locator)
		}
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(names, sorted) {
		t.Fatalf("manifest names = %v, want lexical order %v", names, sorted)
	}

	var decoded packagedfactorycatalog.Manifest
	if err := json.Unmarshal(result.JSON, &decoded); err != nil {
		t.Fatalf("decode manifest JSON: %v", err)
	}
	if !reflect.DeepEqual(decoded, result.Manifest) {
		t.Fatal("manifest JSON does not decode to the projected manifest")
	}
	if !strings.HasSuffix(string(result.JSON), "\n") {
		t.Fatal("manifest JSON must end with one newline")
	}
}

func TestGenerateManifestPreservesDescriptionAndInvocationExampleProjection(t *testing.T) {
	t.Parallel()

	result, err := packagedfactorycatalog.GenerateManifest(
		context.Background(),
		artifactFixtureFS(),
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("GenerateManifest: %v", err)
	}
	entry := result.Manifest.Factories[0]
	if entry.PublicName != "@you/example" || entry.Project != "factory.example" || entry.Slug != "example" {
		t.Fatalf("identity projection = %#v", entry)
	}
	if entry.Description == nil ||
		entry.Description.ID != "description.asset" ||
		!reflect.DeepEqual(entry.Description.Locales, []string{"en-US", "fr-FR"}) ||
		entry.Description.Values["fr-FR"] != "Description française" {
		t.Fatalf("description projection changed: %#v", entry.Description)
	}
	if len(entry.Examples) != 1 ||
		entry.Examples[0].Name != "exact-payload" ||
		entry.Examples[0].Description.Value != "Example" ||
		entry.Examples[0].Args["payload"] != "line one\n  line two\n" {
		t.Fatalf("invocation example projection changed: %#v", entry.Examples)
	}
}

func TestProjectManifestRejectsUnsafeAndCollidingLocators(t *testing.T) {
	t.Parallel()

	artifacts, err := packagedfactorycatalog.GenerateArtifacts(
		context.Background(),
		artifactFixtureFS(),
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("GenerateArtifacts: %v", err)
	}
	base := artifacts[0]

	t.Run("unsafe slug", func(t *testing.T) {
		unsafe := base
		unsafe.Slug = "../escape"
		_, err := packagedfactorycatalog.ProjectManifest([]packagedfactorycatalog.ArtifactPair{unsafe}, "schema-id")
		assertErrorContains(t, err, "manifest collision", "@you/example", "../escape", "unsafe")
	})

	t.Run("normalized locator collision", func(t *testing.T) {
		upper := base
		upper.Slug = "Example"
		upper.PublicName = "@you/example-upper"
		upper.SourcePath = "factories/Example/factory.json"
		upper.Factory = cloneArtifactFactory(base)
		upper.Factory.Project = "factory.example-upper"

		_, err := packagedfactorycatalog.ProjectManifest(
			[]packagedfactorycatalog.ArtifactPair{base, upper},
			"schema-id",
		)
		assertErrorContains(
			t,
			err,
			"manifest collision",
			"generated/factories/example/factory.json",
			"@you/example",
			"@you/example-upper",
			"factories/example/factory.json",
			"factories/Example/factory.json",
		)
	})
}

func TestProjectManifestRejectsUnsupportedProjectionData(t *testing.T) {
	t.Parallel()

	artifacts, err := packagedfactorycatalog.GenerateArtifacts(
		context.Background(),
		artifactFixtureFS(),
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("GenerateArtifacts: %v", err)
	}
	artifact := artifacts[0]
	artifact.Factory = cloneArtifactFactory(artifact)
	artifact.Factory.Examples[0].Args["payload"] = map[string]string{"unsupported": "value"}

	_, err = packagedfactorycatalog.ProjectManifest(
		[]packagedfactorycatalog.ArtifactPair{artifact},
		"schema-id",
	)
	assertErrorContains(t, err, "manifest projection", "@you/example", "examples[0]", "args.payload")
}

func assertManifestIntegrity(
	t *testing.T,
	projected packagedfactorycatalog.ManifestArtifact,
	payload []byte,
) {
	t.Helper()
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])
	if projected.SHA256 != want {
		t.Fatalf("%s sha256 = %q, want %q", projected.Locator, projected.SHA256, want)
	}
	if projected.SHA256 != strings.ToLower(projected.SHA256) || len(projected.SHA256) != 64 {
		t.Fatalf("%s sha256 = %q, want lowercase SHA-256", projected.Locator, projected.SHA256)
	}
}

func cloneArtifactFactory(
	artifact packagedfactorycatalog.ArtifactPair,
) *factorydefinitions.FactoryConfig {
	cloned := *artifact.Factory
	cloned.Examples = append([]factorydefinitions.InvocationExampleConfig(nil), artifact.Factory.Examples...)
	for index := range cloned.Examples {
		args := make(factorydefinitions.InvocationExampleArguments, len(cloned.Examples[index].Args))
		for name, value := range cloned.Examples[index].Args {
			args[name] = value
		}
		cloned.Examples[index].Args = args
	}
	return &cloned
}
