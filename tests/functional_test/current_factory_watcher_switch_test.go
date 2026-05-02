package functional_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCurrentFactoryWatcherSwitchSmoke_ActivatedFactoryOwnsWatchedInputWithoutDuplicateConsumption(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := copyNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := copyNamedFactoryFixture(t, rootDir, "beta")
	createCurrentFactoryWatchChannel(t, alphaDir, "task", "activated")
	createCurrentFactoryWatchChannel(t, betaDir, "task", "activated")

	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	provider := testutil.NewMockProvider(
		acceptedProviderResponse(),
		acceptedProviderResponse(),
		acceptedProviderResponse(),
	)
	core, observedLogs := observer.New(zap.InfoLevel)
	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:              rootDir,
		RuntimeMode:      interfaces.RuntimeModeService,
		ProviderOverride: provider,
		Logger:           zap.New(core),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()
	defer stopCurrentFactoryWatcherSwitchService(t, cancelRun, errCh)

	waitForObservedLogCount(t, observedLogs, "factory started", 1, 5*time.Second)
	waitForCurrentFactoryRuntimeIdle(t, svc, 5*time.Second)

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer(beta): %v", err)
	} else if got != "beta" {
		t.Fatalf("current factory pointer = %q, want beta", got)
	}
	waitForObservedLogCount(t, observedLogs, "file watcher started", 2, 5*time.Second)

	writeCurrentFactoryWatchedInput(t, betaDir, "task", "activated", "beta-work.json", []byte(`{"title":"beta watched work"}`))
	waitForCurrentFactoryWatchedCompletion(t, rootDir, betaDir, svc, provider, observedLogs, 1, 5*time.Second)

	writeCurrentFactoryWatchedInput(t, alphaDir, "task", "activated", "alpha-work.json", []byte(`{"title":"alpha watched work"}`))
	assertNoAdditionalCurrentFactoryWork(t, rootDir, betaDir, svc, provider, 750*time.Millisecond)
}

func copyNamedFactoryFixture(t *testing.T, rootDir, name string) string {
	t.Helper()

	srcDir := fixtureDir(t, "filewatcher_flow")
	dstDir := filepath.Join(rootDir, name)
	if err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatalf("copy fixture %q: %v", name, err)
	}
	return dstDir
}

func stopCurrentFactoryWatcherSwitchService(t *testing.T, cancel context.CancelFunc, errCh <-chan error) {
	t.Helper()

	cancel()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("service-mode run error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service-mode run to stop")
	}
}
