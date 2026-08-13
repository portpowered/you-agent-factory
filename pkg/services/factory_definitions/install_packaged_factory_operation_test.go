package factorydefinitions

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestInstallPackagedFactoryOperationSelectsAndInstallsDetachedDefinition(t *testing.T) {
	var resolvedName string
	var installed PackagedFactoryInstallParams
	operation := NewInstallPackagedFactoryOperation(
		PackagedFactoryCatalogOperations{
			Resolve: func(_ context.Context, request ResolveBuiltInPackagedFactoryRequest) (ResolveBuiltInPackagedFactoryResult, error) {
				resolvedName = request.Name
				return ResolveBuiltInPackagedFactoryResult{
					Definition: PackagedDefinition{Name: request.Name, Project: "builtin-goal"},
				}, nil
			},
		},
		PackagedFactoryInstallationOperations{
			Install: func(_ context.Context, params PackagedFactoryInstallParams) (PackagedFactoryInstallResult, error) {
				installed = params
				return PackagedFactoryInstallResult{
					Name:       params.Definition.Name,
					FactoryDir: "/factories/goal",
					Outcome:    PackagedFactoryInstallReplaced,
					Format:     params.Format,
				}, nil
			},
		},
	)
	result, err := operation(context.Background(), InstallPackagedFactoryRequest{
		RootDir: "/factories",
		Name:    "@you/goal",
		Format:  PackagedFactoryFormatYAML,
		Replace: true,
	})
	if err != nil {
		t.Fatalf("operation() error = %v", err)
	}
	if resolvedName != "@you/goal" || installed.NamedFactoriesRoot != "/factories" || !installed.Replace || installed.Definition.Name != "@you/goal" {
		t.Fatalf("operation inputs = name %q params %#v", resolvedName, installed)
	}
	if result.Definition.Name != "@you/goal" || result.Definition.FactoryDir != "/factories/goal" || result.Outcome != PackagedFactoryInstallReplaced || result.Format != PackagedFactoryFormatYAML {
		t.Fatalf("operation result = %#v", result)
	}
}

func TestInstallPackagedFactoryOperationClassifiesBoundaryFailures(t *testing.T) {
	resolveErr := errors.New("catalog unavailable")
	baseCatalog := PackagedFactoryCatalogOperations{
		Resolve: func(context.Context, ResolveBuiltInPackagedFactoryRequest) (ResolveBuiltInPackagedFactoryResult, error) {
			return ResolveBuiltInPackagedFactoryResult{}, resolveErr
		},
	}
	baseRequest := InstallPackagedFactoryRequest{RootDir: "/factories", Name: "@you/goal"}

	operation := NewInstallPackagedFactoryOperation(baseCatalog, PackagedFactoryInstallationOperations{})
	if _, err := operation(context.Background(), InstallPackagedFactoryRequest{Scaffold: CreateFactoryScaffoldRequest{TargetDir: "/scaffold"}}); !errors.Is(err, ErrIncompatibleFactoryDistributeOptions) {
		t.Fatalf("incompatible request error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := operation(ctx, baseRequest); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request error = %v", err)
	}
	if _, err := operation(context.Background(), baseRequest); !errors.Is(err, resolveErr) {
		t.Fatalf("catalog error = %v", err)
	}

	resolvedCatalog := PackagedFactoryCatalogOperations{
		Resolve: func(context.Context, ResolveBuiltInPackagedFactoryRequest) (ResolveBuiltInPackagedFactoryResult, error) {
			return ResolveBuiltInPackagedFactoryResult{Definition: PackagedDefinition{Name: "@you/goal"}}, nil
		},
	}
	missingInstaller := NewInstallPackagedFactoryOperation(resolvedCatalog, PackagedFactoryInstallationOperations{})
	if _, err := missingInstaller(context.Background(), baseRequest); err == nil || !errors.Is(err, ErrFactoryDistributeFailed) {
		t.Fatalf("missing installer error = %v", err)
	}

	installErr := errors.New("disk full")
	failedInstaller := NewInstallPackagedFactoryOperation(resolvedCatalog, PackagedFactoryInstallationOperations{
		Install: func(context.Context, PackagedFactoryInstallParams) (PackagedFactoryInstallResult, error) {
			return PackagedFactoryInstallResult{}, installErr
		},
	})
	if _, err := failedInstaller(context.Background(), baseRequest); err == nil || !errors.Is(err, installErr) || !strings.Contains(err.Error(), ErrFactoryDistributeFailed.Error()) {
		t.Fatalf("installer error = %v", err)
	}
}
