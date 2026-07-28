package ownershipinventory_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
)

func TestProductOwnerTopLevelSpecsCoverAllOwners(t *testing.T) {
	t.Parallel()

	specs := ownershipinventory.ProductOwnerTopLevelSpecs()
	if len(specs) != len(ownershipinventory.ProductOwners) {
		t.Fatalf("spec count = %d, want %d", len(specs), len(ownershipinventory.ProductOwners))
	}
	for i, owner := range ownershipinventory.ProductOwners {
		if specs[i].Owner != owner {
			t.Fatalf("spec[%d].Owner = %q, want %q", i, specs[i].Owner, owner)
		}
	}
}

func TestOwnerTopLevelChildrenMatchCommittedInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, spec := range ownershipinventory.ProductOwnerTopLevelSpecs() {
		spec := spec
		t.Run(spec.Owner, func(t *testing.T) {
			t.Parallel()

			live, err := ownershipinventory.ListOwnerTopLevelChildren(root, spec.Owner)
			if err != nil {
				t.Fatalf("ListOwnerTopLevelChildren(%q) error = %v", spec.Owner, err)
			}
			want, ok := ownershipinventory.OwnerTopLevelInventory(spec.Owner)
			if !ok {
				t.Fatalf("OwnerTopLevelInventory(%q) ok = false", spec.Owner)
			}
			if !slices.Equal(live, want) {
				t.Fatalf("live top-level children = %v, want committed inventory %v", live, want)
			}
		})
	}
}

func TestOwnerTopLevelClassificationPartitionsLiveTree(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, spec := range ownershipinventory.ProductOwnerTopLevelSpecs() {
		spec := spec
		t.Run(spec.Owner, func(t *testing.T) {
			t.Parallel()

			live, err := ownershipinventory.ListOwnerTopLevelChildren(root, spec.Owner)
			if err != nil {
				t.Fatalf("ListOwnerTopLevelChildren(%q) error = %v", spec.Owner, err)
			}
			for _, name := range live {
				kind, ok := ownershipinventory.ClassifyOwnerTopLevelChild(spec.Owner, name)
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

	for _, spec := range ownershipinventory.ProductOwnerTopLevelSpecs() {
		for _, name := range spec.Unexpected {
			if slices.Contains(spec.ExpectedRetain, name) {
				t.Fatalf("owner %q child %q is both expected retain and unexpected", spec.Owner, name)
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
		spec, ok := ownershipinventory.OwnerTopLevelSpecFor(owner)
		if !ok {
			t.Fatalf("OwnerTopLevelSpecFor(%q) ok = false", owner)
		}
		for _, name := range required {
			if !slices.Contains(spec.Unexpected, name) {
				t.Fatalf("owner %q unexpected inventory missing required child %q", owner, name)
			}
		}
	}
}

func TestOwnerTopLevelRestClassificationMatchesTopLevelInventory(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	packages, err := ownershipinventory.ListProductionPackages(root)
	if err != nil {
		t.Fatalf("ListProductionPackages() error = %v", err)
	}

	for _, spec := range ownershipinventory.ProductOwnerTopLevelSpecs() {
		ownerPrefix := "pkg/services/" + spec.Owner + "/"
		for _, packagePath := range packages {
			if packagePath == "pkg/services/"+spec.Owner {
				continue
			}
			if !strings.HasPrefix(packagePath, ownerPrefix) {
				continue
			}
			rest := strings.TrimPrefix(packagePath, ownerPrefix)
			canonical := ownershipinventory.IsOwnerCanonicalRetainRest(spec.Owner, rest)
			unexpected := ownershipinventory.IsOwnerUnexpectedTopLevelRest(spec.Owner, rest)
			if canonical && unexpected {
				t.Fatalf("owner %q path %q is both canonical retain and unexpected", spec.Owner, packagePath)
			}
			if !canonical && !unexpected {
				t.Fatalf("owner %q path %q is neither canonical retain nor unexpected top-level sibling", spec.Owner, packagePath)
			}
		}
	}
}

func TestRecordingsTopLevelInventoryMatchesUnifiedRegistry(t *testing.T) {
	t.Parallel()

	spec, ok := ownershipinventory.OwnerTopLevelSpecFor("recordings")
	if !ok {
		t.Fatal("OwnerTopLevelSpecFor(recordings) ok = false")
	}
	if !slices.Equal(spec.ExpectedRetain, ownershipinventory.RecordingsTopLevelExpectedRetain) {
		t.Fatalf("recordings expected retain drift: unified=%v recordings=%v", spec.ExpectedRetain, ownershipinventory.RecordingsTopLevelExpectedRetain)
	}
	if !slices.Equal(spec.Unexpected, ownershipinventory.RecordingsTopLevelUnexpected) {
		t.Fatalf("recordings unexpected drift: unified=%v recordings=%v", spec.Unexpected, ownershipinventory.RecordingsTopLevelUnexpected)
	}
}
