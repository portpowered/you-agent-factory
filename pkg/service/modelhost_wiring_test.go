package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	"go.uber.org/zap"
)

func TestNewLocalModelDomain_WiresProcessWideModelHost(t *testing.T) {
	domain := newRuntimeLocalModelDependencies(&FactoryServiceConfig{
		ModelCacheDir: t.TempDir(),
	})
	if domain.host == nil {
		t.Fatal("local model domain host = nil, want process-wide modelhost.Host")
	}
	if _, ok := domain.host.(*modelhost.CatalogHost); !ok {
		t.Fatalf("host type = %T, want *modelhost.CatalogHost", domain.host)
	}
}

func TestBuildFactoryService_StartupModelHostMatchesRuntimeBundle(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	startupHost := svc.core.ModelHost()
	if startupHost == nil {
		t.Fatal("startup model host = nil")
	}
	if svc.startupBundle != nil && svc.startupBundle.modelHost != startupHost {
		t.Fatal("startup bundle model host does not match service collaborator host")
	}
}
