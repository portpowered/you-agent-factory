package factorydefinition

import (
	"context"
	"errors"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type packagedCatalogStub struct{}

func (packagedCatalogStub) ListBuiltInPackagedFactories(
	context.Context,
	factoryroot.ListBuiltInPackagedFactoriesRequest,
) (factoryroot.ListBuiltInPackagedFactoriesResult, error) {
	return factoryroot.ListBuiltInPackagedFactoriesResult{Entries: []factoryroot.BuiltInPackagedFactoryEntry{{
		Name: "@you/goal", Project: "builtin-goal",
		Formats: []factoryroot.PackagedFactoryFormat{factoryroot.PackagedFactoryFormatJSON},
	}}}, nil
}

func (packagedCatalogStub) ResolveBuiltInPackagedFactory(
	context.Context,
	factoryroot.ResolveBuiltInPackagedFactoryRequest,
) (factoryroot.ResolveBuiltInPackagedFactoryResult, error) {
	return factoryroot.ResolveBuiltInPackagedFactoryResult{
		Definition: factoryroot.PackagedDefinition{
			Name: "@you/goal", Project: "builtin-goal",
			Formats: []factoryroot.PackagedFactoryFormat{factoryroot.PackagedFactoryFormatJSON},
		},
		Formats: []factoryroot.PackagedFactoryFormat{factoryroot.PackagedFactoryFormatJSON},
	}, nil
}

func TestService_PromotedUnimplementedRootSlices(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newTestService(stubDefinitionHost{})

	if _, err := svc.ListNamedFactories(ctx, factoryroot.ListNamedFactoriesRequest{}); err == nil {
		t.Fatal("promoted ListNamedFactories: expected collaborator-required error")
	}
	if _, err := svc.GetNamedFactory(ctx, factoryroot.GetNamedFactoryRequest{Name: "missing"}); !errors.Is(err, factoryroot.ErrNamedFactoryNotFound) {
		t.Fatalf("promoted GetNamedFactory: got %v", err)
	}
	if _, err := svc.GetCurrentFactoryPointer(ctx, factoryroot.GetCurrentFactoryPointerRequest{}); !errors.Is(err, factoryroot.ErrCurrentFactoryNotFound) {
		t.Fatalf("promoted GetCurrentFactoryPointer: got %v", err)
	}
	if _, err := svc.SetCurrentFactoryPointer(ctx, factoryroot.SetCurrentFactoryPointerRequest{Name: "alpha"}); !errors.Is(err, factoryroot.ErrNamedFactoryNotFound) {
		t.Fatalf("promoted SetCurrentFactoryPointer: got %v", err)
	}
	if _, err := svc.PrepareFactoryLayout(ctx, factoryroot.PrepareFactoryLayoutRequest{}); !errors.Is(err, factoryroot.ErrMalformedFactoryLayoutPayload) {
		t.Fatalf("promoted PrepareFactoryLayout: got %v", err)
	}
	if _, err := svc.CompileEffectiveFactorySource(ctx, factoryroot.CompileEffectiveFactorySourceRequest{}); !errors.Is(err, factoryroot.ErrInvalidAuthoredFactorySource) {
		t.Fatalf("promoted CompileEffectiveFactorySource: got %v", err)
	}
	if _, err := svc.ValidateStructuralFactoryDefinition(ctx, factoryroot.ValidateStructuralFactoryDefinitionRequest{}); !errors.Is(err, factoryroot.ErrInvalidFactoryDefinitionPayload) {
		t.Fatalf("promoted ValidateStructuralFactoryDefinition: got %v", err)
	}
	if _, err := svc.CaptureFactorySnapshot(ctx, factoryroot.CaptureFactorySnapshotRequest{}); !errors.Is(err, factoryroot.ErrInvalidFactorySnapshotPayload) {
		t.Fatalf("promoted CaptureFactorySnapshot: got %v", err)
	}
	if _, err := svc.InstallPackagedFactory(ctx, factoryroot.InstallPackagedFactoryRequest{Name: "@you/missing"}); !errors.Is(err, factoryroot.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("promoted InstallPackagedFactory: got %v", err)
	}
	if _, err := svc.CreateFactoryScaffold(ctx, factoryroot.CreateFactoryScaffoldRequest{}); !errors.Is(err, factoryroot.ErrFactoryDistributeFailed) {
		t.Fatalf("promoted CreateFactoryScaffold: got %v", err)
	}
}

func TestServiceDelegatesBuiltInCatalogThroughDefinitionsRoot(t *testing.T) {
	t.Parallel()
	svc := NewWithCatalogAndPackages(
		stubDefinitionHost{},
		stubActivationGateway{},
		nil,
		factoryroot.PackagedFactoryCatalogOperations{
			List:    packagedCatalogStub{}.ListBuiltInPackagedFactories,
			Resolve: packagedCatalogStub{}.ResolveBuiltInPackagedFactory,
		},
	)
	listed, err := svc.ListBuiltInPackagedFactories(
		t.Context(),
		factoryroot.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil || len(listed.Entries) != 1 || listed.Entries[0].Name != "@you/goal" {
		t.Fatalf("ListBuiltInPackagedFactories() = %#v, %v", listed, err)
	}
	resolved, err := svc.ResolveBuiltInPackagedFactory(
		t.Context(),
		factoryroot.ResolveBuiltInPackagedFactoryRequest{Name: "@you/goal"},
	)
	if err != nil || resolved.Definition.Project != "builtin-goal" {
		t.Fatalf("ResolveBuiltInPackagedFactory() = %#v, %v", resolved, err)
	}
}

func TestServiceResolvesThenInstallsBuiltInPackage(t *testing.T) {
	t.Parallel()
	var installed factoryroot.PackagedDefinition
	var installedFormat factoryroot.PackagedFactoryFormat
	svc := NewWithCatalogPackagesAndInstallation(
		stubDefinitionHost{},
		stubActivationGateway{},
		nil,
		factoryroot.PackagedFactoryCatalogOperations{
			List:    packagedCatalogStub{}.ListBuiltInPackagedFactories,
			Resolve: packagedCatalogStub{}.ResolveBuiltInPackagedFactory,
		},
		factoryroot.PackagedFactoryInstallationOperations{
			Install: func(
				_ context.Context,
				params factoryroot.PackagedFactoryInstallParams,
			) (factoryroot.PackagedFactoryInstallResult, error) {
				if params.NamedFactoriesRoot != "/customer/factories" {
					t.Fatalf("rootDir = %q", params.NamedFactoriesRoot)
				}
				installed = params.Definition
				installedFormat = params.Format
				return factoryroot.PackagedFactoryInstallResult{
					Name:       params.Definition.Name,
					FactoryDir: "/customer/factories/@you/goal",
					Outcome:    factoryroot.PackagedFactoryInstallCreated,
					Format:     params.Format,
				}, nil
			},
		},
	)

	result, err := svc.InstallPackagedFactory(
		t.Context(),
		factoryroot.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/goal",
			Format:  factoryroot.PackagedFactoryFormatYML,
		},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory() error = %v", err)
	}
	if installed.Name != "@you/goal" ||
		installed.Project != "builtin-goal" ||
		installedFormat != factoryroot.PackagedFactoryFormatYML {
		t.Fatalf("installation input = %#v, %q", installed, installedFormat)
	}
	if result.Definition.Name != "@you/goal" ||
		result.Definition.FactoryDir != "/customer/factories/@you/goal" ||
		result.Outcome != factoryroot.PackagedFactoryInstallCreated ||
		result.Format != factoryroot.PackagedFactoryFormatYML {
		t.Fatalf("InstallPackagedFactory() = %#v", result)
	}
}

func TestInstallPackagedFactory_RejectsIncompatibleScaffoldOptionsBeforeCatalogLookup(t *testing.T) {
	t.Parallel()
	svc := NewWithCatalogPackagesAndInstallation(
		stubDefinitionHost{},
		stubActivationGateway{},
		nil,
		factoryroot.PackagedFactoryCatalogOperations{
			List: func(
				context.Context,
				factoryroot.ListBuiltInPackagedFactoriesRequest,
			) (factoryroot.ListBuiltInPackagedFactoriesResult, error) {
				t.Fatal("catalog lookup should not run for incompatible distribute request")
				return factoryroot.ListBuiltInPackagedFactoriesResult{}, nil
			},
			Resolve: func(
				context.Context,
				factoryroot.ResolveBuiltInPackagedFactoryRequest,
			) (factoryroot.ResolveBuiltInPackagedFactoryResult, error) {
				t.Fatal("catalog resolve should not run for incompatible distribute request")
				return factoryroot.ResolveBuiltInPackagedFactoryResult{}, nil
			},
		},
		factoryroot.PackagedFactoryInstallationOperations{},
	)
	_, err := svc.InstallPackagedFactory(
		t.Context(),
		factoryroot.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/goal",
			Scaffold: factoryroot.CreateFactoryScaffoldRequest{
				Executor: "claude",
			},
		},
	)
	if err != factoryroot.ErrIncompatibleFactoryDistributeOptions {
		t.Fatalf("InstallPackagedFactory() error = %v, want %v", err, factoryroot.ErrIncompatibleFactoryDistributeOptions)
	}
}
