package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAcceptsApprovedCatalogConsumptionSurface(t *testing.T) {
	root := fixtureRepository(t)

	var stdout bytes.Buffer
	if err := run(config{root: root}, &stdout); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "consumption is constrained") {
		t.Fatalf("stdout = %q, want success diagnostic", stdout.String())
	}
}

func TestRunRejectsDirectPackagedFactoriesImportOutsideCatalogLoader(t *testing.T) {
	root := fixtureRepository(t)
	writeFixture(
		t,
		root,
		"pkg/services/factory_definitions/bypass.go",
		`package factorydefinitions

import packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"

func bypass() packagedfactories.Source {
	return packagedfactories.Source()
}
`,
	)

	err := run(config{root: root}, &bytes.Buffer{})
	assertErrorContains(
		t,
		err,
		"pkg/services/factory_definitions/bypass.go imports",
		"packages/packaged-factories",
	)
}

func TestRunRejectsDirectCatalogLoaderOutsideApprovedFiles(t *testing.T) {
	root := fixtureRepository(t)
	writeFixture(
		t,
		root,
		"pkg/services/work/bypass_test.go",
		`package factorydefinitions

import "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"

func bypass() error {
	_, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	return err
}
`,
	)

	err := run(config{root: root}, &bytes.Buffer{})
	assertErrorContains(
		t,
		err,
		"pkg/services/work/bypass_test.go imports github.com/portpowered/infinite-you/internal/packagedfactorycatalog",
	)
}

func TestRunRejectsDirectCatalogDiscoveryOutsideMaterializationBoundary(t *testing.T) {
	root := fixtureRepository(t)
	writeFixture(
		t,
		root,
		"tests/functional/catalog/bypass_test.go",
		`package catalog

import "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"

func bypass() error {
	_, err := packagedfactorycatalog.Discover(nil, nil, "factories")
	return err
}
`,
	)

	err := run(config{root: root}, &bytes.Buffer{})
	assertErrorContains(
		t,
		err,
		"tests/functional/catalog/bypass_test.go imports github.com/portpowered/infinite-you/internal/packagedfactorycatalog",
	)
}

func TestRunRejectsDirectPackagedFactoriesImportFromTest(t *testing.T) {
	root := fixtureRepository(t)
	writeFixture(
		t,
		root,
		"pkg/services/work/bypass_test.go",
		`package work

import packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"

func bypass() string {
	return packagedfactories.Source().String()
}
`,
	)

	err := run(config{root: root}, &bytes.Buffer{})
	assertErrorContains(
		t,
		err,
		"pkg/services/work/bypass_test.go imports",
		"packages/packaged-factories",
	)
}

func fixtureRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, relative := range []string{
		"cmd/packagedfactorycatalogcheck",
		"cmd/packagedfactorycataloggenerate",
		"cmd/packagedfactorysourcecheck",
		"internal/packagedfactorycatalog",
		"pkg/services/factory_definitions/internal/services/distribution/packagedcatalog",
		"packages/packaged-factories",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(relative)), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", relative, err)
		}
	}
	writeFixture(t, root, "pkg/services/factory_definitions/internal/services/distribution/packagedcatalog/published.go", `package packagedcatalog

import "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"

func promptDrift() error {
	_, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	return err
}
`)
	writeFixture(t, root, "cmd/packagedfactorysourcecheck/main.go", `package main

import "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"

func source() error {
	_, err := packagedfactorycatalog.Discover(nil, nil, "factories")
	return err
}
`)
	writeFixture(t, root, "cmd/packagedfactorycataloggenerate/main.go", `package main

import "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"

func generate() error {
	return packagedfactorycatalog.Generate(".")
}
`)
	writeFixture(t, root, "cmd/packagedfactorycatalogcheck/main.go", `package main

import "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"

func check() error {
	_, err := packagedfactorycatalog.Check(".")
	return err
}
`)
	writeFixture(t, root, "internal/packagedfactorycatalog/definition_catalog.go", `package packagedfactorycatalog

import packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"

func LoadPublishedDefinitionCatalog() error {
	_ = packagedfactories.Published()
	return nil
}
`)
	writeFixture(t, root, "packages/packaged-factories/embed.go", `package packagedfactories

func Source() string { return "" }
`)
	return root
}

func writeFixture(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	message := err.Error()
	for _, part := range parts {
		if !strings.Contains(message, part) {
			t.Fatalf("error = %q, want substring %q", message, part)
		}
	}
}
