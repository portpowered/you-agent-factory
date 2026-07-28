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

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	factorydefinitionsinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

// DEL-DEF story 005 (pss-del-def-005) proves emptied transitional packages are
// gone, remaining children trend toward wire/ + internal/ + transports/, parent-
// private internal/services/* subservices remain, and factory_definitions/wire
// constructs the published Service root without deleted transitional imports.
// Deeper catalog/authoring/validate/snapshot/distribute behavioral proofs live in
// wire/fold_behavior_preservation_test.go.

var deletedTransitionalTopLevelDirs = []string{
	"authoredlayout",
	"portableconfig",
	"loading",
	"namedfactories",
	"runtimeconfig",
}

var canonicalDefinitionsRootDirs = []string{"internal", "transports", "wire"}

var definitionsInternalSubservices = []string{
	"catalog",
	"authoring_layout",
	"compilation",
	"validation",
	"snapshots_portability",
	"distribution",
	"invocation_policy",
}

func TestDelDefRootShape_CompletionInvariants(t *testing.T) {
	t.Parallel()

	serviceRoot := definitionsServiceRootDir(t)

	t.Run("deleted_transitional_top_level_packages_absent", func(t *testing.T) {
		t.Parallel()
		for _, relativeDir := range deletedTransitionalTopLevelDirs {
			_, err := os.Stat(filepath.Join(serviceRoot, relativeDir))
			if !os.IsNotExist(err) {
				t.Fatalf("transitional package %s must be deleted; stat = %v", relativeDir, err)
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

	t.Run("service_shim_held_under_automations_lease", func(t *testing.T) {
		t.Parallel()
		info, err := os.Stat(filepath.Join(serviceRoot, "service"))
		if err != nil {
			t.Fatalf("service compile shim must remain while root pkg/wire holds Automations lease; stat = %v", err)
		}
		if !info.IsDir() {
			t.Fatal("service path must remain a directory while root pkg/wire holds Automations lease")
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

func TestDelDefRootShape_WireDoesNotImportDeletedTransitionalPackages(t *testing.T) {
	t.Parallel()

	const wirePackage = "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	cmd := exec.Command("go", "list", "-f", "{{.Imports}}", wirePackage)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list imports for %s: %v\n%s", wirePackage, err, output)
	}

	imports := strings.Fields(strings.Trim(string(output), "[]"))
	for _, importPath := range imports {
		for _, deleted := range deletedTransitionalImports {
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

func TestDelDefRootShape_WireConstructsPublishedServiceSurfaces(t *testing.T) {
	t.Parallel()

	packagedCatalog, err := factorydefinitionsinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/del-def-root-shape",
		Project: "del-def-root-shape",
		JSON:    []byte(`{"name":"del-def-root-shape"}`),
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
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "@you/del-def-root-shape" {
		t.Fatalf("ListBuiltInPackagedFactories() = %#v, want one del-def-root-shape entry", listed.Entries)
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
}

type stubDelDefRootShapeSessionHost struct{}

func (stubDelDefRootShapeSessionHost) PersistRootDir() string { return "" }
func (stubDelDefRootShapeSessionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}
func (stubDelDefRootShapeSessionHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	return nil
}
func (stubDelDefRootShapeSessionHost) WorkflowID() string { return "" }
func (stubDelDefRootShapeSessionHost) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return nil, errors.New("session not found")
}
func (stubDelDefRootShapeSessionHost) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	return nil, errors.New("session not found")
}
func (stubDelDefRootShapeSessionHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return ""
}
func (stubDelDefRootShapeSessionHost) ValidateEditableFactorySnapshot(context.Context, *factorydefinitions.FactorySnapshot) error {
	return nil
}
func (stubDelDefRootShapeSessionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return nil, errors.New("session not found")
}
func (stubDelDefRootShapeSessionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

type stubDelDefRootShapeActivationGateway struct{}

func (stubDelDefRootShapeActivationGateway) RunSessionID() string { return "" }
func (stubDelDefRootShapeActivationGateway) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return nil
}
func (stubDelDefRootShapeActivationGateway) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return nil, errors.New("session not found")
}
func (stubDelDefRootShapeActivationGateway) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return ""
}
func (stubDelDefRootShapeActivationGateway) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return "", ""
}
func (stubDelDefRootShapeActivationGateway) SaveNow() time.Time { return time.Unix(0, 0) }
func (stubDelDefRootShapeActivationGateway) WithActivationLock(fn func() error) error { return fn() }
func (stubDelDefRootShapeActivationGateway) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}
func (stubDelDefRootShapeActivationGateway) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorydefinitions.DefinitionSession) error {
	return nil
}
func (stubDelDefRootShapeActivationGateway) ActivateSessionEditableFactory(
	context.Context,
	*factorydefinitions.DefinitionSession,
	string, string, string, string, string,
) error {
	return nil
}
func (stubDelDefRootShapeActivationGateway) SwapPersistedNamedFactoryRuntime(
	context.Context,
	string,
	*factorydefinitions.DefinitionSession,
	string, string, string, string,
) error {
	return nil
}

type stubDelDefRootShapeValidator struct{}

func (stubDelDefRootShapeValidator) Validate(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}
func (stubDelDefRootShapeValidator) ValidateBlockingLoad(context.Context, *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}
func (stubDelDefRootShapeValidator) ValidateTopology(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RequiredToolChecker,
) factorydefinitions.TopologyValidationResult {
	return factorydefinitions.TopologyValidationResult{}
}
func (stubDelDefRootShapeValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*factorydefinitions.FactoryConfig,
) []factorydefinitions.ValidationTarget {
	return nil
}
func (stubDelDefRootShapeValidator) WorkTypeHandlingBehavior(
	context.Context,
	*factorydefinitions.FactoryConfig,
	bool,
) []factorydefinitions.ValidationTarget {
	return nil
}
func (stubDelDefRootShapeValidator) PruneLayout(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.PendingFactoryGraphTopology,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}

type stubDelDefRootShapePersistence struct{}

func (stubDelDefRootShapePersistence) PersistNamedFactory(
	context.Context,
	factorydefinitions.NamedFactoryPersistenceRequest,
) (factorydefinitions.NamedFactoryPersistenceResult, error) {
	return factorydefinitions.NamedFactoryPersistenceResult{}, nil
}
func (stubDelDefRootShapePersistence) PrepareFactoryLayout(
	context.Context,
	string,
	[]byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return nil, nil
}
func (stubDelDefRootShapePersistence) ValidateFactoryLayout(string) error { return nil }
func (stubDelDefRootShapePersistence) FlattenFactoryLayout(string) ([]byte, error) {
	return nil, nil
}
func (stubDelDefRootShapePersistence) ExpandFactoryLayout(string) (string, factorydefinitions.LayoutExpansionReport, error) {
	return "", factorydefinitions.LayoutExpansionReport{}, nil
}
func (stubDelDefRootShapePersistence) CreateNamedFactory(string, string, *factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
	return "", nil
}
func (stubDelDefRootShapePersistence) ReplaceNamedFactory(string, string, *factorydefinitions.PreparedFactoryLayoutPayload) (string, error) {
	return "", nil
}
func (stubDelDefRootShapePersistence) ReplaceFactoryLayout(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

type stubDelDefRootShapeNamedPaths struct{}

func (stubDelDefRootShapeNamedPaths) ResolveCandidatePaths(string, string, string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	return factorydefinitions.NamedFactoryCandidatePaths{}, nil
}
func (stubDelDefRootShapeNamedPaths) ResolveExistingDir(string, string) (string, error) {
	return "", factorydefinitions.ErrNamedFactoryNotFound
}
func (stubDelDefRootShapeNamedPaths) RequireDefinitionDir(string) error { return errors.New("not found") }
func (stubDelDefRootShapeNamedPaths) ResolveCurrentDir(string) (string, error) {
	return "", errors.New("not found")
}
func (stubDelDefRootShapeNamedPaths) ReadCurrentPointer(string) (string, error) {
	return "", errors.New("not found")
}
func (stubDelDefRootShapeNamedPaths) WriteCurrentPointer(string, string) error { return nil }

type stubDelDefRootShapeRequiredToolChecker struct{}

func (stubDelDefRootShapeRequiredToolChecker) Check(
	factorydefinitions.RequiredToolConfig,
) factorydefinitions.RequiredToolCheckResult {
	return factorydefinitions.RequiredToolCheckResult{}
}

type stubDelDefRootShapeOrchestratorValidator struct{}

func (stubDelDefRootShapeOrchestratorValidator) ValidateJavaScriptFactoryDefinition(
	context.Context,
	*factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	factorydefinitions.WorkflowSourceReader,
) []factorydefinitions.ValidationTarget {
	return nil
}

type stubDelDefRootShapeDirectoryReplacementStore struct{}

func (stubDelDefRootShapeDirectoryReplacementStore) Commit(string, string, string) (string, error) {
	return "", nil
}
func (stubDelDefRootShapeDirectoryReplacementStore) Restore(string, string) {}
