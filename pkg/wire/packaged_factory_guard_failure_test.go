package wire

import (
	"context"
	"errors"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func TestPackagedFactoryGuardFailure_UnknownResolveAndInstallRejectWithCatalogInventory(t *testing.T) {
	definitions, err := providePackagedFactoryDefinitions()
	if err != nil {
		t.Fatalf("providePackagedFactoryDefinitions() error = %v", err)
	}
	catalog, err := providePackagedFactoryCatalog(definitions)
	if err != nil {
		t.Fatalf("providePackagedFactoryCatalog() error = %v", err)
	}

	_, err = catalog.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/missing"},
	)
	if !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("ResolveBuiltInPackagedFactory(@you/missing) error = %v", err)
	}
	if !strings.Contains(err.Error(), "@you/goal") ||
		!strings.Contains(err.Error(), "@you/tts") ||
		strings.Contains(err.Error(), "generated/") {
		t.Fatalf("resolve error = %q, want stable public inventory", err.Error())
	}

	install := factorydefinitionswire.NewInstallPackagedFactoryOperation(
		catalog,
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				_ context.Context,
				_ factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				t.Fatal("installer should not run for unknown packaged identity")
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
	)
	_, err = install(
		t.Context(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/missing",
		},
	)
	if !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("InstallPackagedFactory(@you/missing) error = %v", err)
	}
	if !strings.Contains(err.Error(), "@you/deep-research") ||
		!strings.Contains(err.Error(), "@you/review") ||
		strings.Contains(err.Error(), "generated/") {
		t.Fatalf("install error = %q, want stable public inventory", err.Error())
	}
}
