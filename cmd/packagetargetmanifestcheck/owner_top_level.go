package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
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

// productOwnerTopLevelSpecs records the owners whose immediate children deviate
// from canonicalOwnerTopLevelRetain. Recordings delegates to the INV-REC
// top-level lists.
//
// This is not a service roster: an owner with no entry takes the canonical
// default from ownerTopLevelSpecFor, so adding a service needs no entry here.
var productOwnerTopLevelSpecs = map[string]ownerTopLevelSpec{
	"automations": {
		owner:          "automations",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"chat_sessions": {
		owner:          "chat_sessions",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"events": {
		owner:          "events",
		expectedRetain: []string{"internal", "wire"},
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
		expectedRetain: []string{"internal", "transports", "wire"},
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
	"worker_sessions": {
		owner:          "worker_sessions",
		expectedRetain: []string{"internal", "transports", "wire"},
	},
	"webhooks": {
		owner:          "webhooks",
		expectedRetain: []string{"internal", "wire"},
	},
}

// ownerTopLevelSpecFor returns the top-level inventory for one product owner,
// defaulting to the canonical retain set when the owner declares no deviation.
func ownerTopLevelSpecFor(owner string) ownerTopLevelSpec {
	if spec, ok := productOwnerTopLevelSpecs[owner]; ok {
		return spec
	}
	return ownerTopLevelSpec{
		owner:          owner,
		expectedRetain: slices.Clone(canonicalOwnerTopLevelRetain),
	}
}

// productOwnerTopLevelSpecsList returns the top-level inventory for every owner
// derived from repoRoot's pkg/services tree, ordered by owner name.
func productOwnerTopLevelSpecsList(repoRoot string) ([]ownerTopLevelSpec, error) {
	owners, err := ownershipinventory.DiscoverProductOwners(repoRoot)
	if err != nil {
		return nil, err
	}
	specs := make([]ownerTopLevelSpec, 0, len(owners))
	for _, owner := range owners {
		if spec, declared := productOwnerTopLevelSpecs[owner]; declared {
			specs = append(specs, spec)
			continue
		}
		spec, err := derivedOwnerTopLevelSpec(repoRoot, owner)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// derivedOwnerTopLevelSpec builds the spec for an owner that declares no
// deviation by intersecting its live children with the canonical retain set, so
// the reconciliation assertion reduces to "every live top-level child is
// canonical" instead of demanding children the owner never had.
func derivedOwnerTopLevelSpec(repoRoot, owner string) (ownerTopLevelSpec, error) {
	children, err := listOwnerTopLevelChildren(repoRoot, owner)
	if err != nil {
		return ownerTopLevelSpec{}, err
	}
	spec := ownerTopLevelSpec{owner: owner}
	for _, child := range children {
		if slices.Contains(canonicalOwnerTopLevelRetain, child) {
			spec.expectedRetain = append(spec.expectedRetain, child)
		}
	}
	return spec, nil
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

// inventory returns the spec's accounted-for immediate children: approved retain
// children plus declared unexpected siblings, stable-sorted.
func (s ownerTopLevelSpec) inventory() []string {
	inventory := make([]string, 0, len(s.expectedRetain)+len(s.unexpected))
	inventory = append(inventory, s.expectedRetain...)
	inventory = append(inventory, s.unexpected...)
	slices.Sort(inventory)
	return inventory
}

func classifyOwnerTopLevelChild(owner, name string) (kind string, ok bool) {
	spec := ownerTopLevelSpecFor(owner)
	if slices.Contains(spec.expectedRetain, name) {
		return "expected_retain", true
	}
	if slices.Contains(spec.unexpected, name) {
		return "unexpected_move", true
	}
	return "", false
}

func ownerCanonicalRetainRest(owner, rest string) bool {
	spec := ownerTopLevelSpecFor(owner)
	top, _, _ := strings.Cut(rest, "/")
	if top == "" {
		return false
	}
	return slices.Contains(spec.expectedRetain, top)
}

func ownerUnexpectedTopLevelRest(owner, rest string) bool {
	spec := ownerTopLevelSpecFor(owner)
	top, _, _ := strings.Cut(rest, "/")
	return slices.Contains(spec.unexpected, top)
}
