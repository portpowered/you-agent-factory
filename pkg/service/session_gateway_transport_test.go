package service

import (
	"context"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuildFactoryService_WiresSessionGatewayCollaborator(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.sessionGateway == nil {
		t.Fatal("expected session gateway collaborator on FactoryService")
	}
	if _, ok := svc.sessionGateway.(*factorysessionservice.Service); !ok {
		t.Fatalf("session gateway type = %T, want *factorysessionservice.Service", svc.sessionGateway)
	}
}
