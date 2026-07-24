package factorydefinition

import (
	"context"
	"path/filepath"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

type distributeOwnershipInstaller struct {
	result []factoryroot.PackagedFactoryInstallResult
}

func (s distributeOwnershipInstaller) EnsurePackagedFactories(
	_ context.Context,
	_ string,
	_ []factoryroot.PackagedDefinition,
) ([]factoryroot.PackagedFactoryInstallResult, error) {
	return append([]factoryroot.PackagedFactoryInstallResult(nil), s.result...), nil
}

func TestRootService_DistributeSuccessThroughPrivateDistribution(t *testing.T) {
	t.Parallel()

	factoryDir := filepath.Join("/factories", "goal")
	distributionService, err := distributionwire.NewService(
		[]factoryroot.PackagedDefinition{{
			Name:    "@you/goal",
			Project: "builtin-goal",
			JSON:    []byte(`{"name":"goal"}`),
		}},
		distribution.Dependencies{
			Installer: distributeOwnershipInstaller{result: []factoryroot.PackagedFactoryInstallResult{{
				Name:       "@you/goal",
				FactoryDir: factoryDir,
				Outcome:    factoryroot.PackagedFactoryInstallCreated,
			}}},
			Scaffold: func(factoryroot.ScaffoldConfig) error { return nil },
		},
	)
	if err != nil {
		t.Fatalf("distributionwire.NewService: %v", err)
	}

	var service factoryroot.Service = New(stubDefinitionHost{}).AttachDistribution(distributionService)
	ctx := context.Background()

	listed, err := service.ListBuiltInPackagedFactories(
		ctx,
		factoryroot.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "@you/goal" {
		t.Fatalf("ListBuiltInPackagedFactories = %#v, want @you/goal through private ownership", listed)
	}

	installed, err := service.InstallPackagedFactory(
		ctx,
		factoryroot.InstallPackagedFactoryRequest{RootDir: "/factories", Name: "@you/goal"},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory: %v", err)
	}
	if installed.Definition.Name != "@you/goal" || installed.Definition.FactoryDir != filepath.Clean(factoryDir) {
		t.Fatalf("InstallPackagedFactory = %#v, want private distribution aggregate facts", installed)
	}

	scaffolded, err := service.CreateFactoryScaffold(
		ctx,
		factoryroot.CreateFactoryScaffoldRequest{
			TargetDir: factoryDir,
			Type:      string(factoryroot.DefaultScaffoldType),
			Executor:  factoryroot.DefaultStarterExecutor,
		},
	)
	if err != nil {
		t.Fatalf("CreateFactoryScaffold: %v", err)
	}
	if scaffolded.Definition.FactoryDir != filepath.Clean(factoryDir) || scaffolded.ScaffoldType == "" {
		t.Fatalf("CreateFactoryScaffold = %#v, want private distribution scaffold facts", scaffolded)
	}
}
