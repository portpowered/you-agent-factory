package compose_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/service"
)

func TestInjectFactoryService_RejectsMissingFactoryDir(t *testing.T) {
	t.Parallel()

	_, err := compose.InjectFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir: filepath.Join(t.TempDir(), "missing-factory"),
	})
	if err == nil {
		t.Fatal("expected error for missing factory dir")
	}
}

func TestInjectFactoryService_BuildsMinimalFactory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{"name":"wire-bootstrap","workTypes":[]}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	svc, err := compose.InjectFactoryService(context.Background(), &service.FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("InjectFactoryService: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil FactoryService")
	}
}
