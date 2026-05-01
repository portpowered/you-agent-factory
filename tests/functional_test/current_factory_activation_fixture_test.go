package functional_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/service"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCurrentFactoryActivationFixture_ActivatesSecondPersistedFactoryAndResolvesCurrentFactory(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", functionalNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", functionalNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	initialLoaded, err := config.LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(initial): %v", err)
	}
	if initialLoaded.FactoryConfig().Project != "alpha" {
		t.Fatalf("initial project = %q, want alpha", initialLoaded.FactoryConfig().Project)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}

	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer(beta): %v", err)
	} else if got != "beta" {
		t.Fatalf("current factory pointer = %q, want beta", got)
	}

	wantDir := filepath.Join(rootDir, "beta")
	if got, err := config.ResolveCurrentFactoryDir(rootDir); err != nil {
		t.Fatalf("ResolveCurrentFactoryDir(beta): %v", err)
	} else if got != wantDir {
		t.Fatalf("resolved current dir = %q, want %q", got, wantDir)
	}

	loaded, err := config.LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(after activation): %v", err)
	}
	if loaded.FactoryDir() != wantDir {
		t.Fatalf("loaded factory dir = %q, want %q", loaded.FactoryDir(), wantDir)
	}
	if loaded.FactoryConfig().Project != "beta" {
		t.Fatalf("activated project = %q, want beta", loaded.FactoryConfig().Project)
	}
}

func TestCurrentFactoryActivationFixture_WatchedFileExecutionFollowsActivatedFactory(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := copyCurrentFactoryFixture(t, rootDir, "alpha")
	betaDir := copyCurrentFactoryFixture(t, rootDir, "beta")
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
	defer stopCurrentFactoryActivationService(t, cancelRun, errCh)

	waitForObservedLogCount(t, observedLogs, "factory started", 1, 5*time.Second)
	waitForCurrentFactoryRuntimeIdle(t, svc, 5*time.Second)

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}
	assertCurrentFactoryReadback(t, rootDir, "beta", betaDir)
	waitForObservedLogCount(t, observedLogs, "file watcher started", 2, 5*time.Second)

	writeCurrentFactoryWatchedInput(t, betaDir, "task", "activated", "beta-work.json", []byte(`{"title":"beta watched work"}`))
	waitForCurrentFactoryWatchedCompletion(t, rootDir, betaDir, svc, provider, observedLogs, 1, 5*time.Second)

	writeCurrentFactoryWatchedInput(t, alphaDir, "task", "activated", "alpha-work.json", []byte(`{"title":"alpha watched work"}`))
	assertNoAdditionalCurrentFactoryWork(t, rootDir, betaDir, svc, provider, 750*time.Millisecond)
}

func TestCurrentFactoryActivationFixture_LiveAPIReadsFollowActivatedFactory(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", functionalNamedFactoryPayloadWithTerminalState(t, "alpha", "alpha-complete")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", functionalNamedFactoryPayloadWithTerminalState(t, "beta", "beta-complete")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	provider := testutil.NewMockProvider(
		acceptedProviderResponse(),
		acceptedProviderResponse(),
	)
	server := StartFunctionalServerWithConfig(t, rootDir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.ProviderOverride = provider
	})

	waitForCurrentFactoryRuntimeIdle(t, server.service, 5*time.Second)

	if err := server.service.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}
	assertCurrentFactoryReadback(t, rootDir, "beta", filepath.Join(rootDir, "beta"))
	waitForCurrentFactoryActivatedRuntime(t, server, "task:beta-complete", 5*time.Second)

	traceID := server.SubmitWork(t, "task", json.RawMessage(`{"title":"beta api work"}`))
	if traceID == "" {
		t.Fatal("POST /work returned an empty trace ID after activation")
	}
	work := waitForGeneratedWorkAtPlace(t, server.URL(), traceID, "task:beta-complete", 5*time.Second)
	if len(work.Results) != 1 {
		snapshot := server.GetEngineStateSnapshot(t)
		t.Fatalf(
			"GET /work result count after activation = %d, want 1; provider_calls=%d factory_state=%q runtime_status=%q dispatch_history=%d tokens=%d",
			len(work.Results),
			provider.CallCount(),
			snapshot.FactoryState,
			snapshot.RuntimeStatus,
			len(snapshot.DispatchHistory),
			len(snapshot.Marking.Tokens),
		)
	}
	if work.Results[0].PlaceId != "task:beta-complete" {
		t.Fatalf("GET /work place_id after activation = %q, want task:beta-complete", work.Results[0].PlaceId)
	}

	status := getGeneratedJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("GET /status runtime_status after activation = %q, want %q", status.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}
	if status.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens after activation = %d, want 1", status.TotalTokens)
	}
	if status.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count after activation = %d, want 1", status.Categories.Terminal)
	}

	if lastCall := provider.LastCall(); lastCall.ProjectID != "beta" {
		t.Fatalf("API submit project = %q, want beta", lastCall.ProjectID)
	}
}

func copyCurrentFactoryFixture(t *testing.T, rootDir, name string) string {
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

func waitForCurrentFactoryActivatedRuntime(
	t *testing.T,
	server *FunctionalServer,
	wantPlaceID string,
	timeout time.Duration,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastFactoryState string
	var lastRuntimeStatus interfaces.RuntimeStatus
	var sawPlace bool
	for time.Now().Before(deadline) {
		snapshot := server.GetEngineStateSnapshot(t)
		lastFactoryState = snapshot.FactoryState
		lastRuntimeStatus = snapshot.RuntimeStatus
		sawPlace = snapshot.Topology != nil && snapshot.Topology.Places[wantPlaceID] != nil
		if snapshot.RuntimeStatus == interfaces.RuntimeStatusIdle &&
			sawPlace {
			return snapshot
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf(
		"timed out waiting for activated runtime %q; factory_state=%q runtime_status=%q saw_place=%t",
		wantPlaceID,
		lastFactoryState,
		lastRuntimeStatus,
		sawPlace,
	)
	return nil
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

func stopCurrentFactoryActivationService(t *testing.T, cancel context.CancelFunc, errCh <-chan error) {
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

func functionalNamedFactoryPayload(t *testing.T, project string) []byte {
	return functionalNamedFactoryPayloadWithTerminalState(t, project, "complete")
}

func functionalNamedFactoryPayloadWithTerminalState(t *testing.T, project, terminalState string) []byte {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"id": project,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": terminalState, "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":             "worker-a",
			"type":             "MODEL_WORKER",
			"body":             "You are worker " + project + ".",
			"modelProvider":    "claude",
			"executorProvider": "script_wrap",
			"model":            "claude-sonnet-4-20250514",
		}},
		"workstations": []map[string]any{{
			"name":           "process",
			"worker":         "worker-a",
			"inputs":         []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":        []map[string]string{{"workType": "task", "state": terminalState}},
			"onFailure":      map[string]string{"workType": "task", "state": "failed"},
			"type":           "MODEL_WORKSTATION",
			"promptTemplate": "Do the " + project + " work.",
		}},
	})
	if err != nil {
		t.Fatalf("marshal functional named factory payload: %v", err)
	}
	return payload
}
