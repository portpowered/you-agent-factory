package run

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/terminalpolicy"
	"github.com/portpowered/infinite-you/pkg/service"
)

func TestFailureBaseline_QuietPreservesRuntimeFileDiagnosticsWhileTerminalStaysMute(t *testing.T) {
	dir, workFile := writeDashboardRunFixture(t)
	logDir := t.TempDir()
	runtimeInstanceID := "quiet-file-diagnostics"

	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	buildFactoryService = func(ctx context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		cfg.RuntimeInstanceID = runtimeInstanceID
		return service.BuildFactoryService(ctx, cfg)
	}

	policy := terminalpolicy.Resolve(terminalpolicy.Options{Quiet: true})
	logger, err := policy.BuildLogger()
	if err != nil {
		t.Fatalf("BuildLogger: %v", err)
	}

	var stdout, stderr bytes.Buffer
	runErr := Run(context.Background(), RunConfig{
		Dir:                        dir,
		WorkFile:                   workFile,
		MockWorkersEnabled:         true,
		SuppressDashboardRendering: true,
		DisableDefaultRecording:    true,
		RuntimeLogDir:              logDir,
		TerminalPolicy:             policy,
		Logger:                     logger,
		Output:                     &stdout,
		Port:                       0,
	})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty quiet terminal output", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty quiet terminal output", stderr.String())
	}
	assertQuietLeakContractForbidden(t, stdout.String()+stderr.String())

	logPath := requireQuietRuntimeLogPath(t, logDir, runtimeInstanceID)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		t.Fatalf("runtime log %s is empty, want diagnostic content under quiet", logPath)
	}
	if !strings.Contains(string(data), "factory started") {
		t.Fatalf("runtime log %s missing factory started record:\n%s", logPath, data)
	}
}

func requireQuietRuntimeLogPath(t *testing.T, logDir, runtimeInstanceID string) string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(logDir, "*", "*", "*-"+runtimeInstanceID+"-*.log"))
	if err != nil {
		t.Fatalf("glob runtime log path: %v", err)
	}
	if len(matches) != 1 {
		logFiles := collectQuietRuntimeLogFiles(t, logDir)
		t.Fatalf("runtime log paths for %q under %s = %v, all log files = %v, want exactly one", runtimeInstanceID, logDir, matches, logFiles)
	}
	return matches[0]
}

func collectQuietRuntimeLogFiles(t *testing.T, dir string) []string {
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
