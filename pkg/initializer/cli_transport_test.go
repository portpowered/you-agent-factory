package initializer_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
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
}

func TestInjectCLITransport_MatchesInitializeCLITransport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	initCfg := &initializer.Config{Dir: dir}

	direct, err := initializer.InitializeCLITransport(ctx, initCfg)
	if err != nil {
		t.Fatalf("InitializeCLITransport: %v", err)
	}
	wired, err := compose.InjectCLITransport(ctx, initCfg)
	if err != nil {
		t.Fatalf("InjectCLITransport: %v", err)
	}
	if direct.Runner() == nil || wired.Runner() == nil {
		t.Fatal("expected runtime runners from both composition paths")
	}
	if direct.Services == nil || wired.Services == nil {
		t.Fatal("expected composed domain services from both composition paths")
	}
}
