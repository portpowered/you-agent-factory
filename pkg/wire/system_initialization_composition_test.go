package wire

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	managedchild "github.com/portpowered/infinite-you/pkg/platform/process/managedchild"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	managedbackend "github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
)

func TestSystemInitializationInspectPathPreservesOverrideAndSelectsProcessDefault(t *testing.T) {
	t.Parallel()

	path := t.TempDir()
	info, err := provideSystemInitializationInspectPath(serviceedges.Edges{})(path)
	if err != nil {
		t.Fatalf("default inspect path: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("default inspect path IsDir() = false for %q", path)
	}

	inspected := ""
	override := func(path string) (fs.FileInfo, error) {
		inspected = path
		return nil, fs.ErrPermission
	}
	_, err = provideSystemInitializationInspectPath(serviceedges.Edges{
		SystemInitializationInspectPath: override,
	})("customer-path")
	if !errors.Is(err, fs.ErrPermission) || inspected != "customer-path" {
		t.Fatalf("override inspect path = (%q, %v), want customer-path and permission error", inspected, err)
	}
}

func TestModelsManagedProcessRetainsCleanupErrorOnce(t *testing.T) {
	t.Parallel()
	cleanupErr := errors.New("bounded cleanup failure")
	cleanupCalls := 0
	process := &modelsManagedProcess{
		cleanup: func() error {
			cleanupCalls++
			return cleanupErr
		},
		finished: make(chan struct{}),
	}
	close(process.finished)

	process.cleanupResources()
	process.cleanupResources()
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want once", cleanupCalls)
	}
	if err := process.Wait(); !errors.Is(err, cleanupErr) {
		t.Fatalf("process wait error = %v, want retained cleanup error", err)
	}
}

func TestModelsProcessLauncherUsesIndependentChildLifetimeContext(t *testing.T) {
	t.Parallel()
	parentContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	startErr := errors.New("controlled child start failure")
	var childContext context.Context
	launcher := modelsProcessLauncher{
		resolveLaunch: func(context.Context, serviceedges.HostProcessStartSpec) (managedbackend.ManagedBackendLaunch, error) {
			return managedbackend.ManagedBackendLaunch{Command: "controlled-managed-backend"}, nil
		},
		startProcess: func(ctx context.Context, _ managedchild.Spec) (*managedchild.Process, error) {
			childContext = ctx
			return nil, startErr
		},
	}
	if _, err := launcher.Start(parentContext, serviceedges.HostProcessStartSpec{}); !errors.Is(err, startErr) {
		t.Fatalf("modelsProcessLauncher.Start() error = %v, want controlled start error", err)
	}
	cancel()
	if childContext == nil {
		t.Fatal("modelsProcessLauncher did not supply a child context")
	}
	if childContext.Done() != nil || childContext.Err() != nil {
		t.Fatalf("child lifetime context = (done=%v, err=%v), want cancellation-independent context", childContext.Done(), childContext.Err())
	}
}
