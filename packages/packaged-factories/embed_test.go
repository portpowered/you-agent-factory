package packagedfactories_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"

	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
)

const manifestPath = "generated/manifest.json"

type publicationManifest struct {
	Factories []struct {
		JSON publicationArtifact `json:"json"`
		YAML publicationArtifact `json:"yaml"`
	} `json:"factories"`
}

type publicationArtifact struct {
	Locator string `json:"locator"`
}

func TestSourceReadsAuthoredFactoryAndAsset(t *testing.T) {
	t.Parallel()

	source := packagedfactories.Source()
	for _, path := range []string{
		"factories/deep-research/factory.json",
		"factories/deep-research/scripts/deep-research.workflow.js",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			content, err := fs.ReadFile(source, path)
			if err != nil {
				t.Fatalf("read embedded source %q: %v", path, err)
			}
			if len(content) == 0 {
				t.Fatalf("embedded source %q is empty", path)
			}
		})
	}
}

func TestPublishedBytesMatchNpmPackageContract(t *testing.T) {
	t.Parallel()

	published := packagedfactories.Published()
	paths := []string{
		manifestPath,
		"schemas/factory.schema.json",
		"schemas/factory.schema.yaml",
	}
	manifestPayload, err := fs.ReadFile(published, manifestPath)
	if err != nil {
		t.Fatalf("read embedded manifest: %v", err)
	}
	var manifest publicationManifest
	if err := json.Unmarshal(manifestPayload, &manifest); err != nil {
		t.Fatalf("decode embedded manifest: %v", err)
	}
	if len(manifest.Factories) == 0 {
		t.Fatal("embedded manifest has no Factory entries")
	}
	for _, factory := range manifest.Factories {
		paths = append(paths, factory.JSON.Locator, factory.YAML.Locator)
	}

	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			got, err := fs.ReadFile(published, path)
			if err != nil {
				t.Fatalf("read embedded publication file %q: %v", path, err)
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read npm package file %q: %v", path, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("embedded publication file %q differs from npm package bytes", path)
			}
		})
	}
}

func TestSourceReadsEquivalentPackagedFactorySchemas(t *testing.T) {
	t.Parallel()

	source := packagedfactories.Source()
	jsonPayload, err := fs.ReadFile(source, "schemas/factory.schema.json")
	if err != nil {
		t.Fatalf("read packaged JSON Factory schema: %v", err)
	}
	yamlPayload, err := fs.ReadFile(source, "schemas/factory.schema.yaml")
	if err != nil {
		t.Fatalf("read packaged YAML Factory schema: %v", err)
	}
	if len(jsonPayload) == 0 || len(yamlPayload) == 0 {
		t.Fatal("packaged Factory schemas must not be empty")
	}
}

func TestPublishedReadBytesAreDetachedAcrossCallers(t *testing.T) {
	t.Parallel()

	const path = "generated/factories/goal/factory.json"
	first, err := fs.ReadFile(packagedfactories.Published(), path)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	second, err := fs.ReadFile(packagedfactories.Published(), path)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("equivalent reads returned different content")
	}

	first[0] ^= 0xff
	third, err := fs.ReadFile(packagedfactories.Published(), path)
	if err != nil {
		t.Fatalf("read after caller mutation: %v", err)
	}
	if !bytes.Equal(second, third) {
		t.Fatal("mutating one read affected a later read")
	}
}

func TestPublishedReturnsOrdinaryReadErrors(t *testing.T) {
	t.Parallel()

	for _, path := range []string{
		"generated/factories/missing/factory.json",
		"../factory.json",
		"generated/factories/deep-research",
	} {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			_, err := fs.ReadFile(packagedfactories.Published(), path)
			if err == nil {
				t.Fatalf("read %q unexpectedly succeeded", path)
			}
			var pathErr *fs.PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("read %q returned %T, want *fs.PathError", path, err)
			}
		})
	}
}

func TestPublishedDoesNotExposeWriteCapabilities(t *testing.T) {
	t.Parallel()

	published := packagedfactories.Published()
	if _, ok := published.(interface {
		WriteFile(string, []byte, fs.FileMode) error
	}); ok {
		t.Fatal("embedded publication exposes WriteFile")
	}
	if _, ok := published.(interface{ Remove(string) error }); ok {
		t.Fatal("embedded publication exposes Remove")
	}
	if _, ok := published.(interface{ Rename(string, string) error }); ok {
		t.Fatal("embedded publication exposes Rename")
	}

	file, err := published.Open("generated/factories/goal/factory.json")
	if err != nil {
		t.Fatalf("open embedded publication: %v", err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close embedded source: %v", err)
		}
	})
	if _, ok := file.(io.Writer); ok {
		t.Fatal("embedded publication file is writable")
	}
}
