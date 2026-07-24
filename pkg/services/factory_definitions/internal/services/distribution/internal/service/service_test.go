package service_test

import (
	"context"
	"errors"
	"fmt"
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
			FactoryDir: filepath.Join("/factories", "goal"),
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
		distribution.Dependencies{
			Installer: installer,
			Scaffold:  scaffold,
		},
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
			TargetDir: filepath.Join("/factories", "goal"),
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
	wantFactoryDir := filepath.Clean(filepath.Join("/factories", "goal"))
	if scaffolded.Definition.FactoryDir != wantFactoryDir ||
		scaffolded.Definition.Name == "" ||
		scaffolded.ScaffoldType == "" {
		t.Fatalf("CreateFactoryScaffold = %#v, want aggregate identity facts at %q", scaffolded, wantFactoryDir)
	}
}

func TestDistributionService_InstallAndScaffoldShareAggregateFacts(t *testing.T) {
	t.Parallel()

	factoryDir := filepath.Join("/factories", "@you", "goal")
	installer := &stubInstaller{
		result: []factorydefinitions.PackagedFactoryInstallResult{{
			Name:       "@you/goal",
			FactoryDir: factoryDir,
			Outcome:    factorydefinitions.PackagedFactoryInstallCreated,
		}},
	}
	svc, err := distributionwire.NewService(
		[]factorydefinitions.PackagedDefinition{{
			Name:    "@you/goal",
			Project: "builtin-goal",
			JSON:    []byte(`{"name":"goal"}`),
		}},
		distribution.Dependencies{
			Installer: installer,
			Scaffold:  func(factorydefinitions.ScaffoldConfig) error { return nil },
		},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
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
	installedFacts := installed.Definition
	if installedFacts.Name == "" || installedFacts.FactoryDir == "" {
		t.Fatalf("InstallPackagedFactory Definition = %#v, want non-empty Name and FactoryDir", installedFacts)
	}
	if installedFacts != (factorydefinitions.DistributedFactoryDefinitionFacts{
		Name:       "@you/goal",
		FactoryDir: filepath.Clean(factoryDir),
	}) {
		t.Fatalf(
			"InstallPackagedFactory Definition = %#v, want CTR-DEF DistributedFactoryDefinitionFacts",
			installedFacts,
		)
	}

	// Equivalent location with unclean path separators must not diverge on FactoryDir identity.
	uncleanTarget := factoryDir + string(filepath.Separator) + "."
	scaffolded, err := svc.CreateFactoryScaffold(
		ctx,
		factorydefinitions.CreateFactoryScaffoldRequest{
			TargetDir: uncleanTarget,
			Type:      string(factorydefinitions.DefaultScaffoldType),
			Executor:  factorydefinitions.DefaultStarterExecutor,
		},
	)
	if err != nil {
		t.Fatalf("CreateFactoryScaffold: %v", err)
	}
	scaffoldedFacts := scaffolded.Definition
	if scaffoldedFacts.Name == "" || scaffolded.ScaffoldType == "" {
		t.Fatalf("CreateFactoryScaffold = %#v, want Name plus ScaffoldType identity", scaffolded)
	}
	if scaffoldedFacts.FactoryDir != installedFacts.FactoryDir {
		t.Fatalf(
			"install and scaffold FactoryDir diverge for equivalent location: install=%q scaffold=%q",
			installedFacts.FactoryDir,
			scaffoldedFacts.FactoryDir,
		)
	}
	if scaffoldedFacts != (factorydefinitions.DistributedFactoryDefinitionFacts{
		Name:       scaffoldedFacts.Name,
		FactoryDir: installedFacts.FactoryDir,
	}) {
		t.Fatalf(
			"CreateFactoryScaffold Definition = %#v, want CTR-DEF DistributedFactoryDefinitionFacts shape",
			scaffoldedFacts,
		)
	}
}

func TestDistributionService_BuiltInListAndTypedDistributeFailures(t *testing.T) {
	t.Parallel()

	installer := &stubInstaller{
		err: errors.New("installer refused creation"),
	}
	svc, err := distributionwire.NewService(
		[]factorydefinitions.PackagedDefinition{{
			Name:    "@you/goal",
			Project: "builtin-goal",
			JSON:    []byte(`{"name":"goal","secret":true}`),
		}},
		distribution.Dependencies{
			Installer: installer,
			Scaffold: func(cfg factorydefinitions.ScaffoldConfig) error {
				if strings.TrimSpace(cfg.Type) == "unsupported" {
					return fmt.Errorf("unsupported scaffold type %q", cfg.Type)
				}
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()

	listed, err := svc.ListBuiltInPackagedFactories(
		ctx,
		factorydefinitions.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories: %v", err)
	}
	if len(listed.Entries) != 1 {
		t.Fatalf("ListBuiltInPackagedFactories entries = %#v, want one detached entry", listed.Entries)
	}
	entry := listed.Entries[0]
	if entry != (factorydefinitions.BuiltInPackagedFactoryEntry{
		Name:    "@you/goal",
		Project: "builtin-goal",
	}) {
		t.Fatalf(
			"ListBuiltInPackagedFactories entry = %#v, want detached Name/Project without package payload fields",
			entry,
		)
	}

	_, unknownErr := svc.InstallPackagedFactory(
		ctx,
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/missing",
		},
	)
	if !errors.Is(unknownErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf(
			"InstallPackagedFactory unknown identity = %v, want %v",
			unknownErr,
			factorydefinitions.ErrUnknownPackagedFactoryIdentity,
		)
	}
	if errors.Is(unknownErr, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatal("unknown-identity failure must not also match ErrFactoryDistributeFailed")
	}

	_, emptyIdentityErr := svc.InstallPackagedFactory(
		ctx,
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "   ",
		},
	)
	if !errors.Is(emptyIdentityErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf(
			"InstallPackagedFactory empty identity = %v, want %v",
			emptyIdentityErr,
			factorydefinitions.ErrUnknownPackagedFactoryIdentity,
		)
	}

	_, installFailedErr := svc.InstallPackagedFactory(
		ctx,
		factorydefinitions.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/goal",
		},
	)
	if !errors.Is(installFailedErr, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf(
			"InstallPackagedFactory distribute failure = %v, want %v",
			installFailedErr,
			factorydefinitions.ErrFactoryDistributeFailed,
		)
	}
	if errors.Is(installFailedErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatal("install distribute failure must not also match ErrUnknownPackagedFactoryIdentity")
	}

	_, scaffoldFailedErr := svc.CreateFactoryScaffold(
		ctx,
		factorydefinitions.CreateFactoryScaffoldRequest{
			TargetDir: filepath.Join("/factories", "alpha"),
			Type:      "unsupported",
		},
	)
	if !errors.Is(scaffoldFailedErr, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf(
			"CreateFactoryScaffold distribute failure = %v, want %v",
			scaffoldFailedErr,
			factorydefinitions.ErrFactoryDistributeFailed,
		)
	}
	if errors.Is(scaffoldFailedErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatal("scaffold distribute failure must not also match ErrUnknownPackagedFactoryIdentity")
	}
}
