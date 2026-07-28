package factory_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// DEL-RUN-ENGINE-PIPELINE story 006 proves the thin Runtime root, wire
// construction, and reduced structure debt for deleted engine/pipeline packages.
// Each subtest asserts one observable completion invariant for reviewers.
// Baseline burn-down subtests live in wire/engine_pipeline_baseline_gate_test.go;
// deletion and test-support proofs live in sibling wire proof tests.

func TestEnginePipelineThinRootProofGate_EndToEndCompletionInvariants(t *testing.T) {
	root := testutil.MustRepoRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "factory_runtime")

	t.Run("canonical_root_directories", func(t *testing.T) {
		t.Parallel()

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
		for _, moved := range []string{"testkit", "exhaustiontests"} {
			if slices.Contains(gotRootDirs, moved) {
				t.Fatalf("internalized public test-support directory %q must not remain at Runtime root", moved)
			}
		}
	})

	t.Run("unexpected_root_children_recorded_as_move_debt_only", func(t *testing.T) {
		t.Parallel()

		live, err := ownershipinventory.ListOwnerTopLevelChildren(root, "factory_runtime")
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
		}
	})

	t.Run("deleted_public_pipeline_directories_absent", func(t *testing.T) {
		t.Parallel()

		for _, name := range foldedEnginePipelineTopLevelChildren() {
			path := filepath.Join(serviceRoot, name)
			if _, err := os.Stat(path); err == nil {
				t.Fatalf("deleted public pipeline package %q still exists at %s", name, path)
			} else if !os.IsNotExist(err) {
				t.Fatalf("stat %s: %v", path, err)
			}
		}
	})

	t.Run("service_directory_absent", func(t *testing.T) {
		t.Parallel()

		for _, rel := range []string{"service", filepath.Join("service", "host")} {
			path := filepath.Join(serviceRoot, rel)
			if _, err := os.Stat(path); err == nil {
				t.Fatalf("deleted public package directory still exists: %s", path)
			} else if !os.IsNotExist(err) {
				t.Fatalf("stat %s: %v", path, err)
			}
		}
	})

	t.Run("checkpoint_recovery_undisturbed", func(t *testing.T) {
		t.Parallel()

		recoveryRoot := filepath.Join(serviceRoot, "internal", "services", "checkpoint_recovery")
		info, err := os.Stat(recoveryRoot)
		if err != nil {
			t.Fatalf("checkpoint_recovery nested service missing (IMP-RUN-04 must remain undisturbed): %v", err)
		}
		if !info.IsDir() {
			t.Fatal("checkpoint_recovery nested service is not a directory")
		}
	})

	t.Run("wire_constructs_published_control_observation_dispatch", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		service, err := factoryruntimewire.NewService(
			func() string { return "del-run-engine-pipeline-thin-root-proof-id" },
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
		var published factoryruntime.Service = service
		if published == nil {
			t.Fatal("NewService() returned nil published Service root")
		}

		_, err = published.Observe(ctx, factoryruntime.ObserveRequest{
			Scope: factoryruntime.ObservationScopeStatus,
		})
		if !errors.Is(err, factoryruntime.ErrNotRunning) {
			t.Fatalf("Observe(STATUS) error = %v, want ErrNotRunning", err)
		}

		_, err = published.PlanDispatch(ctx, factoryruntime.PlanDispatchRequest{
			DispatchID: "del-run-engine-pipeline-thin-root-proof-dispatch",
		})
		if !errors.Is(err, factoryruntime.ErrNotRunning) {
			t.Fatalf("PlanDispatch() error = %v, want ErrNotRunning", err)
		}

		_, err = published.ControlPause(ctx, factoryruntime.PauseRequest{})
		if !errors.Is(err, factoryruntime.ErrNotRunning) {
			t.Fatalf("ControlPause() error = %v, want ErrNotRunning", err)
		}
	})

	t.Run("ownership_inventory_omits_deleted_public_pipeline_packages", func(t *testing.T) {
		t.Parallel()

		inventory, err := ownershipinventory.Load(root)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		for _, row := range inventory.Packages {
			for _, deleted := range foldedEnginePipelineTopLevelChildren() {
				needle := "factory_runtime/" + deleted
				if strings.Contains(row.PackagePath, needle) {
					t.Fatalf("ownership inventory still lists deleted pipeline path: %s", row.PackagePath)
				}
			}
		}
	})

	t.Run("package_structure_baseline_omits_deleted_public_pipeline_directories", func(t *testing.T) {
		t.Parallel()

		for _, entry := range loadPackageStructureBaselineEntries(t, root) {
			for _, deleted := range foldedEnginePipelineTopLevelChildren() {
				deletedPath := "pkg/services/factory_runtime/" + deleted
				if entry.FilePath == deletedPath || strings.HasPrefix(entry.FilePath, deletedPath+"/") {
					t.Fatalf(
						"package-structure baseline still lists deleted pipeline path %q under rule %q",
						entry.FilePath,
						entry.Rule,
					)
				}
			}
		}
	})

	t.Run("package_target_manifest_omits_deleted_public_pipeline_packages", func(t *testing.T) {
		t.Parallel()

		manifest := loadPackageTargetManifestBaseline(t, root)
		for _, packagePath := range manifest.Inventory {
			for _, deleted := range foldedEnginePipelineTopLevelChildren() {
				deletedPath := "pkg/services/factory_runtime/" + deleted
				if packagePath == deletedPath || strings.HasPrefix(packagePath, deletedPath+"/") {
					t.Fatalf("package-target manifest inventory still lists deleted pipeline package %q", packagePath)
				}
			}
		}
		for _, row := range manifest.Packages {
			for _, deleted := range foldedEnginePipelineTopLevelChildren() {
				deletedPath := "pkg/services/factory_runtime/" + deleted
				if row.PackagePath == deletedPath || strings.HasPrefix(row.PackagePath, deletedPath+"/") {
					t.Fatalf("package-target manifest packages still list deleted pipeline package %q", row.PackagePath)
				}
			}
		}
	})

	t.Run("internal_services_layout", func(t *testing.T) {
		t.Parallel()

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
	})
}

type packageTargetManifestBaseline struct {
	Inventory []string `json:"inventory"`
	Packages  []struct {
		PackagePath string `json:"packagePath"`
	} `json:"packages"`
}

type packageStructureBaselineEntry struct {
	Rule     string `json:"rule"`
	FilePath string `json:"filePath"`
}

func loadPackageTargetManifestBaseline(t *testing.T, root string) packageTargetManifestBaseline {
	t.Helper()

	path := filepath.Join(root, "docs", "internal", "packaged-service-structure", "package-target-manifest.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var manifest packageTargetManifestBaseline
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return manifest
}

func loadPackageStructureBaselineEntries(t *testing.T, root string) []packageStructureBaselineEntry {
	t.Helper()

	path := filepath.Join(root, "docs", "internal", "baselines", "package-structure-baseline.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	var baseline struct {
		Entries []packageStructureBaselineEntry `json:"entries"`
	}
	if err := json.Unmarshal(payload, &baseline); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return baseline.Entries
}
