package compose_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/service"
)

func TestInjectRuntimeRunner_RejectsMissingFactoryConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &service.FactoryServiceConfig{Dir: t.TempDir()}

	_, errAPI := compose.InjectRuntimeRunner(ctx, &service.FactoryServiceConfig{
		Dir:  cfg.Dir,
		Port: 8080,
	})
	_, errCLI := compose.InjectRuntimeRunner(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, cfg)

	if errAPI == nil {
		t.Fatal("expected InjectRuntimeRunner to fail without factory.json for API transport")
	}
	if errCLI == nil {
		t.Fatal("expected InjectRuntimeRunner to fail without factory.json for CLI transport")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail without factory.json")
	}
	if errService.Error() != errAPI.Error() {
		t.Fatalf("API transport error = %q, want %q", errAPI, errService)
	}
	if errService.Error() != errCLI.Error() {
		t.Fatalf("CLI transport error = %q, want %q", errCLI, errService)
	}
}

func TestInjectRuntimeRunner_SelectsCLITransportWhenPortZero(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{"name":"cli-runner","workTypes":[]}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	ctx := context.Background()
	runner, err := compose.InjectRuntimeRunner(ctx, &service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("InjectRuntimeRunner without port: %v", err)
	}
	if runner == nil {
		t.Fatal("expected CLI transport runtime runner")
	}
}

func TestInjectRuntimeRunner_SelectsAPITransportWhenPortConfigured(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{"name":"api-runner","workTypes":[]}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	ctx := context.Background()
	runner, err := compose.InjectRuntimeRunner(ctx, &service.FactoryServiceConfig{
		Dir:  dir,
		Port: 8080,
	})
	if err != nil {
		t.Fatalf("InjectRuntimeRunner with port: %v", err)
	}
	if runner == nil {
		t.Fatal("expected API transport runner")
	}
}
