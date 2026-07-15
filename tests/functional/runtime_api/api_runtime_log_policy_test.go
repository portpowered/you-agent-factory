package runtime_api

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

func TestFunctionalAPIServer_CanOptIntoRuntimeFileLogging(t *testing.T) {
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
			cfg.RuntimeFileLoggingPolicy = service.RuntimeFileLoggingPolicyEnabled
			cfg.RuntimeLogDir = logDir
			cfg.RuntimeInstanceID = "functional-api-runtime-log"
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
	if diagnostics.Path == "" {
		t.Fatal("RuntimeLogDiagnostics().Path = empty, want runtime log path when file logging is enabled")
	}
	rel, err := filepath.Rel(logDir, diagnostics.Path)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("runtime log path = %q, want path under %q", diagnostics.Path, logDir)
	}

	logFiles := collectRuntimeLogFiles(t, logDir)
	if len(logFiles) != 1 {
		t.Fatalf("runtime log files = %v, want exactly one", logFiles)
	}
	if logFiles[0] != diagnostics.Path {
		t.Fatalf("runtime log file = %q, want diagnostics path %q", logFiles[0], diagnostics.Path)
	}

	startup := requireRuntimeAPILogMessage(t, diagnostics.Path, "factory started")
	if startup["runtime_log_path"] != diagnostics.Path {
		t.Fatalf("runtime_log_path = %#v, want %q in startup record %#v", startup["runtime_log_path"], diagnostics.Path, startup)
	}
	if startup["runtime_log_root"] != logDir {
		t.Fatalf("runtime_log_root = %#v, want %q in startup record %#v", startup["runtime_log_root"], logDir, startup)
	}
	if startup["runtime_log_appender"] != logging.RuntimeLogAppenderZapRollingFile {
		t.Fatalf("runtime_log_appender = %#v, want %q in startup record %#v", startup["runtime_log_appender"], logging.RuntimeLogAppenderZapRollingFile, startup)
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

func requireRuntimeAPILogMessage(t *testing.T, path, message string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		if record["msg"] == message {
			return record
		}
	}
	t.Fatalf("runtime log %s missing msg %q", path, message)
	return nil
}
