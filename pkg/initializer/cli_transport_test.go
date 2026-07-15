package initializer_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/runtimehost"
	"github.com/portpowered/infinite-you/pkg/service"
)

func TestInitializeCLITransport_RejectsMissingFactoryConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &initializer.Config{Dir: t.TempDir()}

	_, errInit := initializer.InitializeCLITransport(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, service.FactoryServiceConfigFromRuntimeHost(cfg))

	if errInit == nil {
		t.Fatal("expected InitializeCLITransport to fail without factory.json")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail without factory.json")
	}
	if errService.Error() != errInit.Error() {
		t.Fatalf("InitializeCLITransport error = %q, want %q", errInit, errService)
	}
}

func TestInitializeCLITransportComposesConfiguredDashboard(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	transport, err := initializer.InitializeCLITransport(context.Background(), &initializer.Config{
		Dir:                     dir,
		SimpleDashboardRenderer: func(runtimehost.SimpleDashboardRenderInput) {},
	})
	if err != nil {
		t.Fatalf("InitializeCLITransport() error = %v", err)
	}
	if transport.Runner() == nil {
		t.Fatal("InitializeCLITransport() returned no runtime runner")
	}
}

func TestInitializeCLITransport_ComposesRuntimeRunnerWithoutBuildFactoryService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	transport, err := initializer.InitializeCLITransport(ctx, &initializer.Config{Dir: dir})
	if err != nil {
		t.Fatalf("InitializeCLITransport: %v", err)
	}
	if transport.Services == nil || transport.Host == nil {
		t.Fatal("expected initializer CLI transport bundle")
	}
	runner := transport.Runner()
	if runner == nil {
		t.Fatal("expected session runtime runner")
	}
	if transport.Services.Models == nil || transport.Services.FactoryDefinition == nil {
		t.Fatal("expected initializer-produced model and factory-definition services")
	}
	if transport.Services.Models != transport.Host.ModelService() {
		t.Fatal("expected CLI transport and runtime host to share one model service")
	}
}
