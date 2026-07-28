package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// mappingIsRetainToOwnerRoot reports the false-retain anti-pattern rejected for
// inventoried unexpected product-owner top-level siblings.
func mappingIsRetainToOwnerRoot(mapping PackageMapping, owner string) bool {
	return mapping.Disposition == DispositionRetain && mapping.Destination == owner
}

func TestRetainToOwnerRootGuardDetectsDeliberateViolations(t *testing.T) {
	t.Parallel()

	deliberate := PackageMapping{
		PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/engine",
		Disposition: DispositionRetain,
		Destination: "factory_runtime",
	}
	if !mappingIsRetainToOwnerRoot(deliberate, "factory_runtime") {
		t.Fatal("guard failed to detect deliberate retain→factory_runtime")
	}

	valid := PackageMapping{
		PackagePath: "pkg/services/factory_runtime/internal/services/orchestration/engine",
		Disposition: DispositionMove,
		Destination: "factory_runtime/internal/services/orchestration",
	}
	if mappingIsRetainToOwnerRoot(valid, "factory_runtime") {
		t.Fatal("guard falsely flagged valid move mapping as retain→factory_runtime")
	}
}

func TestDeliberateRetainToOwnerMappingRejectedByInventorySweepGuard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		owner string
		child string
	}{
		{owner: "factory_runtime", child: "token"},
		{owner: "workers", child: "execution"},
		{owner: "operator_settings", child: "identityinventory"},
		{owner: "work", child: "stateaccessrecordings"},
		{owner: "factory_definitions", child: "clonetests"},
		{owner: "recordings", child: "artifacts"},
		{owner: "provider_sessions", child: "service"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.owner+"/"+tc.child, func(t *testing.T) {
			t.Parallel()

			path := "pkg/services/" + tc.owner + "/" + tc.child
			got, ok := mapCommittedOwnerPackage(path)
			if !ok {
				t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", path)
			}
			if mappingIsRetainToOwnerRoot(got, tc.owner) {
				t.Fatalf("live mapping for %q is already retain→%s: %#v", path, tc.owner, got)
			}

			deliberate := got
			deliberate.Disposition = DispositionRetain
			deliberate.Destination = tc.owner
			deliberate.DeletionSuccessor = ""
			deliberate.DeletionCondition = ""
			if !mappingIsRetainToOwnerRoot(deliberate, tc.owner) {
				t.Fatalf("guard failed to detect deliberate retain→%s for %q", tc.owner, path)
			}
		})
	}
}

func TestAllProductOwnerUnexpectedSiblingPackagePathsRejectRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	for _, spec := range productOwnerTopLevelSpecsList() {
		if len(spec.unexpected) == 0 {
			continue
		}
		for _, child := range spec.unexpected {
			child := child
			t.Run(spec.owner+"/"+child, func(t *testing.T) {
				t.Parallel()

				path := "pkg/services/" + spec.owner + "/" + child
				got, ok := mapCommittedOwnerPackage(path)
				if !ok {
					t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", path)
				}
				if mappingIsRetainToOwnerRoot(got, spec.owner) {
					t.Fatalf("unexpected sibling %q maps retain→%s: %#v", path, spec.owner, got)
				}
				if got.Disposition != DispositionMove && got.Disposition != DispositionDelete {
					t.Fatalf("unexpected sibling %q disposition = %q, want move or delete", path, got.Disposition)
				}
			})
		}
	}
}

func TestLiveProductionUnexpectedSiblingPackagesRejectRetainToOwnerRoot(t *testing.T) {
	t.Parallel()

	repoRoot := findRepoRoot(t)
	manifest, err := loadManifest(filepath.Join(repoRoot, manifestRelativePath))
	if err != nil {
		t.Fatalf("loadManifest() error = %v", err)
	}

	for _, packagePath := range manifest.Inventory {
		owner, rest, ok := strings.Cut(strings.TrimPrefix(packagePath, "pkg/services/"), "/")
		if !ok || owner == "" || rest == "" {
			continue
		}
		if !ownerUnexpectedTopLevelRest(owner, rest) {
			continue
		}

		got, ok := mapCommittedOwnerPackage(packagePath)
		if !ok {
			t.Fatalf("mapCommittedOwnerPackage(%q) ok = false", packagePath)
		}
		if mappingIsRetainToOwnerRoot(got, owner) {
			t.Fatalf("unexpected sibling path %q maps retain→%s: %#v", packagePath, owner, got)
		}
		if got.Disposition != DispositionMove && got.Disposition != DispositionDelete {
			t.Fatalf("unexpected sibling path %q disposition = %q, want move or delete", packagePath, got.Disposition)
		}
	}
}
