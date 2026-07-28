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

const deletedServiceImportPrefix = "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service"

// DEL-RUN-SERVICE proof tests verify the transitional factory_runtime/service
// public tree is gone, wire still constructs the published Service root, and
// engine/pipeline public packages remain for DEL-RUN-ENGINE-PIPELINE.

func TestServiceDeletionProof_NoPublicServiceDirectory(t *testing.T) {
	t.Parallel()

	root := serviceDeletionRepoRoot(t)
	runtimeRoot := filepath.Join(root, "pkg", "services", "factory_runtime")
	for _, rel := range []string{"service", filepath.Join("service", "host")} {
		path := filepath.Join(runtimeRoot, rel)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("deleted public package directory still exists: %s", path)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", path, err)
		}
	}
}

func TestServiceDeletionProof_NoModuleImportsOfDeletedServicePath(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./...")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list deps: %v\n%s", err, output)
	}
	for _, importPath := range strings.Fields(string(output)) {
		if importPath == deletedServiceImportPrefix || strings.HasPrefix(importPath, deletedServiceImportPrefix+"/") {
			t.Fatalf("module still imports deleted service path: %s", importPath)
		}
	}
}

func TestServiceDeletionProof_DeletedPathsAbsentFromOwnershipInventory(t *testing.T) {
	t.Parallel()

	root := serviceDeletionRepoRoot(t)
	inventory, err := ownershipinventory.Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, row := range inventory.Packages {
		if strings.Contains(row.PackagePath, "factory_runtime/service") {
			t.Fatalf("ownership inventory still lists deleted service path: %s", row.PackagePath)
		}
	}
}

func TestServiceDeletionProof_WireConstructsPublishedControlObservationDispatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, err := factoryruntimewire.NewService(
		func() string { return "del-run-service-proof-id" },
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

	_, err = root.PlanDispatch(ctx, factoryruntime.PlanDispatchRequest{DispatchID: "del-run-proof-dispatch"})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("PlanDispatch() error = %v, want ErrNotRunning", err)
	}

	_, err = root.ControlPause(ctx, factoryruntime.PauseRequest{})
	if !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("ControlPause() error = %v, want ErrNotRunning", err)
	}
}

func TestServiceDeletionProof_PipelinePublicPackagesRemain(t *testing.T) {
	t.Parallel()

	root := serviceDeletionRepoRoot(t)
	runtimeRoot := filepath.Join(root, "pkg", "services", "factory_runtime")
	for _, name := range []string{
		"build", "engine", "javascript", "runtime", "scheduler", "state", "subsystems", "token",
	} {
		path := filepath.Join(runtimeRoot, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("pipeline public package %q missing: %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("pipeline public package %q is not a directory", name)
		}
	}
}

func TestServiceDeletionProof_CheckpointPackagesRemainUndisturbed(t *testing.T) {
	t.Parallel()

	root := serviceDeletionRepoRoot(t)
	runtimeRoot := filepath.Join(root, "pkg", "services", "factory_runtime")
	for _, name := range []string{"checkpointstore", "checkpointsummary"} {
		path := filepath.Join(runtimeRoot, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("checkpoint public package %q missing (IMP-RUN-04 must remain undisturbed): %v", name, err)
		}
		if !info.IsDir() {
			t.Fatalf("checkpoint public package %q is not a directory", name)
		}
	}
}

func serviceDeletionRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := ownershipinventory.FindRepositoryRoot()
	if err != nil {
		t.Fatalf("FindRepositoryRoot() error = %v", err)
	}
	return root
}
