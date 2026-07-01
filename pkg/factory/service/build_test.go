package service_test

import (
	"context"
	"testing"

	"github.com/jonboulle/clockwork"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestBuild_ConstructsRunnableBundleWithoutRootService(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := configload.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	bundle, err := factoryservice.Build(context.Background(), factoryservice.BuildInput{
		Dir:              dir,
		FolderPath:       dir,
		SessionID:        "~default",
		Config:           factoryservice.Config{RuntimeMode: interfaces.RuntimeModeBatch, RuntimeFileLoggingPolicy: factoryservice.RuntimeFileLoggingPolicyDisabled},
		LoadedFactoryCfg: loaded,
		BaseLogger:       zap.NewNop(),
		Clock:              factory.EnsureClock(clockwork.NewFakeClock()),
		LoadWorkerOpts: func(*factoryevents.FactoryEventHistory, *zap.Logger) ([]factory.FactoryOption, error) {
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if bundle == nil {
		t.Fatal("bundle = nil")
	}
	if bundle.Factory == nil {
		t.Fatal("bundle.Factory = nil, want runnable factory runtime")
	}
	if bundle.EventHistory == nil {
		t.Fatal("bundle.EventHistory = nil")
	}
	if bundle.Net == nil {
		t.Fatal("bundle.Net = nil")
	}
}
