package runtime_api

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFunctionalAPIServer_UsesProductionRuntimeFileLoggingDefault(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, persistTestPipelineConfig())
	logDir := t.TempDir()

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--runtime-log-dir", logDir},
	})

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtimeStatus")
	}
	logFiles := collectRuntimeLogFiles(t, logDir)
	if len(logFiles) != 1 {
		t.Fatalf("runtime log files = %v, want one production-default log", logFiles)
	}
}

func TestFunctionalAPIServer_RuntimeLogDirectoryIsAProcessInput(t *testing.T) {
	dir := testutil.ScaffoldFactoryDir(t, persistTestPipelineConfig())
	logDir := t.TempDir()

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--runtime-log-dir", logDir},
	})

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.RuntimeStatus == "" {
		t.Fatal("GET /status returned empty runtimeStatus")
	}
	logFiles := collectRuntimeLogFiles(t, logDir)
	if len(logFiles) != 1 {
		t.Fatalf("runtime log files = %v, want exactly one", logFiles)
	}
	rel, err := filepath.Rel(logDir, logFiles[0])
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("runtime log path = %q, want path under %q", logFiles[0], logDir)
	}

	startup := requireRuntimeAPILogMessage(t, logFiles[0], "factory started")
	if startup["runtime_log_path"] != logFiles[0] {
		t.Fatalf("runtime_log_path = %#v, want %q in startup record %#v", startup["runtime_log_path"], logFiles[0], startup)
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
