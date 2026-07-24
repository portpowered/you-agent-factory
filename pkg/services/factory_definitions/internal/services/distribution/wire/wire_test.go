package wire_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

type recordingInstaller struct {
	called bool
}

func (r *recordingInstaller) EnsurePackagedFactories(
	_ context.Context,
	_ string,
	_ []factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	r.called = true
	return []factorydefinitions.PackagedFactoryInstallResult{{
		Name:       "@you/goal",
		FactoryDir: filepath.Join("/factories", "@you", "goal"),
		Outcome:    factorydefinitions.PackagedFactoryInstallCreated,
	}}, nil
}

func TestNewService_RequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	catalog := []factorydefinitions.PackagedDefinition{{
		Name:    "@you/goal",
		Project: "builtin-goal",
	}}
	installer := &recordingInstaller{}
	scaffold := func(factorydefinitions.ScaffoldConfig) error { return nil }

	if svc, err := distributionwire.NewService(catalog, distribution.Dependencies{
		Installer: nil,
		Scaffold:  scaffold,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "installer is required") {
		t.Fatalf("NewService(nil installer) = %#v, %v; want installer required error", svc, err)
	}
	if svc, err := distributionwire.NewService(catalog, distribution.Dependencies{
		Installer: installer,
		Scaffold:  nil,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "scaffold initializer is required") {
		t.Fatalf("NewService(nil scaffold) = %#v, %v; want scaffold required error", svc, err)
	}

	svc, err := distributionwire.NewService(catalog, distribution.Dependencies{
		Installer: installer,
		Scaffold:  scaffold,
	})
	if err != nil {
		t.Fatalf("NewService with exact injected ports: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ distribution.Service = svc
}

func TestNewService_HostEffectsComeOnlyFromInjectedPorts(t *testing.T) {
	t.Parallel()

	installer := &recordingInstaller{}
	var scaffoldCalled bool
	scaffold := func(cfg factorydefinitions.ScaffoldConfig) error {
		scaffoldCalled = true
		if strings.TrimSpace(cfg.Dir) == "" {
			t.Fatal("scaffold received empty Dir")
		}
		return nil
	}

	svc, err := distributionwire.NewService(
		[]factorydefinitions.PackagedDefinition{{
			Name:    "@you/goal",
			Project: "builtin-goal",
		}},
		distribution.Dependencies{
			Installer: installer,
			Scaffold:  scaffold,
		},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	if _, err := svc.InstallPackagedFactory(ctx, factorydefinitions.InstallPackagedFactoryRequest{
		RootDir: "/factories",
		Name:    "@you/goal",
	}); err != nil {
		t.Fatalf("InstallPackagedFactory: %v", err)
	}
	if !installer.called {
		t.Fatal("install did not use the injected PackagedFactoryInstaller port")
	}

	if _, err := svc.CreateFactoryScaffold(ctx, factorydefinitions.CreateFactoryScaffoldRequest{
		TargetDir: filepath.Join("/factories", "alpha"),
		Type:      string(factorydefinitions.DefaultScaffoldType),
	}); err != nil {
		t.Fatalf("CreateFactoryScaffold: %v", err)
	}
	if !scaffoldCalled {
		t.Fatal("scaffold did not use the injected ScaffoldInitializer port")
	}
}
