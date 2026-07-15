package service_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	factoryservice "github.com/portpowered/infinite-you/pkg/factory/service"
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
		Clock:            factory.EnsureClock(clockwork.NewFakeClock()),
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

func TestBuild_ProductionObservabilityPoliciesEnableRuntimeSinksByDefault(t *testing.T) {
	dir := t.TempDir()
	logDir := t.TempDir()
	metricsDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	loaded, err := configload.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	bundle, err := factoryservice.Build(context.Background(), factoryservice.BuildInput{
		Dir:        dir,
		FolderPath: dir,
		SessionID:  "~default",
		Config: factoryservice.Config{
			RuntimeMode:       interfaces.RuntimeModeBatch,
			RuntimeLogDir:     logDir,
			RuntimeMetricsDir: metricsDir,
		},
		LoadedFactoryCfg: loaded,
		BaseLogger:       zap.NewNop(),
		Clock:            factory.EnsureClock(clockwork.NewFakeClock()),
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
	if bundle.LogSink == nil {
		t.Fatal("LogSink = nil, want runtime log sink when production policy is unset")
	}
	if bundle.MetricsSink == nil {
		t.Fatal("MetricsSink = nil, want runtime metrics sink when production policy is unset")
	}
	if bundle.LogSink.RootDir() != logDir {
		t.Fatalf("LogSink.RootDir() = %q, want %q", bundle.LogSink.RootDir(), logDir)
	}
	if bundle.MetricsSink.RootDir() != metricsDir {
		t.Fatalf("MetricsSink.RootDir() = %q, want %q", bundle.MetricsSink.RootDir(), metricsDir)
	}
	if filepath.Base(bundle.LogSink.Path()) == "" {
		t.Fatal("LogSink.Path() = empty")
	}
	if filepath.Base(bundle.MetricsSink.Path()) == "" {
		t.Fatal("MetricsSink.Path() = empty")
	}

	disabledBundle, err := factoryservice.Build(context.Background(), factoryservice.BuildInput{
		Dir:        dir,
		FolderPath: dir,
		SessionID:  "~default",
		Config: factoryservice.Config{
			RuntimeMode:              interfaces.RuntimeModeBatch,
			RuntimeLogDir:            logDir,
			RuntimeMetricsDir:        metricsDir,
			RuntimeFileLoggingPolicy: factoryservice.RuntimeFileLoggingPolicyDisabled,
			RuntimeMetricsPolicy:     factoryservice.RuntimeMetricsPolicyDisabled,
		},
		LoadedFactoryCfg: loaded,
		BaseLogger:       zap.NewNop(),
		Clock:            factory.EnsureClock(clockwork.NewFakeClock()),
		LoadWorkerOpts: func(*factoryevents.FactoryEventHistory, *zap.Logger) ([]factory.FactoryOption, error) {
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("Build disabled policy: %v", err)
	}
	if disabledBundle == nil {
		t.Fatal("disabled policy bundle = nil")
	}
	if disabledBundle.LogSink != nil {
		t.Fatal("LogSink = non-nil, want nil when runtime file logging is explicitly disabled")
	}
	if disabledBundle.MetricsSink != nil {
		t.Fatal("MetricsSink = non-nil, want nil when runtime metrics policy is explicitly disabled")
	}
}
