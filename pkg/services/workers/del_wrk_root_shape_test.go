package workers_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/runner"
)

// DEL-WRK story 004 proves the Workers root matches canonical shape plus thin
// contracts after transitional shim deletion. Deeper runtime-assembly,
// workstation, and runner behavioral proofs live in wire/wire_test.go and
// wire/construction_boundary_test.go.

var canonicalWorkersRootDirs = []string{"internal", "wire"}

var workersInternalSubservices = []string{
	"runners",
	"runtime_assembly",
	"workstations",
}

var providersExtractionTopLevelDirsWithTests = []string{
	"agypty",
	"cliprovider",
	"provider",
	"provider_test",
}

func TestDelWrkRootShape_CompletionInvariants(t *testing.T) {
	t.Parallel()

	root := delWrkRepoRoot(t)
	workersDir := workersRootDir(t)
	manifest := loadDelWrkDeleteReadyInventoryManifest(t)

	t.Run("deleted_transitional_packages_absent", func(t *testing.T) {
		t.Parallel()
		for _, relative := range manifest.DeleteReadyRelativeDirs {
			relative := relative
			t.Run(relative, func(t *testing.T) {
				t.Parallel()

				packageDir := filepath.Join(workersDir, filepath.FromSlash(relative))
				if relative == "executor" {
					goFiles, err := listGoSourceFiles(packageDir)
					if err != nil {
						if os.IsNotExist(err) {
							return
						}
						t.Fatalf("list Go sources in %s: %v", relative, err)
					}
					if len(goFiles) != 0 {
						t.Fatalf("%s Go sources = %v, want no package files after deletion", relative, goFiles)
					}
					return
				}

				_, err := os.Stat(packageDir)
				if !os.IsNotExist(err) {
					t.Fatalf("deleted transitional package %s must be absent; stat = %v", relative, err)
				}
			})
		}
	})

	t.Run("canonical_root_directories_present", func(t *testing.T) {
		t.Parallel()

		entries, err := os.ReadDir(workersDir)
		if err != nil {
			t.Fatalf("ReadDir(%q) = %v", workersDir, err)
		}
		var gotRootDirs []string
		for _, entry := range entries {
			if entry.IsDir() {
				gotRootDirs = append(gotRootDirs, entry.Name())
			}
		}
		for _, want := range canonicalWorkersRootDirs {
			if !slices.Contains(gotRootDirs, want) {
				t.Fatalf("workers root directories = %v, missing canonical %q", gotRootDirs, want)
			}
		}
	})

	t.Run("providers_extraction_sources_remain", func(t *testing.T) {
		t.Parallel()
		for _, relative := range providersExtractionTopLevelDirsWithTests {
			if _, err := os.Stat(filepath.Join(workersDir, relative)); err != nil {
				t.Fatalf("Providers extraction source %q must remain at workers top level: %v", relative, err)
			}
		}
	})

	t.Run("internal_services_subservices_remain", func(t *testing.T) {
		t.Parallel()

		subservicesRoot := filepath.Join(workersDir, "internal", "services")
		entries, err := os.ReadDir(subservicesRoot)
		if err != nil {
			t.Fatalf("ReadDir(%q) = %v", subservicesRoot, err)
		}
		var gotSubservices []string
		for _, entry := range entries {
			if entry.IsDir() {
				gotSubservices = append(gotSubservices, entry.Name())
			}
		}
		slices.Sort(gotSubservices)
		wantSubservices := slices.Clone(workersInternalSubservices)
		slices.Sort(wantSubservices)
		if !slices.Equal(gotSubservices, wantSubservices) {
			t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
		}
	})

	t.Run("thin_root_contract_inventory_sealed", func(t *testing.T) {
		t.Parallel()

		live, err := ownershipinventory.ListWorkersRootGoFiles(root)
		if err != nil {
			t.Fatalf("ListWorkersRootGoFiles() error = %v", err)
		}
		if len(live) != ownershipinventory.WorkersRootContractBaselineFileCount {
			t.Fatalf(
				"live root .go file count = %d, want post-cutover baseline %d",
				len(live),
				ownershipinventory.WorkersRootContractBaselineFileCount,
			)
		}
		want := ownershipinventory.WorkersRootContractInventory()
		if !slices.Equal(live, want) {
			t.Fatalf("live root .go files = %v, want committed inventory %v", live, want)
		}
	})

	t.Run("wire_construction_bridge_present", func(t *testing.T) {
		t.Parallel()
		if _, err := os.Stat(filepath.Join(workersDir, "wire", "wire.go")); err != nil {
			t.Fatalf("wire/wire.go must construct Workers; stat = %v", err)
		}
	})
}

func TestDelWrkRootShape_UnexpectedChildrenRemainMoveDebtOnly(t *testing.T) {
	t.Parallel()

	root := delWrkRepoRoot(t)
	live, err := ownershipinventory.ListOwnerTopLevelChildren(root, "workers")
	if err != nil {
		t.Fatalf("ListOwnerTopLevelChildren(workers) = %v", err)
	}
	spec, ok := ownershipinventory.OwnerTopLevelSpecFor("workers")
	if !ok {
		t.Fatal("OwnerTopLevelSpecFor(workers) ok = false")
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

	manifest := loadDelWrkDeleteReadyInventoryManifest(t)
	heldBackTopLevel := delWrkHeldBackTopLevelDirs(manifest.HeldBack)
	for _, deleted := range manifest.DeleteReadyRelativeDirs {
		top, _, _ := strings.Cut(deleted, "/")
		if slices.Contains(heldBackTopLevel, top) {
			continue
		}
		if slices.Contains(live, top) && top != "executor" {
			t.Fatalf("deleted transitional package %q must not remain as a public top-level directory", top)
		}
		if top == "executor" && deleted == "executor" {
			packageDir := filepath.Join(root, "pkg", "services", "workers", "executor")
			goFiles, err := listGoSourceFiles(packageDir)
			if err != nil && !os.IsNotExist(err) {
				t.Fatalf("list Go sources in executor/: %v", err)
			}
			if len(goFiles) != 0 {
				t.Fatalf("executor/ Go sources = %v, want no package files after parent shim deletion", goFiles)
			}
		}
	}
}

func delWrkHeldBackTopLevelDirs(entries []delWrkHeldBackEntry) []string {
	topLevel := make([]string, 0, len(entries))
	for _, entry := range entries {
		top, _, _ := strings.Cut(entry.RelativeDir, "/")
		if top != "" {
			topLevel = append(topLevel, top)
		}
	}
	slices.Sort(topLevel)
	return slices.Compact(topLevel)
}

func TestDelWrkRootShape_WireDoesNotImportDeletedTransitionalPackages(t *testing.T) {
	t.Parallel()

	const wirePackage = "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", wirePackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", wirePackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, deleted := range deletedTransitionalWorkersImportPaths() {
			if importPath != deleted && !strings.HasPrefix(importPath, deleted+"/") {
				continue
			}
			t.Fatalf(
				"%s must not import deleted transitional package %s; use internal/services/* owner paths",
				wirePackage,
				importPath,
			)
		}
	}
}

func TestDelWrkRootShape_WireConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	service, err := delWrkRootShapeValidNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root workers.Service = service
	if root == nil {
		t.Fatal("constructed root is not assignable to workers.Service")
	}
}

func TestDelWrkRootShape_RuntimeAssemblyWorkstationAndRunnerPathsRemainReachable(t *testing.T) {
	t.Parallel()

	service, err := delWrkRootShapeValidNewServiceInputs().callNewService()
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.BuildRuntime(t.Context(), workers.RuntimeBuildRequest{
		RunnerID: runners.AgentIdentity,
		Roles: []workers.RuntimeBuildRoleRequest{
			{Name: "writer", Kind: workers.RuntimeBuildRoleKindWorker},
			{Name: "review", Kind: workers.RuntimeBuildRoleKindWorkstation},
		},
	})
	if err != nil {
		t.Fatalf("BuildRuntime(agent) error = %v", err)
	}
	if result.RunnerSelection.RunnerID != runners.AgentIdentity {
		t.Fatalf("BuildRuntime(agent) selection = %#v, want agent runner", result.RunnerSelection)
	}
	if len(result.Bindings) != 2 {
		t.Fatalf("BuildRuntime(agent) bindings = %#v, want two", result.Bindings)
	}

	selection := workerrunner.ResolveRunnerSelection(" opencode ", workers.RunnerIDGemini, workers.RunnerIDCodex)
	if selection.RunnerID != workers.RunnerIDOpenCode {
		t.Fatalf("runner selection = %#v, want opencode from workstation override", selection)
	}

	ctx := context.Background()
	started, err := service.StartWorkstationPool(ctx, workers.WorkstationPoolStartRequest{
		Bindings: []workers.AssembledRuntimeBinding{
			{RoleName: "review", RoleKind: workers.RuntimeBuildRoleKindWorkstation},
		},
	})
	if err != nil {
		t.Fatalf("StartWorkstationPool() error = %v", err)
	}
	if started.Outcome != workers.WorkstationPoolLifecycleOutcomeStarted {
		t.Fatalf("StartWorkstationPool() outcome = %q, want STARTED", started.Outcome)
	}
}

type delWrkRootShapeNewServiceInputs struct {
	agentDependencies     runners.AgentDependencies
	scriptConfig          runners.ScriptConfig
	scriptDependencies    runners.ScriptDependencies
	inferenceConfig       runners.InferenceConfig
	inferenceDependencies runners.InferenceDependencies
}

func delWrkRootShapeValidNewServiceInputs() delWrkRootShapeNewServiceInputs {
	return delWrkRootShapeNewServiceInputs{
		agentDependencies: runners.AgentDependencies{
			Providers: &delWrkRootShapeProvidersFake{},
			Publish:   func(workers.ProgressFragment) {},
		},
		scriptConfig: runners.ScriptConfig{
			Command:          "fixture",
			Args:             []string{"arg"},
			FactoryDirectory: "factory-root",
		},
		scriptDependencies: runners.ScriptDependencies{
			CommandRunner: &delWrkRootShapeCommandRunner{},
			FactoryDocs:   func(string) (map[string]string, error) { return map[string]string{}, nil },
			Now:           func() time.Time { return time.Unix(1, 0) },
			Publish:       func(workers.ProgressFragment) {},
			Record:        func(workers.ScriptEvent) {},
		},
		inferenceConfig: runners.InferenceConfig{
			Worker: models.LocalWorker{
				Name:  "inference-worker",
				Type:  interfaces.WorkerTypeInference,
				Model: "WHISPER",
			},
			Resources: []models.LocalResource{{
				Name: "gpu",
				Type: "gpu",
			}},
		},
		inferenceDependencies: runners.InferenceDependencies{
			Models: &delWrkRootShapeInferenceInvoker{},
		},
	}
}

func (in delWrkRootShapeNewServiceInputs) callNewService() (workers.Service, error) {
	return workerswire.NewService(
		in.agentDependencies,
		in.scriptConfig,
		in.scriptDependencies,
		in.inferenceConfig,
		in.inferenceDependencies,
	)
}

type delWrkRootShapeProvidersFake struct{}

func (*delWrkRootShapeProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{Content: "fixture"}, nil
}

func (*delWrkRootShapeProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (*delWrkRootShapeProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

type delWrkRootShapeCommandRunner struct{}

func (*delWrkRootShapeCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{}, nil
}

func (*delWrkRootShapeCommandRunner) RunStreaming(
	context.Context,
	workers.CommandRequest,
	platformprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	return workers.CommandResult{ExitCode: 0}, nil
}

type delWrkRootShapeInferenceInvoker struct{}

func (*delWrkRootShapeInferenceInvoker) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{}, nil
}

var _ providers.Service = (*delWrkRootShapeProvidersFake)(nil)
