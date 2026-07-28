package factory_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/testutil"
)

// pss-cln-run-fold-engine-pipeline-007: Factory Runtime root children trend toward
// wire/, internal/, transports/, plus contract files. Former public engine-pipeline
// packages must not remain as peer-importable top-level directories.
func TestPackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	serviceRoot := filepath.Join(repoRoot, "pkg", "services", "factory_runtime")
	entries, err := os.ReadDir(serviceRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
	}

	spec, ok := ownershipinventory.OwnerTopLevelSpecFor("factory_runtime")
	if !ok {
		t.Fatal("OwnerTopLevelSpecFor(factory_runtime) ok = false")
	}

	var gotRootDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			gotRootDirs = append(gotRootDirs, entry.Name())
		}
	}
	slices.Sort(gotRootDirs)

	wantRetain := slices.Clone(spec.ExpectedRetain)
	slices.Sort(wantRetain)
	for _, name := range wantRetain {
		if !slices.Contains(gotRootDirs, name) {
			t.Fatalf("service root missing canonical retain directory %q; got %v", name, gotRootDirs)
		}
	}

	for _, moved := range foldedEnginePipelineTopLevelChildren() {
		if slices.Contains(gotRootDirs, moved) {
			t.Fatalf("folded engine-pipeline package %q must not remain as a public top-level directory", moved)
		}
	}

	subservicesRoot := filepath.Join(serviceRoot, "internal", "services")
	subentries, err := os.ReadDir(subservicesRoot)
	if err != nil {
		t.Fatalf("ReadDir(%q) = %v", subservicesRoot, err)
	}
	var gotSubservices []string
	for _, entry := range subentries {
		if entry.IsDir() {
			gotSubservices = append(gotSubservices, entry.Name())
		}
	}
	slices.Sort(gotSubservices)
	wantSubservices := []string{
		"checkpoint_recovery",
		"dispatch_planning",
		"instance_host",
		"orchestration",
	}
	slices.Sort(wantSubservices)
	if !slices.Equal(gotSubservices, wantSubservices) {
		t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
	}
}

func foldedEnginePipelineTopLevelChildren() []string {
	return []string{
		"build",
		"checkpointstore",
		"checkpointsummary",
		"context",
		"definitionmapping",
		"engine",
		"javascript",
		"metrics",
		"orchestrationowner",
		"orchestratorcontract",
		"replayhooks",
		"runtime",
		"runtimecontract",
		"scheduler",
		"state",
		"subsystems",
		"throttle",
		"token",
		"token_transformer",
		"tooling",
	}
}

func TestPackagedRootUnexpectedChildrenRemainMoveDebtOnly(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	live, err := ownershipinventory.ListOwnerTopLevelChildren(repoRoot, "factory_runtime")
	if err != nil {
		t.Fatalf("ListOwnerTopLevelChildren(factory_runtime) = %v", err)
	}
	spec, ok := ownershipinventory.OwnerTopLevelSpecFor("factory_runtime")
	if !ok {
		t.Fatal("OwnerTopLevelSpecFor(factory_runtime) ok = false")
	}

	for _, name := range live {
		if slices.Contains(spec.ExpectedRetain, name) {
			continue
		}
		if !slices.Contains(spec.Unexpected, name) {
			t.Fatalf(
				"live top-level child %q is neither canonical retain %v nor committed unexpected move debt %v",
				name,
				spec.ExpectedRetain,
				spec.Unexpected,
			)
		}
		if strings.HasPrefix(name, "internal") || strings.HasPrefix(name, "wire") || strings.HasPrefix(name, "transports") {
			t.Fatalf("unexpected move-debt child %q overlaps canonical retain prefix", name)
		}
	}
}
