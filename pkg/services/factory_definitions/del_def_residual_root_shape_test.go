package factorydefinitions_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/ownershipinventory"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// DEL-DEF-RESIDUAL story 005 proves emptied residual transitional packages are
// gone, remaining children trend toward wire/ + internal/ + transports/ plus
// committed move debt, parent-private internal/services/* subservices remain,
// factory_definitions/wire constructs the published Service root without
// deleted transitional imports, and invocation_policy contracts remain reachable
// through the Definitions wire surface.

func TestDelDefResidualRootShape_CompletionInvariants(t *testing.T) {
	t.Parallel()

	root := delDefResidualRepoRoot(t)
	serviceRoot := filepath.Join(root, "pkg", "services", "factory_definitions")

	t.Run("deleted_residual_transitional_top_level_packages_absent", func(t *testing.T) {
		t.Parallel()
		for _, relativeDir := range residualTransitionalPublicDirsHeldUntilDeletion() {
			_, err := os.Stat(filepath.Join(serviceRoot, relativeDir))
			if !os.IsNotExist(err) {
				t.Fatalf("residual transitional package %s must be deleted; stat = %v", relativeDir, err)
			}
		}
	})

	t.Run("deleted_del_def_transitional_top_level_packages_absent", func(t *testing.T) {
		t.Parallel()
		for _, relativeDir := range deletedTransitionalTopLevelDirs {
			_, err := os.Stat(filepath.Join(serviceRoot, relativeDir))
			if !os.IsNotExist(err) {
				t.Fatalf("DEL-DEF transitional package %s must be deleted; stat = %v", relativeDir, err)
			}
		}
	})

	t.Run("canonical_root_directories_present", func(t *testing.T) {
		t.Parallel()
		entries, err := os.ReadDir(serviceRoot)
		if err != nil {
			t.Fatalf("ReadDir(%q) = %v", serviceRoot, err)
		}
		var gotRootDirs []string
		for _, entry := range entries {
			if entry.IsDir() {
				gotRootDirs = append(gotRootDirs, entry.Name())
			}
		}
		for _, want := range canonicalDefinitionsRootDirs {
			if !slices.Contains(gotRootDirs, want) {
				t.Fatalf("service root directories = %v, missing canonical %q", gotRootDirs, want)
			}
		}
	})

	t.Run("internal_services_subservices_remain", func(t *testing.T) {
		t.Parallel()
		subservicesRoot := filepath.Join(serviceRoot, "internal", "services")
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
		wantSubservices := slices.Clone(definitionsInternalSubservices)
		slices.Sort(wantSubservices)
		if !slices.Equal(gotSubservices, wantSubservices) {
			t.Fatalf("internal/services directories = %v, want %v", gotSubservices, wantSubservices)
		}
	})

	t.Run("del_def_overlap_paths_remain_under_automations_lease", func(t *testing.T) {
		t.Parallel()
		for _, relativeDir := range []string{"service", "definition"} {
			info, err := os.Stat(filepath.Join(serviceRoot, relativeDir))
			if err != nil {
				t.Fatalf("%s must remain while Automations/DEL-DEF leases hold; stat = %v", relativeDir, err)
			}
			if !info.IsDir() {
				t.Fatalf("%s must remain a directory while Automations/DEL-DEF leases hold", relativeDir)
			}
		}
	})

	t.Run("wire_construction_bridge_present", func(t *testing.T) {
		t.Parallel()
		_, err := os.Stat(filepath.Join(serviceRoot, "wire", "wire.go"))
		if err != nil {
			t.Fatalf("wire/wire.go must construct Definitions; stat = %v", err)
		}
	})
}

func TestDelDefResidualRootShape_UnexpectedChildrenRemainMoveDebtOnly(t *testing.T) {
	t.Parallel()

	root := delDefResidualRepoRoot(t)
	live, err := ownershipinventory.ListOwnerTopLevelChildren(root, "factory_definitions")
	if err != nil {
		t.Fatalf("ListOwnerTopLevelChildren(factory_definitions) = %v", err)
	}
	spec, ok := ownershipinventory.OwnerTopLevelSpecFor("factory_definitions")
	if !ok {
		t.Fatal("OwnerTopLevelSpecFor(factory_definitions) ok = false")
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

	for _, deleted := range residualTransitionalPublicDirsHeldUntilDeletion() {
		if slices.Contains(live, deleted) {
			t.Fatalf("deleted residual transitional package %q must not remain as a public top-level directory", deleted)
		}
	}
	for _, deleted := range deletedTransitionalTopLevelDirs {
		if slices.Contains(live, deleted) {
			t.Fatalf("deleted DEL-DEF transitional package %q must not remain as a public top-level directory", deleted)
		}
	}
}

func TestDelDefResidualRootShape_WireDoesNotImportDeletedTransitionalPackages(t *testing.T) {
	t.Parallel()

	const wirePackage = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", wirePackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", wirePackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	forbidden := append(
		slices.Clone(deletedTransitionalImports),
		deletedTransitionalResidualImports...,
	)
	for _, importPath := range imports {
		for _, deleted := range forbidden {
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

func TestDelDefResidualRootShape_WireConstructsPublishedServiceSurfaces(t *testing.T) {
	t.Parallel()

	packagedCatalog, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/del-def-residual-root-shape",
		Project: "del-def-residual-root-shape",
		JSON:    []byte(`{"name":"del-def-residual-root-shape"}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}

	service, err := factorydefinitionswire.NewService(
		stubDelDefRootShapeSessionHost{},
		stubDelDefRootShapeActivationGateway{},
		stubDelDefRootShapeValidator{},
		stubDelDefRootShapePersistence{},
		&compilationloading.Loader{},
		func(string, *factorydefinitions.FactoryConfig, bool, bool) error { return nil },
		func(string, *factorydefinitions.FactoryConfig) error { return nil },
		stubDelDefRootShapeNamedPaths{},
		platformfilesystem.Local{},
		factorydefinitionswire.StaticClock(time.Unix(0, 0)),
		platformfilesystem.Local{},
		func(
			context.Context,
			factorydefinitions.ListEffectiveFactoriesRequest,
		) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{}, nil
		},
		packagedCatalog,
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		stubDelDefRootShapeRequiredToolChecker{},
		stubDelDefRootShapeOrchestratorValidator{},
		platformfilesystem.Local{},
		stubDelDefRootShapeDirectoryReplacementStore{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	var root factorydefinitions.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to factorydefinitions.Service")
	}

	listed, err := root.ListBuiltInPackagedFactories(
		t.Context(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories() error = %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "@you/del-def-residual-root-shape" {
		t.Fatalf("ListBuiltInPackagedFactories() = %#v, want one del-def-residual-root-shape entry", listed.Entries)
	}

	_, invalidErr := root.CompileEffectiveFactorySource(
		context.Background(),
		factorydefinitions.CompileEffectiveFactorySourceRequest{Canonical: []byte("{")},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidAuthoredFactorySource) {
		t.Fatalf(
			"CompileEffectiveFactorySource(invalid) error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidAuthoredFactorySource,
		)
	}

	_, snapshotErr := root.CaptureFactorySnapshot(
		t.Context(),
		factorydefinitions.CaptureFactorySnapshotRequest{Canonical: []byte(`"not-object"`)},
	)
	if !errors.Is(snapshotErr, factorydefinitions.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf(
			"CaptureFactorySnapshot(invalid) error = %v, want %v",
			snapshotErr,
			factorydefinitions.ErrInvalidFactorySnapshotPayload,
		)
	}

	_, validateErr := root.ValidateStructuralFactoryDefinition(
		t.Context(),
		factorydefinitions.ValidateStructuralFactoryDefinitionRequest{Canonical: []byte("{")},
	)
	if !errors.Is(validateErr, factorydefinitions.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf(
			"ValidateStructuralFactoryDefinition(invalid) error = %v, want %v",
			validateErr,
			factorydefinitions.ErrInvalidFactoryDefinitionPayload,
		)
	}
}

func TestDelDefResidualRootShape_InvocationPolicyContractThroughDefinitionsWire(t *testing.T) {
	t.Parallel()

	ports, err := factorydefinitionswire.InvocationPolicyPortsFromNestedOwner()
	if err != nil {
		t.Fatalf("InvocationPolicyPortsFromNestedOwner() error = %v", err)
	}
	if ports.DecisionEnvelope == nil {
		t.Fatal("DecisionEnvelope is nil")
	}

	workstation := &factorydefinitions.FactoryWorkstationConfig{
		OutcomeFormat: factorydefinitions.DecisionEnvelopeOutcomeFormat,
	}
	if !ports.DecisionEnvelope.UsesDecisionEnvelopeOutcome(workstation) {
		t.Fatal("UsesDecisionEnvelopeOutcome() = false, want true for decision-envelope workstation")
	}

	raw := `{"decision":"ACCEPTED","feedback":"Residual deletion preserved invocation_policy.","output":"done"}`
	result := ports.DecisionEnvelope.WorkResultFromDecisionEnvelopeJSONOrFailed(
		"dispatch-residual",
		"transition-residual",
		raw,
	)
	if result.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("WorkResultFromDecisionEnvelopeJSONOrFailed() outcome = %q, want %q", result.Outcome, workerexecution.OutcomeAccepted)
	}
	if result.Feedback != "Residual deletion preserved invocation_policy." {
		t.Fatalf(
			"WorkResultFromDecisionEnvelopeJSONOrFailed() feedback = %q, want %q",
			result.Feedback,
			"Residual deletion preserved invocation_policy.",
		)
	}
}
