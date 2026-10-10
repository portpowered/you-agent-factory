package service_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionserviceimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/internal/service"
)

func TestDistributionDelegatesCatalogRequestsResultsAndFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("catalog unavailable")
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			listRequest := factorydefinitions.ListBuiltInPackagedFactoriesRequest{}
			resolveRequest := factorydefinitions.ResolveBuiltInPackagedFactoryRequest{Name: "@you/goal"}
			listed := factorydefinitions.ListBuiltInPackagedFactoriesResult{Entries: []factorydefinitions.BuiltInPackagedFactoryEntry{{Name: "@you/goal", Project: "builtin-goal", Formats: []factorydefinitions.PackagedFactoryFormat{factorydefinitions.PackagedFactoryFormatJSON}}}}
			resolved := factorydefinitions.ResolveBuiltInPackagedFactoryResult{Definition: factorydefinitions.PackagedDefinition{Name: "@you/goal", JSON: []byte(`{"name":"goal"}`)}}
			calls := []string{}
			catalog := factorydefinitions.PackagedFactoryCatalogOperations{
				List: func(gotCtx context.Context, request factorydefinitions.ListBuiltInPackagedFactoriesRequest) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
					calls = append(calls, "list")
					if gotCtx != ctx || !reflect.DeepEqual(request, listRequest) {
						t.Fatal("list request changed")
					}
					if fail {
						return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, failure
					}
					return listed, nil
				},
				Resolve: func(gotCtx context.Context, request factorydefinitions.ResolveBuiltInPackagedFactoryRequest) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
					calls = append(calls, "resolve")
					if gotCtx != ctx || request != resolveRequest {
						t.Fatal("resolve request changed")
					}
					if fail {
						return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, failure
					}
					return resolved, nil
				},
			}
			svc := newDistributionService(t, catalog, factorydefinitions.PackagedFactoryInstallationOperations{Install: func(context.Context, factorydefinitions.PackagedFactoryInstallParams) (factorydefinitions.PackagedFactoryInstallResult, error) {
				t.Fatal("catalog called installer")
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			}})
			if len(calls) != 0 {
				t.Fatal("construction called catalog")
			}
			listResult, listErr := svc.ListBuiltInPackagedFactories(ctx, listRequest)
			result, resolveErr := svc.ResolveBuiltInPackagedFactory(ctx, resolveRequest)
			var wantErr error
			wantList := listed
			wantResult := resolved
			if fail {
				wantErr = failure
				wantList = factorydefinitions.ListBuiltInPackagedFactoriesResult{}
				wantResult = factorydefinitions.ResolveBuiltInPackagedFactoryResult{}
			}
			assertCatalogErrorIdentity(t, listErr, wantErr)
			assertCatalogErrorIdentity(t, resolveErr, wantErr)
			if !reflect.DeepEqual(listResult, wantList) || !errors.Is(listErr, wantErr) || !errors.Is(resolveErr, wantErr) || !reflect.DeepEqual(result, wantResult) || !reflect.DeepEqual(calls, []string{"list", "resolve"}) {
				t.Fatalf("catalog result=%#v errors=%v/%v calls=%v", result, listErr, resolveErr, calls)
			}
		})
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
	if !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) || !errors.Is(err, installErr) {
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

func TestDistributionCreateFactoryScaffoldReturnsDistributedFacts(t *testing.T) {
	t.Parallel()

	var scaffoldDir string
	svc := newDistributionServiceWithScaffold(
		t,
		goalPackagedCatalog(t),
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		func(cfg factorydefinitions.ScaffoldConfig) error {
			scaffoldDir = cfg.Dir
			return nil
		},
		scaffoldNameResolver("alpha"),
	)

	targetDir := "/customer/factories/alpha"
	result, err := svc.CreateFactoryScaffold(
		t.Context(),
		factorydefinitions.CreateFactoryScaffoldRequest{
			TargetDir: targetDir,
			Type:      factorydefinitions.DefaultScaffoldType,
			Executor:  "codex",
		},
	)
	if err != nil {
		t.Fatalf("CreateFactoryScaffold: %v", err)
	}
	if scaffoldDir != targetDir {
		t.Fatalf("scaffold dir = %q, want %q", scaffoldDir, targetDir)
	}
	if result.Definition.Name != "alpha" ||
		result.Definition.FactoryDir != targetDir ||
		result.ScaffoldType != factorydefinitions.DefaultScaffoldType {
		t.Fatalf("CreateFactoryScaffold() = %#v", result)
	}
}

func TestDistributionCreateFactoryScaffoldRejectsBlankTargetDir(t *testing.T) {
	t.Parallel()

	scaffoldCalls := 0
	svc := newDistributionServiceWithScaffold(
		t,
		goalPackagedCatalog(t),
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		func(factorydefinitions.ScaffoldConfig) error {
			scaffoldCalls++
			return nil
		},
		scaffoldNameResolver("factory"),
	)

	_, err := svc.CreateFactoryScaffold(
		t.Context(),
		factorydefinitions.CreateFactoryScaffoldRequest{TargetDir: "  "},
	)
	if !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf("CreateFactoryScaffold(blank) error = %v", err)
	}
	if scaffoldCalls != 0 {
		t.Fatalf("scaffold calls = %d, want 0 before validation rejection", scaffoldCalls)
	}
}

func TestDistributionCreateFactoryScaffoldRejectsUnsupportedType(t *testing.T) {
	t.Parallel()

	scaffoldCalls := 0
	svc := newDistributionServiceWithScaffold(
		t,
		goalPackagedCatalog(t),
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		func(factorydefinitions.ScaffoldConfig) error {
			scaffoldCalls++
			return nil
		},
		scaffoldNameResolver("factory"),
	)

	_, err := svc.CreateFactoryScaffold(
		t.Context(),
		factorydefinitions.CreateFactoryScaffoldRequest{
			TargetDir: "/customer/factories/alpha",
			Type:      "unsupported",
		},
	)
	if !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf("CreateFactoryScaffold(unsupported) error = %v", err)
	}
	if scaffoldCalls != 0 {
		t.Fatalf("scaffold calls = %d, want 0 before unsupported-type rejection", scaffoldCalls)
	}
}

func TestDistributionCreateFactoryScaffoldWrapsInitializerFailure(t *testing.T) {
	t.Parallel()

	initErr := fmt.Errorf("disk full")
	svc := newDistributionServiceWithScaffold(
		t,
		goalPackagedCatalog(t),
		factorydefinitions.PackagedFactoryInstallationOperations{
			Install: func(
				context.Context,
				factorydefinitions.PackagedFactoryInstallParams,
			) (factorydefinitions.PackagedFactoryInstallResult, error) {
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			},
		},
		func(factorydefinitions.ScaffoldConfig) error {
			return initErr
		},
		scaffoldNameResolver("factory"),
	)

	_, err := svc.CreateFactoryScaffold(
		t.Context(),
		factorydefinitions.CreateFactoryScaffoldRequest{
			TargetDir: "/customer/factories/alpha",
		},
	)
	if !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) || !errors.Is(err, initErr) {
		t.Fatalf("CreateFactoryScaffold() error = %v, want ErrFactoryDistributeFailed", err)
	}
}

func newDistributionService(
	t *testing.T,
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
	installer factorydefinitions.PackagedFactoryInstallationOperations,
) distributionservice.Service {
	return newDistributionServiceWithScaffold(
		t,
		catalog,
		installer,
		func(factorydefinitions.ScaffoldConfig) error { return nil },
		scaffoldNameResolver("factory"),
	)
}

func newDistributionServiceWithScaffold(
	t *testing.T,
	catalog factorydefinitions.PackagedFactoryCatalogOperations,
	installer factorydefinitions.PackagedFactoryInstallationOperations,
	scaffoldInitializer factorydefinitions.ScaffoldInitializer,
	scaffoldFactoryNameResolver distributionservice.ScaffoldFactoryNameResolver,
) distributionservice.Service {
	svc := distributionserviceimpl.New(
		catalog,
		installer,
		scaffoldInitializer,
		scaffoldFactoryNameResolver,
	)
	if svc == nil {
		t.Fatal("component rejected complete test fixture")
	}
	return svc
}

func scaffoldNameResolver(name string) distributionservice.ScaffoldFactoryNameResolver {
	return func(string) (string, error) {
		return name, nil
	}
}

// Controlled catalog answers keep Distribution tests inside their orchestration
// boundary. Catalog identity/format policy has its own component suite.
func goalPackagedCatalog(t *testing.T) factorydefinitions.PackagedFactoryCatalogOperations {
	t.Helper()
	return factorydefinitions.PackagedFactoryCatalogOperations{
		List: func(context.Context, factorydefinitions.ListBuiltInPackagedFactoriesRequest) (factorydefinitions.ListBuiltInPackagedFactoriesResult, error) {
			t.Fatal("install/scaffold listed catalog")
			return factorydefinitions.ListBuiltInPackagedFactoriesResult{}, nil
		},
		Resolve: func(_ context.Context, request factorydefinitions.ResolveBuiltInPackagedFactoryRequest) (factorydefinitions.ResolveBuiltInPackagedFactoryResult, error) {
			if request.Name != "@you/goal" {
				return factorydefinitions.ResolveBuiltInPackagedFactoryResult{}, factorydefinitions.ErrUnknownPackagedFactoryIdentity
			}
			return factorydefinitions.ResolveBuiltInPackagedFactoryResult{Definition: factorydefinitions.PackagedDefinition{Name: "@you/goal", Project: "builtin-goal", JSON: []byte(`{"name":"goal"}`)}}, nil
		},
	}
}

func TestScaffoldResolvesNameAfterInitializationAndPreservesResolutionFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("name unavailable")
	for _, tc := range []struct {
		name     string
		resolved string
		err      error
	}{
		{name: "resolved", resolved: "alpha"},
		{name: "failure", err: failure},
		{name: "blank", resolved: " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := []string{}
			svc := newDistributionServiceWithScaffold(t, unusedScaffoldCatalog(t), factorydefinitions.PackagedFactoryInstallationOperations{Install: func(context.Context, factorydefinitions.PackagedFactoryInstallParams) (factorydefinitions.PackagedFactoryInstallResult, error) {
				t.Fatal("scaffold called installer")
				return factorydefinitions.PackagedFactoryInstallResult{}, nil
			}},
				func(cfg factorydefinitions.ScaffoldConfig) error {
					calls = append(calls, "initialize")
					if cfg.Dir != "/target" {
						t.Fatalf("initialize dir=%q", cfg.Dir)
					}
					return nil
				},
				func(dir string) (string, error) {
					calls = append(calls, "resolve")
					if dir != "/target" {
						t.Fatalf("resolve dir=%q", dir)
					}
					return tc.resolved, tc.err
				})
			if len(calls) != 0 {
				t.Fatal("construction invoked scaffold ports")
			}
			result, err := svc.CreateFactoryScaffold(t.Context(), factorydefinitions.CreateFactoryScaffoldRequest{TargetDir: " /target "})
			if !reflect.DeepEqual(calls, []string{"initialize", "resolve"}) {
				t.Fatalf("calls=%v", calls)
			}
			assertScaffoldResolution(t, result, err, tc.resolved, tc.err)
		})
	}
}

func assertCatalogErrorIdentity(t *testing.T, got, want error) {
	t.Helper()
	if got != want { //nolint:errorlint // Catalog delegation must forward the identical error without wrapping.
		t.Fatalf("catalog error = %v, want identical %v", got, want)
	}
}

func assertScaffoldResolution(t *testing.T, result factorydefinitions.CreateFactoryScaffoldResult, err error, resolved string, resolutionErr error) {
	t.Helper()
	if resolutionErr != nil || resolved == " " {
		if !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) || (resolutionErr != nil && !errors.Is(err, resolutionErr)) || !reflect.DeepEqual(result, factorydefinitions.CreateFactoryScaffoldResult{}) {
			t.Fatalf("result=%#v error=%v", result, err)
		}
	} else if err != nil || result.Definition.Name != resolved || result.Definition.FactoryDir != "/target" || result.ScaffoldType != factorydefinitions.DefaultScaffoldType {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}
