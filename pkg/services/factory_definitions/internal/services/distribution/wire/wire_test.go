package wire_test

import (
	"context"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

func TestNewService_RequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	catalog := factorydefinitions.PackagedFactoryCatalogOperations{
		List: func(
			context.Context,
			factorydefinitions.ListBuiltInPackagedFactoriesRequest,
		) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
			return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, nil
		},
		Resolve: func(
			context.Context,
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
		) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
			return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, nil
		},
	}
	installer := factorydefinitions.PackagedFactoryInstallationOperations{
		Install: func(
			context.Context,
			factorydefinitions.PackagedFactoryInstallParams,
		) (factorydefinitions.PackagedFactoryInstallResult, error) {
			return factorydefinitions.PackagedFactoryInstallResult{}, nil
		},
	}
	scaffold := func(factorydefinitions.ScaffoldConfig) error { return nil }
	resolver := func(string) (string, error) { return "factory", nil }

	if svc, err := distributionwire.NewService(distributionservice.Dependencies{
		PackagedCatalog: factorydefinitions.PackagedFactoryCatalogOperations{
			List:    nil,
			Resolve: catalog.Resolve,
		},
		PackagedInstaller:           installer,
		ScaffoldInitializer:         scaffold,
		ScaffoldFactoryNameResolver:   resolver,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "list operation is required") {
		t.Fatalf("NewService(nil list) = %#v, %v; want list operation required error", svc, err)
	}
	if svc, err := distributionwire.NewService(distributionservice.Dependencies{
		PackagedCatalog: factorydefinitions.PackagedFactoryCatalogOperations{
			List:    catalog.List,
			Resolve: nil,
		},
		PackagedInstaller:           installer,
		ScaffoldInitializer:         scaffold,
		ScaffoldFactoryNameResolver: resolver,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "resolve operation is required") {
		t.Fatalf("NewService(nil resolve) = %#v, %v; want resolve operation required error", svc, err)
	}
	if svc, err := distributionwire.NewService(distributionservice.Dependencies{
		PackagedCatalog:     catalog,
		PackagedInstaller:           factorydefinitions.PackagedFactoryInstallationOperations{},
		ScaffoldInitializer:         scaffold,
		ScaffoldFactoryNameResolver: resolver,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "installer is required") {
		t.Fatalf("NewService(nil installer) = %#v, %v; want installer required error", svc, err)
	}

	svc, err := distributionwire.NewService(distributionservice.Dependencies{
		PackagedCatalog:             catalog,
		PackagedInstaller:           installer,
		ScaffoldInitializer:         scaffold,
		ScaffoldFactoryNameResolver: resolver,
	})
	if err != nil {
		t.Fatalf("NewService with exact injected ports: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ distributionservice.Service = svc
}

func TestNewService_ConstructsInertOwnerWithoutLifecycle(t *testing.T) {
	t.Parallel()

	listCalls := 0
	catalog := factorydefinitions.PackagedFactoryCatalogOperations{
		List: func(
			context.Context,
			factorydefinitions.ListBuiltInPackagedFactoriesRequest,
		) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
			listCalls++
			return factorydefinitions.ListBuiltInPackagedFactoriesResult{
				Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{{
					Name:    "demo",
					Project: "demo-project",
				}},
			}, nil
		},
		Resolve: func(
			context.Context,
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
		) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
			return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, nil
		},
	}
	installer := factorydefinitions.PackagedFactoryInstallationOperations{
		Install: func(
			context.Context,
			factorydefinitions.PackagedFactoryInstallParams,
		) (factorydefinitions.PackagedFactoryInstallResult, error) {
			return factorydefinitions.PackagedFactoryInstallResult{}, nil
		},
	}
	scaffoldCalls := 0
	scaffold := func(factorydefinitions.ScaffoldConfig) error {
		scaffoldCalls++
		return nil
	}
	resolver := func(string) (string, error) { return "factory", nil }

	svc, err := distributionwire.NewService(distributionservice.Dependencies{
		PackagedCatalog:             catalog,
		PackagedInstaller:           installer,
		ScaffoldInitializer:         scaffold,
		ScaffoldFactoryNameResolver: resolver,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if listCalls != 0 || scaffoldCalls != 0 {
		t.Fatalf(
			"construction invoked collaborators before use: list=%d scaffold=%d",
			listCalls,
			scaffoldCalls,
		)
	}

	listed, err := svc.ListBuiltInPackagedFactories(
		context.Background(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "demo" {
		t.Fatalf("ListBuiltInPackagedFactories = %#v, want demo through injected catalog", listed)
	}
	if listCalls != 1 {
		t.Fatalf("list collaborator calls = %d, want 1 after explicit operation", listCalls)
	}
}
