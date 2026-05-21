package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func writePortableServiceBundledFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func assertPortableServiceBundledFile(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("file %s = %q, want %q", path, string(got), want)
	}
}

func assertServiceBundledFactoryEntry(t *testing.T, bundledFile factoryapi.BundledFile, wantType factoryapi.BundledFileType, wantPath, wantContent string) {
	t.Helper()
	if bundledFile.Type != wantType {
		t.Fatalf("bundled file type = %q, want %q", bundledFile.Type, wantType)
	}
	if bundledFile.TargetPath != wantPath {
		t.Fatalf("bundled file targetPath = %q, want %q", bundledFile.TargetPath, wantPath)
	}
	if bundledFile.Content.Encoding != factoryapi.Utf8 {
		t.Fatalf("bundled file %q encoding = %q, want %q", wantPath, bundledFile.Content.Encoding, factoryapi.Utf8)
	}
	if bundledFile.Content.Inline != wantContent {
		t.Fatalf("bundled file %q content = %q, want %q", wantPath, bundledFile.Content.Inline, wantContent)
	}
}

func assertServiceBundledFactoryEntryWithoutInline(t *testing.T, bundledFile factoryapi.BundledFile, wantType factoryapi.BundledFileType, wantPath string) {
	t.Helper()
	if bundledFile.Type != wantType {
		t.Fatalf("bundled file type = %q, want %q", bundledFile.Type, wantType)
	}
	if bundledFile.TargetPath != wantPath {
		t.Fatalf("bundled file targetPath = %q, want %q", bundledFile.TargetPath, wantPath)
	}
	if bundledFile.Content.Encoding != factoryapi.Utf8 {
		t.Fatalf("bundled file %q encoding = %q, want %q", wantPath, bundledFile.Content.Encoding, factoryapi.Utf8)
	}
	if bundledFile.Content.Inline != "" {
		t.Fatalf("bundled file %q content = %q, want omitted inline content", wantPath, bundledFile.Content.Inline)
	}
}

func TestFactoryService_NamedFactoryPersistenceActivationAndRestartSmoke(t *testing.T) {
	rootDir := t.TempDir()

	persistNamedFactoryAndSelectCurrent(t, rootDir, "alpha")
	svc := buildNamedFactoryServiceForTest(t, rootDir)

	created, err := svc.CreateNamedFactory(context.Background(), serviceNamedFactoryContract(t, "beta"))
	if err != nil {
		t.Fatalf("CreateNamedFactory(beta): %v", err)
	}
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	assertCurrentFactoryPointer(t, rootDir, "beta", "after create")
	assertServiceCurrentNamedFactory(t, svc, "beta", "after create")

	if _, err := config.PersistNamedFactory(rootDir, "gamma", serviceNamedFactoryPayload(t, "gamma")); err != nil {
		t.Fatalf("PersistNamedFactory(gamma): %v", err)
	}
	corruptNamedFactoryConfig(t, rootDir, "gamma")

	if err := svc.ActivateNamedFactory(context.Background(), "gamma"); err == nil {
		t.Fatal("expected gamma activation to fail")
	}
	assertCurrentFactoryPointer(t, rootDir, "beta", "after failed activation")
	assertServiceCurrentNamedFactory(t, svc, "beta", "after failed activation")

	restarted := buildNamedFactoryServiceForTest(t, rootDir)
	if restarted.cfg.Dir != filepath.Join(rootDir, "beta") {
		t.Fatalf("restarted service dir = %q, want %q", restarted.cfg.Dir, filepath.Join(rootDir, "beta"))
	}
	assertServiceCurrentNamedFactory(t, restarted, "beta", "after restart")
}

func TestFactoryService_ActivateNamedFactory_LiveServiceModeStartsReplacementRuntime(t *testing.T) {
	rootDir := t.TempDir()
	persistNamedFactoryWithWorkTypeAndSelectCurrent(t, rootDir, "alpha", "alpha-task")
	logCore, observedLogs := observer.New(zap.InfoLevel)

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.New(logCore),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRun()

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	waitForRuntimeStatus(t, svc, interfaces.RuntimeStatusIdle, time.Second, "initial alpha runtime")
	submitServiceWork(t, svc, "alpha-task", "trace-alpha-runtime-before-activation")
	waitForTokenInPlace(t, svc, "alpha-task:complete", time.Second)
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayloadWithWorkType(t, "beta", "beta-task")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}

	assertCurrentFactoryPointer(t, rootDir, "beta", "after live activation")
	assertServiceCurrentNamedFactory(t, svc, "beta", "after live activation")
	waitForRuntimeStatus(t, svc, interfaces.RuntimeStatusIdle, time.Second, "activated beta runtime")
	writeWatchedInputRequest(t, filepath.Join(rootDir, "alpha"), "stale-alpha.json", interfaces.SubmitRequest{
		WorkID:     "trace-alpha-stale-input",
		WorkTypeID: "alpha-task",
		TraceID:    "trace-alpha-stale-input",
		Payload:    json.RawMessage(`{"title":"stale alpha input"}`),
	})
	assertWatcherDidNotDetectWorkType(t, observedLogs, "alpha-task", 300*time.Millisecond)
	submitServiceWork(t, svc, "beta-task", "trace-beta-runtime")
	waitForTokenInPlace(t, svc, "beta-task:complete", time.Second)

	if err := submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkID:     "trace-alpha-runtime",
		WorkTypeID: "alpha-task",
		TraceID:    "trace-alpha-runtime",
		Payload:    json.RawMessage(`{"title":"alpha task"}`),
	}}); err == nil {
		t.Fatal("expected alpha-task submission to fail after activating beta")
	}

	cancelRun()
	if err := <-errCh; err != nil {
		t.Fatalf("Run after cancellation: %v", err)
	}
}

func TestFactoryService_ActivateNamedFactory_WaitsForInFlightSubmitWorkRequest(t *testing.T) {
	rootDir := t.TempDir()
	persistNamedFactoryWithWorkTypeAndSelectCurrent(t, rootDir, "alpha", "alpha-task")
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayloadWithWorkType(t, "beta", "beta-task")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}

	submitStarted := make(chan struct{})
	releaseSubmit := make(chan struct{})
	svc := &FactoryService{
		factoryRootDir: rootDir,
		factory: &aggregateSnapshotFactory{
			engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
				RuntimeStatus: interfaces.RuntimeStatusIdle,
			},
			submitFunc: func(context.Context, interfaces.WorkRequest) error {
				close(submitStarted)
				<-releaseSubmit
				return nil
			},
		},
		cfg: &FactoryServiceConfig{
			Dir:               filepath.Join(rootDir, "alpha"),
			Logger:            zap.NewNop(),
			MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		},
		logger: zap.NewNop(),
	}

	submitErrCh := make(chan error, 1)
	go func() {
		submitErrCh <- submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
			WorkID:     "trace-alpha-submit",
			WorkTypeID: "alpha-task",
			TraceID:    "trace-alpha-submit",
			Payload:    json.RawMessage(`{"title":"alpha task"}`),
		}})
	}()
	<-submitStarted

	activationErrCh := make(chan error, 1)
	go func() {
		activationErrCh <- svc.ActivateNamedFactory(context.Background(), "beta")
	}()

	assertCurrentFactoryPointer(t, rootDir, "alpha", "while activation waits for submit")
	select {
	case err := <-activationErrCh:
		t.Fatalf("ActivateNamedFactory completed before in-flight submit drained: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseSubmit)
	if err := <-submitErrCh; err != nil {
		t.Fatalf("SubmitWorkRequest(alpha-task): %v", err)
	}
	if err := <-activationErrCh; err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}
	assertCurrentFactoryPointer(t, rootDir, "beta", "after activation unblocks")
}

func buildNamedFactoryServiceForTest(t *testing.T, rootDir string) *FactoryService {
	t.Helper()

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService(%s): %v", rootDir, err)
	}
	return svc
}

func persistNamedFactoryAndSelectCurrent(t *testing.T, rootDir, name string) {
	t.Helper()
	persistNamedFactoryWithWorkTypeAndSelectCurrent(t, rootDir, name, "task")
}

func persistNamedFactoryWithWorkTypeAndSelectCurrent(t *testing.T, rootDir, name, workType string) {
	t.Helper()

	if _, err := config.PersistNamedFactory(rootDir, name, serviceNamedFactoryPayloadWithWorkType(t, name, workType)); err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, name); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(%s): %v", name, err)
	}
}

func assertCurrentFactoryPointer(t *testing.T, rootDir, want, contextLabel string) {
	t.Helper()

	got, err := config.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer %s: %v", contextLabel, err)
	}
	if got != want {
		t.Fatalf("current factory pointer %s = %q, want %q", contextLabel, got, want)
	}
}

func assertCurrentFactoryPointerMissing(t *testing.T, rootDir, contextLabel string) {
	t.Helper()

	if _, err := config.ReadCurrentFactoryPointer(rootDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadCurrentFactoryPointer %s err = %v, want %v", contextLabel, err, os.ErrNotExist)
	}
}

func assertServiceCurrentNamedFactory(t *testing.T, svc *FactoryService, want, contextLabel string) {
	t.Helper()

	current, err := svc.GetCurrentNamedFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory %s: %v", contextLabel, err)
	}
	if current.Name != factoryapi.FactoryName(want) {
		t.Fatalf("current factory %s = %q, want %q", contextLabel, current.Name, want)
	}
}

func waitForRuntimeStatus(
	t *testing.T,
	svc *FactoryService,
	want interfaces.RuntimeStatus,
	timeout time.Duration,
	contextLabel string,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot %s: %v", contextLabel, err)
		}
		if snap.RuntimeStatus == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for runtime status %q during %s", want, contextLabel)
}

func submitServiceWork(t *testing.T, svc *FactoryService, workType, traceID string) {
	t.Helper()

	err := submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkID:     traceID,
		WorkTypeID: workType,
		TraceID:    traceID,
		Payload:    json.RawMessage(`{"title":"` + workType + `"}`),
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest(%s): %v", workType, err)
	}
}

func corruptNamedFactoryConfig(t *testing.T, rootDir, name string) {
	t.Helper()

	factoryPath := filepath.Join(rootDir, name, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, []byte(`{"id":"`+name+`","workTypes":[`), 0o644); err != nil {
		t.Fatalf("corrupt %s factory.json: %v", name, err)
	}
}

// writeWorkerAgentsMD writes a minimal MODEL_WORKER AGENTS.md into the given
// worker directory (creates the directory if needed).
func writeWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKER\nmodel: claude-3-5-haiku-20241022\n---\nYou are a helpful assistant.\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func writeWorkerAgentsMDWithContent(t *testing.T, factoryDir, workerName, content string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func writeScriptWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	writeScriptWorkerAgentsMDWithCommand(t, factoryDir, workerName, "echo", []string{"ok"})
}

func writeScriptWorkerAgentsMDWithCommand(t *testing.T, factoryDir, workerName, command string, args []string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	var argsYAML strings.Builder
	for _, arg := range args {
		argsYAML.WriteString("  - ")
		argsYAML.WriteString(strconv.Quote(arg))
		argsYAML.WriteString("\n")
	}
	agentsMD := fmt.Sprintf("---\ntype: SCRIPT_WORKER\ncommand: %s\nargs:\n%s---\n", command, argsYAML.String())
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

// writeWorkstationAgentsMD writes a minimal MODEL_WORKSTATION AGENTS.md into the given
// workstation directory (creates the directory if needed).
func writeWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

type serviceTestRuntimeConfig = runtimefixtures.RuntimeDefinitionLookupFixture

func newLoadedFactoryConfigForServiceTest(
	t *testing.T,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	workers map[string]*interfaces.WorkerConfig,
	workstations map[string]*interfaces.FactoryWorkstationConfig,
) *config.LoadedFactoryConfig {
	t.Helper()
	loaded, err := config.NewLoadedFactoryConfig(factoryDir, factoryCfg, serviceTestRuntimeConfig{
		Workers:      workers,
		Workstations: workstations,
	})
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}
