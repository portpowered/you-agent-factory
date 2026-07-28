package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionpackagedcatalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/packagedcatalog"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packagedinstallation"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
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

func TestDistributionInstallPackagedFactoryReturnsDistributedFacts(t *testing.T) {
	t.Parallel()

	var installed factorydefinitions.PackagedDefinition
	var installedFormat factorydefinitions.PackagedFactoryFormat
	svc := newDistributionService(t, goalPackagedCatalog(t), factorydefinitions.PackagedFactoryInstallationOperations{
		Install: func(
			_ context.Context,
			params factorydefinitions.PackagedFactoryInstallParams,
		) (factorydefinitions.PackagedFactoryInstallResult, error) {
			if params.NamedFactoriesRoot != "/customer/factories" {
				t.Fatalf("rootDir = %q", params.NamedFactoriesRoot)
			}
			installed = params.Definition
			installedFormat = params.Format
			return factorydefinitions.PackagedFactoryInstallResult{
				Name:       params.Definition.Name,
				FactoryDir: "/customer/factories/@you/goal",
				Outcome:    factorydefinitions.PackagedFactoryInstallCreated,
				Format:     params.Format,
			}, nil
		},
	})

	result, err := svc.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/goal",
			Format:  factorydefinitions.PackagedFactoryFormatYML,
		},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory: %v", err)
	}
	if installed.Name != "@you/goal" ||
		installed.Project != "builtin-goal" ||
		installedFormat != factorydefinitions.PackagedFactoryFormatYML {
		t.Fatalf("installation input = %#v, %q", installed, installedFormat)
	}
	if result.Definition.Name != "@you/goal" ||
		result.Definition.FactoryDir != "/customer/factories/@you/goal" ||
		result.Outcome != factorydefinitions.PackagedFactoryInstallCreated ||
		result.Format != factorydefinitions.PackagedFactoryFormatYML {
		t.Fatalf("InstallPackagedFactory() = %#v", result)
	}
}

func TestDistributionInstallPackagedFactoryUnknownIdentityFailsClosed(t *testing.T) {
	t.Parallel()

	installCalls := 0
	svc := newDistributionService(t, goalPackagedCatalog(t), factorydefinitions.PackagedFactoryInstallationOperations{
		Install: func(
			context.Context,
			factorydefinitions.PackagedFactoryInstallParams,
		) (factorydefinitions.PackagedFactoryInstallResult, error) {
			installCalls++
			return factorydefinitions.PackagedFactoryInstallResult{}, nil
		},
	})

	_, err := svc.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/missing",
		},
	)
	if !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("InstallPackagedFactory(missing) error = %v", err)
	}
	if installCalls != 0 {
		t.Fatalf("installer calls = %d, want 0 before unknown identity rejection", installCalls)
	}
}

func TestDistributionInstallPackagedFactoryWrapsInstallerFailure(t *testing.T) {
	t.Parallel()

	installErr := fmt.Errorf("disk full")
	svc := newDistributionService(t, goalPackagedCatalog(t), factorydefinitions.PackagedFactoryInstallationOperations{
		Install: func(
			context.Context,
			factorydefinitions.PackagedFactoryInstallParams,
		) (factorydefinitions.PackagedFactoryInstallResult, error) {
			return factorydefinitions.PackagedFactoryInstallResult{}, installErr
		},
	})

	_, err := svc.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/goal",
		},
	)
	if !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf("InstallPackagedFactory() error = %v, want ErrFactoryDistributeFailed", err)
	}
}

func TestDistributionInstallPackagedFactorySkipAndReplaceOutcomes(t *testing.T) {
	t.Parallel()

	installCalls := 0
	svc := newDistributionService(t, goalPackagedCatalog(t), factorydefinitions.PackagedFactoryInstallationOperations{
		Install: func(
			_ context.Context,
			params factorydefinitions.PackagedFactoryInstallParams,
		) (factorydefinitions.PackagedFactoryInstallResult, error) {
			installCalls++
			outcome := factorydefinitions.PackagedFactoryInstallCreated
			switch installCalls {
			case 2:
				outcome = factorydefinitions.PackagedFactoryInstallSkipped
			case 3:
				if !params.Replace {
					t.Fatal("replace install expected Replace=true")
				}
				outcome = factorydefinitions.PackagedFactoryInstallReplaced
			}
			return factorydefinitions.PackagedFactoryInstallResult{
				Name:       "@you/goal",
				FactoryDir: "/customer/factories/@you/goal",
				Outcome:    outcome,
				Format:     factorydefinitions.PackagedFactoryFormatJSON,
			}, nil
		},
	})

	request := factorydefinitions.InstallPackagedFactoryRequest{
		RootDir: "/customer/factories",
		Name:    "@you/goal",
		Format:  factorydefinitions.PackagedFactoryFormatJSON,
	}

	created, err := svc.InstallPackagedFactory(t.Context(), request)
	if err != nil {
		t.Fatalf("initial InstallPackagedFactory: %v", err)
	}
	if created.Outcome != factorydefinitions.PackagedFactoryInstallCreated ||
		created.Definition.Name != "@you/goal" ||
		created.Definition.FactoryDir == "" {
		t.Fatalf("created = %#v", created)
	}

	skipped, err := svc.InstallPackagedFactory(t.Context(), request)
	if err != nil {
		t.Fatalf("repeat InstallPackagedFactory: %v", err)
	}
	if skipped.Outcome != factorydefinitions.PackagedFactoryInstallSkipped {
		t.Fatalf("skipped outcome = %q", skipped.Outcome)
	}

	request.Replace = true
	replaced, err := svc.InstallPackagedFactory(t.Context(), request)
	if err != nil {
		t.Fatalf("replace InstallPackagedFactory: %v", err)
	}
	if replaced.Outcome != factorydefinitions.PackagedFactoryInstallReplaced {
		t.Fatalf("replaced outcome = %q", replaced.Outcome)
	}
}

func TestDistributionInstallPackagedFactoryRejectsIncompatibleScaffoldOptions(t *testing.T) {
	t.Parallel()

	resolveCalls := 0
	svc := newDistributionService(t, factorydefinitions.PackagedFactoryCatalogOperations{
		List: func(
			context.Context,
			factorydefinitions.ListBuiltInPackagedFactoriesRequest,
		) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
			t.Fatal("catalog list should not run for incompatible distribute request")
			return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, nil
		},
		Resolve: func(
			context.Context,
			factorydefinitions.ResolveBuiltInPackagedFactoryRequest,
		) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
			resolveCalls++
			return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, nil
		},
	}, factorydefinitions.PackagedFactoryInstallationOperations{
		Install: func(
			context.Context,
			factorydefinitions.PackagedFactoryInstallParams,
		) (factorydefinitions.PackagedFactoryInstallResult, error) {
			t.Fatal("installer should not run for incompatible distribute request")
			return factorydefinitions.PackagedFactoryInstallResult{}, nil
		},
	})

	_, err := svc.InstallPackagedFactory(
		t.Context(),
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/customer/factories",
			Name:    "@you/goal",
			Scaffold: factorydefinitions.CreateFactoryScaffoldRequest{
				Executor: "claude",
			},
		},
	)
	if err != factorydefinitions.ErrIncompatibleFactoryDistributeOptions {
		t.Fatalf("InstallPackagedFactory() error = %v, want %v", err, factorydefinitions.ErrIncompatibleFactoryDistributeOptions)
	}
	if resolveCalls != 0 {
		t.Fatalf("resolve calls = %d, want 0 before incompatible-option rejection", resolveCalls)
	}
}

func TestDistributionInstallPackagedFactoryThroughInjectedPorts(t *testing.T) {
	t.Parallel()

	goalJSON, err := json.Marshal(factoryfixtures.MinimalFactoryConfig())
	if err != nil {
		t.Fatalf("marshal goal factory: %v", err)
	}
	catalog, err := distributionpackagedcatalog.New([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		Project: "builtin-goal",
		JSON:    goalJSON,
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("New catalog: %v", err)
	}

	fileSystem := platformfilesystem.Local{}
	persistence := factorydefinitioncomposition.FactoryDefinitionPersistenceWithValidator(
		factoryvalidation.New(nil),
	)
	installer := packagedinstallation.New(persistence, fileSystem)
	svc := newDistributionService(t, catalog, factorydefinitions.PackagedFactoryInstallationOperations{
		Install: installer.InstallPackagedFactory,
	})

	root := t.TempDir()
	request := factorydefinitions.InstallPackagedFactoryRequest{
		RootDir: root,
		Name:    "@you/goal",
		Format:  factorydefinitions.PackagedFactoryFormatJSON,
	}

	created, err := svc.InstallPackagedFactory(t.Context(), request)
	if err != nil {
		t.Fatalf("initial InstallPackagedFactory: %v", err)
	}
	if created.Definition.Name != "@you/goal" ||
		created.Definition.FactoryDir == "" ||
		created.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("created = %#v", created)
	}

	skipped, err := svc.InstallPackagedFactory(t.Context(), request)
	if err != nil {
		t.Fatalf("repeat InstallPackagedFactory: %v", err)
	}
	if skipped.Outcome != factorydefinitions.PackagedFactoryInstallSkipped {
		t.Fatalf("skipped outcome = %q", skipped.Outcome)
	}

	request.Replace = true
	replaced, err := svc.InstallPackagedFactory(t.Context(), request)
	if err != nil {
		t.Fatalf("replace InstallPackagedFactory: %v", err)
	}
	if replaced.Outcome != factorydefinitions.PackagedFactoryInstallReplaced {
		t.Fatalf("replaced outcome = %q", replaced.Outcome)
	}
	if replaced.Definition.Name != "@you/goal" || replaced.Definition.FactoryDir == "" {
		t.Fatalf("replaced facts = %#v", replaced.Definition)
	}
}

func newDistributionService(
	t *testing.T,
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
	installer factorydefinitions.PackagedFactoryInstallationOperations,
) distributionservice.Service {
	svc, err := distributionwire.NewService(distributionservice.Dependencies{
		PackagedCatalog:     catalog,
		PackagedInstaller:   installer,
		ScaffoldInitializer: func(factorydefinitions.ScaffoldConfig) error { return nil },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func goalPackagedCatalog(t *testing.T) factorydefinitions.PackagedFactoryCatalogOperations {
	catalog, err := distributionpackagedcatalog.New([]factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		Project: "builtin-goal",
		JSON:    []byte(`{"name":"goal"}`),
		Formats: []factorydefinitions.PackagedFactoryFormat{
			factorydefinitions.PackagedFactoryFormatJSON,
		},
	}})
	if err != nil {
		t.Fatalf("New catalog: %v", err)
	}
	return catalog
}
