package main

import (
	"testing"
)

func TestLoadOwnerInventoryCoversCommittedServiceTreeAndProcessEdges(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go": "package factory_sessions\n",
		"pkg/services/factory_runtime/doc.go":  "package factory_runtime\n",
		"pkg/services/edges/doc.go":            "package edges\n",
		"pkg/services/README.md":               "# not an owner\n",
	})

	inventory, err := loadOwnerInventory(root)
	if err != nil {
		t.Fatalf("loadOwnerInventory: %v", err)
	}

	for _, owner := range []string{"factory_sessions", "factory_runtime", "edges"} {
		if !inventory.hasOwner(owner) {
			t.Fatalf("inventory missing owner %q: %#v", owner, inventory.owners)
		}
	}
	if inventory.hasOwner("README.md") {
		t.Fatalf("inventory unexpectedly includes non-directory entry: %#v", inventory.owners)
	}
	if got := inventory.kind("edges"); got != ownerKindProcessEdges {
		t.Fatalf("edges kind = %q, want %q", got, ownerKindProcessEdges)
	}
	if got := inventory.kind("factory_sessions"); got != ownerKindService {
		t.Fatalf("factory_sessions kind = %q, want %q", got, ownerKindService)
	}
}

func TestClassifyServicePackageRootVsNonRootForDistinctOwners(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go": "package factory_sessions\n",
		"pkg/services/workers/doc.go":          "package workers\n",
	})

	inventory, err := loadOwnerInventory(root)
	if err != nil {
		t.Fatalf("loadOwnerInventory: %v", err)
	}

	cases := []struct {
		path    string
		owner   string
		surface packageSurface
	}{
		{path: "pkg/services/factory_sessions", owner: "factory_sessions", surface: surfaceRoot},
		{path: "pkg/services/factory_sessions/internal/execution", owner: "factory_sessions", surface: surfaceNonRoot},
		{path: "pkg/services/workers", owner: "workers", surface: surfaceRoot},
		{path: "pkg/services/providers/internal/services/execution/internal/provider", owner: "workers", surface: surfaceNonRoot},
		{path: "pkg/platform/logging", owner: "", surface: surfaceNone},
	}
	for _, tc := range cases {
		owner, surface := inventory.classify(tc.path)
		if owner != tc.owner || surface != tc.surface {
			t.Fatalf(
				"classify(%q) = (%q, %q), want (%q, %q)",
				tc.path,
				owner,
				surface,
				tc.owner,
				tc.surface,
			)
		}
	}
}

func TestClassifyTreatsNewDeeperPathAsNonRootWithoutPrivateCatalog(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_definitions/doc.go": "package factory_definitions\n",
		"pkg/services/recordings/doc.go":          "package recordings\n",
	})

	inventory, err := loadOwnerInventory(root)
	if err != nil {
		t.Fatalf("loadOwnerInventory: %v", err)
	}

	// Previously unlisted nested implementation path under an inventoried owner.
	newPath := "pkg/services/factory_definitions/brand_new_adapter/internal"
	owner, surface := inventory.classify(newPath)
	if owner != "factory_definitions" || surface != surfaceNonRoot {
		t.Fatalf(
			"classify(%q) = (%q, %q), want (factory_definitions, %q)",
			newPath,
			owner,
			surface,
			surfaceNonRoot,
		)
	}

	// Nested subservice under a second inventoried owner is also non-root.
	nested := "pkg/services/recordings/services/dashboard"
	owner, surface = inventory.classify(nested)
	if owner != "recordings" || surface != surfaceNonRoot {
		t.Fatalf(
			"classify(%q) = (%q, %q), want (recordings, %q)",
			nested,
			owner,
			surface,
			surfaceNonRoot,
		)
	}
}

func TestServiceImplementationImportUsesOwnerInventory(t *testing.T) {
	root := fixtureRepository(t, map[string]string{
		"pkg/services/factory_sessions/doc.go": "package factory_sessions\n",
		"pkg/services/workers/doc.go":          "package workers\n",
		"pkg/initializer/runtime.go": `package initializer
import (
  sessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
  provider "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider"
  novelty "github.com/portpowered/infinite-you/pkg/services/workers/never_listed_before"
)
var (
  _ sessions.ExecutionService
  _ provider.Service
  _ novelty.Adapter
)`,
	})

	findings := scanFixture(t, root)
	assertFinding(t, findings, ruleInitializerServiceImplementation, "/workers/provider")
	assertFinding(t, findings, ruleInitializerServiceImplementation, "/workers/never_listed_before")
	for _, item := range findings {
		if item.Rule == ruleInitializerServiceImplementation &&
			item.Target == modulePath+"/pkg/services/factory_sessions" {
			t.Fatalf("peer/service root import incorrectly classified as implementation: %#v", item)
		}
	}
}

