package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProductOwnerTopLevelSpecsCoverAllOwners(t *testing.T) {
	t.Parallel()

	specs := productOwnerTopLevelSpecsList()
	owners := closedDestinationVocabulary().ProductOwners
	if len(specs) != len(owners) {
		t.Fatalf("spec count = %d, want %d", len(specs), len(owners))
	}
	for i, owner := range owners {
		if specs[i].owner != owner {
			t.Fatalf("spec[%d].owner = %q, want %q", i, specs[i].owner, owner)
		}
	}
}

func TestOwnerTopLevelChildrenMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	for _, spec := range productOwnerTopLevelSpecsList() {
		spec := spec
		t.Run(spec.owner, func(t *testing.T) {
			t.Parallel()

			live, err := listOwnerTopLevelChildren(repoRoot, spec.owner)
			if err != nil {
				t.Fatalf("listOwnerTopLevelChildren(%q) error = %v", spec.owner, err)
			}
			want, ok := ownerTopLevelInventory(spec.owner)
			if !ok {
				t.Fatalf("ownerTopLevelInventory(%q) ok = false", spec.owner)
			}
			if !slices.Equal(live, want) {
				t.Fatalf("live top-level children = %v, want committed inventory %v", live, want)
			}
		})
	}
}

func TestOwnerTopLevelClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	for _, spec := range productOwnerTopLevelSpecsList() {
		spec := spec
		t.Run(spec.owner, func(t *testing.T) {
			t.Parallel()

			live, err := listOwnerTopLevelChildren(repoRoot, spec.owner)
			if err != nil {
				t.Fatalf("listOwnerTopLevelChildren(%q) error = %v", spec.owner, err)
			}
			for _, name := range live {
				kind, ok := classifyOwnerTopLevelChild(spec.owner, name)
				if !ok {
					t.Fatalf("live top-level child %q is not classified", name)
				}
				if kind != "expected_retain" && kind != "unexpected_move" {
					t.Fatalf("live top-level child %q has unknown kind %q", name, kind)
				}
			}
		})
	}
}

func TestOwnerTopLevelUnexpectedDoesNotOverlapExpectedRetain(t *testing.T) {
	t.Parallel()

	for _, spec := range productOwnerTopLevelSpecsList() {
		for _, name := range spec.unexpected {
			if slices.Contains(spec.expectedRetain, name) {
				t.Fatalf("owner %q child %q is both expected retain and unexpected", spec.owner, name)
			}
		}
	}
}

func TestOwnerTopLevelInventoryCoversPacketGaps(t *testing.T) {
	t.Parallel()

	mustUnexpected := map[string][]string{
		"factory_runtime": {
			"build", "engine", "runtime", "scheduler", "state", "subsystems", "token",
		},
		"workers": {
			"construction", "diagnostics", "execution",
		},
		"operator_settings": {
			"identityinventory", "servicewire", "testlink",
		},
		"work": {
			"stateaccessrecordings",
		},
		"factory_definitions": {
			"authoredlayout", "definition", "loading", "validation",
		},
		"recordings": {
			"artifacts", "events", "projections", "replay", "service",
		},
	}

	for owner, required := range mustUnexpected {
		spec, ok := productOwnerTopLevelSpecs[owner]
		if !ok {
			t.Fatalf("productOwnerTopLevelSpecs[%q] missing", owner)
		}
		for _, name := range required {
			if !slices.Contains(spec.unexpected, name) {
				t.Fatalf("owner %q unexpected inventory missing required child %q", owner, name)
			}
		}
	}
}

func TestOwnerTopLevelRestClassificationMatchesTopLevelInventory(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	inventory, err := loadManifestInventory(repoRoot)
	if err != nil {
		t.Fatalf("loadManifestInventory() error = %v", err)
	}

	for _, spec := range productOwnerTopLevelSpecsList() {
		ownerPrefix := "pkg/services/" + spec.owner + "/"
		for _, packagePath := range inventory {
			if packagePath == "pkg/services/"+spec.owner {
				continue
			}
			if !strings.HasPrefix(packagePath, ownerPrefix) {
				continue
			}
			rest := strings.TrimPrefix(packagePath, ownerPrefix)
			canonical := ownerCanonicalRetainRest(spec.owner, rest)
			unexpected := ownerUnexpectedTopLevelRest(spec.owner, rest)
			if canonical && unexpected {
				t.Fatalf("owner %q path %q is both canonical retain and unexpected", spec.owner, packagePath)
			}
			if !canonical && !unexpected {
				t.Fatalf("owner %q path %q is neither canonical retain nor unexpected top-level sibling", spec.owner, packagePath)
			}
		}
	}
}

func TestRecordingsTopLevelInventoryMatchesUnifiedRegistry(t *testing.T) {
	t.Parallel()

	spec := productOwnerTopLevelSpecs["recordings"]
	if !slices.Equal(spec.expectedRetain, recordingsTopLevelExpectedRetain) {
		t.Fatalf("recordings expected retain drift: unified=%v recordings=%v", spec.expectedRetain, recordingsTopLevelExpectedRetain)
	}
	if !slices.Equal(spec.unexpected, recordingsTopLevelUnexpected) {
		t.Fatalf("recordings unexpected drift: unified=%v recordings=%v", spec.unexpected, recordingsTopLevelUnexpected)
	}
}

func loadManifestInventory(repoRoot string) ([]string, error) {
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		return nil, err
	}
	return manifest.Inventory, nil
}
