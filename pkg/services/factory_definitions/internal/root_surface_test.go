package internal_test

import (
	"context"
	"io/fs"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
	invocationpolicyservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy"
	invocationpolicywire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/wire"
)

func TestNewWithAuthoringLayoutConstructsPublishedRootCatalogSurface(t *testing.T) {
	t.Parallel()

	packagedCatalog, err := factoryinternal.NewPackagedFactoryCatalog([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/internal-root",
		Project: "internal-root",
		JSON:    []byte(`{"name":"internal-root"}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}
	invocationPolicy, err := invocationpolicywire.NewService(invocationpolicyservice.Dependencies{})
	if err != nil {
		t.Fatalf("New invocation policy service: %v", err)
	}

	root, err := factoryinternal.NewService(factoryinternal.Dependencies{
		Validator:                     rootSurfaceValidator{},
		DefinitionValidation:          rootSurfaceValidator{},
		EffectiveDefinitionValidation: rootSurfaceValidator{},
		LoadCanonical: func([]byte, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return nil, nil
		},
		NamedPaths:                    rootSurfaceNamedPaths{},
		NamedFactoryCatalogFileSystem: platformfilesystem.Local{},
		PackagedCatalog:               packagedCatalog,
		PackagedInstaller: factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(context.Context, factorydefinitions.PackagedFactoryInstallParams) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		RequiredToolChecker:   rootSurfaceRequiredToolChecker{},
		OrchestratorValidator: rootSurfaceOrchestratorValidator{},
		AuthoringLayout:       stubAuthoringLayout{},
		InvocationPolicy:      invocationPolicy,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if root == nil {
		t.Fatal("NewService() = nil, want composed Factory Definitions root")
	}

	payload := []byte(`{"name":"alpha"}`)
	prepared, err := root.PrepareFactoryLayout(
		t.Context(),
		factorydefinitions.PrepareFactoryLayoutRequest{Name: "alpha", Payload: payload},
	)
	if err != nil {
		t.Fatalf("PrepareFactoryLayout() error = %v", err)
	}
	if string(prepared.Prepared.Canonical) != string(payload) {
		t.Fatalf("PrepareFactoryLayout canonical = %q, want %q", prepared.Prepared.Canonical, payload)
	}

	listed, err := root.ListBuiltInPackagedFactories(
		t.Context(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories() error = %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "@you/internal-root" {
		t.Fatalf("ListBuiltInPackagedFactories() = %#v, want one @you/internal-root entry", listed.Entries)
	}
}

type rootSurfaceValidator struct{}

func (rootSurfaceValidator) ValidateDefinition(
	context.Context,
	factorydefinitions.DefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	return factorydefinitions.ValidationResult{}, nil
}

func (rootSurfaceValidator) ValidateEffectiveDefinition(
	context.Context,
	factorydefinitions.EffectiveDefinitionValidationRequest,
) (factorydefinitions.ValidationResult, error) {
	return factorydefinitions.ValidationResult{}, nil
}

func (rootSurfaceValidator) Validate(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.WorkflowSourceReader,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}
func (rootSurfaceValidator) ValidateBlockingLoad(context.Context, *factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}
func (rootSurfaceValidator) ValidateTopology(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RequiredToolChecker,
) factorydefinitions.TopologyValidationResult {
	return factorydefinitions.TopologyValidationResult{}
}
func (rootSurfaceValidator) WorkerWorkstationBehaviorCompatibility(
	context.Context,
	*factorydefinitions.FactoryConfig,
) []factorydefinitions.ValidationTarget {
	return nil
}
func (rootSurfaceValidator) WorkTypeHandlingBehavior(
	context.Context,
	*factorydefinitions.FactoryConfig,
	bool,
) []factorydefinitions.ValidationTarget {
	return nil
}
func (rootSurfaceValidator) PruneLayout(
	context.Context,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.PendingFactoryGraphTopology,
) factorydefinitions.ValidationResult {
	return factorydefinitions.ValidationResult{}
}

type rootSurfaceNamedPaths struct{}

func (rootSurfaceNamedPaths) ResolveCandidatePaths(string, string, string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	return factorydefinitions.NamedFactoryCandidatePaths{}, nil
}
func (rootSurfaceNamedPaths) ResolveExistingDir(string, string) (string, error) {
	return "/factories", nil
}
func (rootSurfaceNamedPaths) RequireDefinitionDir(string) error { return nil }
func (rootSurfaceNamedPaths) ResolveCurrentDir(string) (string, error) {
	return "", fs.ErrNotExist
}
func (rootSurfaceNamedPaths) ReadCurrentPointer(string) (string, error) { return "", nil }
func (rootSurfaceNamedPaths) WriteCurrentPointer(string, string) error  { return nil }

type rootSurfaceRequiredToolChecker struct{}

func (rootSurfaceRequiredToolChecker) Check(
	factorydefinitions.RequiredToolConfig,
) factorydefinitions.RequiredToolCheckResult {
	return factorydefinitions.RequiredToolCheckResult{}
}

type rootSurfaceOrchestratorValidator struct{}

func (rootSurfaceOrchestratorValidator) ValidateJavaScriptFactoryDefinition(
	context.Context,
	*factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	factorydefinitions.WorkflowSourceReader,
) []factorydefinitions.ValidationTarget {
	return nil
}
