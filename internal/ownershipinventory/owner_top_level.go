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

// productOwnerTopLevelSpecs is the reviewer-verifiable inventory for every
// committed product owner. Recordings delegates to the INV-REC top-level lists.
var productOwnerTopLevelSpecs = map[string]OwnerTopLevelSpec{
	"automations": {
		Owner:          "automations",
		ExpectedRetain: []string{"internal", "transports", "wire"},
	},
	"factory_definitions": {
		Owner:          "factory_definitions",
		ExpectedRetain: []string{"internal", "transports", "wire"},
		Unexpected: []string{
			"clonetests",
			"decisionenvelope",
			"definition",
			"editable",
			"invocationinterpolation",
			"invocationoutput",
			"invocationworktype",
			"loadedsource",
			"namevalue",
			"namedpaths",
			"packagedinstallation",
			"packages",
			"persistence",
			"quorumpolicy",
			"replayconfig",
			"resource",
			"service",
			"snapshotcapture",
			"systeminitializationtests",
			"ttsobservability",
			"validation",
			"workers",
			"workpropagation",
			"workstationexecution",
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
		Unexpected:     []string{"identityinventory", "servicewire", "testdata", "testlink", "testproviders"},
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
		Unexpected:     []string{"testdata"},
	},
	"workers": {
		Owner:          "workers",
		ExpectedRetain: []string{"internal", "wire"},
		Unexpected: []string{
			"agypty",
			"cliprovider",
			"construction",
			"diagnostics",
			"execution",
			"executor",
			"interface",
			"invocation",
			"process",
			"prompting",
			"provider",
			"provider_test",
			"runner",
			"service",
			"services",
			"skippermissions",
			"worktree",
		},
	},
}

// ProductOwnerTopLevelSpecs returns the stable-sorted committed inventory for
// every product owner in the closed destination vocabulary.
func ProductOwnerTopLevelSpecs() []OwnerTopLevelSpec {
	specs := make([]OwnerTopLevelSpec, 0, len(ProductOwners))
	for _, owner := range ProductOwners {
		spec, ok := productOwnerTopLevelSpecs[owner]
		if !ok {
			panic(fmt.Sprintf("missing top-level inventory for product owner %q", owner))
		}
		specs = append(specs, spec)
	}
	return specs
}

// OwnerTopLevelSpecFor returns the committed inventory for one product owner.
func OwnerTopLevelSpecFor(owner string) (OwnerTopLevelSpec, bool) {
	spec, ok := productOwnerTopLevelSpecs[owner]
	return spec, ok
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

// OwnerTopLevelInventory returns the closed committed inventory of immediate
// children for one product owner.
func OwnerTopLevelInventory(owner string) ([]string, bool) {
	spec, ok := productOwnerTopLevelSpecs[owner]
	if !ok {
		return nil, false
	}
	inventory := make([]string, 0, len(spec.ExpectedRetain)+len(spec.Unexpected))
	inventory = append(inventory, spec.ExpectedRetain...)
	inventory = append(inventory, spec.Unexpected...)
	slices.Sort(inventory)
	return inventory, true
}

// ClassifyOwnerTopLevelChild reports whether name is an expected retain child
// or an inventoried unexpected sibling for the given product owner.
func ClassifyOwnerTopLevelChild(owner, name string) (kind string, ok bool) {
	spec, exists := productOwnerTopLevelSpecs[owner]
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
	spec, ok := productOwnerTopLevelSpecs[owner]
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
	spec, ok := productOwnerTopLevelSpecs[owner]
	if !ok {
		return false
	}
	top, _, _ := strings.Cut(rest, "/")
	return slices.Contains(spec.Unexpected, top)
}
