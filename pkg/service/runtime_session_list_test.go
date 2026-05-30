package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestFactoryService_ListFactorySessions_DefaultSessionUsesAbsolutePathsFromRelativeDir(t *testing.T) {
	harness := startRunningSessionServiceFromRelativeDir(t)
	defer harness.stop(t)

	wantAbs := cleanResolvedPath(harness.absFactoryDir)
	summary := requireDefaultSessionSummary(t, harness.svc)
	assertAbsoluteFactorySessionPaths(t, summary, wantAbs, wantAbs)

	defaultSession := harness.svc.defaultSession()
	if defaultSession == nil || liveSessionHandle(defaultSession) == nil || liveSessionHandle(defaultSession).runtime == nil {
		t.Fatal("expected live default session runtime")
	}
	if got := cleanResolvedPath(liveSessionHandle(defaultSession).runtime.dir); got != wantAbs {
		t.Fatalf("default runtime dir = %q, want %q", got, wantAbs)
	}
}

func TestFactoryService_ListFactorySessions_DefaultSessionAbsolutePathsMatchRuntimeWithCurrentPointer(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	defaultSummary := requireDefaultSessionSummary(t, harness.svc)

	wantFolder := cleanResolvedPath(harness.rootDir)
	wantFactory := cleanResolvedPath(harness.factoryDirs["alpha"])
	assertAbsoluteFactorySessionPaths(t, defaultSummary, wantFolder, wantFactory)

	defaultSession := harness.requireSession(t, defaultFactorySessionID)
	if got := cleanResolvedPath(defaultSession.FactoryDir); got != wantFactory {
		t.Fatalf("default session factoryDir = %q, want %q", got, wantFactory)
	}
	if got := cleanResolvedPath(defaultSession.FolderPath); got != wantFolder {
		t.Fatalf("default session folderPath = %q, want %q", got, wantFolder)
	}
	if got := cleanResolvedPath(liveSessionHandle(defaultSession).runtime.dir); got != wantFactory {
		t.Fatalf("default runtime dir = %q, want %q", got, wantFactory)
	}
}

func TestFactoryService_DiscoverFactorySessionTargets_DefaultTargetMatchesAbsoluteSessionSummary(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig:     minimalFactoryConfig(),
		namedFactories: []string{"alpha"},
	})
	defer harness.stop(t)

	targets, err := harness.svc.discoverFactorySessionTargets(harness.rootDir)
	if err != nil {
		t.Fatalf("discoverFactorySessionTargets: %v", err)
	}
	if len(targets) == 0 || targets[0].Ref.Kind != FactorySessionTargetKindDefault {
		t.Fatalf("targets = %#v, want default target first", targets)
	}

	wantFolder := cleanResolvedPath(harness.rootDir)
	if got := cleanResolvedPath(targets[0].FolderPath); got != wantFolder {
		t.Fatalf("default target folderPath = %q, want %q", got, wantFolder)
	}
	if got := cleanResolvedPath(targets[0].FactoryDir); got != wantFolder {
		t.Fatalf("default target factoryDir = %q, want %q", got, wantFolder)
	}
	if !filepath.IsAbs(targets[0].FolderPath) || !filepath.IsAbs(targets[0].FactoryDir) {
		t.Fatalf("default target paths = folder %q factory %q, want absolute", targets[0].FolderPath, targets[0].FactoryDir)
	}

	defaultSummary := requireDefaultSessionSummary(t, harness.svc)
	assertAbsoluteFactorySessionPaths(t, defaultSummary, wantFolder, wantFolder)
}

type relativeDirSessionHarness struct {
	svc           *FactoryService
	absFactoryDir string
	runErrCh      chan error
	cancelRun     context.CancelFunc
}

func startRunningSessionServiceFromRelativeDir(t *testing.T) *relativeDirSessionHarness {
	t.Helper()

	parent := t.TempDir()
	relativeName := "factory"
	absFactory := filepath.Join(parent, relativeName)
	if err := os.MkdirAll(absFactory, 0o755); err != nil {
		t.Fatalf("create factory dir: %v", err)
	}
	writeFactoryJSON(t, absFactory, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, absFactory, "process")
	if err := os.MkdirAll(filepath.Join(absFactory, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	withWorkingDirectory(t, parent)

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               relativeName,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()
	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")

	return &relativeDirSessionHarness{
		svc:           svc,
		absFactoryDir: absFactory,
		runErrCh:      runErrCh,
		cancelRun:     cancelRun,
	}
}

func (h *relativeDirSessionHarness) stop(t *testing.T) {
	t.Helper()

	h.cancelRun()
	select {
	case err := <-h.runErrCh:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service shutdown")
	}
}

func withWorkingDirectory(t *testing.T, dir string) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s): %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

func requireDefaultSessionSummary(t *testing.T, svc *FactoryService) *factoryapi.FactorySessionSummary {
	t.Helper()

	response, err := svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	for i := range response.Sessions {
		if response.Sessions[i].Id == defaultFactorySessionID {
			return &response.Sessions[i]
		}
	}
	t.Fatalf("sessions = %#v, want default session %q", response.Sessions, defaultFactorySessionID)
	return nil
}

func assertAbsoluteFactorySessionPaths(
	t *testing.T,
	summary *factoryapi.FactorySessionSummary,
	wantFolderPath string,
	wantFactoryDir string,
) {
	t.Helper()
	if summary == nil {
		t.Fatal("summary is required")
	}
	if !filepath.IsAbs(summary.FolderPath) {
		t.Fatalf("folderPath = %q, want absolute path", summary.FolderPath)
	}
	if !filepath.IsAbs(summary.FactoryDir) {
		t.Fatalf("factoryDir = %q, want absolute path", summary.FactoryDir)
	}
	if got, want := cleanResolvedPath(summary.FolderPath), cleanResolvedPath(wantFolderPath); got != want {
		t.Fatalf("folderPath = %q, want %q", got, want)
	}
	if got, want := cleanResolvedPath(summary.FactoryDir), cleanResolvedPath(wantFactoryDir); got != want {
		t.Fatalf("factoryDir = %q, want %q", got, want)
	}
}

func cleanResolvedPath(path string) string {
	cleaned := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return cleaned
	}
	return filepath.Clean(resolved)
}
