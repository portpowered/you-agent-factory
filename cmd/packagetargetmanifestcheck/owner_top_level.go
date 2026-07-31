package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const servicesRootRelative = "pkg/services"

// canonicalOwnerTopLevelRetain lists durable public top-level children that may
// retain at the product-owner root for every owner that hosts them.
var canonicalOwnerTopLevelRetain = []string{
	"internal",
	"transports",
	"wire",
}

type ownerTopLevelSpec struct {
	owner          string
	expectedRetain []string
	unexpected     []string
}

// productOwnerTopLevelSpecs is the reviewer-verifiable inventory for every
// committed product owner. Recordings delegates to the INV-REC top-level lists.
var productOwnerTopLevelSpecs = map[string]ownerTopLevelSpec{
	"automations": {
		owner:          "automations",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"factory_definitions": {
		owner:          "factory_definitions",
		expectedRetain: []string{"internal", "transports", "wire"},
		unexpected: []string{
			"clonetests",
			"definition",
			"systeminitializationtests",
		},
	},
	"factory_runtime": {
		owner:          "factory_runtime",
		expectedRetain: []string{"internal", "transports", "wire"},
		unexpected: []string{
			"testdata",
		},
	},
	"factory_sessions": {
		owner:          "factory_sessions",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"factory_visualization": {
		owner:          "factory_visualization",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"models": {
		owner:          "models",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"operator_settings": {
		owner:          "operator_settings",
		expectedRetain: []string{"internal", "transports", "wire"},
		unexpected:     []string{"testdata"},
	},
	"provider_sessions": {
		owner:          "provider_sessions",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"providers": {
		owner:          "providers",
		expectedRetain: []string{"inference", "internal", "transports", "wire"},
	},
	"recordings": {
		owner:          "recordings",
		expectedRetain: recordingsTopLevelExpectedRetain,
		unexpected:     recordingsTopLevelUnexpected,
	},
	"system_initialization": {
		owner:          "system_initialization",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"work": {
		owner:          "work",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"workers": {
		owner:          "workers",
		expectedRetain: []string{"internal", "wire"},
	},
}

func productOwnerTopLevelSpecsList() []ownerTopLevelSpec {
	specs := make([]ownerTopLevelSpec, 0, len(closedDestinationVocabulary().ProductOwners))
	for _, owner := range closedDestinationVocabulary().ProductOwners {
		spec, ok := productOwnerTopLevelSpecs[owner]
		if !ok {
			panic(fmt.Sprintf("missing top-level inventory for product owner %q", owner))
		}
		specs = append(specs, spec)
	}
	return specs
}

func listOwnerTopLevelChildren(root, owner string) ([]string, error) {
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

func ownerTopLevelInventory(owner string) ([]string, bool) {
	spec, ok := productOwnerTopLevelSpecs[owner]
	if !ok {
		return nil, false
	}
	inventory := make([]string, 0, len(spec.expectedRetain)+len(spec.unexpected))
	inventory = append(inventory, spec.expectedRetain...)
	inventory = append(inventory, spec.unexpected...)
	slices.Sort(inventory)
	return inventory, true
}

func classifyOwnerTopLevelChild(owner, name string) (kind string, ok bool) {
	spec, exists := productOwnerTopLevelSpecs[owner]
	if !exists {
		return "", false
	}
	if slices.Contains(spec.expectedRetain, name) {
		return "expected_retain", true
	}
	if slices.Contains(spec.unexpected, name) {
		return "unexpected_move", true
	}
	return "", false
}

func ownerCanonicalRetainRest(owner, rest string) bool {
	spec, ok := productOwnerTopLevelSpecs[owner]
	if !ok {
		return false
	}
	top, _, _ := strings.Cut(rest, "/")
	if top == "" {
		return false
	}
	return slices.Contains(spec.expectedRetain, top)
}

func ownerUnexpectedTopLevelRest(owner, rest string) bool {
	spec, ok := productOwnerTopLevelSpecs[owner]
	if !ok {
		return false
	}
	top, _, _ := strings.Cut(rest, "/")
	return slices.Contains(spec.unexpected, top)
}
