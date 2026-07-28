package wire_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// DEL-RUN-ENGINE-PIPELINE story 002 proves emptied superseded public pipeline
// packages are gone, no module imports reference the deleted paths, and wire still
// constructs the published Service root without peers importing deleted packages.

func deletedEnginePipelinePublicTopLevelChildren() []string {
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

func TestEnginePipelineDeletionProof_NoPublicPipelineDirectories(t *testing.T) {
	t.Parallel()

	root := serviceDeletionRepoRoot(t)
	runtimeRoot := filepath.Join(root, "pkg", "services", "factory_runtime")
	for _, name := range deletedEnginePipelinePublicTopLevelChildren() {
		path := filepath.Join(runtimeRoot, name)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("deleted public pipeline package %q still exists at %s", name, path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}

func TestEnginePipelineDeletionProof_NoModuleImportsOfDeletedPipelinePaths(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		for _, deleted := range deletedEnginePipelinePublicTopLevelChildren() {
			prefix := "github.com/portpowered/infinite-you/pkg/services/factory_runtime/" + deleted
			if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
				t.Fatalf("module still imports deleted pipeline path: %s", importPath)
			}
		}
	}
}

func TestEnginePipelineDeletionProof_DeletedPathsAbsentFromOwnershipInventory(t *testing.T) {
	t.Parallel()

	root := serviceDeletionRepoRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, row := range inventory.Packages {
		for _, deleted := range deletedEnginePipelinePublicTopLevelChildren() {
			needle := "factory_runtime/" + deleted
			if strings.Contains(row.PackagePath, needle) {
				t.Fatalf("ownership inventory still lists deleted pipeline path: %s", row.PackagePath)
			}
		}
	}
}

func TestEnginePipelineDeletionProof_WireConstructsPublishedControlObservationDispatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, err := factoryruntimewire.NewService(
		func() string { return "del-run-engine-pipeline-proof-id" },
		nil,
		nil,
		clockwork.NewFakeClock(),
		func(context.Context, workers.WorkstationDispatchRequest) error { return nil },
		func(
			context.Context,
			workers.WorkstationDispatchCancelRequest,
		) (workers.WorkstationDispatchCancelResult, error) {
			return workers.WorkstationDispatchCancelResult{}, nil
		},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var root factoryruntime.Service = service
	if root == nil {
		t.Fatal("NewService() returned nil published Service root")
	}

	_, err = root.Observe(ctx, factoryruntime.ObserveRequest{
		Scope: factoryruntime.ObservationScopeStatus,
	})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("Observe(STATUS) error = %v, want ErrNotRunning", err)
	}

	_, err = root.PlanDispatch(ctx, factoryruntime.PlanDispatchRequest{DispatchID: "del-run-engine-pipeline-proof-dispatch"})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("PlanDispatch() error = %v, want ErrNotRunning", err)
	}

	_, err = root.ControlPause(ctx, factoryruntime.PauseRequest{})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("ControlPause() error = %v, want ErrNotRunning", err)
	}
}
