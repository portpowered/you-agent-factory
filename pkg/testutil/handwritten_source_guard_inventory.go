package testutil

// HandwrittenSourcePathClass classifies how a handwritten-source guard should
// treat a filesystem path when scanning for checked-in source.
type HandwrittenSourcePathClass string

const (
	HandwrittenSourcePathClassScanHandwritten   HandwrittenSourcePathClass = "scan-handwritten"
	HandwrittenSourcePathClassExcludeGenerated  HandwrittenSourcePathClass = "exclude-generated"
	HandwrittenSourcePathClassExcludeHiddenRoot HandwrittenSourcePathClass = "exclude-hidden-root"
)

// HandwrittenSourcePathRule captures one scanned root or excluded subtree for a
// handwritten-source contract guard.
type HandwrittenSourcePathRule struct {
	Path  string
	Class HandwrittenSourcePathClass
	Why   string
}

// HandwrittenSourceGuardInventoryEntry records the intended handwritten-source
// contract for one guard that walks the filesystem.
type HandwrittenSourceGuardInventoryEntry struct {
	GuardFile string
	WalkRoot  string
	Rules     []HandwrittenSourcePathRule
}

// HandwrittenSourceGuardInventory is the source of truth for broad
// filesystem-walking handwritten-source guards that need consistent skip policy.
func HandwrittenSourceGuardInventory() []HandwrittenSourceGuardInventoryEntry {
	return []HandwrittenSourceGuardInventoryEntry{
		{
			GuardFile: "pkg/api/legacy_model_guard_test.go",
			WalkRoot:  "repo-root",
			Rules: []HandwrittenSourcePathRule{
				{
					Path:  "repo-root/**/*.go",
					Class: HandwrittenSourcePathClassScanHandwritten,
					Why:   "scan checked-in handwritten Go source across the repository",
				},
				{
					Path:  "pkg/api/generated",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "generated API output is not handwritten contract source",
				},
				{
					Path:  "ui/dist",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "compiled dashboard assets are generated artifacts",
				},
				{
					Path:  "ui/node_modules",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "dependency install output is not handwritten source",
				},
				{
					Path:  "ui/storybook-static",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "storybook build output is generated",
				},
				{
					Path:  ".*/",
					Class: HandwrittenSourcePathClassExcludeHiddenRoot,
					Why:   "hidden repository metadata such as .git, .claude, and nested worktree state must not count as handwritten source",
				},
			},
		},
		{
			GuardFile: "pkg/petri/transition_contract_guard_test.go",
			WalkRoot:  "repo-root",
			Rules: []HandwrittenSourcePathRule{
				{
					Path:  "repo-root/**/*.go",
					Class: HandwrittenSourcePathClassScanHandwritten,
					Why:   "scan checked-in handwritten Go source across the repository",
				},
				{
					Path:  "pkg/api/generated",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "generated API output is not handwritten contract source",
				},
				{
					Path:  "ui/dist",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "compiled dashboard assets are generated artifacts",
				},
				{
					Path:  "ui/node_modules",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "dependency install output is not handwritten source",
				},
				{
					Path:  "ui/storybook-static",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "storybook build output is generated",
				},
				{
					Path:  ".*/",
					Class: HandwrittenSourcePathClassExcludeHiddenRoot,
					Why:   "hidden repository metadata such as .git, .claude, and nested worktree state must not count as handwritten source",
				},
			},
		},
		{
			GuardFile: "pkg/interfaces/world_view_contract_guard_test.go#boundary",
			WalkRoot:  "pkg/interfaces",
			Rules: []HandwrittenSourcePathRule{
				{
					Path:  "pkg/interfaces/*.go",
					Class: HandwrittenSourcePathClassScanHandwritten,
					Why:   "boundary mirror names are only guarded inside the handwritten interfaces package",
				},
			},
		},
		{
			GuardFile: "pkg/interfaces/world_view_contract_guard_test.go#canonical",
			WalkRoot:  "pkg",
			Rules: []HandwrittenSourcePathRule{
				{
					Path:  "pkg/**/*.go",
					Class: HandwrittenSourcePathClassScanHandwritten,
					Why:   "scan checked-in handwritten package Go source under pkg",
				},
				{
					Path:  "pkg/api/generated",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "generated API output is not handwritten pkg source",
				},
			},
		},
		{
			GuardFile: "pkg/interfaces/runtime_lookup_contract_guard_test.go",
			WalkRoot:  "pkg",
			Rules: []HandwrittenSourcePathRule{
				{
					Path:  "pkg/**/*.go",
					Class: HandwrittenSourcePathClassScanHandwritten,
					Why:   "scan checked-in handwritten package Go source under pkg",
				},
				{
					Path:  "pkg/api/generated",
					Class: HandwrittenSourcePathClassExcludeGenerated,
					Why:   "generated API output is not handwritten pkg source",
				},
			},
		},
	}
}
