package factorydefinitions_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
)

func TestInstallPackagedFactoryOperation_UsesEmbeddedCatalogForKnownIdentity(t *testing.T) {
	t.Parallel()

	catalog, err := newPublishedPackagedFactoryCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	var installedName string
	install := factorydefinitions.NewInstallPackagedFactoryOperation(
		catalog,
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				_ context.Context,
				params factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				installedName = params.Definition.Name
				return factorydefinitions.PackagedFactoryInstallResult{
					Name:       params.Definition.Name,
					FactoryDir: "/customer/factories/@you/review",
					Outcome:    factorydefinitions.PackagedFactoryInstallCreated,
					Format:     params.Format,
				}, nil
			},
		},
	)

	result, err := install(
		t.Context(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/review",
			Format:  factorydefinitions.PackagedFactoryFormatJSON,
		},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory() error = %v", err)
	}
	if installedName != "@you/review" || result.Definition.Name != "@you/review" {
		t.Fatalf("install result = %#v, installedName = %q", result, installedName)
	}
}

func TestInstallPackagedFactoryOperation_UnknownIdentityFailsClosedWithInventory(t *testing.T) {
	t.Parallel()

	catalog, err := newPublishedPackagedFactoryCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	install := factorydefinitions.NewInstallPackagedFactoryOperation(
		catalog,
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
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
		t.Fatalf("InstallPackagedFactory() error = %v", err)
	}
	if !strings.Contains(err.Error(), "@you/goal") ||
		!strings.Contains(err.Error(), "@you/review") ||
		strings.Contains(err.Error(), "generated/") {
		t.Fatalf("install error = %q, want stable public inventory", err.Error())
	}
}

func newPublishedPackagedFactoryCatalog(
	t *testing.T,
) (factorydefinitions.PackagedFactoryCatalogOperations, error) {
	t.Helper()
	published, err := packagedfactorycatalog.LoadPublishedDefinitionCatalog()
	if err != nil {
		return factorydefinitions.PackagedFactoryCatalogOperations{}, err
	}
	return factorydefinitionsservice.NewPackagedFactoryCatalog(published.All())
}
