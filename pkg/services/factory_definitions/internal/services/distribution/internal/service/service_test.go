package service_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionpackagedcatalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedcatalog"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

func TestDistributionListsAndResolvesBuiltInPackagedFactories(t *testing.T) {
	t.Parallel()

	catalog, err := distributionpackagedcatalog.New([]factorydefinitions.PackagedDefinition{
		{
			Name: "@you/review", Project: "builtin-review",
			JSON: []byte(`{"name":"review"}`),
			Formats: []factorydefinitions.PackagedFactoryFormat{
				factorydefinitions.PackagedFactoryFormatJSON,
			},
		},
		{
			Name: "@you/goal", Project: "builtin-goal",
			JSON: []byte(`{"name":"goal"}`),
			Formats: []factorydefinitions.PackagedFactoryFormat{
				factorydefinitions.PackagedFactoryFormatJSON,
			},
		},
	})
	if err != nil {
		t.Fatalf("New catalog: %v", err)
	}

	svc, err := distributionwire.NewService(distributionservice.Dependencies{
		PackagedCatalog: catalog,
		PackagedInstaller: factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		ScaffoldInitializer: func(factorydefinitions.ScaffoldConfig) error { return nil },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	listed, err := svc.ListBuiltInPackagedFactories(
		t.Context(),
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories: %v", err)
	}
	gotNames := []string{listed.Entries[0].Name, listed.Entries[1].Name}
	if !reflect.DeepEqual(gotNames, []string{"@you/goal", "@you/review"}) {
		t.Fatalf("listed names = %v", gotNames)
	}
	if listed.Entries[0].Project != "builtin-goal" ||
		!reflect.DeepEqual(listed.Entries[0].Formats, []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		}) {
		t.Fatalf("listed goal entry = %#v", listed.Entries[0])
	}

	resolved, err := svc.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/goal"},
	)
	if err != nil {
		t.Fatalf("ResolveBuiltInPackagedFactory: %v", err)
	}
	if resolved.Definition.Project != "builtin-goal" ||
		!reflect.DeepEqual(resolved.Formats, []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		}) {
		t.Fatalf("resolved = %#v", resolved)
	}
}

func TestDistributionResolveUnknownOrBlankNameFailsClosed(t *testing.T) {
	t.Parallel()

	catalog, err := distributionpackagedcatalog.New([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON},
	}})
	if err != nil {
		t.Fatalf("New catalog: %v", err)
	}

	svc, err := distributionwire.NewService(distributionservice.Dependencies{
		PackagedCatalog: catalog,
		PackagedInstaller: factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		ScaffoldInitializer: func(factorydefinitions.ScaffoldConfig) error { return nil },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, unknownErr := svc.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/missing"},
	)
	if !errors.Is(unknownErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("ResolveBuiltInPackagedFactory(missing) error = %v", unknownErr)
	}
	if !strings.Contains(unknownErr.Error(), "@you/goal") {
		t.Fatalf("ResolveBuiltInPackagedFactory(missing) error = %q, want stable public inventory", unknownErr.Error())
	}

	_, blankErr := svc.ResolveBuiltInPackagedFactory(
		t.Context(),
		factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: ""},
	)
	if !errors.Is(blankErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("ResolveBuiltInPackagedFactory(blank) error = %v, want ErrUnknownPackagedFactoryIdentity", blankErr)
	}
}
