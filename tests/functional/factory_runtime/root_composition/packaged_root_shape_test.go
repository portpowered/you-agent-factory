package root_composition_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const factoryRuntimeServiceRootRelative = "pkg/services/factory_runtime"

// TestFactoryRuntimePackagedRootShapeMatchesCanonicalServiceLayout proves Factory
// Runtime ships the canonical packaged-service root: wire/, internal/, and
// transports/ plus thin root contract files. Residual public testdata/ may
// remain as committed unexpected move debt; folded engine-pipeline top-level
// packages must not reappear.
func TestFactoryRuntimePackagedRootShapeMatchesCanonicalServiceLayout(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	serviceRoot := filepath.Join(repoRoot, filepath.FromSlash(factoryRuntimeServiceRootRelative))
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

	for _, unexpected := range spec.Unexpected {
		if !slices.Contains(gotRootDirs, unexpected) {
			t.Fatalf("committed unexpected move-debt directory %q missing from service root; got %v", unexpected, gotRootDirs)
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
