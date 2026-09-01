package runtime_api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	runtimeAPILogObservationTimeout  = 2 * time.Second
	runtimeAPILogObservationInterval = 10 * time.Millisecond
)

func TestFunctionalAPIServer_UsesProductionRuntimeFileLoggingDefault(t *testing.T) {
	t.Parallel()
	// C06-ISOLATED CASE-19: --runtime-log-dir is a process input and the
	// production log/selector-404 witness must own its log root.
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
	response, err := http.Get(server.URL() + "/factory-sessions/%20")
	if err != nil {
		t.Fatalf("GET whitespace Factory Session selector: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET whitespace Factory Session selector status = %d, want 404", response.StatusCode)
	}
}

func TestFunctionalAPIServer_RuntimeLogDirectoryIsAProcessInput(t *testing.T) {
	t.Parallel()
	// C06-ISOLATED CASE-20: the startup record's exact path/root/appender
	// fields are process-scoped output under a caller-selected log directory.
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

	// The status endpoint can become ready before the asynchronous runtime-log
	// appender flushes its startup record. Observe only the target file and
	// message at a bounded interval so this test does not sleep or retry the
	// whole application scenario.
	deadline := time.NewTimer(runtimeAPILogObservationTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(runtimeAPILogObservationInterval)
	defer ticker.Stop()

	for {
		record, found, err := readRuntimeAPILogMessage(path, message)
		if err != nil {
			t.Fatalf("observe runtime log %q for msg %q: %v", path, message, err)
		}
		if found {
			return record
		}

		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out after %s waiting for runtime log %q to contain msg %q", runtimeAPILogObservationTimeout, path, message)
		}
	}
}

func readRuntimeAPILogMessage(path, message string) (map[string]any, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read file: %w", err)
	}
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, false, fmt.Errorf("decode structured JSON at line %d: %w; line: %s", lineNumber+1, err, line)
		}
		if record["msg"] == message {
			return record, true, nil
		}
	}
	return nil, false, nil
}
