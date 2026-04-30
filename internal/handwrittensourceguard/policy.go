package handwrittensourceguard

import (
	"path/filepath"
	"strings"
)

type PathClass string

const (
	PathClassScanHandwritten   PathClass = "scan-handwritten"
	PathClassExcludeGenerated  PathClass = "exclude-generated"
	PathClassExcludeHiddenRoot PathClass = "exclude-hidden-root"
)

type PathRule struct {
	Path  string
	Class PathClass
	Why   string
}

type InventoryEntry struct {
	GuardFile string
	WalkRoot  string
	Rules     []PathRule
}

func Inventory() []InventoryEntry {
	return []InventoryEntry{
		{
			GuardFile: "pkg/api/legacy_model_guard_test.go",
			WalkRoot:  "repo-root",
			Rules: []PathRule{
				{
					Path:  "repo-root/**/*.go",
					Class: PathClassScanHandwritten,
					Why:   "scan checked-in handwritten Go source across the repository",
				},
				{
					Path:  "pkg/api/generated",
					Class: PathClassExcludeGenerated,
					Why:   "generated API output is not handwritten contract source",
				},
				{
					Path:  "ui/dist",
					Class: PathClassExcludeGenerated,
					Why:   "compiled dashboard assets are generated artifacts",
				},
				{
					Path:  "ui/node_modules",
					Class: PathClassExcludeGenerated,
					Why:   "dependency install output is not handwritten source",
				},
				{
					Path:  "ui/storybook-static",
					Class: PathClassExcludeGenerated,
					Why:   "storybook build output is generated",
				},
				{
					Path:  ".*/",
					Class: PathClassExcludeHiddenRoot,
					Why:   "hidden repository metadata such as .git, .claude, and nested worktree state must not count as handwritten source",
				},
			},
		},
		{
			GuardFile: "pkg/petri/transition_contract_guard_test.go",
			WalkRoot:  "repo-root",
			Rules: []PathRule{
				{
					Path:  "repo-root/**/*.go",
					Class: PathClassScanHandwritten,
					Why:   "scan checked-in handwritten Go source across the repository",
				},
				{
					Path:  "pkg/api/generated",
					Class: PathClassExcludeGenerated,
					Why:   "generated API output is not handwritten contract source",
				},
				{
					Path:  "ui/dist",
					Class: PathClassExcludeGenerated,
					Why:   "compiled dashboard assets are generated artifacts",
				},
				{
					Path:  "ui/node_modules",
					Class: PathClassExcludeGenerated,
					Why:   "dependency install output is not handwritten source",
				},
				{
					Path:  "ui/storybook-static",
					Class: PathClassExcludeGenerated,
					Why:   "storybook build output is generated",
				},
				{
					Path:  ".*/",
					Class: PathClassExcludeHiddenRoot,
					Why:   "hidden repository metadata such as .git, .claude, and nested worktree state must not count as handwritten source",
				},
			},
		},
		{
			GuardFile: "pkg/interfaces/world_view_contract_guard_test.go#boundary",
			WalkRoot:  "pkg/interfaces",
			Rules: []PathRule{
				{
					Path:  "pkg/interfaces/*.go",
					Class: PathClassScanHandwritten,
					Why:   "boundary mirror names are only guarded inside the handwritten interfaces package",
				},
			},
		},
		{
			GuardFile: "pkg/interfaces/world_view_contract_guard_test.go#canonical",
			WalkRoot:  "pkg",
			Rules: []PathRule{
				{
					Path:  "pkg/**/*.go",
					Class: PathClassScanHandwritten,
					Why:   "scan checked-in handwritten package Go source under pkg",
				},
				{
					Path:  "pkg/api/generated",
					Class: PathClassExcludeGenerated,
					Why:   "generated API output is not handwritten pkg source",
				},
				{
					Path:  ".*/",
					Class: PathClassExcludeHiddenRoot,
					Why:   "hidden package metadata and nested worker state must not count as handwritten pkg source",
				},
			},
		},
		{
			GuardFile: "pkg/interfaces/runtime_lookup_contract_guard_test.go",
			WalkRoot:  "pkg",
			Rules: []PathRule{
				{
					Path:  "pkg/**/*.go",
					Class: PathClassScanHandwritten,
					Why:   "scan checked-in handwritten package Go source under pkg",
				},
				{
					Path:  "pkg/api/generated",
					Class: PathClassExcludeGenerated,
					Why:   "generated API output is not handwritten pkg source",
				},
				{
					Path:  ".*/",
					Class: PathClassExcludeHiddenRoot,
					Why:   "hidden package metadata and nested worker state must not count as handwritten pkg source",
				},
			},
		},
	}
}

func ShouldSkipDir(guardFile, walkRoot, path string) bool {
	entry, ok := inventoryEntry(guardFile)
	if !ok {
		return false
	}

	rel, err := filepath.Rel(filepath.Clean(walkRoot), filepath.Clean(path))
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." {
		return false
	}
	if hasRule(entry, PathClassExcludeHiddenRoot) && containsHiddenSegment(rel) {
		return true
	}

	for _, rule := range entry.Rules {
		if rule.Class != PathClassExcludeGenerated {
			continue
		}
		if matchesRule(entry.WalkRoot, rel, rule.Path) {
			return true
		}
	}

	return false
}

func inventoryEntry(guardFile string) (InventoryEntry, bool) {
	for _, entry := range Inventory() {
		if entry.GuardFile == guardFile {
			return entry, true
		}
	}
	return InventoryEntry{}, false
}

func hasRule(entry InventoryEntry, class PathClass) bool {
	for _, rule := range entry.Rules {
		if rule.Class == class {
			return true
		}
	}
	return false
}

func matchesRule(walkRoot, relPath, rulePath string) bool {
	rulePath = strings.TrimSuffix(filepath.ToSlash(filepath.Clean(rulePath)), "/")
	if walkRoot == "pkg" {
		rulePath = strings.TrimPrefix(rulePath, "pkg/")
	}
	if walkRoot == "pkg/interfaces" {
		rulePath = strings.TrimPrefix(rulePath, "pkg/interfaces/")
	}
	return relPath == rulePath || strings.HasPrefix(relPath, rulePath+"/")
}

func containsHiddenSegment(relPath string) bool {
	for _, segment := range strings.Split(relPath, "/") {
		if strings.HasPrefix(segment, ".") && segment != "." && segment != ".." {
			return true
		}
	}
	return false
}
