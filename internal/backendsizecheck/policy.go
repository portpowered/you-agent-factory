package backendsizecheck

import (
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractguard"
)

type PathClass string

const (
	PathClassScanOwnedRoot      PathClass = "scan-owned-root"
	PathClassExcludeGenerated   PathClass = "exclude-generated"
	PathClassExcludeFixtureData PathClass = "exclude-fixture-data"
	PathClassExcludeVendor      PathClass = "exclude-vendor"
	PathClassExcludeHiddenRoot  PathClass = "exclude-hidden-root"
)

type PathRule struct {
	Path  string
	Class PathClass
	Why   string
}

type InventoryEntry struct {
	Root  string
	Rules []PathRule
}

func Inventory() []InventoryEntry {
	return []InventoryEntry{
		{
			Root: "cmd",
			Rules: []PathRule{
				scanRule("cmd/**/*.go", "scan maintained backend command entrypoints"),
				hiddenRule("hidden repository metadata and nested worktree state are not maintained backend command source"),
			},
		},
		{
			Root: "internal",
			Rules: []PathRule{
				scanRule("internal/**/*.go", "scan maintained backend internal implementation"),
				hiddenRule("hidden repository metadata and nested worktree state are not maintained backend internal source"),
			},
		},
		{
			Root: "pkg",
			Rules: []PathRule{
				scanRule("pkg/**/*.go", "scan maintained backend package source"),
				generatedRule("pkg/api/generated", "generated API server output is not maintained handwritten backend source"),
				generatedRule("pkg/generatedclient", "generated API client output is not maintained handwritten backend source"),
				fixtureRule("pkg/**/testdata", "Go fixtures under testdata are test inputs rather than maintained backend source"),
				hiddenRule("hidden repository metadata and nested worktree state are not maintained backend package source"),
			},
		},
		{
			Root: "tests",
			Rules: []PathRule{
				scanRule("tests/**/*.go", "scan maintained backend functional and stress test source"),
				fixtureRule("tests/**/testdata", "Go fixtures under testdata are test inputs rather than maintained backend source"),
				hiddenRule("hidden repository metadata and nested worktree state are not maintained backend test source"),
			},
		},
		{
			Root: "vendor",
			Rules: []PathRule{
				vendorRule("vendor", "vendored third-party code is not maintained repository backend source"),
			},
		},
	}
}

func scanRule(path string, why string) PathRule {
	return PathRule{Path: path, Class: PathClassScanOwnedRoot, Why: why}
}

func generatedRule(path string, why string) PathRule {
	return PathRule{Path: path, Class: PathClassExcludeGenerated, Why: why}
}

func fixtureRule(path string, why string) PathRule {
	return PathRule{Path: path, Class: PathClassExcludeFixtureData, Why: why}
}

func vendorRule(path string, why string) PathRule {
	return PathRule{Path: path, Class: PathClassExcludeVendor, Why: why}
}

func hiddenRule(why string) PathRule {
	return PathRule{Path: ".*/", Class: PathClassExcludeHiddenRoot, Why: why}
}

func ShouldSkipDir(scanRoot, path string) bool {
	rootName := filepath.Base(filepath.Clean(scanRoot))
	switch filepath.ToSlash(rootName) {
	case "cmd", "internal":
		return contractguard.ShouldSkipDir(scanRoot, path)
	case "pkg":
		return shouldSkipByRules(scanRoot, path, []string{"api/generated", "generatedclient", "testdata"})
	case "tests":
		return shouldSkipByRules(scanRoot, path, []string{"testdata"})
	case "vendor":
		return true
	default:
		return false
	}
}

func shouldSkipByRules(scanRoot string, path string, explicitSkips []string) bool {
	if contractguard.ShouldSkipDir(scanRoot, path, explicitSkips...) {
		return true
	}

	relativePath, err := filepath.Rel(scanRoot, path)
	if err != nil {
		return false
	}
	relativePath = filepath.ToSlash(filepath.Clean(relativePath))
	if relativePath == "." {
		return false
	}

	for _, segment := range strings.Split(relativePath, "/") {
		if segment == "testdata" {
			return true
		}
	}
	return false
}
