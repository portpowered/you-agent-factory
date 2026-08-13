package factorydefinitions

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomerVisibleFactoryName_PrefersPackagedPublicName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *FactoryConfig
		want string
	}{
		{
			name: "public packaged name",
			cfg:  &FactoryConfig{Name: "@you/fusion"},
			want: "@you/fusion",
		},
		{
			name: "generated runtime name",
			cfg:  &FactoryConfig{Name: "fusion", Project: "builtin-fusion"},
			want: "@you/fusion",
		},
		{
			name: "customer local factory",
			cfg:  &FactoryConfig{Name: "customer-workshop"},
			want: "customer-workshop",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := CustomerVisibleFactoryName(test.cfg); got != test.want {
				t.Fatalf("CustomerVisibleFactoryName() = %q, want %q", got, test.want)
			}
		})
	}
}

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

func TestNamedFactoryPathRootWrappersPreserveCanonicalLayouts(t *testing.T) {
	if err := ValidateName("@you/goal"); err != nil {
		t.Fatalf("ValidateName(@you/goal): %v", err)
	}
	segments, err := PathSegments("@you/goal")
	if err != nil || len(segments) != 2 || segments[0] != "@you" || segments[1] != "goal" {
		t.Fatalf("PathSegments(@you/goal) = %#v, %v", segments, err)
	}
	if got, err := NameFromPathSegments(segments); err != nil || got != "@you/goal" {
		t.Fatalf("NameFromPathSegments(%#v) = %q, %v", segments, got, err)
	}
	root := filepath.Join("home", "factories")
	if got, err := MapDir(root, "@you/goal"); err != nil || got != filepath.Join(root, "@you", "goal") {
		t.Fatalf("MapDir() = %q, %v", got, err)
	}
	if got := LegacyNamedFactoriesRoot("home"); !strings.HasSuffix(got, filepath.Join(".you-agent-factory", "you-agent-factories")) {
		t.Fatalf("LegacyNamedFactoriesRoot() = %q", got)
	}

	if err := ValidateName("../escape"); err == nil || !errors.Is(err, ErrInvalidName) {
		t.Fatalf("ValidateName(../escape) = %v, want ErrInvalidName", err)
	}
	if _, err := NameFromPathSegments([]string{"@you"}); err == nil {
		t.Fatal("NameFromPathSegments(scope-only) unexpectedly succeeded")
	}
}

func TestResolveNamedFactoryRootsReportsMissingExplicitEdges(t *testing.T) {
	if _, err := ResolveNamedFactoryRoots("", "repo"); err == nil {
		t.Fatal("ResolveNamedFactoryRoots() accepted an empty home directory")
	}
	if _, err := ResolveNamedFactoryRoots("home", ""); err == nil {
		t.Fatal("ResolveNamedFactoryRoots() accepted an empty working directory")
	}
}

func TestPolicyAndResolutionContractHelpersRemainDetached(t *testing.T) {
	workstation := &FactoryWorkstationConfig{Name: "workstation"}
	if got := WorkPropagationPolicyFunc(func(got *FactoryWorkstationConfig) WorkPropagationMode {
		if got != workstation {
			t.Fatalf("policy received %p, want %p", got, workstation)
		}
		return WorkPropagationMode("clone")
	}).Mode(workstation); got != WorkPropagationMode("clone") {
		t.Fatalf("WorkPropagationPolicyFunc.Mode() = %q", got)
	}

	if got := (&ExecutionCatalogError{}).Error(); got != "execution catalog resolution failed" {
		t.Fatalf("empty ExecutionCatalogError = %q", got)
	}
	if got := (&ExecutionCatalogError{Diagnostics: []ExecutionCatalogDiagnostic{{Message: "unknown worker"}}}).Error(); !strings.Contains(got, "unknown worker") {
		t.Fatalf("diagnostic ExecutionCatalogError = %q", got)
	}
	original := ResolveExecutionCatalogResult{
		ResolvedExecutionCatalog: ResolvedExecutionCatalog{
			DefinitionVersion: "v1",
			Workers:           map[string]ResolvedWorkerDefinition{"worker": {Name: "worker"}},
		},
		Diagnostics: []ExecutionCatalogDiagnostic{{Message: "diagnostic"}},
	}
	cloned := original.Clone()
	cloned.Workers["worker"] = ResolvedWorkerDefinition{Name: "changed"}
	if original.Workers["worker"].Name != "worker" || cloned.Diagnostics[0].Message != "diagnostic" {
		t.Fatalf("ResolveExecutionCatalogResult.Clone() did not detach values: original=%#v clone=%#v", original, cloned)
	}
}
