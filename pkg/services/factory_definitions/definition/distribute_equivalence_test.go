package factorydefinition

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

type equivalenceInstaller struct {
	result []factoryroot.PackagedFactoryInstallResult
	err    error
}

func (s equivalenceInstaller) EnsurePackagedFactories(
	_ context.Context,
	_ string,
	_ []factoryroot.PackagedDefinition,
) ([]factoryroot.PackagedFactoryInstallResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]factoryroot.PackagedFactoryInstallResult(nil), s.result...), nil
}

// newRootDistributeServiceForPeer attaches private distribution behind the
// public root Service. Construction may import owner-local wire; peer exercise
// below must not.
func newRootDistributeServiceForPeer(
	t *testing.T,
	installer factoryroot.PackagedFactoryInstaller,
	scaffold factoryroot.ScaffoldInitializer,
) factoryroot.Service {
	t.Helper()
	distributionService, err := distributionwire.NewService(
		[]factoryroot.PackagedDefinition{{
			Name:    "@you/goal",
			Project: "builtin-goal",
			JSON:    []byte(`{"name":"goal"}`),
		}},
		installer,
		scaffold,
	)
	if err != nil {
		t.Fatalf("distributionwire.NewService: %v", err)
	}
	return New(stubDefinitionHost{}).AttachDistribution(distributionService)
}

// peerExerciseRootDistributeSuccess proves a peer-shaped consumer can drive
// CTR-DEF success cases through the attached private implementation while
// depending only on the root Service vocabulary.
func peerExerciseRootDistributeSuccess(t *testing.T, service factoryroot.Service, factoryDir string) {
	t.Helper()
	ctx := context.Background()

	listed, err := service.ListBuiltInPackagedFactories(
		ctx,
		factoryroot.ListBuiltInPackagedFactoriesRequest{},
	)
	if err != nil {
		t.Fatalf("ListBuiltInPackagedFactories: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "@you/goal" {
		t.Fatalf("ListBuiltInPackagedFactories result = %#v, want @you/goal entry", listed)
	}

	installed, err := service.InstallPackagedFactory(
		ctx,
		factoryroot.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/goal",
		},
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory: %v", err)
	}
	wantDir := filepath.Clean(factoryDir)
	if installed.Definition.Name != "@you/goal" ||
		installed.Definition.FactoryDir != wantDir ||
		installed.Outcome != factoryroot.PackagedFactoryInstallCreated {
		t.Fatalf("InstallPackagedFactory result = %#v, want goal aggregate facts at %q", installed, wantDir)
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
	if scaffolded.Definition.FactoryDir != installed.Definition.FactoryDir {
		t.Fatalf(
			"install and scaffold FactoryDir diverge: install=%q scaffold=%q",
			installed.Definition.FactoryDir,
			scaffolded.Definition.FactoryDir,
		)
	}
	if scaffolded.Definition.Name == "" || scaffolded.ScaffoldType == "" {
		t.Fatalf("CreateFactoryScaffold result = %#v, want aggregate identity facts", scaffolded)
	}
}

// peerExerciseRootDistributeTypedFailures proves a peer-shaped consumer can
// distinguish CTR-DEF typed distribute failures through the attached private
// implementation using only root vocabulary.
func peerExerciseRootDistributeTypedFailures(t *testing.T, service factoryroot.Service) {
	t.Helper()
	ctx := context.Background()

	_, unknownErr := service.InstallPackagedFactory(
		ctx,
		factoryroot.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/missing",
		},
	)
	if !errors.Is(unknownErr, factoryroot.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf(
			"InstallPackagedFactory unknown-identity error = %v, want %v",
			unknownErr,
			factoryroot.ErrUnknownPackagedFactoryIdentity,
		)
	}

	_, installErr := service.InstallPackagedFactory(
		ctx,
		factoryroot.InstallPackagedFactoryRequest{
			RootDir: "/factories",
			Name:    "@you/goal",
		},
	)
	if !errors.Is(installErr, factoryroot.ErrFactoryDistributeFailed) {
		t.Fatalf(
			"InstallPackagedFactory distribute failure = %v, want %v",
			installErr,
			factoryroot.ErrFactoryDistributeFailed,
		)
	}
	if errors.Is(installErr, factoryroot.ErrUnknownPackagedFactoryIdentity) {
		t.Fatal("install failure must not also match ErrUnknownPackagedFactoryIdentity")
	}

	_, scaffoldErr := service.CreateFactoryScaffold(
		ctx,
		factoryroot.CreateFactoryScaffoldRequest{
			TargetDir: filepath.Join("/factories", "alpha"),
			Type:      "unsupported",
		},
	)
	if !errors.Is(scaffoldErr, factoryroot.ErrFactoryDistributeFailed) {
		t.Fatalf(
			"CreateFactoryScaffold distribute failure = %v, want %v",
			scaffoldErr,
			factoryroot.ErrFactoryDistributeFailed,
		)
	}
	if errors.Is(scaffoldErr, factoryroot.ErrUnknownPackagedFactoryIdentity) {
		t.Fatal("scaffold failure must not also match ErrUnknownPackagedFactoryIdentity")
	}
}

func TestRootDistributeEquivalence_CTRDEFSuccessThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	factoryDir := filepath.Join("/factories", "goal")
	service := newRootDistributeServiceForPeer(
		t,
		equivalenceInstaller{result: []factoryroot.PackagedFactoryInstallResult{{
			Name:       "@you/goal",
			FactoryDir: factoryDir,
			Outcome:    factoryroot.PackagedFactoryInstallCreated,
		}}},
		func(factoryroot.ScaffoldConfig) error { return nil },
	)

	peerExerciseRootDistributeSuccess(t, service, factoryDir)
}

func TestRootDistributeEquivalence_CTRDEFTypedFailuresThroughPrivateImplementation(t *testing.T) {
	t.Parallel()

	service := newRootDistributeServiceForPeer(
		t,
		equivalenceInstaller{err: errors.New("installer refused creation")},
		func(cfg factoryroot.ScaffoldConfig) error {
			if strings.TrimSpace(cfg.Type) == "unsupported" {
				return fmt.Errorf("unsupported scaffold type %q", cfg.Type)
			}
			return nil
		},
	)

	peerExerciseRootDistributeTypedFailures(t, service)
}

func TestRootDistributeEquivalence_PeerExercisesRootWithoutDistributionImport(t *testing.T) {
	t.Parallel()

	// Owner-local construction attaches private distribution. The peer exercise
	// helpers accept only factoryroot.Service and root request/result/error
	// types, proving a peer can drive the slice end-to-end without importing
	// distribution or other Definitions internals.
	factoryDir := filepath.Join("/factories", "goal")
	successService := newRootDistributeServiceForPeer(
		t,
		equivalenceInstaller{result: []factoryroot.PackagedFactoryInstallResult{{
			Name:       "@you/goal",
			FactoryDir: factoryDir,
			Outcome:    factoryroot.PackagedFactoryInstallCreated,
		}}},
		func(factoryroot.ScaffoldConfig) error { return nil },
	)
	peerExerciseRootDistributeSuccess(t, successService, factoryDir)

	failureService := newRootDistributeServiceForPeer(
		t,
		equivalenceInstaller{err: errors.New("installer refused creation")},
		func(cfg factoryroot.ScaffoldConfig) error {
			if strings.TrimSpace(cfg.Type) == "unsupported" {
				return fmt.Errorf("unsupported scaffold type %q", cfg.Type)
			}
			return nil
		},
	)
	peerExerciseRootDistributeTypedFailures(t, failureService)
}
