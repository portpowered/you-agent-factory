package runtime_api

import (
	"io/fs"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFunctionalAPIServer_DisablesRuntimeFileLoggingByDefault(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, persistTestPipelineConfig())
	logDir := t.TempDir()

	var capturedService *service.FactoryService
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		CaptureService: func(svc *service.FactoryService) {
			capturedService = svc
		},
		Configure: func(cfg *service.FactoryServiceConfig) {
			cfg.RuntimeMode = interfaces.RuntimeModeService
			cfg.RuntimeLogDir = logDir
		},
	})

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtimeStatus")
	}
	if capturedService == nil {
		t.Fatal("expected functional API server to capture factory service")
	}

	diagnostics := capturedService.RuntimeLogDiagnostics()
	if diagnostics.Path != "" {
		t.Fatalf("RuntimeLogDiagnostics().Path = %q, want empty when runtime file logging is disabled", diagnostics.Path)
	}

	logFiles := collectRuntimeLogFiles(t, logDir)
	if len(logFiles) != 0 {
		t.Fatalf("runtime log files = %v, want none", logFiles)
	}
}

func collectRuntimeLogFiles(t *testing.T, dir string) []string {
	t.Helper()

	var logFiles []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".log" {
			logFiles = append(logFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s): %v", dir, err)
	}
	return logFiles
}
