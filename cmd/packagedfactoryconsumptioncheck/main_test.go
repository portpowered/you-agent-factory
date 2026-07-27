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
		"pkg/services/factory_definitions/bypass.go",
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
		"pkg/services/factory_definitions/bypass.go calls packagedfactorycatalog.LoadPublishedDefinitionCatalog",
	)
}

func fixtureRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, relative := range []string{
		"pkg/wire",
		"pkg/transports/http",
		"pkg/services/factory_definitions/packages/goal",
		"internal/packagedfactorycatalog",
		"packages/packaged-factories",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(relative)), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", relative, err)
		}
	}
	writeFixture(t, root, "pkg/wire/profiles.go", `package wire

import "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"

func profiles() error {
	_, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	return err
}
`)
	writeFixture(t, root, "pkg/transports/http/handlers_models.go", `package http

import "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"

func handlers() error {
	_, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	return err
}
`)
	writeFixture(t, root, "pkg/services/factory_definitions/packages/goal/prompt_drift.go", `package goal

import "github.com/portpowered/infinite-you/internal/packagedfactorycatalog"

func promptDrift() error {
	_, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
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
