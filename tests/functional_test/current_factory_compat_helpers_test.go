package functional_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/service"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"go.uber.org/zap/zaptest/observer"
)

func createCurrentFactoryWatchChannel(t *testing.T, factoryDir, workType, channel string) {
	t.Helper()

	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, workType, channel)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create watched input dir %q: %v", inputDir, err)
	}
}

func writeCurrentFactoryWatchedInput(t *testing.T, factoryDir, workType, channel, name string, payload []byte) {
	t.Helper()

	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, workType, channel)
	if err := os.WriteFile(filepath.Join(inputDir, name), payload, 0o644); err != nil {
		t.Fatalf("write watched input %q: %v", name, err)
	}
}

func waitForCurrentFactoryRuntimeIdle(t *testing.T, svc *service.FactoryService, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastStatus interfaces.RuntimeStatus
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err == nil && snap.RuntimeStatus == interfaces.RuntimeStatusIdle {
			return
		}
		if err == nil {
			lastStatus = snap.RuntimeStatus
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for idle runtime; last status=%q", lastStatus)
}

func waitForObservedLogCount(
	t *testing.T,
	observedLogs *observer.ObservedLogs,
	message string,
	wantCount int,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if observedLogs.FilterMessage(message).Len() >= wantCount {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for log %q count >= %d; got %d", message, wantCount, observedLogs.FilterMessage(message).Len())
}

func waitForCurrentFactoryWatchedCompletion(
	t *testing.T,
	rootDir, wantDir string,
	svc *service.FactoryService,
	provider *testutil.MockProvider,
	observedLogs *observer.ObservedLogs,
	wantCalls int,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		assertCurrentFactoryReadback(t, rootDir, "beta", wantDir)

		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err == nil &&
			snap.RuntimeStatus == interfaces.RuntimeStatusIdle &&
			provider.CallCount() == wantCalls &&
			len(snap.DispatchHistory) == wantCalls &&
			len(snap.Marking.Tokens) == wantCalls &&
			len(snap.Marking.TokensInPlace("task:complete")) == wantCalls &&
			len(snap.Marking.TokensInPlace("task:failed")) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after watched completion timeout: %v", err)
	}
	t.Fatalf(
		"timed out waiting for activated-factory watched completion: provider_calls=%d runtime_status=%q dispatch_history=%d total_tokens=%d complete_tokens=%d failed_tokens=%d new_input_logs=%d watcher_started_logs=%d",
		provider.CallCount(),
		snap.RuntimeStatus,
		len(snap.DispatchHistory),
		len(snap.Marking.Tokens),
		len(snap.Marking.TokensInPlace("task:complete")),
		len(snap.Marking.TokensInPlace("task:failed")),
		observedLogs.FilterMessage("new input detected").Len(),
		observedLogs.FilterMessage("file watcher started").Len(),
	)
}

func assertNoAdditionalCurrentFactoryWork(
	t *testing.T,
	rootDir, wantDir string,
	svc *service.FactoryService,
	provider *testutil.MockProvider,
	stableFor time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(stableFor)
	for time.Now().Before(deadline) {
		assertCurrentFactoryReadback(t, rootDir, "beta", wantDir)

		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot during old-factory stability check: %v", err)
		}
		if provider.CallCount() != 1 {
			t.Fatalf("old factory directory still triggered work: provider call count = %d, want 1", provider.CallCount())
		}
		if snap.RuntimeStatus != interfaces.RuntimeStatusIdle {
			t.Fatalf("runtime status after old-factory write = %q, want %q", snap.RuntimeStatus, interfaces.RuntimeStatusIdle)
		}
		if len(snap.DispatchHistory) != 1 {
			t.Fatalf("dispatch history after old-factory write = %d, want 1", len(snap.DispatchHistory))
		}
		if len(snap.Marking.Tokens) != 1 || len(snap.Marking.TokensInPlace("task:complete")) != 1 {
			t.Fatalf(
				"terminal tokens after old-factory write = total:%d complete:%d, want total:1 complete:1",
				len(snap.Marking.Tokens),
				len(snap.Marking.TokensInPlace("task:complete")),
			)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func assertCurrentFactoryReadback(t *testing.T, rootDir, wantName, wantDir string) {
	t.Helper()

	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer(%s): %v", wantName, err)
	} else if got != wantName {
		t.Fatalf("current factory pointer = %q, want %q", got, wantName)
	}

	if got, err := config.ResolveCurrentFactoryDir(rootDir); err != nil {
		t.Fatalf("ResolveCurrentFactoryDir(%s): %v", wantName, err)
	} else if got != wantDir {
		t.Fatalf("resolved current dir = %q, want %q", got, wantDir)
	}

	loaded, err := config.LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(%s): %v", wantName, err)
	}
	if loaded.FactoryDir() != wantDir {
		t.Fatalf("loaded factory dir = %q, want %q", loaded.FactoryDir(), wantDir)
	}
}
