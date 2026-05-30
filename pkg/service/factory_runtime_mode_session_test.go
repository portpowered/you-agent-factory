package service

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestFactoryService_Run_ClearsStartupBundleAfterDefaultRegisters(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.startupRuntimeBundle() == nil {
		t.Fatal("expected startup bundle before Run")
	}

	runFactoryServiceWithCleanup(t, svc)
	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")
	if svc.startupRuntimeBundle() != nil {
		t.Fatal("startup bundle should be cleared after ~default registers at Run")
	}
	defaultHandle := liveSessionHandle(svc.defaultSession())
	if bundle := svc.currentRuntimeBundle(); bundle == nil || defaultHandle == nil || bundle != defaultHandle.runtime {
		t.Fatal("currentRuntimeBundle should resolve only through the default session handle after Run")
	}
}

func TestBuildFactoryService_PreservesSessionsRegistryAcrossRuntimeReplacement(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.sessions == nil {
		t.Fatal("expected factorysessions.Registry on FactoryService")
	}
	registryBefore := svc.sessions

	if _, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, defaultFactorySessionID); err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if svc.sessions != registryBefore {
		t.Fatal("buildReplacementFactoryRuntime replaced sessions registry; session ownership should stay on FactoryService")
	}
	if svc.cfg.Dir != alphaDir {
		t.Fatalf("service cfg.Dir = %q, want unchanged %q until activation", svc.cfg.Dir, alphaDir)
	}
}

func TestBuildFactoryService_InitializesFactorySessionsRegistry(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.sessions == nil {
		t.Fatal("expected factorysessions.Registry on FactoryService")
	}
	if svc.cfg.Dir != alphaDir {
		t.Fatalf("service cfg.Dir = %q, want %q", svc.cfg.Dir, alphaDir)
	}
	if svc.sessions.Count() != 0 {
		t.Fatalf("sessions.Count() = %d before Run, want 0 until live sessions register", svc.sessions.Count())
	}
}

func TestFactoryService_Run_RegistersDefaultSessionInRegistry(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.sessions.Count() != 0 {
		t.Fatalf("sessions.Count() = %d before Run, want 0 until default registers", svc.sessions.Count())
	}

	runFactoryServiceWithCleanup(t, svc)
	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")
	assertDefaultSessionRegisteredAfterRun(t, svc, rootDir, alphaDir)
}

func runFactoryServiceWithCleanup(t *testing.T, svc *FactoryService) {
	t.Helper()

	runCtx, cancelRun := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()
	t.Cleanup(func() {
		cancelRun()
		select {
		case err := <-runErrCh:
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for service shutdown")
		}
	})
}

func assertDefaultSessionRegisteredAfterRun(t *testing.T, svc *FactoryService, rootDir, alphaDir string) {
	t.Helper()

	defaultSession := svc.defaultSession()
	if defaultSession == nil {
		t.Fatal("defaultSession = nil after Run, want ~default registry entry")
	}
	if defaultSession.ID != defaultFactorySessionID {
		t.Fatalf("default session id = %q, want %q", defaultSession.ID, defaultFactorySessionID)
	}
	if !defaultSession.IsDefault {
		t.Fatal("default session IsDefault = false, want true")
	}
	if got := cleanResolvedPath(defaultSession.FactoryDir); got != cleanResolvedPath(alphaDir) {
		t.Fatalf("default session factoryDir = %q, want %q", defaultSession.FactoryDir, alphaDir)
	}
	if got := cleanResolvedPath(defaultSession.FolderPath); got != cleanResolvedPath(rootDir) {
		t.Fatalf("default session folderPath = %q, want %q", defaultSession.FolderPath, rootDir)
	}

	defaultHandle := liveSessionHandle(defaultSession)
	if defaultHandle == nil || defaultHandle.runtime == nil {
		t.Fatal("default session live handle is required after Run")
	}
	if got := cleanResolvedPath(defaultHandle.runtime.dir); got != cleanResolvedPath(alphaDir) {
		t.Fatalf("default live handle runtime dir = %q, want %q", defaultHandle.runtime.dir, alphaDir)
	}

	runState := svc.currentRunState()
	if runState == nil {
		t.Fatal("runState = nil after Run, want default session run state")
	}
	if runState.sessionID != defaultFactorySessionID {
		t.Fatalf("runState.sessionID = %q, want %q", runState.sessionID, defaultFactorySessionID)
	}
	if runState.runtime != defaultHandle {
		t.Fatal("runState.runtime != default session live handle")
	}
	if current := svc.currentSession(); current == nil || current.ID != defaultFactorySessionID {
		t.Fatalf("currentSession = %#v, want selected %q", current, defaultFactorySessionID)
	}
	if bundle := svc.currentRuntimeBundle(); bundle != defaultHandle.runtime {
		t.Fatal("currentRuntimeBundle should resolve through the default session registry handle after Run")
	}
}
