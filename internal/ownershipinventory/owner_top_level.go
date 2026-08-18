package ownershipinventory

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const servicesRootRelative = "pkg/services"

// CanonicalOwnerTopLevelRetain lists durable public top-level children that
// may retain at the product-owner root for every owner that hosts them.
var CanonicalOwnerTopLevelRetain = []string{
	"internal",
	"transports",
	"wire",
}

// OwnerTopLevelSpec is the committed closed inventory of immediate children
// under pkg/services/<owner>/.
type OwnerTopLevelSpec struct {
	Owner          string
	ExpectedRetain []string
	Unexpected     []string
}

// productOwnerTopLevelSpecs records the owners whose immediate children deviate
// from the canonical retain set, plus the owners whose canonical set is narrower
// than CanonicalOwnerTopLevelRetain. Recordings delegates to the INV-REC
// top-level lists.
//
// This is not a service roster. An owner absent from this map is reconciled
// against the canonical retain set instead (see derivedOwnerTopLevelSpec), so
// adding a service whose immediate children are all canonical requires no entry
// here.
var productOwnerTopLevelSpecs = map[string]OwnerTopLevelSpec{
	"automations": {
		Owner:          "automations",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"chat_sessions": {
		Owner:          "chat_sessions",
		ExpectedRetain: []string{"internal", "wire"},
	},
	"events": {
		Owner:          "events",
		ExpectedRetain: []string{"internal", "wire"},
	},
	"factory_definitions": {
		Owner:          "factory_definitions",
		ExpectedRetain: []string{"internal", "transports", "wire"},
		Unexpected: []string{
			"clonetests",
			"definition",
			"systeminitializationtests",
		},
	},
	"factory_runtime": {
		Owner:          "factory_runtime",
		ExpectedRetain: []string{"internal", "transports", "wire"},
		Unexpected: []string{
			"testdata",
		},
	},
	"factory_sessions": {
		Owner:          "factory_sessions",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"factory_visualization": {
		Owner:          "factory_visualization",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"models": {
		Owner:          "models",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"operator_settings": {
		Owner:          "operator_settings",
		ExpectedRetain: []string{"internal", "transports", "wire"},
		Unexpected:     []string{"testdata"},
	},
	"provider_sessions": {
		Owner:          "provider_sessions",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"providers": {
		Owner:          "providers",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"recordings": {
		Owner:          "recordings",
		ExpectedRetain: RecordingsTopLevelExpectedRetain,
		Unexpected:     RecordingsTopLevelUnexpected,
	},
	"system_initialization": {
		Owner:          "system_initialization",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"work": {
		Owner:          "work",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"workers": {
		Owner:          "workers",
		ExpectedRetain: []string{"internal", "wire"},
	},
	"worker_sessions": {
		Owner:          "worker_sessions",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"webhooks": {
		Owner:          "webhooks",
		ExpectedRetain: []string{"internal", "wire"},
	},
}

// ProductOwnerTopLevelSpecs returns the top-level inventory for every product
// owner derived from root's pkg/services tree, ordered by owner name.
//
// The owner roster comes from the directory listing rather than a Go literal, so
// a newly added service is covered here the moment its directory exists.
func ProductOwnerTopLevelSpecs(root string) ([]OwnerTopLevelSpec, error) {
	owners, err := DiscoverProductOwners(root)
	if err != nil {
		return nil, err
	}
	specs := make([]OwnerTopLevelSpec, 0, len(owners))
	for _, owner := range owners {
		if spec, declared := productOwnerTopLevelSpecs[owner]; declared {
			specs = append(specs, spec)
			continue
		}
		spec, err := derivedOwnerTopLevelSpec(root, owner)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// derivedOwnerTopLevelSpec builds the top-level spec for an owner that declares
// no deviation, by intersecting its live children with the canonical retain set.
//
// Expecting the full canonical set would be wrong: an owner that legitimately has
// no transports/ (Workers and Webhooks both declare this today) would fail for a
// child it never had. Intersecting instead means the reconciliation assertion
// reduces to "every live top-level child is canonical", which is the actual
// architectural rule and needs no per-service registry entry.
func derivedOwnerTopLevelSpec(root, owner string) (OwnerTopLevelSpec, error) {
	children, err := ListOwnerTopLevelChildren(root, owner)
	if err != nil {
		return OwnerTopLevelSpec{}, err
	}
	spec := OwnerTopLevelSpec{Owner: owner}
	for _, child := range children {
		if slices.Contains(CanonicalOwnerTopLevelRetain, child) {
			spec.ExpectedRetain = append(spec.ExpectedRetain, child)
		}
	}
	return spec, nil
}

// OwnerTopLevelSpecFor returns the top-level inventory for one product owner.
//
// An owner with no committed entry takes the canonical retain set. That default
// is what makes a new service free to add: a service whose immediate children are
// the canonical internal/, transports/ and wire/ needs no entry in
// productOwnerTopLevelSpecs, and a service that deviates still has to declare the
// deviation.
func OwnerTopLevelSpecFor(owner string) (OwnerTopLevelSpec, bool) {
	if strings.TrimSpace(owner) == "" {
		return OwnerTopLevelSpec{}, false
	}
	if spec, ok := productOwnerTopLevelSpecs[owner]; ok {
		return spec, true
	}
	return OwnerTopLevelSpec{
		Owner:          owner,
		ExpectedRetain: slices.Clone(CanonicalOwnerTopLevelRetain),
	}, true
}

// ListOwnerTopLevelChildren returns every live directory name immediately under
// pkg/services/<owner>/.
func ListOwnerTopLevelChildren(root, owner string) ([]string, error) {
	ownerRoot := filepath.Join(root, filepath.FromSlash(servicesRootRelative), owner)
	entries, err := os.ReadDir(ownerRoot)
	if err != nil {
		return nil, fmt.Errorf("read owner root %s: %w", owner, err)
	}
	children := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		children = append(children, entry.Name())
	}
	slices.Sort(children)
	return children, nil
}

// Inventory returns the spec's full accounted-for set of immediate children:
// approved retain children plus declared unexpected siblings, stable-sorted.
func (s OwnerTopLevelSpec) Inventory() []string {
	inventory := make([]string, 0, len(s.ExpectedRetain)+len(s.Unexpected))
	inventory = append(inventory, s.ExpectedRetain...)
	inventory = append(inventory, s.Unexpected...)
	slices.Sort(inventory)
	return inventory
}

// OwnerTopLevelInventory returns the accounted-for inventory of immediate
// children for one product owner.
func OwnerTopLevelInventory(owner string) ([]string, bool) {
	spec, ok := OwnerTopLevelSpecFor(owner)
	if !ok {
		return nil, false
	}
	return spec.Inventory(), true
}

// ClassifyOwnerTopLevelChild reports whether name is an expected retain child
// or an inventoried unexpected sibling for the given product owner.
func ClassifyOwnerTopLevelChild(owner, name string) (kind string, ok bool) {
	spec, exists := OwnerTopLevelSpecFor(owner)
	if !exists {
		return "", false
	}
	if slices.Contains(spec.ExpectedRetain, name) {
		return "expected_retain", true
	}
	if slices.Contains(spec.Unexpected, name) {
		return "unexpected_move", true
	}
	return "", false
}

// IsOwnerCanonicalRetainRest reports whether rest (path after <owner>/) lives
// under a canonical retain top-level child for the given product owner.
func IsOwnerCanonicalRetainRest(owner, rest string) bool {
	spec, ok := OwnerTopLevelSpecFor(owner)
	if !ok {
		return false
	}
	top, _, _ := strings.Cut(rest, "/")
	if top == "" {
		return false
	}
	return slices.Contains(spec.ExpectedRetain, top)
}

// IsOwnerUnexpectedTopLevelRest reports whether rest names an unexpected public
// top-level sibling (exact or nested) for the given product owner.
func IsOwnerUnexpectedTopLevelRest(owner, rest string) bool {
	spec, ok := OwnerTopLevelSpecFor(owner)
	if !ok {
		return false
	}
	top, _, _ := strings.Cut(rest, "/")
	return slices.Contains(spec.Unexpected, top)
}
