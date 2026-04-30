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
	return handwrittenSourceInventory()
}

func handwrittenSourceInventory() []InventoryEntry {
	return []InventoryEntry{
		repoRootInventoryEntry("pkg/api/legacy_model_guard_test.go"),
		repoRootInventoryEntry("pkg/petri/transition_contract_guard_test.go"),
		boundaryWorldViewInventoryEntry(),
		pkgRootInventoryEntry("pkg/interfaces/world_view_contract_guard_test.go#canonical"),
		pkgRootInventoryEntry("pkg/interfaces/runtime_lookup_contract_guard_test.go"),
	}
}

func repoRootInventoryEntry(guardFile string) InventoryEntry {
	return InventoryEntry{
		GuardFile: guardFile,
		WalkRoot:  "repo-root",
		Rules:     repoRootRules(),
	}
}

func boundaryWorldViewInventoryEntry() InventoryEntry {
	return InventoryEntry{
		GuardFile: "pkg/interfaces/world_view_contract_guard_test.go#boundary",
		WalkRoot:  "pkg/interfaces",
		Rules: []PathRule{
			handwrittenScanRule(
				"pkg/interfaces/*.go",
				"boundary mirror names are only guarded inside the handwritten interfaces package",
			),
		},
	}
}

func pkgRootInventoryEntry(guardFile string) InventoryEntry {
	return InventoryEntry{
		GuardFile: guardFile,
		WalkRoot:  "pkg",
		Rules:     pkgRootRules(),
	}
}

func repoRootRules() []PathRule {
	return []PathRule{
		handwrittenScanRule("repo-root/**/*.go", "scan checked-in handwritten Go source across the repository"),
		generatedRule("pkg/api/generated", "generated API output is not handwritten contract source"),
		generatedRule("ui/dist", "compiled dashboard assets are generated artifacts"),
		generatedRule("ui/node_modules", "dependency install output is not handwritten source"),
		generatedRule("ui/storybook-static", "storybook build output is generated"),
		hiddenRule("hidden repository metadata such as .git, .claude, and nested worktree state must not count as handwritten source"),
	}
}

func pkgRootRules() []PathRule {
	return []PathRule{
		handwrittenScanRule("pkg/**/*.go", "scan checked-in handwritten package Go source under pkg"),
		generatedRule("pkg/api/generated", "generated API output is not handwritten pkg source"),
		hiddenRule("hidden package metadata and nested worker state must not count as handwritten pkg source"),
	}
}

func handwrittenScanRule(path string, why string) PathRule {
	return PathRule{Path: path, Class: PathClassScanHandwritten, Why: why}
}

func generatedRule(path string, why string) PathRule {
	return PathRule{Path: path, Class: PathClassExcludeGenerated, Why: why}
}

func hiddenRule(why string) PathRule {
	return PathRule{Path: ".*/", Class: PathClassExcludeHiddenRoot, Why: why}
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
