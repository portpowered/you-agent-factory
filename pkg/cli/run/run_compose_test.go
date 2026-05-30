package run

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/service"
)

func TestWireBuildFactoryService_MatchesInjectFactoryServiceInvalidConfig(t *testing.T) {
	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{
		Dir: t.TempDir(),
	}

	_, errWire := compose.InjectFactoryService(ctx, cfg)
	_, errRun := wireBuildFactoryService(ctx, cfg)

	if errWire == nil {
		t.Fatal("expected InjectFactoryService to fail without factory.json")
	}
	if errRun == nil {
		t.Fatal("expected wireBuildFactoryService to fail without factory.json")
	}
	if errRun.Error() != errWire.Error() {
		t.Fatalf("wireBuildFactoryService error = %q, want %q", errRun, errWire)
	}
}

func TestBuildFactoryService_OverrideableWithoutWire(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		return stubFactoryService{
			run: func(context.Context) error {
				return errors.New("stub run")
			},
		}, nil
	}

	_, err := buildFactoryService(context.Background(), &service.FactoryServiceConfig{})
	if err != nil {
		t.Fatalf("override buildFactoryService: %v", err)
	}

	err = Run(context.Background(), RunConfig{
		Dir:                        t.TempDir(),
		SuppressDashboardRendering: true,
	})
	if err == nil || err.Error() != "stub run" {
		t.Fatalf("Run with stub builder = %v, want stub run", err)
	}
}
