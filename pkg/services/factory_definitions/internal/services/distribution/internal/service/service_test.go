package service_test

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

type stubInstaller struct {
	lastRoot string
	lastDefs []factorydefinitions.PackagedDefinition
	result   []factorydefinitions.PackagedFactoryInstallResult
	err      error
}

func (s *stubInstaller) EnsurePackagedFactories(
	_ context.Context,
	namedFactoriesRoot string,
	definitions []factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	s.lastRoot = namedFactoriesRoot
	s.lastDefs = append([]factorydefinitions.PackagedDefinition(nil), definitions...)
	if s.err != nil {
		return nil, s.err
	}
	return append([]factorydefinitions.PackagedFactoryInstallResult(nil), s.result...), nil
}

func TestDistributionService_OwnsBuiltInInstallAndScaffoldSuccess(t *testing.T) {
	t.Parallel()

	installer := &stubInstaller{
		result: []factorydefinitions.PackagedFactoryInstallResult{{
			Name:       "@you/goal",
			FactoryDir: filepath.ToSlash(filepath.Join("/factories", "goal")),
			Outcome:    factorydefinitions.PackagedFactoryInstallCreated,
		}},
	}
	var scaffoldCalls int
	scaffold := func(cfg factorydefinitions.ScaffoldConfig) error {
		scaffoldCalls++
		if strings.TrimSpace(cfg.Dir) == "" {
			return factorydefinitions.ErrFactoryDistributeFailed
		}
		if cfg.Output == nil {
			cfg.Output = io.Discard
		}
		return nil
	}

	svc, err := distributionwire.NewService(
		[]factorydefinitions.PackagedDefinition{{
			Name:    "@you/goal",
			Project: "builtin-goal",
			JSON:    []byte(`{"name":"goal"}`),
		}},
		installer,
		scaffold,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ distribution.Service = svc

	ctx := context.Background()
	listed, err := svc.ListBuiltInPackagedFactories(
		ctx,
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories: %v", err)
	}
	if len(listed.Entries) != 1 ||
		listed.Entries[0].Name != "@you/goal" ||
		listed.Entries[0].Project != "builtin-goal" {
		t.Fatalf("ListBuiltInPackagedFactories = %#v, want @you/goal builtin-goal", listed)
	}

	installed, err := svc.InstallPackagedFactory(
		ctx,
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/goal",
		},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory: %v", err)
	}
	if installed.Definition.Name != "@you/goal" ||
		installed.Definition.FactoryDir == "" ||
		installed.Outcome != factorydefinitions.PackagedFactoryInstallCreated {
		t.Fatalf("InstallPackagedFactory = %#v, want goal aggregate facts", installed)
	}
	if installer.lastRoot != "/factories" || len(installer.lastDefs) != 1 || installer.lastDefs[0].Name != "@you/goal" {
		t.Fatalf("installer received root=%q defs=%#v", installer.lastRoot, installer.lastDefs)
	}

	scaffolded, err := svc.CreateFactoryScaffold(
		ctx,
		factorydefinitions.CreateFactoryScaffoldRequest{
			TargetDir: "/factories/goal",
			Type:      string(factorydefinitions.DefaultScaffoldType),
			Executor:  factorydefinitions.DefaultStarterExecutor,
		},
	)
	if err != nil {
		t.Fatalf("CreateFactoryScaffold: %v", err)
	}
	if scaffoldCalls != 1 {
		t.Fatalf("scaffold calls = %d, want 1", scaffoldCalls)
	}
	if scaffolded.Definition.FactoryDir != "/factories/goal" ||
		scaffolded.Definition.Name == "" ||
		scaffolded.ScaffoldType == "" {
		t.Fatalf("CreateFactoryScaffold = %#v, want aggregate identity facts", scaffolded)
	}
}
