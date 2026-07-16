package runtimelogtests

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

func TestFactoryService_RuntimeMetricsPolicy_PreservesProductionDefaultAndAllowsExplicitDisable(t *testing.T) {
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	t.Run("default policy stays enabled for direct service construction", func(t *testing.T) {
		metricsDir := t.TempDir()
		runtimeInstanceID := "runtime-metrics-default-enabled"

		svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
			Dir:               dir,
			MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
			Logger:            zap.NewNop(),
			RuntimeMetricsDir: metricsDir,
			RuntimeInstanceID: runtimeInstanceID,
		})
		if err != nil {
			t.Fatalf("BuildFactoryService: %v", err)
		}

		bundle := svc.CurrentRuntimeBundle()
		if bundle == nil {
			t.Fatal("expected startup runtime bundle")
		}
		if bundle.MetricsSink == nil {
			t.Fatal("MetricsSink = nil, want runtime metrics sink when production-facing policy is unset")
		}
		if bundle.MetricsSink.RootDir() != metricsDir {
			t.Fatalf("MetricsSink.RootDir() = %q, want %q", bundle.MetricsSink.RootDir(), metricsDir)
		}
		if bundle.MetricsSink.Path() == "" {
			t.Fatal("MetricsSink.Path() = empty, want runtime metrics path when production-facing policy is unset")
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := svc.Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}

		metricFiles := collectRuntimeMetricsFiles(t, metricsDir)
		if len(metricFiles) == 0 {
			t.Fatalf("runtime metrics files under %s = none, want at least one metrics artifact", metricsDir)
		}
	})

	t.Run("explicit disabled policy suppresses runtime metrics artifacts", func(t *testing.T) {
		metricsDir := t.TempDir()

		svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
			Dir:                  dir,
			MockWorkersConfig:    config.NewEmptyMockWorkersConfig(),
			Logger:               zap.NewNop(),
			RuntimeMetricsDir:    metricsDir,
			RuntimeInstanceID:    "runtime-metrics-disabled",
			RuntimeMetricsPolicy: service.RuntimeMetricsPolicyDisabled,
		})
		if err != nil {
			t.Fatalf("BuildFactoryService: %v", err)
		}

		bundle := svc.CurrentRuntimeBundle()
		if bundle == nil {
			t.Fatal("expected startup runtime bundle")
		}
		if bundle.MetricsSink != nil {
			t.Fatal("MetricsSink = non-nil, want nil when runtime metrics policy is disabled")
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := svc.Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}

		metricFiles := collectRuntimeMetricsFiles(t, metricsDir)
		if len(metricFiles) != 0 {
			t.Fatalf("runtime metrics files = %v, want none", metricFiles)
		}
	})
}

func collectRuntimeMetricsFiles(t *testing.T, dir string) []string {
	t.Helper()

	var metricFiles []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		metricFiles = append(metricFiles, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", dir, err)
	}
	return metricFiles
}
