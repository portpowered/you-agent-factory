package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"time"

	"github.com/jonboulle/clockwork"
	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/apisurface"
	"github.com/portpowered/agent-factory/pkg/cli/dashboard"
	"github.com/portpowered/agent-factory/pkg/cli/dashboardrender"
	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/internal/submission"
	"github.com/portpowered/agent-factory/pkg/logging"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/replay"
	"github.com/portpowered/agent-factory/pkg/testutil/runtimefixtures"
	"github.com/portpowered/agent-factory/pkg/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// minimalFactoryConfig returns a minimal factory.json config for testing.
func minimalFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
}

func serviceNamedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	return serviceNamedFactoryPayloadWithWorkType(t, project, "task")
}

func serviceNamedFactoryPayloadWithWorkType(t *testing.T, project, workType string) []byte {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"project": project,
		"workTypes": []map[string]any{{
			"name": workType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "worker-a",
			"type": "MODEL_WORKER",
			"body": "You are worker " + project + ".",
		}},
		"workstations": []map[string]any{{
			"name":           "process",
			"worker":         "worker-a",
			"inputs":         []map[string]string{{"workType": workType, "state": "init"}},
			"outputs":        []map[string]string{{"workType": workType, "state": "complete"}},
			"onFailure":      map[string]string{"workType": workType, "state": "failed"},
			"type":           "MODEL_WORKSTATION",
			"promptTemplate": "Do the " + project + " work.",
		}},
	})
	if err != nil {
		t.Fatalf("marshal named factory payload: %v", err)
	}
	return payload
}

func serviceNamedFactoryContract(t *testing.T, name string) factoryapi.Factory {
	t.Helper()
	return serviceNamedFactoryContractWithWorkType(t, name, "task")
}

func serviceNamedFactoryContractWithWorkType(t *testing.T, name, workType string) factoryapi.Factory {
	t.Helper()

	generated, err := config.GeneratedFactoryFromOpenAPIJSON([]byte(`{
		"project":"` + name + `",
		"workTypes":[{"name":"` + workType + `","states":[
			{"name":"init","type":"INITIAL"},
			{"name":"complete","type":"TERMINAL"},
			{"name":"failed","type":"FAILED"}
		]}],
		"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"You are worker ` + name + `."}],
		"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","promptTemplate":"Do the ` + name + ` work.","inputs":[{"workType":"` + workType + `","state":"init"}],"outputs":[{"workType":"` + workType + `","state":"complete"}],"onFailure":{"workType":"` + workType + `","state":"failed"}}]
	}`))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(%s): %v", name, err)
	}

	generated.Name = factoryapi.FactoryName(name)
	return generated
}

func submitWorkRequestsToService(ctx context.Context, svc *FactoryService, reqs []interfaces.SubmitRequest) error {
	workRequest := submission.WorkRequestFromSubmitRequests(reqs)
	_, err := svc.SubmitWorkRequest(ctx, workRequest)
	return err
}

func writeWorkRequestFile(t *testing.T, path string, req interfaces.SubmitRequest) {
	t.Helper()
	data, err := json.Marshal(submission.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{req}))
	if err != nil {
		t.Fatalf("marshal work request file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write work request file: %v", err)
	}
}

func writeWatchedInputRequest(t *testing.T, factoryDir, filename string, req interfaces.SubmitRequest) {
	t.Helper()

	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, req.WorkTypeID, interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create watched input dir: %v", err)
	}
	writeWorkRequestFile(t, filepath.Join(inputDir, filename), req)
}

func assertWatcherDidNotDetectWorkType(t *testing.T, logs *observer.ObservedLogs, workType string, wait time.Duration) {
	t.Helper()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if logs.FilterMessage("new input detected").FilterField(zap.String("work-type", workType)).Len() > 0 {
			t.Fatalf("expected no watcher activity for work type %q after activation", workType)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// writeFactoryJSON writes a factory.json into the given directory.
func writeFactoryJSON(t *testing.T, dir string, cfg map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

func TestBuildFactoryService_LoadsFromFactoryJSON(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")

	// Create the inputs/ directory that the file watcher expects.
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	// Verify the service was constructed with the correct net topology.
	if svc.net == nil {
		t.Fatal("expected non-nil net")
	}
	if _, ok := svc.net.WorkTypes["task"]; !ok {
		t.Error("expected 'task' work type in net topology")
	}

	// Verify factory is accessible internally.
	if svc.factory == nil {
		t.Fatal("expected non-nil factory")
	}

}

func TestBuildFactoryService_ResolvesCurrentFactoryFromNamedLayoutPointer(t *testing.T) {
	rootDir := t.TempDir()

	alphaPayload := serviceNamedFactoryPayload(t, "alpha")
	if _, err := config.PersistNamedFactory(rootDir, "alpha", alphaPayload); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	wantDir := filepath.Join(rootDir, "alpha")
	if svc.cfg.Dir != wantDir {
		t.Fatalf("service dir = %q, want %q", svc.cfg.Dir, wantDir)
	}
	if svc.runtimeCfg == nil {
		t.Fatal("expected runtime config")
	}
	if svc.runtimeCfg.FactoryDir() != wantDir {
		t.Fatalf("runtime config dir = %q, want %q", svc.runtimeCfg.FactoryDir(), wantDir)
	}
	if svc.runtimeCfg.FactoryConfig().Project != "alpha" {
		t.Fatalf("project = %q, want alpha", svc.runtimeCfg.FactoryConfig().Project)
	}
}

func TestFactoryService_ActivateNamedFactory_SwapsPersistedFactoryAndUpdatesCurrentPointer(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}

	wantDir := filepath.Join(rootDir, "beta")
	if svc.cfg.Dir != wantDir {
		t.Fatalf("service dir = %q, want %q", svc.cfg.Dir, wantDir)
	}
	if svc.runtimeCfg == nil {
		t.Fatal("expected runtime config after activation")
	}
	if got := svc.runtimeCfg.FactoryConfig().Project; got != "beta" {
		t.Fatalf("active project = %q, want beta", got)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	} else if got != "beta" {
		t.Fatalf("current factory pointer = %q, want beta", got)
	}
	if got, err := config.ResolveCurrentFactoryDir(rootDir); err != nil {
		t.Fatalf("ResolveCurrentFactoryDir: %v", err)
	} else if got != wantDir {
		t.Fatalf("resolved current dir = %q, want %q", got, wantDir)
	}
}

func TestFactoryService_ActivateNamedFactory_CanActivateSecondPersistedFactory(t *testing.T) {
	rootDir := t.TempDir()

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := config.PersistNamedFactory(rootDir, name, serviceNamedFactoryPayload(t, name)); err != nil {
			t.Fatalf("PersistNamedFactory(%s): %v", name, err)
		}
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}
	if err := svc.ActivateNamedFactory(context.Background(), "gamma"); err != nil {
		t.Fatalf("ActivateNamedFactory(gamma): %v", err)
	}

	if got := svc.runtimeCfg.FactoryConfig().Project; got != "gamma" {
		t.Fatalf("active project after second activation = %q, want gamma", got)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	} else if got != "gamma" {
		t.Fatalf("current factory pointer = %q, want gamma", got)
	}
}

func TestFactoryService_ActivateNamedFactory_RejectsNonIdleRuntime(t *testing.T) {
	svc := &FactoryService{
		factory: &aggregateSnapshotFactory{
			engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
				RuntimeStatus: interfaces.RuntimeStatusActive,
			},
		},
		logger: zap.NewNop(),
	}

	err := svc.ActivateNamedFactory(context.Background(), "beta")
	if err == nil {
		t.Fatal("expected non-idle activation to fail")
	}
	if !errors.Is(err, ErrFactoryActivationRequiresIdle) {
		t.Fatalf("expected ErrFactoryActivationRequiresIdle, got %v", err)
	}
}

func TestFactoryService_ActivateNamedFactory_RollsBackCurrentPointerWhenReplacementBuildFails(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	betaFactoryPath := filepath.Join(rootDir, "beta", interfaces.FactoryConfigFile)
	if err := os.WriteFile(betaFactoryPath, []byte(`{"project":"beta","workTypes":[`), 0o644); err != nil {
		t.Fatalf("corrupt beta factory.json: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err == nil {
		t.Fatal("expected replacement build failure")
	}

	wantCurrentDir := filepath.Join(rootDir, "alpha")
	if svc.cfg.Dir != wantCurrentDir {
		t.Fatalf("service dir after failed activation = %q, want %q", svc.cfg.Dir, wantCurrentDir)
	}
	if got := svc.runtimeCfg.FactoryConfig().Project; got != "alpha" {
		t.Fatalf("active project after failed activation = %q, want alpha", got)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	} else if got != "alpha" {
		t.Fatalf("current factory pointer after failed activation = %q, want alpha", got)
	}
	if got, err := config.ResolveCurrentFactoryDir(rootDir); err != nil {
		t.Fatalf("ResolveCurrentFactoryDir: %v", err)
	} else if got != wantCurrentDir {
		t.Fatalf("resolved current dir after failed activation = %q, want %q", got, wantCurrentDir)
	}
}

func TestFactoryService_GetCurrentNamedFactory_ReadsDurablePointerAndCanonicalPayload(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "beta"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(beta): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	current, err := svc.GetCurrentNamedFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentNamedFactory: %v", err)
	}
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current factory name = %q, want alpha", current.Name)
	}
	if current.Project == nil || *current.Project != "alpha" {
		t.Fatalf("current factory project = %#v, want alpha", current.Project)
	}
	if svc.runtimeCfg == nil || svc.runtimeCfg.FactoryConfig().Project != "beta" {
		t.Fatalf("service runtime project = %q, want unchanged beta runtime", svc.runtimeCfg.FactoryConfig().Project)
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
	if err := os.WriteFile(factoryPath, []byte(`{"project":"`+name+`","workTypes":[`), 0o644); err != nil {
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

func TestBuildFactoryService_LoadsWorkersFromConfig(t *testing.T) {
	dir := t.TempDir()

	// Config with a "worker-a" worker entry.
	cfg := map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
	writeFactoryJSON(t, dir, cfg)
	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestBuildFactoryService_WorkerWithoutAgentsMD_SkippedSilently(t *testing.T) {
	dir := t.TempDir()

	// Config with a "worker-a" worker entry, but no AGENTS.md on disk.
	cfg := map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": map[string]string{"workType": "task", "state": "failed"},
			},
		},
	}
	writeFactoryJSON(t, dir, cfg)
	writeWorkstationAgentsMD(t, dir, "process")
	// No worker AGENTS.md — worker should be silently skipped.

	ctx := context.Background()
	_, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService should succeed even with no AGENTS.md: %v", err)
	}
}

func TestBuildFactoryService_MissingFactoryJSON(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	_, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:    dir,
		Logger: zap.NewNop(),
	})
	if err == nil {
		t.Fatal("expected error when factory.json is missing")
	}
}

func TestBuildFactoryService_MockWorkersConfigPassedThrough(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStateIdle) {
		t.Errorf("expected IDLE state, got %s", snap.FactoryState)
	}
	if svc.cfg.MockWorkersConfig == nil {
		t.Fatal("expected mock-worker config to be preserved")
	}
	if len(svc.cfg.MockWorkersConfig.MockWorkers) != 0 {
		t.Fatalf("mock worker count = %d, want empty default accept config", len(svc.cfg.MockWorkersConfig.MockWorkers))
	}
}

func TestBuildFactoryService_RuntimeModePassedThrough(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before cancellation: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode factory service to stop")
	}
}

func cronFactoryConfig(schedule string) map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":    "poll-for-work",
				"kind":    "cron",
				"worker":  "cron-worker",
				"cron":    map[string]string{"schedule": schedule, "expiryWindow": "500ms"},
				"outputs": []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func cronFactoryConfigWithTriggerAtStart(schedule string, triggerAtStart bool) map[string]any {
	cfg := cronFactoryConfig(schedule)
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["cron"] = map[string]any{
		"schedule":       schedule,
		"expiryWindow":   "500ms",
		"triggerAtStart": triggerAtStart,
	}
	return cfg
}

func cronLoadedFactoryConfigForServiceTest(t *testing.T, factoryDir string, triggerAtStart bool) *config.LoadedFactoryConfig {
	t.Helper()

	ws := interfaces.FactoryWorkstationConfig{
		Name: "poll-for-work",
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:       "* * * * *",
			TriggerAtStart: triggerAtStart,
		},
		Outputs: []interfaces.IOConfig{{
			WorkTypeName: "task",
			StateName:    "init",
		}},
	}
	return newLoadedFactoryConfigForServiceTest(
		t,
		factoryDir,
		&interfaces.FactoryConfig{
			WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task"}},
			Workstations: []interfaces.FactoryWorkstationConfig{ws},
		},
		nil,
		map[string]*interfaces.FactoryWorkstationConfig{ws.Name: &ws},
	)
}

func cronFactoryConfigWithOutputState(schedule, outputState string) map[string]any {
	cfg := cronFactoryConfig(schedule)
	workTypes := cfg["workTypes"].([]map[string]any)
	task := workTypes[0]
	task["states"] = []map[string]string{
		{"name": "init", "type": "INITIAL"},
		{"name": "ready", "type": "PROCESSING"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["outputs"] = []map[string]string{{"workType": "task", "state": outputState}}
	return cfg
}

func requiredInputCronFactoryConfigWithExpiry(schedule, expiryWindow string) map[string]any {
	cron := map[string]string{"schedule": schedule}
	if expiryWindow != "" {
		cron["expiryWindow"] = expiryWindow
	}
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "signal",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":    "poll-with-input",
				"kind":    "cron",
				"worker":  "cron-worker",
				"cron":    cron,
				"inputs":  []map[string]string{{"workType": "signal", "state": "init"}},
				"outputs": []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func TestFactoryService_ServiceModeCronScheduleConfigStartsAndStopsService(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfig("* * * * *"))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode factory service to stop")
	}
}

func TestFactoryService_ServiceModeCronSchedulerUsesFakeClockAndStopsOnCancel(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfigWithTriggerAtStart("* * * * *", false))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 8)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronSubmissionQueued(t, observedSubmissions)

	fakeClock.Advance(time.Minute)
	record := waitForCronSubmission(t, observedSubmissions, time.Second)
	wantNominalAt := start.Add(time.Minute).Format(time.RFC3339Nano)
	if record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt] != wantNominalAt {
		cancelRun()
		t.Fatalf("cron nominal_at tag = %q, want %q", record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt], wantNominalAt)
	}
	if record.Request.Tags[cronWorkstationTag] != "poll-for-work" {
		cancelRun()
		t.Fatalf("cron workstation tag = %q, want poll-for-work", record.Request.Tags[cronWorkstationTag])
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode cron scheduler to stop")
	}

	fakeClock.Advance(time.Minute)
	assertNoCronSubmissionQueued(t, observedSubmissions)
}

func TestFactoryService_ServiceModeCronTriggerAtStartSubmitsOnceAndKeepsSchedule(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfigWithTriggerAtStart("* * * * *", true))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 8)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	startupRecord := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionNominalAt(t, startupRecord, start)
	waitForCompletedDispatchConsumingWorkID(t, svc, startupRecord.Request.WorkID, time.Second)

	waitForFakeClockWaiters(t, fakeClock, 1)
	assertNoCronSubmissionQueued(t, observedSubmissions)
	fakeClock.Advance(time.Minute)
	scheduledRecord := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionNominalAt(t, scheduledRecord, start.Add(time.Minute))
	if scheduledRecord.Request.WorkID == startupRecord.Request.WorkID {
		cancelRun()
		t.Fatal("scheduled cron fire reused startup trigger work ID")
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode cron scheduler to stop")
	}
}

func TestFactoryService_StartLiveRuntimeSidecars_BindsCronTriggerAtStartToReplacementRuntime(t *testing.T) {
	fakeClock := clockwork.NewFakeClock()
	currentFactory := &aggregateSnapshotFactory{}
	replacementFactory := &aggregateSnapshotFactory{}
	svc := &FactoryService{
		cfg:        &FactoryServiceConfig{RuntimeMode: interfaces.RuntimeModeService},
		factory:    currentFactory,
		runtimeCfg: cronLoadedFactoryConfigForServiceTest(t, "alpha", true),
		logger:     zap.NewNop(),
		clock:      fakeClock,
	}
	handle := &liveRuntimeHandle{
		runtime: &replacementFactoryRuntime{
			factory:    replacementFactory,
			runtimeCfg: cronLoadedFactoryConfigForServiceTest(t, "beta", true),
		},
	}
	sidecarCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.startLiveRuntimeSidecars(sidecarCtx, handle); err != nil {
		t.Fatalf("startLiveRuntimeSidecars: %v", err)
	}
	defer svc.stopLiveRuntimeSidecars(handle)

	if currentFactory.submitCalls != 0 {
		t.Fatalf("current runtime submit calls = %d, want 0", currentFactory.submitCalls)
	}
	if replacementFactory.submitCalls != 1 {
		t.Fatalf("replacement runtime submit calls = %d, want 1", replacementFactory.submitCalls)
	}
	if got := replacementFactory.submissions[0].Works[0].WorkTypeID; got != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("replacement runtime submission work type = %q, want %q", got, interfaces.SystemTimeWorkTypeID)
	}
}

// portos:func-length-exception owner=agent-factory reason=cron-service-fixture review=2026-07-18 removal=split-cron-ingress-fixture-before-next-cron-service-change
func TestFactoryService_CronTickSubmitsThroughEngineIngressAndAppearsInSnapshot(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfig("* * * * *"))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 16)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(func(record interfaces.FactorySubmissionRecord) {
				observedSubmissions <- record
			}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	ws := configuredCronWorkstationForServiceTest(t, svc, "poll-for-work")
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		cancelRun()
		t.Fatalf("submitCronTick: %v", err)
	}

	var record interfaces.FactorySubmissionRecord
	select {
	case record = <-observedSubmissions:
	case <-time.After(time.Second):
		cancelRun()
		t.Fatal("timed out waiting for cron submission record")
	}

	if record.Source != "external-submit" {
		t.Fatalf("cron submission source = %q, want external-submit", record.Source)
	}
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if record.Request.TargetState != interfaces.SystemTimePendingState {
		t.Fatalf("cron submission target state = %q, want %q", record.Request.TargetState, interfaces.SystemTimePendingState)
	}
	if record.Request.Tags[cronSourceTag] != "cron" {
		t.Fatalf("cron submission source tag = %q, want cron", record.Request.Tags[cronSourceTag])
	}
	if record.Request.Tags[cronWorkstationTag] != "poll-for-work" {
		t.Fatalf("cron submission workstation tag = %q, want poll-for-work", record.Request.Tags[cronWorkstationTag])
	}

	dispatch := waitForCompletedDispatchConsumingWorkID(t, svc, record.Request.WorkID, time.Second)
	matched := consumedTokenWithWorkID(dispatch.ConsumedTokens, record.Request.WorkID)
	if matched == nil {
		cancelRun()
		t.Fatalf("completed cron dispatch did not retain consumed time token %q: %#v", record.Request.WorkID, dispatch.ConsumedTokens)
	}
	if matched.Color.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron token work type = %q, want %q", matched.Color.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if matched.Color.TraceID == "" {
		t.Fatal("expected cron token to receive a trace ID")
	}
	if matched.Color.Name != cronSubmissionNamePref+"poll-for-work" {
		t.Fatalf("cron token name = %q, want %q", matched.Color.Name, cronSubmissionNamePref+"poll-for-work")
	}
	if matched.Color.Tags[cronSourceTag] != "cron" {
		t.Fatalf("cron token source tag = %q, want cron", matched.Color.Tags[cronSourceTag])
	}

	var payload map[string]string
	if err := json.Unmarshal(matched.Color.Payload, &payload); err != nil {
		t.Fatalf("cron token payload is not JSON: %v\npayload=%s", err, matched.Color.Payload)
	}
	if payload["cron_workstation"] != "poll-for-work" {
		t.Fatalf("cron payload workstation = %q, want poll-for-work", payload["cron_workstation"])
	}
	for _, key := range []string{"nominal_at", "due_at", "expires_at", "jitter", "source"} {
		if payload[key] == "" {
			t.Fatalf("expected cron payload to include %s, got %#v", key, payload)
		}
	}
	if tokens := matched.Color.Tags; tokens[interfaces.TimeWorkTagKeyNominalAt] == "" || tokens[interfaces.TimeWorkTagKeyDueAt] == "" || tokens[interfaces.TimeWorkTagKeyExpiresAt] == "" {
		t.Fatalf("expected cron timing tags, got %#v", tokens)
	}
	output := waitForTokenInPlaceByParent(t, svc, "task:init", record.Request.WorkID, time.Second)
	if output.Color.WorkTypeID != "task" {
		cancelRun()
		t.Fatalf("cron worker-backed output work type = %q, want task", output.Color.WorkTypeID)
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode cron watcher to stop")
	}
}

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func waitForCronSubmission(t *testing.T, submissions <-chan interfaces.FactorySubmissionRecord, timeout time.Duration) interfaces.FactorySubmissionRecord {
	t.Helper()
	select {
	case record := <-submissions:
		if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
			t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
		}
		return record
	case <-time.After(timeout):
		t.Fatal("timed out waiting for cron submission")
	}
	return interfaces.FactorySubmissionRecord{}
}

func assertCronSubmissionNominalAt(t *testing.T, record interfaces.FactorySubmissionRecord, want time.Time) {
	t.Helper()
	got := record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt]
	if got != want.Format(time.RFC3339Nano) {
		t.Fatalf("cron nominal_at tag = %q, want %q", got, want.Format(time.RFC3339Nano))
	}
	if record.Request.Tags[cronWorkstationTag] != "poll-for-work" {
		t.Fatalf("cron workstation tag = %q, want poll-for-work", record.Request.Tags[cronWorkstationTag])
	}
}

func assertNoCronSubmissionQueued(t *testing.T, submissions <-chan interfaces.FactorySubmissionRecord) {
	t.Helper()
	select {
	case record := <-submissions:
		t.Fatalf("cron submission observed unexpectedly: %#v", record)
	default:
	}
}

func matchedTokenSnapshotTokensInPlace(t *testing.T, svc *FactoryService, placeID string) []interfaces.Token {
	t.Helper()
	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return snap.Marking.TokensInPlace(placeID)
}

func configuredCronWorkstationForServiceTest(t *testing.T, svc *FactoryService, name string) interfaces.FactoryWorkstationConfig {
	t.Helper()
	if svc == nil || svc.runtimeCfg == nil {
		t.Fatal("expected loaded service runtime config")
	}
	ws, ok := svc.runtimeCfg.Workstation(name)
	if !ok {
		t.Fatalf("expected cron workstation config %q", name)
	}
	return *ws
}

func waitForCompletedDispatchConsumingWorkID(t *testing.T, svc *FactoryService, workID string, timeout time.Duration) interfaces.CompletedDispatch {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot dispatch history: %v", err)
		}
		for _, dispatch := range snap.DispatchHistory {
			if consumedTokenWithWorkID(dispatch.ConsumedTokens, workID) != nil {
				return dispatch
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for completed dispatch consuming work %q", workID)
	return interfaces.CompletedDispatch{}
}

func consumedTokenWithWorkID(tokens []interfaces.Token, workID string) *interfaces.Token {
	for i := range tokens {
		if tokens[i].Color.WorkID == workID {
			return &tokens[i]
		}
	}
	return nil
}

func nonBlockingSubmissionRecorder(records chan<- interfaces.FactorySubmissionRecord) func(interfaces.FactorySubmissionRecord) {
	return func(record interfaces.FactorySubmissionRecord) {
		select {
		case records <- record:
		default:
		}
	}
}

func waitForTokenInPlaceByParent(t *testing.T, svc *FactoryService, placeID string, parentID string, timeout time.Duration) interfaces.Token {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot output token: %v", err)
		}
		for _, token := range snap.Marking.TokensInPlace(placeID) {
			if token.Color.ParentID == parentID {
				return token
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for token in %s with parent %q", placeID, parentID)
	return interfaces.Token{}
}

func waitForTokenInPlace(t *testing.T, svc *FactoryService, placeID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if tokens := matchedTokenSnapshotTokensInPlace(t, svc, placeID); len(tokens) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for any token in %s", placeID)
}

func TestFactoryService_CronTickTargetsInternalTimePlaceDespiteConfiguredOutputState(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfigWithOutputState("* * * * *", "ready"))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 16)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	ws := configuredCronWorkstationForServiceTest(t, svc, "poll-for-work")
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		cancelRun()
		t.Fatalf("submitCronTick: %v", err)
	}

	var record interfaces.FactorySubmissionRecord
	select {
	case record = <-observedSubmissions:
	case <-time.After(time.Second):
		cancelRun()
		t.Fatal("timed out waiting for cron submission record")
	}

	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		cancelRun()
		t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if record.Request.TargetState != interfaces.SystemTimePendingState {
		cancelRun()
		t.Fatalf("cron submission target state = %q, want %q", record.Request.TargetState, interfaces.SystemTimePendingState)
	}

	dispatch := waitForCompletedDispatchConsumingWorkID(t, svc, record.Request.WorkID, time.Second)
	if consumedTokenWithWorkID(dispatch.ConsumedTokens, record.Request.WorkID) == nil {
		cancelRun()
		t.Fatalf("completed cron dispatch did not retain consumed time token %q: %#v", record.Request.WorkID, dispatch.ConsumedTokens)
	}
	output := waitForTokenInPlaceByParent(t, svc, "task:ready", record.Request.WorkID, time.Second)
	if output.Color.WorkTypeID != "task" {
		cancelRun()
		t.Fatalf("cron worker-backed output work type = %q, want task", output.Color.WorkTypeID)
	}
	if tokens := matchedTokenSnapshotTokensInPlace(t, svc, "task:init"); len(tokens) != 0 {
		cancelRun()
		t.Fatalf("cron created customer token in initial state despite configured output state: %#v", tokens)
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode cron watcher to stop")
	}
}

// portos:func-length-exception owner=agent-factory reason=cron-expiry-service-fixture review=2026-07-18 removal=split-cron-expiry-fixture-before-next-cron-service-change
func TestFactoryService_RequiredInputCronKeepsTimeWorkPendingWhenInputMissing(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := t.TempDir()
	writeFactoryJSON(t, dir, requiredInputCronFactoryConfigWithExpiry("* * * * *", "40ms"))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 16)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	ws := configuredCronWorkstationForServiceTest(t, svc, "poll-with-input")
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		cancelRun()
		t.Fatalf("submitCronTick: %v", err)
	}

	var firstRecord interfaces.FactorySubmissionRecord
	select {
	case firstRecord = <-observedSubmissions:
	case <-time.After(time.Second):
		cancelRun()
		t.Fatal("timed out waiting for required-input cron time submission")
	}
	if firstRecord.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		cancelRun()
		t.Fatalf("required-input cron submission work type = %q, want %q", firstRecord.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if firstRecord.Request.Tags[cronWorkstationTag] != "poll-with-input" {
		cancelRun()
		t.Fatalf("required-input cron workstation tag = %q, want poll-with-input", firstRecord.Request.Tags[cronWorkstationTag])
	}

	var pendingSnap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	pendingDeadline := time.Now().Add(time.Second)
	for time.Now().Before(pendingDeadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			cancelRun()
			t.Fatalf("GetEngineStateSnapshot pending time work: %v", err)
		}
		for _, token := range snap.Marking.TokensInPlace(interfaces.SystemTimePendingPlaceID) {
			if token.Color.WorkID == firstRecord.Request.WorkID {
				pendingSnap = snap
				break
			}
		}
		if pendingSnap != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pendingSnap == nil {
		cancelRun()
		t.Fatalf("timed out waiting for required-input cron time token in %s", interfaces.SystemTimePendingPlaceID)
	}
	if pendingSnap.InFlightCount != 0 || len(pendingSnap.Dispatches) != 0 {
		cancelRun()
		t.Fatalf("required-input cron dispatched while input was missing: inflight=%d dispatches=%#v", pendingSnap.InFlightCount, pendingSnap.Dispatches)
	}
	if tokens := pendingSnap.Marking.TokensInPlace("task:init"); len(tokens) != 0 {
		cancelRun()
		t.Fatalf("required-input cron created task output before input existed: %#v", tokens)
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode cron watcher to stop")
	}
}

func TestFactoryService_CronTickTimeoutFailureIsClassifiedAndBounded(t *testing.T) {
	logCore, observedLogs := observer.New(zap.InfoLevel)
	mock := &aggregateSnapshotFactory{
		submitFunc: func(ctx context.Context, _ interfaces.WorkRequest) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	svc := &FactoryService{
		factory:    mock,
		logger:     zap.New(logCore),
		runtimeCfg: newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{Workstations: []interfaces.FactoryWorkstationConfig{{Name: "poll-for-work", Limits: interfaces.WorkstationLimits{MaxExecutionTime: "1ms"}}}}, nil, nil),
	}

	err := svc.submitCronTick(context.Background(), cronWorkstationConfigForTest("poll-for-work"), time.Now())
	if err == nil {
		t.Fatal("expected timed-out cron tick to fail after bounded retries")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cron tick error = %v, want deadline exceeded classification", err)
	}
	if mock.submitCalls != cronMaxRetries+1 {
		t.Fatalf("cron submit attempts = %d, want %d", mock.submitCalls, cronMaxRetries+1)
	}
	if len(mock.submissions) != cronMaxRetries+1 {
		t.Fatalf("recorded cron work requests = %d, want %d", len(mock.submissions), cronMaxRetries+1)
	}
	submitted := mock.submissions[len(mock.submissions)-1]
	if submitted.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("cron submitted request type = %q, want %q", submitted.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
	}
	if len(submitted.Works) != 1 || submitted.Works[0].WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron submitted works = %#v, want one internal time work item", submitted.Works)
	}
	if observedLogs.FilterMessage("cron watcher trigger retrying").Len() != cronMaxRetries {
		t.Fatalf("retry log count = %d, want %d", observedLogs.FilterMessage("cron watcher trigger retrying").Len(), cronMaxRetries)
	}
	if observedLogs.FilterMessage("cron watcher trigger exhausted").Len() != 1 {
		t.Fatal("expected exhausted timeout log after bounded cron retries")
	}

	failure := classifyCronTriggerFailure(err)
	if !failure.retryable || failure.Family != interfaces.ProviderErrorFamilyRetryable || failure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("cron timeout classification = %#v, want retryable timeout", failure)
	}
}

func TestFactoryService_CronTickRetryableFailureRetriesBeforeSuccess(t *testing.T) {
	retryErr := errors.New("temporary submission ingress failure")
	mock := &aggregateSnapshotFactory{}
	attempt := 0
	mock.submitFunc = func(_ context.Context, _ interfaces.WorkRequest) error {
		attempt++
		if attempt <= cronMaxRetries {
			return retryErr
		}
		return nil
	}
	logCore, observedLogs := observer.New(zap.InfoLevel)
	svc := &FactoryService{
		factory: mock,
		logger:  zap.New(logCore),
	}

	if err := svc.submitCronTick(context.Background(), cronWorkstationConfigForTest("poll-for-work"), time.Now()); err != nil {
		t.Fatalf("cron tick should succeed after retryable failures: %v", err)
	}
	if mock.submitCalls != cronMaxRetries+1 {
		t.Fatalf("cron submit attempts = %d, want %d", mock.submitCalls, cronMaxRetries+1)
	}
	if observedLogs.FilterMessage("cron watcher trigger retrying").Len() != cronMaxRetries {
		t.Fatalf("retry log count = %d, want %d", observedLogs.FilterMessage("cron watcher trigger retrying").Len(), cronMaxRetries)
	}
	if observedLogs.FilterMessage("cron watcher trigger exhausted").Len() != 0 {
		t.Fatal("cron retry success should not log exhaustion")
	}
}

func cronWorkstationConfigForTest(name string) interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name: name,
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{Schedule: "* * * * *"},
		Outputs: []interfaces.IOConfig{
			{WorkTypeName: "task", StateName: "init"},
		},
	}
}

func TestFactoryService_BatchModeDoesNotStartCronWatchers(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, cronFactoryConfig("* * * * *"))
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 1)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(func(record interfaces.FactorySubmissionRecord) {
				observedSubmissions <- record
			}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	select {
	case record := <-observedSubmissions:
		t.Fatalf("batch-mode cron watcher submitted unexpectedly: %#v", record)
	default:
	}
}

func TestBuildFactoryService_RecordModeWritesInitialArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RecordPath:        recordPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	if artifact.Factory.Workers == nil {
		t.Fatal("expected embedded factory config")
	}
	if artifact.Factory.FactoryDir == nil || *artifact.Factory.FactoryDir != dir {
		t.Fatalf("factory dir = %#v, want %q", artifact.Factory.FactoryDir, dir)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-runtime-log-fixture review=2026-07-18 removal=split-runtime-log-fixture-before-next-runtime-logging-change
func TestFactoryService_RunWritesStructuredRuntimeLogFile(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	logDir := filepath.Join(homeDir, ".agent-factory", "logs")
	runtimeInstanceID := "runtime-log-test"
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RuntimeInstanceID: runtimeInstanceID,
		RuntimeLogConfig: logging.RuntimeLogConfig{
			MaxSize:    9,
			MaxBackups: 8,
			MaxAge:     7,
			Compress:   true,
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	logPath := filepath.Join(logDir, runtimeInstanceID+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("expected at least one runtime log record in %s", logPath)
	}

	var foundStartup bool
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		if record["runtime_instance_id"] != runtimeInstanceID {
			t.Fatalf("runtime_instance_id = %#v, want %q", record["runtime_instance_id"], runtimeInstanceID)
		}
		if record["msg"] == "factory started" {
			foundStartup = true
			if record["runtime_log_path"] != logPath {
				t.Fatalf("runtime_log_path = %#v, want %q", record["runtime_log_path"], logPath)
			}
			if record["runtime_log_appender"] != logging.RuntimeLogAppenderZapRollingFile {
				t.Fatalf("runtime_log_appender = %#v, want %q", record["runtime_log_appender"], logging.RuntimeLogAppenderZapRollingFile)
			}
			if record["runtime_log_max_size_mb"] != float64(9) {
				t.Fatalf("runtime_log_max_size_mb = %#v, want 9", record["runtime_log_max_size_mb"])
			}
			if record["runtime_log_max_backups"] != float64(8) {
				t.Fatalf("runtime_log_max_backups = %#v, want 8", record["runtime_log_max_backups"])
			}
			if record["runtime_log_max_age_days"] != float64(7) {
				t.Fatalf("runtime_log_max_age_days = %#v, want 7", record["runtime_log_max_age_days"])
			}
			if record["runtime_log_compress"] != true {
				t.Fatalf("runtime_log_compress = %#v, want true", record["runtime_log_compress"])
			}
			if record["runtime_env_log_channel"] != logging.RuntimeEnvLogChannelRecord {
				t.Fatalf("runtime_env_log_channel = %#v, want %q", record["runtime_env_log_channel"], logging.RuntimeEnvLogChannelRecord)
			}
			if record["runtime_success_command_output"] != logging.RuntimeSuccessCommandOutputPolicy {
				t.Fatalf("runtime_success_command_output = %#v, want %q", record["runtime_success_command_output"], logging.RuntimeSuccessCommandOutputPolicy)
			}
			if record["runtime_failure_command_output"] != logging.RuntimeFailureCommandOutputPolicy {
				t.Fatalf("runtime_failure_command_output = %#v, want %q", record["runtime_failure_command_output"], logging.RuntimeFailureCommandOutputPolicy)
			}
			if record["record_command_diagnostics"] != logging.RuntimeRecordCommandDiagnosticsMode {
				t.Fatalf("record_command_diagnostics = %#v, want %q", record["record_command_diagnostics"], logging.RuntimeRecordCommandDiagnosticsMode)
			}
		}
	}
	if !foundStartup {
		t.Fatalf("expected factory started record in runtime log:\n%s", data)
	}
}

func TestFactoryService_RunWritesCorrelationFieldsToRuntimeLog(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		RequestID:  "request-log-context",
		WorkID:     "work-log-context",
		WorkTypeID: "task",
		TraceID:    "trace-log-context",
		Payload:    []byte(`{"task":"correlate me"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	runtimeInstanceID := "runtime-log-context-test"
	logDir := t.TempDir()
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RuntimeInstanceID: runtimeInstanceID,
		RuntimeLogDir:     logDir,
		WorkFile:          workFile,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	logPath := filepath.Join(logDir, runtimeInstanceID+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		if record["msg"] != "dispatcher: dispatching work to worker" {
			continue
		}
		if record["request_id"] != "request-log-context" {
			t.Fatalf("request_id = %#v, want request-log-context in record %#v", record["request_id"], record)
		}
		if record["trace_id"] != "trace-log-context" {
			t.Fatalf("trace_id = %#v, want trace-log-context in record %#v", record["trace_id"], record)
		}
		if record["work_id"] != "work-log-context" {
			t.Fatalf("work_id = %#v, want work-log-context in record %#v", record["work_id"], record)
		}
		return
	}
	t.Fatalf("expected correlated dispatcher log record in runtime log:\n%s", data)
}

// portos:func-length-exception owner=agent-factory reason=legacy-runtime-log-fixture review=2026-07-18 removal=split-worker-pool-log-fixture-before-next-runtime-logging-change
func TestFactoryService_RunWritesWorkerPoolLifecycleEventsToRuntimeLog(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		RequestID:  "request-worker-pool-log",
		WorkID:     "work-worker-pool-log",
		WorkTypeID: "task",
		TraceID:    "trace-worker-pool-log",
		Payload:    []byte(`{"task":"exercise worker pool lifecycle logs"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	runtimeInstanceID := "runtime-worker-pool-log-test"
	logDir := t.TempDir()
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		Logger:            zap.NewNop(),
		RuntimeInstanceID: runtimeInstanceID,
		RuntimeLogDir:     logDir,
		WorkFile:          workFile,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	logPath := filepath.Join(logDir, runtimeInstanceID+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}

	wantStatuses := map[string]string{
		workers.WorkLogEventWorkerPoolSubmitted:         "submitted",
		workers.WorkLogEventWorkerPoolExecutorEntered:   "entered_executor",
		workers.WorkLogEventWorkerPoolResponseSubmitted: "response_submitted",
	}
	found := make(map[string]bool, len(wantStatuses))
	var dispatchID string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		eventName, ok := record["event_name"].(string)
		if !ok {
			continue
		}
		wantStatus, ok := wantStatuses[eventName]
		if !ok {
			continue
		}
		if record["request_id"] != work.RequestID {
			t.Fatalf("request_id = %#v, want %q in record %#v", record["request_id"], work.RequestID, record)
		}
		if record["trace_id"] != work.TraceID {
			t.Fatalf("trace_id = %#v, want %q in record %#v", record["trace_id"], work.TraceID, record)
		}
		if record["work_id"] != work.WorkID {
			t.Fatalf("work_id = %#v, want %q in record %#v", record["work_id"], work.WorkID, record)
		}
		if record["status"] != wantStatus {
			t.Fatalf("status = %#v, want %q in record %#v", record["status"], wantStatus, record)
		}
		if record["dispatch_id"] == "" {
			t.Fatalf("expected dispatch_id in record %#v", record)
		}
		if dispatchID == "" {
			dispatchID, _ = record["dispatch_id"].(string)
		} else if record["dispatch_id"] != dispatchID {
			t.Fatalf("dispatch_id = %#v, want same dispatch_id %q in record %#v", record["dispatch_id"], dispatchID, record)
		}
		found[eventName] = true
	}
	for eventName := range wantStatuses {
		if !found[eventName] {
			t.Fatalf("expected worker-pool lifecycle event %q in runtime log:\n%s", eventName, data)
		}
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-runtime-log-fixture review=2026-07-18 removal=split-command-runner-log-fixture-before-next-runtime-logging-change
func TestFactoryService_RunWritesCommandRunnerEventsWithOutputsToRuntimeLog(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeScriptWorkerAgentsMDWithCommand(t, dir, "worker-a", "script-tool", []string{"--mode", "fixture"})
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		RequestID:  "request-command-runner-log",
		WorkID:     "work-command-runner-log",
		WorkTypeID: "task",
		TraceID:    "trace-command-runner-log",
		Payload:    []byte(`{"task":"exercise command runner logs"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	runtimeInstanceID := "runtime-command-runner-log-test"
	logDir := t.TempDir()
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                   dir,
		Logger:                zap.NewNop(),
		RuntimeInstanceID:     runtimeInstanceID,
		RuntimeLogDir:         logDir,
		WorkFile:              workFile,
		CommandRunnerOverride: recordingDiagnosticsCommandRunner{},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	logPath := filepath.Join(logDir, runtimeInstanceID+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}

	found := map[string]bool{
		workers.WorkLogEventCommandRunnerRequested: false,
		workers.WorkLogEventCommandRunnerCompleted: false,
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		eventName, ok := record["event_name"].(string)
		if !ok {
			continue
		}
		if _, ok := found[eventName]; !ok {
			continue
		}
		if record["request_id"] != work.RequestID {
			t.Fatalf("request_id = %#v, want %q in record %#v", record["request_id"], work.RequestID, record)
		}
		if record["trace_id"] != work.TraceID {
			t.Fatalf("trace_id = %#v, want %q in record %#v", record["trace_id"], work.TraceID, record)
		}
		if record["work_id"] != work.WorkID {
			t.Fatalf("work_id = %#v, want %q in record %#v", record["work_id"], work.WorkID, record)
		}
		if record["command"] != "script-tool" {
			t.Fatalf("command = %#v, want script-tool in record %#v", record["command"], record)
		}
		switch eventName {
		case workers.WorkLogEventCommandRunnerRequested:
			if record["status"] != "requested" {
				t.Fatalf("request status = %#v, want requested in record %#v", record["status"], record)
			}
		case workers.WorkLogEventCommandRunnerCompleted:
			if record["status"] != "succeeded" {
				t.Fatalf("completion status = %#v, want succeeded in record %#v", record["status"], record)
			}
			if _, ok := record["stdout"]; ok {
				t.Fatalf("completion record includes unexpected stdout on success in record %#v", record)
			}
			if _, ok := record["stderr"]; ok {
				t.Fatalf("completion record includes unexpected stderr on success in record %#v", record)
			}
			if record["exit_code"] != float64(0) {
				t.Fatalf("exit_code = %#v, want 0 in record %#v", record["exit_code"], record)
			}
		}
		found[eventName] = true
	}
	for eventName, ok := range found {
		if !ok {
			t.Fatalf("expected command runner event %q in runtime log:\n%s", eventName, data)
		}
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-runtime-log-fixture review=2026-07-18 removal=split-env-dedup-log-fixture-before-next-runtime-logging-change
func TestFactoryService_RunDeduplicatesEnvPayloadBetweenRecordDiagnosticsAndRuntimeSystemLogs(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeScriptWorkerAgentsMDWithCommand(t, dir, "worker-a", "script-tool", []string{"--mode", "fixture"})
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		RequestID:  "request-env-dedupe",
		WorkID:     "work-env-dedupe",
		WorkTypeID: "task",
		TraceID:    "trace-env-dedupe",
		Payload:    []byte(`{"task":"assert env dedupe between channels"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	runner := &recordingCommandRunnerWithCapture{}

	runtimeInstanceID := "runtime-env-dedupe-test"
	logDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                   dir,
		Logger:                zap.NewNop(),
		RuntimeInstanceID:     runtimeInstanceID,
		RuntimeLogDir:         logDir,
		WorkFile:              workFile,
		RecordPath:            recordPath,
		CommandRunnerOverride: runner,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("recorded completions = %d, want 1", len(completions))
	}
	completion := completions[0].Payload
	encodedCompletion, err := json.Marshal(completion)
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}
	if strings.Contains(string(encodedCompletion), "env_count") || strings.Contains(string(encodedCompletion), "env_keys") {
		t.Fatalf("recorded completion leaked environment diagnostics: %s", encodedCompletion)
	}
	if len(runner.request.Env) == 0 {
		t.Fatalf("captured command request had no env entries")
	}

	logPath := filepath.Join(logDir, runtimeInstanceID+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	records := parseRuntimeLogRecords(t, string(data))
	requestRecord := findRuntimeLogEvent(records, workers.WorkLogEventCommandRunnerRequested)
	if requestRecord == nil {
		t.Fatalf("missing requested command-runner event in runtime log:\n%s", data)
	}
	if _, ok := requestRecord["env_count"]; ok {
		t.Fatalf("requested command-runner event includes env_count (should be record-only): %#v", requestRecord["env_count"])
	}
	if requestRecord["command"] != "script-tool" {
		t.Fatalf("command = %#v, want script-tool in requested record %#v", requestRecord["command"], requestRecord)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-runtime-log-fixture review=2026-07-18 removal=split-verbose-log-fixture-before-next-runtime-logging-change
func TestFactoryService_RunMirrorsVerboseCommandRunnerEventsToFileAndLogger(t *testing.T) {
	runCommandLogFixture := func(t *testing.T, verbose bool) (string, *observer.ObservedLogs) {
		t.Helper()

		dir := t.TempDir()
		writeFactoryJSON(t, dir, minimalFactoryConfig())
		writeScriptWorkerAgentsMDWithCommand(t, dir, "worker-a", "script-tool", []string{"--mode", "fixture"})
		writeWorkstationAgentsMD(t, dir, "process")
		if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
			t.Fatalf("create inputs dir: %v", err)
		}

		workFile := filepath.Join(dir, "initial-work.json")
		work := interfaces.SubmitRequest{
			RequestID:  "request-verbose-command-log",
			WorkID:     "work-verbose-command-log",
			WorkTypeID: "task",
			TraceID:    "trace-verbose-command-log",
			Payload:    []byte(`{"task":"exercise verbose command logs"}`),
		}
		writeWorkRequestFile(t, workFile, work)

		core, observed := observer.New(zap.InfoLevel)
		runtimeInstanceID := "runtime-verbose-command-log-test"
		logDir := t.TempDir()
		svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
			Dir:                   dir,
			Logger:                zap.New(core),
			Verbose:               verbose,
			RuntimeInstanceID:     runtimeInstanceID,
			RuntimeLogDir:         logDir,
			WorkFile:              workFile,
			CommandRunnerOverride: recordingDiagnosticsCommandRunner{},
		})
		if err != nil {
			t.Fatalf("BuildFactoryService: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := svc.Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}

		logPath := filepath.Join(logDir, runtimeInstanceID+".log")
		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("read runtime log %s: %v", logPath, err)
		}
		return string(data), observed
	}

	defaultLog, defaultObserved := runCommandLogFixture(t, false)
	if runtimeLogHasEvent(t, defaultLog, workers.WorkLogEventCommandRunnerRequestDetails) {
		t.Fatalf("default runtime log unexpectedly contained verbose request details:\n%s", defaultLog)
	}
	if len(defaultObserved.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerRequestDetails)).All()) != 0 {
		t.Fatal("default logger unexpectedly received verbose request details")
	}

	verboseLog, verboseObserved := runCommandLogFixture(t, true)
	verboseRecord := runtimeLogEventRecord(t, verboseLog, workers.WorkLogEventCommandRunnerRequestDetails)
	if verboseRecord["request_id"] != "request-verbose-command-log" {
		t.Fatalf("request_id = %#v, want request-verbose-command-log in record %#v", verboseRecord["request_id"], verboseRecord)
	}
	if verboseRecord["trace_id"] != "trace-verbose-command-log" {
		t.Fatalf("trace_id = %#v, want trace-verbose-command-log in record %#v", verboseRecord["trace_id"], verboseRecord)
	}
	if verboseRecord["work_id"] != "work-verbose-command-log" {
		t.Fatalf("work_id = %#v, want work-verbose-command-log in record %#v", verboseRecord["work_id"], verboseRecord)
	}
	if !runtimeLogHasEvent(t, verboseLog, workers.WorkLogEventCommandRunnerOutputDetails) {
		t.Fatalf("verbose runtime log missing output details:\n%s", verboseLog)
	}
	if len(verboseObserved.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerRequestDetails)).All()) == 0 {
		t.Fatal("verbose logger did not receive request detail record")
	}
	if len(verboseObserved.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerOutputDetails)).All()) == 0 {
		t.Fatal("verbose logger did not receive output detail record")
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-runtime-log-smoke review=2026-07-18 removal=split-correlated-work-log-smoke-before-next-runtime-logging-change
func TestFactoryService_RunWritesEndToEndCorrelatedWorkLogSmoke(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeScriptWorkerAgentsMDWithCommand(t, dir, "worker-a", "script-tool", []string{"--mode", "smoke"})
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		RequestID:  "request-work-log-smoke",
		WorkID:     "work-log-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-work-log-smoke",
		Payload:    []byte(`{"task":"correlate runtime logs with replay records"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	core, observed := observer.New(zap.InfoLevel)
	runtimeInstanceID := "runtime-work-log-smoke-test"
	logDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                   dir,
		Logger:                zap.New(core),
		Verbose:               true,
		RuntimeInstanceID:     runtimeInstanceID,
		RuntimeLogDir:         logDir,
		RecordPath:            recordPath,
		WorkFile:              workFile,
		CommandRunnerOverride: recordingDiagnosticsCommandRunner{},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	submissions := serviceReplayWorkRequestEvents(t, artifact)
	if len(submissions) != 1 {
		t.Fatalf("recorded submissions = %d, want 1", len(submissions))
	}
	submission := submissions[0]
	if serviceStringValue(submission.Event.Context.RequestId) != work.RequestID ||
		serviceFirstStringValue(submission.Event.Context.TraceIds) != work.TraceID ||
		serviceFirstStringValue(submission.Event.Context.WorkIds) != work.WorkID {
		t.Fatalf("recorded request IDs = (%q, %q, %q), want (%q, %q, %q)",
			serviceStringValue(submission.Event.Context.RequestId),
			serviceFirstStringValue(submission.Event.Context.TraceIds),
			serviceFirstStringValue(submission.Event.Context.WorkIds),
			work.RequestID, work.TraceID, work.WorkID)
	}
	dispatches := serviceReplayDispatchCreatedEvents(t, artifact)
	if len(dispatches) != 1 {
		t.Fatalf("recorded dispatches = %d, want 1", len(dispatches))
	}
	dispatch := dispatches[0]
	if serviceStringValue(dispatch.Event.Context.RequestId) != work.RequestID ||
		serviceFirstStringValue(dispatch.Event.Context.TraceIds) != work.TraceID ||
		serviceFirstStringValue(dispatch.Event.Context.WorkIds) != work.WorkID {
		t.Fatalf("recorded dispatch event metadata = %#v, want request/trace/work IDs from %#v",
			dispatch.Event.Context, work)
	}
	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("recorded completions = %d, want 1", len(completions))
	}
	completion := completions[0].Payload
	dispatchID := serviceStringValue(dispatch.Event.Context.DispatchId)
	completionDispatchID := serviceStringValue(completions[0].Event.Context.DispatchId)
	if completionDispatchID != dispatchID {
		t.Fatalf("recorded completion dispatch ID = %q, want %q", completionDispatchID, dispatchID)
	}
	if serviceStringValue(completion.Output) != "script done" {
		t.Fatalf("recorded completion output = %q, want script done", serviceStringValue(completion.Output))
	}

	logPath := filepath.Join(logDir, runtimeInstanceID+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	records := parseRuntimeLogRecords(t, string(data))
	requiredEvents := map[string]struct {
		status     string
		dispatchID string
	}{
		workers.WorkLogEventWorkerPoolSubmitted:         {status: "submitted", dispatchID: dispatchID},
		workers.WorkLogEventWorkerPoolExecutorEntered:   {status: "entered_executor", dispatchID: dispatchID},
		workers.WorkLogEventWorkerPoolResponseSubmitted: {status: "response_submitted", dispatchID: dispatchID},
		workers.WorkLogEventCommandRunnerRequested:      {status: "requested"},
		workers.WorkLogEventCommandRunnerCompleted:      {status: "succeeded"},
		workers.WorkLogEventCommandRunnerRequestDetails: {status: "verbose"},
		workers.WorkLogEventCommandRunnerOutputDetails:  {status: "verbose"},
	}
	for eventName, want := range requiredEvents {
		record := findRuntimeLogEvent(records, eventName)
		if record == nil {
			t.Fatalf("expected runtime log event %q in:\n%s", eventName, data)
		}
		if record["request_id"] != work.RequestID {
			t.Fatalf("%s request_id = %#v, want %q in record %#v", eventName, record["request_id"], work.RequestID, record)
		}
		if record["trace_id"] != work.TraceID {
			t.Fatalf("%s trace_id = %#v, want %q in record %#v", eventName, record["trace_id"], work.TraceID, record)
		}
		if record["work_id"] != work.WorkID {
			t.Fatalf("%s work_id = %#v, want %q in record %#v", eventName, record["work_id"], work.WorkID, record)
		}
		if record["status"] != want.status {
			t.Fatalf("%s status = %#v, want %q in record %#v", eventName, record["status"], want.status, record)
		}
		if want.dispatchID != "" && record["dispatch_id"] != want.dispatchID {
			t.Fatalf("%s dispatch_id = %#v, want %q in record %#v", eventName, record["dispatch_id"], want.dispatchID, record)
		}
	}

	completionRecord := findRuntimeLogEvent(records, workers.WorkLogEventCommandRunnerCompleted)
	if _, ok := completionRecord["stdout"]; ok {
		t.Fatalf("command completion includes unexpected stdout in record %#v", completionRecord)
	}
	if _, ok := completionRecord["stderr"]; ok {
		t.Fatalf("command completion includes unexpected stderr in record %#v", completionRecord)
	}
	if len(observed.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerRequestDetails)).All()) == 0 {
		t.Fatal("verbose command request details were not mirrored to the command-line logger")
	}
	if len(observed.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerOutputDetails)).All()) == 0 {
		t.Fatal("verbose command output details were not mirrored to the command-line logger")
	}
}

func runtimeLogHasEvent(t *testing.T, data, eventName string) bool {
	t.Helper()
	return runtimeLogEventRecord(t, data, eventName) != nil
}

func runtimeLogEventRecord(t *testing.T, data, eventName string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		if record["event_name"] == eventName {
			return record
		}
	}
	return nil
}

func parseRuntimeLogRecords(t *testing.T, data string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		records = append(records, record)
	}
	return records
}

func findRuntimeLogEvent(records []map[string]any, eventName string) map[string]any {
	for _, record := range records {
		if record["event_name"] == eventName {
			return record
		}
	}
	return nil
}

// portos:func-length-exception owner=agent-factory reason=record-mode-service-fixture review=2026-07-18 removal=split-recording-setup-run-and-artifact-assertions-before-next-record-mode-change
func TestBuildFactoryService_RecordModeRecordsSubmittedWorkAtEngineTick(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		WorkTypeID: "task",
		Name:       "from-work-file",
		TraceID:    "trace-work-file",
		Payload:    []byte(`{"task":"record me"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RecordPath:        recordPath,
		WorkFile:          workFile,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	assertReplayArtifactStoresCanonicalEvents(t, recordPath, artifact, []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeRunResponse,
	})
	submissions := serviceReplayWorkRequestEvents(t, artifact)
	if len(submissions) != 1 {
		t.Fatalf("expected 1 recorded submission, got %d", len(submissions))
	}
	submission := submissions[0]
	if submission.Event.Context.Tick != 1 {
		t.Fatalf("submission observed tick = %d, want 1", submission.Event.Context.Tick)
	}
	if serviceFirstStringValue(submission.Event.Context.TraceIds) != "trace-work-file" {
		t.Fatalf("recorded trace ID = %q, want trace-work-file", serviceFirstStringValue(submission.Event.Context.TraceIds))
	}
	if serviceStringValue(submission.Payload.Source) != "external-submit" {
		t.Fatalf("recorded source = %q, want external-submit", serviceStringValue(submission.Payload.Source))
	}
	dispatches := serviceReplayDispatchCreatedEvents(t, artifact)
	if len(dispatches) != 1 {
		t.Fatalf("expected 1 recorded dispatch, got %d", len(dispatches))
	}
	dispatch := dispatches[0]
	if dispatch.Event.Context.Tick < submission.Event.Context.Tick {
		t.Fatalf("dispatch created tick = %d, want no earlier than submission tick %d", dispatch.Event.Context.Tick, submission.Event.Context.Tick)
	}
	dispatchID := serviceStringValue(dispatch.Event.Context.DispatchId)
	if dispatchID == "" {
		t.Fatal("expected dispatch context to carry dispatch ID")
	}
	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("expected 1 recorded completion, got %d", len(completions))
	}
	completion := completions[0]
	completionDispatchID := serviceStringValue(completion.Event.Context.DispatchId)
	if completionDispatchID != dispatchID {
		t.Fatalf("completion dispatch ID = %q, want %q", completionDispatchID, dispatchID)
	}
	if completion.Event.Context.Tick < dispatch.Event.Context.Tick {
		t.Fatalf("completion observed tick = %d, want no earlier than dispatch tick %d", completion.Event.Context.Tick, dispatch.Event.Context.Tick)
	}
}

func assertReplayArtifactStoresCanonicalEvents(t *testing.T, path string, artifact *interfaces.ReplayArtifact, wantSubsequence []factoryapi.FactoryEventType) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("recording is not JSON: %v", err)
	}
	for _, key := range []string{"schemaVersion", "recordedAt", "events"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("recording missing top-level %s: %s", key, data)
		}
	}
	for _, legacyKey := range []string{"schema_version", "recorded_at", "work_requests", "submissions", "dispatches", "completions"} {
		if _, ok := raw[legacyKey]; ok {
			t.Fatalf("recording persisted legacy top-level key %q: %s", legacyKey, data)
		}
	}
	for _, legacyConfigKey := range forbiddenReplayConfigStorageKeys() {
		if strings.Contains(string(data), legacyConfigKey) {
			t.Fatalf("recording persisted legacy config key %q: %s", legacyConfigKey, data)
		}
	}
	if len(artifact.Events) == 0 {
		t.Fatal("recording has no canonical events")
	}
	for i, event := range artifact.Events {
		if event.Context.Sequence != i {
			t.Fatalf("event %d sequence = %d, want %d", i, event.Context.Sequence, i)
		}
	}
	types := make([]factoryapi.FactoryEventType, 0, len(artifact.Events))
	for _, event := range artifact.Events {
		types = append(types, event.Type)
	}
	next := 0
	for _, eventType := range types {
		if next < len(wantSubsequence) && eventType == wantSubsequence[next] {
			next++
		}
	}
	if next != len(wantSubsequence) {
		t.Fatalf("recording event types = %v, want subsequence %v", types, wantSubsequence)
	}
}

func forbiddenReplayConfigStorageKeys() []string {
	return []string{
		strings.Join([]string{"effective", "Config"}, ""),
		strings.Join([]string{"__replay", "Effective", "Config"}, ""),
		strings.Join([]string{"runtime", "Worker", "Config"}, ""),
	}
}

func TestBuildFactoryService_RecordModeStreamsReadableArtifactBeforeShutdown(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	writeWorkRequestFile(t, workFile, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-streamed-recording",
		Payload:    []byte(`{"task":"record before shutdown"}`),
	})

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                 dir,
		RuntimeMode:         interfaces.RuntimeModeService,
		MockWorkersConfig:   config.NewEmptyMockWorkersConfig(),
		Logger:              zap.NewNop(),
		RecordPath:          recordPath,
		RecordFlushInterval: 10 * time.Millisecond,
		WorkFile:            workFile,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()
	defer func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("Run after cancellation: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for service-mode factory service to stop")
		}
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("Run returned before shutdown: %v", err)
		default:
		}

		artifact, err := replay.Load(recordPath)
		if err == nil &&
			len(serviceReplayWorkRequestEvents(t, artifact)) == 1 &&
			len(serviceReplayDispatchCreatedEvents(t, artifact)) == 1 &&
			len(serviceReplayDispatchCompletedEvents(t, artifact)) == 1 {
			if artifact.WallClock != nil && !artifact.WallClock.FinishedAt.IsZero() {
				t.Fatal("streamed artifact should not have final wall-clock metadata before shutdown")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("record mode did not stream a readable artifact before shutdown")
}

func TestBuildFactoryService_RecordModeCopiesWorkerDiagnosticsToArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-diagnostics",
		Payload:    []byte(`{"task":"record diagnostics"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	provider := &recordingDiagnosticsProvider{}
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:              dir,
		Logger:           zap.NewNop(),
		RecordPath:       recordPath,
		WorkFile:         workFile,
		ProviderOverride: provider,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("expected 1 recorded completion, got %d", len(completions))
	}
	inferenceResponses := serviceReplayInferenceResponseEvents(t, artifact)
	if len(inferenceResponses) != 1 {
		t.Fatalf("expected 1 recorded inference response, got %d", len(inferenceResponses))
	}
	diagnostics := inferenceResponses[0].Payload.Diagnostics
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics on recorded inference response")
	}
	if diagnostics.Provider.ResponseMetadata == nil || (*diagnostics.Provider.ResponseMetadata)["request_id"] != "provider-request-1" {
		t.Fatalf("recorded inference response metadata = %#v", diagnostics.Provider.ResponseMetadata)
	}
	if diagnostics.RenderedPrompt == nil || serviceStringValue(diagnostics.RenderedPrompt.UserMessageHash) == "" {
		t.Fatal("expected rendered prompt metadata on recorded inference response")
	}
}

func TestBuildFactoryService_RecordModeCopiesScriptDiagnosticsToArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	writeWorkRequestFile(t, workFile, interfaces.SubmitRequest{
		WorkTypeID: "task",
		TraceID:    "trace-script-diagnostics",
		Payload:    []byte(`{"task":"record script diagnostics"}`),
	})

	recordPath := filepath.Join(t.TempDir(), "recording.json")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                   dir,
		Logger:                zap.NewNop(),
		RecordPath:            recordPath,
		WorkFile:              workFile,
		CommandRunnerOverride: recordingDiagnosticsCommandRunner{},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("expected 1 recorded completion, got %d", len(completions))
	}
	inferenceResponses := serviceReplayInferenceResponseEvents(t, artifact)
	if len(inferenceResponses) != 0 {
		t.Fatalf("script workers should not record inference responses, got %d", len(inferenceResponses))
	}
	completion := completions[0].Payload
	if serviceStringValue(completion.Output) != "script done" {
		t.Fatalf("recorded script output = %q", serviceStringValue(completion.Output))
	}
}

type recordingDiagnosticsCommandRunner struct{}

func (recordingDiagnosticsCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte("script done\n"), Stderr: []byte("script details\n")}, nil
}

type recordingCommandRunnerWithCapture struct {
	request workers.CommandRequest
}

func (r *recordingCommandRunnerWithCapture) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.request = req
	return workers.CommandResult{Stdout: []byte("script done\n"), Stderr: []byte("script details\n")}, nil
}

type recordingDiagnosticsProvider struct{}

func (recordingDiagnosticsProvider) Infer(_ context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	return interfaces.InferenceResponse{
		Content: "Done. COMPLETE",
		Diagnostics: &interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{
				ResponseMetadata: map[string]string{"request_id": "provider-request-1"},
			},
		},
	}, nil
}

func TestBuildFactoryService_ReplayModeLoadsEmbeddedConfigWithoutFactoryFiles(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	loaded, err := config.LoadRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	artifact := newReplayArtifactFromLoadedFactory(t, time.Now().UTC(), loaded)
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save artifact: %v", err)
	}

	replayDir := t.TempDir()
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               replayDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ReplayPath:        artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	if svc.net == nil {
		t.Fatal("expected replay service to build net from embedded config")
	}
	if _, ok := svc.net.WorkTypes["task"]; !ok {
		t.Fatal("expected task work type from embedded config")
	}
	if _, err := os.Stat(filepath.Join(replayDir, interfaces.FactoryConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("replay should not create or require local factory.json, stat err = %v", err)
	}
}

func TestBuildFactoryService_ReplayModeDefaultsToDeterministicClock(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	loaded, err := config.LoadRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	artifact := newReplayArtifactFromLoadedFactory(t, recordedAt, loaded)
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save artifact: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               t.TempDir(),
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ReplayPath:        artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	if _, ok := svc.clock.(*replay.DeterministicClock); !ok {
		t.Fatalf("expected replay service to use deterministic clock, got %T", svc.clock)
	}
	if got := svc.clock.Now(); !got.Equal(recordedAt) {
		t.Fatalf("replay clock Now() = %s, want %s", got, recordedAt)
	}
}

func TestBuildFactoryService_ReplayModeUsesRecordedProviderSideEffects(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	saveReplayBehaviorArtifact(t, sourceDir, artifactPath, interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-provider",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-replay-provider/work-replay-provider",
			TraceID:   "trace-replay-provider",
			WorkIDs:   []string{"work-replay-provider"},
		},
	}, interfaces.WorkResult{
		DispatchID:   "recorded-dispatch-provider",
		TransitionID: "process",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "replayed provider output",
		Diagnostics: &interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{
				ResponseMetadata: map[string]string{"request_id": "replay-provider-1"},
			},
		},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		Logger:     zap.NewNop(),
		ReplayPath: artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run replay: %v", err)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-replay-fixture review=2026-07-18 removal=split-replay-fixture-before-next-replay-service-change
func TestBuildFactoryService_ReplayModeDeliversRecordedCompletionAtLogicalTick(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	loaded, err := config.LoadRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	recordedDispatch := interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-logical-tick",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-logical-tick/work-logical-tick",
			TraceID:   "trace-logical-tick",
			WorkIDs:   []string{"work-logical-tick"},
		},
	}
	recordedResult := interfaces.WorkResult{
		DispatchID:   recordedDispatch.DispatchID,
		TransitionID: recordedDispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "replayed provider output",
		Diagnostics: &interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{
				ResponseMetadata: map[string]string{"request_id": "logical-tick-provider-1"},
			},
		},
	}
	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	artifact := newReplayArtifactFromLoadedFactory(t, recordedAt, loaded)
	artifact.Events = append(artifact.Events,
		serviceReplayWorkRequestEvent(t, "recorded-submission-logical-tick", 1, "recorded-artifact", []factoryapi.Work{{
			Name:         "work-logical-tick",
			WorkId:       serviceStringPtr("work-logical-tick"),
			WorkTypeName: serviceStringPtr("task"),
			TraceId:      serviceStringPtr("trace-logical-tick"),
			Payload:      map[string]any{"task": "replay logical tick"},
		}}, nil),
		serviceReplayDispatchCreatedEvent(t, recordedDispatch, 1),
		serviceReplayDispatchCompletedEvent(t, "recorded-completion-logical-tick", recordedResult, 4),
	)
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save replay artifact: %v", err)
	}

	var completions []interfaces.FactoryCompletionRecord
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		Logger:     zap.NewNop(),
		ReplayPath: artifactPath,
		ExtraOptions: []factory.FactoryOption{
			factory.WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
				completions = append(completions, record)
			}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run replay: %v", err)
	}

	if len(completions) != 1 {
		t.Fatalf("expected 1 replay completion, got %d", len(completions))
	}
	if completions[0].ObservedTick != 4 {
		t.Fatalf("replay completion observed tick = %d, want 4", completions[0].ObservedTick)
	}

	state, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(state.Marking.TokensInPlace("task:complete")) != 1 {
		t.Fatalf("expected replay token to reach task:complete after tick 4, marking = %#v", state.Marking.Tokens)
	}
}

func TestBuildFactoryService_ReplayModeUsesRecordedCommandRunnerSideEffects(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMDWithCommand(t, sourceDir, "worker-a", "replay-live-command-should-not-run", []string{"ok"})
	writeWorkstationAgentsMD(t, sourceDir, "process")

	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	saveReplayBehaviorArtifact(t, sourceDir, artifactPath, interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-command",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-replay-command/work-replay-command",
			TraceID:   "trace-replay-command",
			WorkIDs:   []string{"work-replay-command"},
		},
	}, interfaces.WorkResult{
		DispatchID:   "recorded-dispatch-command",
		TransitionID: "process",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "replayed command output",
		Diagnostics: &interfaces.WorkDiagnostics{
			Command: &interfaces.CommandDiagnostic{
				Command:  "replay-live-command-should-not-run",
				Args:     []string{"ok"},
				Stdout:   "replayed command output\n",
				Stderr:   "recorded command details\n",
				ExitCode: 0,
			},
		},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		Logger:     zap.NewNop(),
		ReplayPath: artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run replay: %v", err)
	}
}

func TestBuildFactoryService_ReplayModeStopsOnDispatchDivergence(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	saveReplayBehaviorArtifact(t, sourceDir, artifactPath, interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-mismatch",
		TransitionID:    "review",
		WorkerType:      "worker-a",
		WorkstationName: "review",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "review/trace-divergence/work-divergence",
			TraceID:   "trace-divergence",
			WorkIDs:   []string{"work-divergence"},
		},
	}, interfaces.WorkResult{
		DispatchID:   "recorded-dispatch-mismatch",
		TransitionID: "review",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "recorded output",
		Diagnostics: &interfaces.WorkDiagnostics{
			Command: &interfaces.CommandDiagnostic{
				Command:  "echo",
				Args:     []string{"ok"},
				Stdout:   "recorded output\n",
				ExitCode: 0,
			},
		},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		Logger:     zap.NewNop(),
		ReplayPath: artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = svc.Run(ctx)
	if err == nil {
		t.Fatal("expected replay divergence error")
	}
	var divergence *replay.DivergenceError
	if !errors.As(err, &divergence) {
		t.Fatalf("Run error is not replay divergence: %T %v", err, err)
	}
	if divergence.Report.Category != replay.DivergenceCategoryDispatchMismatch {
		t.Fatalf("divergence category = %q, want %q", divergence.Report.Category, replay.DivergenceCategoryDispatchMismatch)
	}
	if !strings.Contains(divergence.Report.Expected, "transition=review") {
		t.Fatalf("expected divergence report to include recorded transition: %#v", divergence.Report)
	}
}

func TestBuildFactoryService_ReplayModeWarnsOnCurrentConfigHashMismatch(t *testing.T) {
	sourceDir := t.TempDir()
	writeFactoryJSON(t, sourceDir, minimalFactoryConfig())
	writeScriptWorkerAgentsMD(t, sourceDir, "worker-a")
	writeWorkstationAgentsMD(t, sourceDir, "process")

	artifactPath := filepath.Join(t.TempDir(), "recording.json")
	saveReplayBehaviorArtifact(t, sourceDir, artifactPath, interfaces.WorkDispatch{
		DispatchID:      "recorded-dispatch-warning",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		Execution: interfaces.ExecutionMetadata{
			ReplayKey: "process/trace-warning/work-warning",
			TraceID:   "trace-warning",
			WorkIDs:   []string{"work-warning"},
		},
	}, interfaces.WorkResult{
		DispatchID:   "recorded-dispatch-warning",
		TransitionID: "process",
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "recorded output",
		Diagnostics: &interfaces.WorkDiagnostics{
			Command: &interfaces.CommandDiagnostic{
				Command:  "echo",
				Args:     []string{"ok"},
				Stdout:   "recorded output\n",
				ExitCode: 0,
			},
		},
	})

	mismatchedConfig := minimalFactoryConfig()
	mismatchedConfig["workers"] = []map[string]string{{"name": "worker-a"}, {"name": "worker-b"}}
	writeFactoryJSON(t, sourceDir, mismatchedConfig)

	core, observedLogs := observer.New(zap.WarnLevel)
	_, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        sourceDir,
		Logger:     zap.New(core),
		ReplayPath: artifactPath,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService replay: %v", err)
	}

	warnings := observedLogs.FilterMessage("replay artifact metadata differs from current checkout").All()
	if len(warnings) == 0 {
		t.Fatal("expected replay metadata mismatch warning")
	}
}

func saveReplayBehaviorArtifact(t *testing.T, sourceDir, artifactPath string, dispatch interfaces.WorkDispatch, result interfaces.WorkResult) {
	t.Helper()

	createdTick := dispatch.Execution.DispatchCreatedTick
	if createdTick == 0 {
		createdTick = 1
		dispatch.Execution.DispatchCreatedTick = createdTick
	}

	loaded, err := config.LoadRuntimeConfig(sourceDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	recordedAt := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	artifact := newReplayArtifactFromLoadedFactory(t, recordedAt, loaded)
	artifact.Events = append(artifact.Events,
		serviceReplayWorkRequestEvent(t, "recorded-submission", 1, "recorded-artifact", serviceReplayWorksFromDispatch(dispatch), nil),
		serviceReplayDispatchCreatedEvent(t, dispatch, createdTick),
		serviceReplayDispatchCompletedEvent(t, "recorded-completion", result, 3),
	)
	if err := replay.Save(artifactPath, artifact); err != nil {
		t.Fatalf("Save replay artifact: %v", err)
	}
}

func newReplayArtifactFromLoadedFactory(t *testing.T, recordedAt time.Time, loaded *config.LoadedFactoryConfig) *interfaces.ReplayArtifact {
	t.Helper()

	generatedFactory, err := replay.GeneratedFactoryFromLoadedConfig(
		loaded,
		replay.WithGeneratedFactorySourceDirectory(loaded.FactoryDir()),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	artifact, err := replay.NewEventLogArtifactFromFactory(recordedAt, generatedFactory, nil, interfaces.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifactFromFactory: %v", err)
	}
	return artifact
}

func serviceReplayDispatchCreatedEvent(t *testing.T, dispatch interfaces.WorkDispatch, tick int) factoryapi.FactoryEvent {
	t.Helper()
	metadata := map[string]string{}
	if dispatch.Execution.ReplayKey != "" {
		metadata["replayKey"] = dispatch.Execution.ReplayKey
	}
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: dispatch.TransitionID,
		Inputs:       serviceReplayDispatchInputRefsFromDispatch(dispatch),
		Resources:    serviceReplayResourcesFromDispatch(dispatch),
		Metadata:     serviceDispatchRequestMetadata(metadata),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch created event: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/" + dispatch.DispatchID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: serviceStringPtr(dispatch.DispatchID),
			RequestId:  serviceStringPtr(dispatch.Execution.RequestID),
			TraceIds:   serviceStringSlicePtr([]string{dispatch.Execution.TraceID}),
			WorkIds:    serviceStringSlicePtr(dispatch.Execution.WorkIDs),
		},
		Payload: union,
	}
}

func serviceReplayWorkRequestEvent(t *testing.T, requestID string, tick int, source string, works []factoryapi.Work, relations []factoryapi.Relation) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.WorkRequestEventPayload{
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     serviceSlicePtr(works),
		Relations: serviceSlicePtr(relations),
		Source:    serviceStringPtr(source),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromWorkRequestEventPayload(payload); err != nil {
		t.Fatalf("encode work request event: %v", err)
	}
	var traceIDs []string
	var workIDs []string
	for _, work := range works {
		traceIDs = append(traceIDs, serviceStringValue(work.TraceId))
		workIDs = append(workIDs, serviceStringValue(work.WorkId))
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/work-request/" + requestID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeWorkRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:      tick,
			RequestId: serviceStringPtr(requestID),
			Source:    serviceStringPtr(source),
			TraceIds:  serviceStringSlicePtr(serviceUniqueNonEmpty(traceIDs)),
			WorkIds:   serviceStringSlicePtr(serviceUniqueNonEmpty(workIDs)),
		},
		Payload: union,
	}
}

func serviceReplayDispatchCompletedEvent(t *testing.T, completionID string, result interfaces.WorkResult, tick int) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.DispatchResponseEventPayload{
		CompletionId:    serviceStringPtr(completionID),
		TransitionId:    result.TransitionID,
		Outcome:         factoryapi.WorkOutcome(result.Outcome),
		Output:          serviceStringPtr(result.Output),
		Error:           serviceStringPtr(result.Error),
		Feedback:        serviceStringPtr(result.Feedback),
		ProviderFailure: serviceProviderFailurePtr(result.ProviderFailure),
		Metrics:         serviceWorkMetricsPtr(result.Metrics),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchResponseEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch completed event: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-completed/" + result.DispatchID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchResponse,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: serviceStringPtr(result.DispatchID),
		},
		Payload: union,
	}
}

func serviceReplayWorkRequestEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayWorkRequestRecord {
	t.Helper()
	var out []serviceReplayWorkRequestRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode work request event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayWorkRequestRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayWorkRequestRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.WorkRequestEventPayload
}

func serviceReplayDispatchCreatedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayDispatchCreatedRecord {
	t.Helper()
	var out []serviceReplayDispatchCreatedRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch created event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayDispatchCreatedRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayDispatchCreatedRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.DispatchRequestEventPayload
}

func serviceReplayDispatchCompletedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayDispatchCompletedRecord {
	t.Helper()
	var out []serviceReplayDispatchCompletedRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch completed event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayDispatchCompletedRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayDispatchCompletedRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.DispatchResponseEventPayload
}

func serviceReplayInferenceResponseEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayInferenceResponseRecord {
	t.Helper()
	var out []serviceReplayInferenceResponseRecord
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayInferenceResponseRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayInferenceResponseRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.InferenceResponseEventPayload
}

func serviceReplayWorksFromDispatch(dispatch interfaces.WorkDispatch) []factoryapi.Work {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	works := make([]factoryapi.Work, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		workID := firstNonEmpty(token.Color.WorkID, token.ID)
		works = append(works, factoryapi.Work{
			Name:         firstNonEmpty(token.Color.Name, workID),
			WorkId:       serviceStringPtr(workID),
			WorkTypeName: serviceStringPtr(token.Color.WorkTypeID),
			TraceId:      serviceStringPtr(token.Color.TraceID),
			Tags:         serviceStringMapPtr(token.Color.Tags),
		})
	}
	if len(works) == 0 {
		for _, workID := range dispatch.Execution.WorkIDs {
			works = append(works, factoryapi.Work{
				Name:         workID,
				WorkId:       serviceStringPtr(workID),
				WorkTypeName: serviceStringPtr("task"),
				TraceId:      serviceStringPtr(dispatch.Execution.TraceID),
			})
		}
	}
	return works
}

func serviceReplayDispatchInputRefsFromDispatch(dispatch interfaces.WorkDispatch) []factoryapi.DispatchConsumedWorkRef {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	refs := make([]factoryapi.DispatchConsumedWorkRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		workID := firstNonEmpty(token.Color.WorkID, token.ID)
		if workID == "" {
			continue
		}
		refs = append(refs, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
	}
	if len(refs) == 0 {
		for _, workID := range dispatch.Execution.WorkIDs {
			if workID == "" {
				continue
			}
			refs = append(refs, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
		}
	}
	return refs
}

func serviceReplayResourcesFromDispatch(dispatch interfaces.WorkDispatch) *[]factoryapi.Resource {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	resources := make([]factoryapi.Resource, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType != interfaces.DataTypeResource {
			continue
		}
		resources = append(resources, factoryapi.Resource{Name: firstNonEmpty(token.Color.WorkTypeID, token.Color.Name)})
	}
	return serviceSlicePtr(resources)
}

func serviceDispatchRequestMetadata(values map[string]string) *factoryapi.DispatchRequestEventMetadata {
	if len(values) == 0 {
		return nil
	}
	return &factoryapi.DispatchRequestEventMetadata{
		ReplayKey: serviceStringPtr(values["replayKey"]),
	}
}

func serviceProviderFailurePtr(failure *interfaces.ProviderFailureMetadata) *factoryapi.ProviderFailureMetadata {
	return interfaces.GeneratedProviderFailureMetadata(failure)
}

func serviceWorkMetricsPtr(metrics interfaces.WorkMetrics) *factoryapi.WorkMetrics {
	if metrics.Duration == 0 && metrics.Cost == 0 && metrics.RetryCount == 0 {
		return nil
	}
	return &factoryapi.WorkMetrics{
		DurationNanos: serviceInt64Ptr(metrics.Duration.Nanoseconds()),
		Cost:          serviceFloat64Ptr(metrics.Cost),
		RetryCount:    serviceIntPtr(metrics.RetryCount),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func serviceStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func serviceFirstStringValue(values *[]string) string {
	if values == nil {
		return ""
	}
	for _, value := range *values {
		if value != "" {
			return value
		}
	}
	return ""
}

func serviceUniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func serviceStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func serviceEnumPtr[T ~string](value T) *T {
	if value == "" {
		return nil
	}
	return &value
}

func serviceIntPtr(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func serviceInt64Ptr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func serviceFloat64Ptr(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

func serviceStringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}

func serviceStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap{}
	for key, value := range values {
		if value != "" {
			converted[key] = value
		}
	}
	if len(converted) == 0 {
		return nil
	}
	return &converted
}

func serviceSlicePtr[T any](values []T) *[]T {
	if len(values) == 0 {
		return nil
	}
	out := append([]T(nil), values...)
	return &out
}

func assertServiceFactoryEventsContainTypes(t *testing.T, events []factoryapi.FactoryEvent, wantTypes []factoryapi.FactoryEventType) {
	t.Helper()
	seen := make(map[factoryapi.FactoryEventType]bool, len(events))
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, wantType := range wantTypes {
		if !seen[wantType] {
			t.Fatalf("factory event types = %v, want %s", serviceFactoryEventTypes(events), wantType)
		}
	}
}

func serviceFactoryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func TestBuildFactoryService_RecordAndReplayTogetherRejected(t *testing.T) {
	_, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:        t.TempDir(),
		RecordPath: "recording.json",
		ReplayPath: "recording.json",
		Logger:     zap.NewNop(),
	})
	if err == nil {
		t.Fatal("expected record and replay combination to fail")
	}
	if !strings.Contains(err.Error(), "--record and --replay cannot be used together") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-service-mode-fixture review=2026-07-18 removal=split-late-submit-fixture-before-next-service-mode-change
func TestBuildFactoryService_ServiceModeAcceptsLateSubmissionAfterIdleStartup(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before late submission: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	snapBeforeSubmit, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before submit: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	snapAfterIdleWait, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after idle wait: %v", err)
	}
	if snapAfterIdleWait.RuntimeStatus != interfaces.RuntimeStatusIdle {
		t.Fatalf("service-mode idle status = %q, want %q", snapAfterIdleWait.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}

	if snapAfterIdleWait.TickCount != snapBeforeSubmit.TickCount {
		t.Fatalf("service-mode idle wait should not busy-spin: tick count advanced from %d to %d",
			snapBeforeSubmit.TickCount,
			snapAfterIdleWait.TickCount,
		)
	}

	err = submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-late-submit",
		Payload:    json.RawMessage(`{"title":"late submit"}`),
	}})
	if err != nil {
		t.Fatalf("Submit late work: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		state, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		for _, token := range state.Marking.Tokens {
			if token.PlaceID == "task:complete" {
				cancel()
				select {
				case err := <-errCh:
					if err != nil {
						t.Fatalf("Run after cancellation: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for service-mode factory service to stop")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-errCh
	t.Fatal("late-submitted service work did not reach task:complete before timeout")
}

func TestBuildFactoryService_BatchModeRejectsLateSubmissionAfterTermination(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after batch completion: %v", err)
	}
	if snap.RuntimeStatus != interfaces.RuntimeStatusFinished {
		t.Fatalf("batch completion status = %q, want %q", snap.RuntimeStatus, interfaces.RuntimeStatusFinished)
	}

	err = submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-after-stop",
	}})
	if err == nil {
		t.Fatal("expected late batch submission to fail after runtime termination")
	}
	if !strings.Contains(err.Error(), "terminated") {
		t.Fatalf("expected terminated error, got %v", err)
	}
}

func TestFactoryService_RunPreservesSnapshotAndFactoryEventObservability(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	inputDir := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "seed.json"), []byte(`{"title":"observe runtime"}`), 0o644); err != nil {
		t.Fatalf("write seed work file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snap, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snap.FactoryState, interfaces.FactoryStateCompleted)
	}
	if snap.RuntimeStatus != interfaces.RuntimeStatusFinished {
		t.Fatalf("runtime status = %q, want %q", snap.RuntimeStatus, interfaces.RuntimeStatusFinished)
	}
	if snap.Topology == nil || snap.Topology.WorkTypes["task"] == nil {
		t.Fatalf("snapshot topology work types = %#v, want task work type", snap.Topology)
	}
	if snap.Marking.Tokens == nil || len(snap.Marking.Tokens) != 1 {
		t.Fatalf("snapshot marking tokens = %#v, want one completed token", snap.Marking.Tokens)
	}
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID != "task:complete" {
			t.Fatalf("snapshot token place = %q, want task:complete", token.PlaceID)
		}
	}
	if snap.TickCount == 0 {
		t.Fatal("snapshot tick count = 0, want runtime activity")
	}
	if len(snap.DispatchHistory) == 0 {
		t.Fatal("snapshot dispatch history is empty, want completed runtime activity")
	}
	if len(snap.DispatchHistory[0].ConsumedTokens) == 0 {
		t.Fatalf("completed dispatch = %#v, want consumed token evidence", snap.DispatchHistory[0])
	}

	events, err := svc.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	assertServiceFactoryEventsContainTypes(t, events, []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchResponse,
	})
}

func TestBuildFactoryService_InvalidWorkFile(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()

	// Build service with a nonexistent work file.
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		WorkFile:          filepath.Join(dir, "nonexistent.json"),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	// submitWorkFile should fail for nonexistent file.
	err = svc.submitWorkFile(ctx)
	if err == nil {
		t.Fatal("expected error for nonexistent work file")
	}
}

func TestBuildFactoryService_WorkFileRejectsRetiredTargetStateAlias(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "request_id": "request-service-target-state",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "draft", "work_type_name": "task", "target_state": "waiting"}
  ]
}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		WorkFile:          workFile,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	err = svc.submitWorkFile(context.Background())
	if err == nil {
		t.Fatal("expected retired target_state alias to fail")
	}
	if !strings.Contains(err.Error(), "target_state") || !strings.Contains(err.Error(), "state") {
		t.Fatalf("error = %q, want target_state rejection with state guidance", err.Error())
	}
}

func TestBuildFactoryService_ConfigWithAllOptions(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	// Create a valid work file.
	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    json.RawMessage(`{"title":"test"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	dashRendered := false
	apiStarted := false

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Port:              9999,
		Logger:            zap.NewNop(),
		WorkFile:          workFile,
		SimpleDashboardRenderer: func(_ SimpleDashboardRenderInput) {
			dashRendered = true
		},
		APIServerStarter: func(ctx context.Context, runtime apisurface.APISurface, port int, l *zap.Logger) error {
			apiStarted = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	// Verify config was preserved.
	if svc.cfg.MockWorkersConfig == nil {
		t.Error("expected MockWorkersConfig to be set")
	}
	if svc.cfg.Port != 9999 {
		t.Errorf("expected Port 9999, got %d", svc.cfg.Port)
	}
	if svc.cfg.WorkFile != workFile {
		t.Errorf("expected WorkFile %q, got %q", workFile, svc.cfg.WorkFile)
	}
	if svc.cfg.SimpleDashboardRenderer == nil {
		t.Error("expected SimpleDashboardRenderer to be set")
	}
	if svc.cfg.APIServerStarter == nil {
		t.Error("expected APIServerStarter to be set")
	}

	// Verify callbacks are callable (but don't test Run here — that needs a full engine).
	_ = dashRendered
	_ = apiStarted
}

func TestFactoryService_SimpleDashboardRenderInputUsesRenderData(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	provider := newDashboardWorldViewProvider()
	rendered := make(chan SimpleDashboardRenderInput, 8)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                     dir,
		RuntimeMode:             interfaces.RuntimeModeService,
		Logger:                  zap.NewNop(),
		ProviderOverride:        provider,
		SimpleDashboardRenderer: func(input SimpleDashboardRenderInput) { rendered <- input },
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()
	defer stopServiceModeRun(t, cancelRun, errCh)

	submitDashboardWorldViewWork(t, svc, "dashboard-world-active", "trace-dashboard-active")
	provider.nextDispatch(t)
	active := renderSimpleDashboardForTest(t, svc, rendered)
	assertDashboardRenderDataActive(t, active.RenderData, "dashboard-world-active")
	assertSimpleDashboardActiveOutput(t, active)

	provider.respond(interfaces.InferenceResponse{
		Content: "COMPLETE",
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-dashboard-success",
		},
	}, nil)
	waitForTokenInPlaceByWorkID(t, svc, "task:complete", "dashboard-world-active", time.Second)
	completed := renderSimpleDashboardForTest(t, svc, rendered)
	if completed.EngineState.Topology == nil {
		t.Fatal("renderer input lost aggregate snapshot topology")
	}
	assertDashboardRenderDataCompleted(t, completed.RenderData, "sess-dashboard-success")

	submitDashboardWorldViewWork(t, svc, "dashboard-world-failed", "trace-dashboard-failed")
	provider.nextDispatch(t)
	provider.respond(interfaces.InferenceResponse{}, workers.NewProviderErrorWithSession(
		interfaces.ProviderErrorTypePermanentBadRequest,
		"provider rejected dashboard world-view work",
		errors.New("provider rejected"),
		&interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-dashboard-failed",
		},
	))
	waitForTokenInPlaceByWorkID(t, svc, "task:failed", "dashboard-world-failed", time.Second)
	failed := renderSimpleDashboardForTest(t, svc, rendered)
	assertDashboardRenderDataFailed(t, failed.RenderData, "dashboard-world-failed")
	assertSimpleDashboardTerminalOutput(t, failed)
	assertSimpleDashboardSessionRowsMatchRenderData(t, failed)
}

func TestFactoryService_BuildSimpleDashboardRenderInputProjectsSelectedTickFromEvents(t *testing.T) {
	topology := &state.Net{ID: "aggregate-topology"}
	engineState := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		Topology:      topology,
		TickCount:     2,
		ActiveThrottlePauses: []interfaces.ActiveThrottlePause{{
			LaneID:      "codex/gpt-5-codex",
			Provider:    "codex",
			Model:       "gpt-5-codex",
			PausedAt:    time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
			PausedUntil: time.Date(2026, 4, 30, 10, 5, 0, 0, time.UTC),
		}},
	}
	dispatch := dashboardProjectionDispatchForTest()
	mock := &aggregateSnapshotFactory{
		engineState:   engineState,
		factoryEvents: dashboardProjectionEventsForTest(t, dispatch),
	}
	svc := &FactoryService{factory: mock, logger: zap.NewNop()}

	input, err := svc.buildSimpleDashboardRenderInput(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("buildSimpleDashboardRenderInput: %v", err)
	}

	if mock.engineStateSnapshotCalls != 1 {
		t.Fatalf("engine snapshot calls = %d, want 1", mock.engineStateSnapshotCalls)
	}
	if mock.factoryEventsCalls != 1 {
		t.Fatalf("factory event calls = %d, want 1", mock.factoryEventsCalls)
	}
	if input.EngineState.Topology != topology {
		t.Fatalf("engine-state topology = %#v, want aggregate topology %#v", input.EngineState.Topology, topology)
	}
	if input.RenderData.InFlightDispatchCount != 1 {
		t.Fatalf("in-flight count = %d, want selected tick active dispatch", input.RenderData.InFlightDispatchCount)
	}
	if input.RenderData.Session.CompletedCount != 0 {
		t.Fatalf("completed count = %d, want future completion excluded", input.RenderData.Session.CompletedCount)
	}
	if len(input.RenderData.Session.ProviderSessions) != 0 {
		t.Fatalf("provider sessions = %#v, want future provider session excluded", input.RenderData.Session.ProviderSessions)
	}
	if got := len(input.RenderData.Session.DispatchHistory); got != 0 {
		t.Fatalf("dispatch history length = %d, want selected tick to exclude future completion", got)
	}
	if len(input.RenderData.ActiveThrottlePauses) != 1 {
		t.Fatalf("active throttle pauses = %d, want 1", len(input.RenderData.ActiveThrottlePauses))
	}
	pause := input.RenderData.ActiveThrottlePauses[0]
	if pause.LaneID != "codex/gpt-5-codex" || pause.Provider != "codex" || pause.Model != "gpt-5-codex" {
		t.Fatalf("active throttle pause = %#v, want codex/gpt-5-codex lane", pause)
	}
	if len(pause.AffectedTransitionIDs) != 1 || pause.AffectedTransitionIDs[0] != dispatch.TransitionID {
		t.Fatalf("affected transition IDs = %#v, want [%s]", pause.AffectedTransitionIDs, dispatch.TransitionID)
	}
}

func TestFactoryService_RenderDashboardLogsEventProjectionErrors(t *testing.T) {
	tests := []struct {
		name          string
		factoryEvents []factoryapi.FactoryEvent
		factoryErr    error
	}{
		{
			name:       "event retrieval",
			factoryErr: errors.New("event history unavailable"),
		},
		{
			name: "event reconstruction",
			factoryEvents: []factoryapi.FactoryEvent{{
				Id:            "factory-event/work-request/malformed",
				SchemaVersion: factoryapi.AgentFactoryEventV1,
				Type:          factoryapi.FactoryEventTypeWorkRequest,
				Context:       factoryapi.FactoryEventContext{Tick: 1, EventTime: time.Now()},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, observedLogs := observer.New(zap.ErrorLevel)
			renderCalls := 0
			svc := &FactoryService{
				factory: &aggregateSnapshotFactory{
					engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
						Topology:  &state.Net{ID: "aggregate-topology"},
						TickCount: 1,
					},
					factoryEvents:    tt.factoryEvents,
					factoryEventsErr: tt.factoryErr,
				},
				cfg: &FactoryServiceConfig{
					SimpleDashboardRenderer: func(SimpleDashboardRenderInput) { renderCalls++ },
				},
				logger: zap.New(core),
			}

			svc.renderDashboard(context.Background())

			if renderCalls != 0 {
				t.Fatalf("renderer calls = %d, want 0 after projection error", renderCalls)
			}
			if observedLogs.FilterMessage("simple dashboard render failed").Len() != 1 {
				t.Fatalf("render error log count = %d, want 1", observedLogs.FilterMessage("simple dashboard render failed").Len())
			}
		})
	}
}

func dashboardProjectionEventsForTest(t *testing.T, dispatch interfaces.WorkDispatch) []factoryapi.FactoryEvent {
	t.Helper()
	result := interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		Output:       "COMPLETE",
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-future-completion",
		},
	}
	return []factoryapi.FactoryEvent{
		dashboardInitialStructureEventForTest(t),
		serviceReplayWorkRequestEvent(t, dispatch.Execution.RequestID, 1, "dashboard-test", serviceReplayWorksFromDispatch(dispatch), nil),
		serviceReplayDispatchCreatedEvent(t, dispatch, 2),
		serviceReplayDispatchCompletedEvent(t, "future-completion", result, 3),
	}
}

func dashboardInitialStructureEventForTest(t *testing.T) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.InitialStructureRequestEventPayload{
		Factory: factoryapi.Factory{
			WorkTypes: &[]factoryapi.WorkType{{
				Name: "task",
				States: []factoryapi.WorkState{
					{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
					{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
					{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
				},
			}},
			Workers: &[]factoryapi.Worker{{
				Name:          "worker-a",
				ModelProvider: serviceEnumPtr(factoryapi.WorkerModelProviderCodex),
				Model:         serviceStringPtr("gpt-5-codex"),
			}},
			Workstations: &[]factoryapi.Workstation{{
				Id:      serviceStringPtr("process"),
				Name:    "process",
				Worker:  "worker-a",
				Inputs:  []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
				Outputs: []factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}},
				OnFailure: &factoryapi.WorkstationIO{
					WorkType: "task",
					State:    "failed",
				},
			}},
		},
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromInitialStructureRequestEventPayload(payload); err != nil {
		t.Fatalf("encode initial structure event: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/initial-structure/dashboard-test",
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeInitialStructureRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC),
			Tick:      0,
		},
		Payload: union,
	}
}

func dashboardProjectionDispatchForTest() interfaces.WorkDispatch {
	token := interfaces.Token{
		ID:      "work-selected",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			Name:       "Selected Tick Work",
			RequestID:  "request-selected",
			WorkID:     "work-selected",
			WorkTypeID: "task",
			DataType:   interfaces.DataTypeWork,
			TraceID:    "trace-selected",
		},
	}
	return interfaces.WorkDispatch{
		DispatchID:      "dispatch-selected",
		TransitionID:    "process",
		WorkerType:      "worker-a",
		WorkstationName: "process",
		InputTokens:     workers.InputTokens(token),
		Execution: interfaces.ExecutionMetadata{
			DispatchCreatedTick: 2,
			RequestID:           "request-selected",
			TraceID:             "trace-selected",
			WorkIDs:             []string{"work-selected"},
		},
	}
}

func assertSimpleDashboardActiveOutput(t *testing.T, input SimpleDashboardRenderInput) {
	t.Helper()
	assertSimpleDashboardOutputContains(t, input, []string{
		"Active Workstations (1)",
		"process",
		"dashboard-world-active",
		"Workstation Activity",
		"Session Metrics",
		"Workstations Dispatched:  1",
		"Workstations Completed:   0",
		"Workstations Failed:      0",
	})
}

func assertSimpleDashboardTerminalOutput(t *testing.T, input SimpleDashboardRenderInput) {
	t.Helper()
	assertSimpleDashboardOutputContains(t, input, []string{
		"Completed Workstations",
		"Success",
		"Failed",
		"dashboard-world-active",
		"dashboard-world-failed",
		"provider rejected dashboard world-view work",
		"Queue Counts",
		"task:complete",
		"task:failed",
		"Session Metrics",
		"Workstations Dispatched:  2",
		"Workstations Completed:   1",
		"Workstations Failed:      1",
		"Failed work: 1",
		"Provider sessions:",
		"codex / session_id / sess-dashboard-success",
		"codex / session_id / sess-dashboard-failed",
	})
}

func assertSimpleDashboardSessionRowsMatchRenderData(t *testing.T, input SimpleDashboardRenderInput) {
	t.Helper()
	session := input.RenderData.Session
	if session.DispatchedCount != len(session.DispatchHistory) {
		t.Fatalf("dispatched count = %d, dispatch history rows = %d", session.DispatchedCount, len(session.DispatchHistory))
	}
	if terminalRows := session.CompletedCount + session.FailedCount; terminalRows != len(session.DispatchHistory) {
		t.Fatalf("terminal count = %d, dispatch history rows = %d", terminalRows, len(session.DispatchHistory))
	}
	output := dashboard.FormatSimpleDashboardWithRenderData(
		input.EngineState,
		input.RenderData,
		time.Now(),
	)
	if renderedProviderRows := strings.Count(output, "codex / session_id /"); renderedProviderRows != len(session.ProviderSessions) {
		t.Fatalf("rendered provider rows = %d, render-data provider sessions = %d\n%s",
			renderedProviderRows,
			len(session.ProviderSessions),
			output,
		)
	}
}

// writeWorkstationAgentsMDWithPrompt writes a MODEL_WORKSTATION AGENTS.md with a
// custom prompt template body into the given workstation directory.
func writeWorkstationAgentsMDWithPrompt(t *testing.T, factoryDir, workstationName, promptBody string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\n---\n" + promptBody + "\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

// writeWorkstationAgentsMDWithPromptFile writes a MODEL_WORKSTATION AGENTS.md that
// references a prompt_file, and writes the prompt file alongside it.
func writeWorkstationAgentsMDWithPromptFile(t *testing.T, factoryDir, workstationName, promptFileName, promptContent string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\npromptFile: " + promptFileName + "\n---\nThis body should be ignored.\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, promptFileName), []byte(promptContent), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
}

func writeRuntimeLookupWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKSTATION\nworker: script-worker\nworkingDirectory: workspace\n---\nRun the script.\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func TestGetEngineStateSnapshot_AggregatesAllState(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	snap, err := svc.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	// Factory state should be IDLE (not yet started).
	if snap.FactoryState != string(interfaces.FactoryStateIdle) {
		t.Errorf("expected FactoryState=IDLE, got %s", snap.FactoryState)
	}
	if snap.RuntimeStatus != interfaces.RuntimeStatusIdle {
		t.Errorf("expected RuntimeStatus=IDLE, got %s", snap.RuntimeStatus)
	}

	// Runtime state: tick count should be 0 for idle factory.
	if snap.TickCount != 0 {
		t.Errorf("expected TickCount=0, got %d", snap.TickCount)
	}
	if snap.Topology == nil {
		t.Fatal("expected aggregate snapshot topology")
	}
	if _, ok := snap.Topology.WorkTypes["task"]; !ok {
		t.Fatalf("expected topology to include task work type, got %#v", snap.Topology.WorkTypes)
	}

	// Uptime is owned by the factory runtime. A factory that has not started
	// reports zero uptime through the aggregate snapshot.
	if snap.Uptime != 0 {
		t.Errorf("expected zero uptime before runtime start, got %v", snap.Uptime)
	}
}

func TestFactoryService_GetEngineStateSnapshot_DelegatesToFactoryAggregateSnapshot(t *testing.T) {
	topology := &state.Net{ID: "aggregate-net"}
	expected := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
		Uptime:        42 * time.Second,
		Topology:      topology,
		InFlightCount: 3,
		TickCount:     7,
	}
	mock := &aggregateSnapshotFactory{engineState: expected}
	svc := &FactoryService{factory: mock}

	got, err := svc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if got != expected {
		t.Fatalf("service returned %#v, want factory aggregate snapshot %#v", got, expected)
	}
	if mock.engineStateSnapshotCalls != 1 {
		t.Fatalf("factory aggregate snapshot calls = %d, want 1", mock.engineStateSnapshotCalls)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-state-snapshot-fixture review=2026-07-18 removal=split-snapshot-state-fixture-before-next-engine-snapshot-change
func TestFactoryService_GetEngineStateSnapshot_ReportsIdleActiveAndFinishedStates(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	writeWorkerAgentsMD(t, dir, "worker-a")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	blocked := make(chan struct{})
	provider := &blockingInferenceProvider{releaseCh: blocked, content: "COMPLETE"}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:              dir,
		RuntimeMode:      interfaces.RuntimeModeService,
		Logger:           zap.NewNop(),
		ProviderOverride: provider,
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

	idleDeadline := time.Now().Add(time.Second)
	for {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot during idle wait: %v", err)
		}
		if snap.RuntimeStatus == interfaces.RuntimeStatusIdle {
			break
		}
		if time.Now().After(idleDeadline) {
			t.Fatalf("timed out waiting for idle runtime status, last status=%q", snap.RuntimeStatus)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-engine-state-statuses",
		Payload:    json.RawMessage(`{"title":"runtime-statuses"}`),
	}}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	activeDeadline := time.Now().Add(time.Second)
	for {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot during active wait: %v", err)
		}
		if snap.RuntimeStatus == interfaces.RuntimeStatusActive && snap.InFlightCount > 0 {
			break
		}
		if time.Now().After(activeDeadline) {
			t.Fatalf("timed out waiting for active runtime status, last status=%q inflight=%d", snap.RuntimeStatus, snap.InFlightCount)
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(blocked)

	completedDeadline := time.Now().Add(time.Second)
	for {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot during idle-after-completion wait: %v", err)
		}
		if snap.RuntimeStatus == interfaces.RuntimeStatusIdle && len(snap.Marking.Tokens) == 1 {
			for _, token := range snap.Marking.Tokens {
				if token.PlaceID != "task:complete" {
					t.Fatalf("service-mode completion token place = %q, want task:complete", token.PlaceID)
				}
			}
			break
		}
		if time.Now().After(completedDeadline) {
			t.Fatalf("timed out waiting for idle runtime status after completion, last status=%q", snap.RuntimeStatus)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancelRun()
	if err := <-errCh; err != nil {
		t.Fatalf("service-mode run error: %v", err)
	}

	batchDir := t.TempDir()
	writeFactoryJSON(t, batchDir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, batchDir, "process")
	if err := os.MkdirAll(filepath.Join(batchDir, interfaces.InputsDir, "task", interfaces.DefaultChannelName), 0o755); err != nil {
		t.Fatalf("create batch inputs dir: %v", err)
	}
	workFile := filepath.Join(batchDir, interfaces.InputsDir, "task", interfaces.DefaultChannelName, "seed.json")
	if err := os.WriteFile(workFile, []byte(`{"title":"terminal-status"}`), 0o644); err != nil {
		t.Fatalf("write seed work file: %v", err)
	}

	batchSvc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               batchDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService batch: %v", err)
	}

	batchCtx, cancelBatch := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelBatch()
	if err := batchSvc.Run(batchCtx); err != nil {
		t.Fatalf("batch Run: %v", err)
	}

	terminalSnap, err := batchSvc.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot terminal: %v", err)
	}
	if terminalSnap.RuntimeStatus != interfaces.RuntimeStatusFinished {
		t.Fatalf("terminal runtime status = %q, want %q", terminalSnap.RuntimeStatus, interfaces.RuntimeStatusFinished)
	}
	if terminalSnap.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("terminal factory state = %q, want %q", terminalSnap.FactoryState, interfaces.FactoryStateCompleted)
	}
}

func TestLoadWorkersFromConfig_PromptTemplateFromBody(t *testing.T) {
	dir := t.TempDir()

	expectedPrompt := "You are a design reviewer. Evaluate the design for {{ .Payload }}."
	writeWorkstationAgentsMDWithPrompt(t, dir, "review", expectedPrompt)
	writeWorkerAgentsMD(t, dir, "worker-a")

	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "review"},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "worker-a"},
		},
	}

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, factoryCfg,
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	opts, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), cfg, logging.NoopLogger{}, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	// Apply options to a FactoryConfig to inspect the registered executor.
	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}

	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	wsDef, ok := wsExec.RuntimeConfig.Workstation("review")
	if !ok {
		t.Fatal("expected 'review' workstation in runtime config")
	}

	if wsDef.PromptTemplate != expectedPrompt {
		t.Errorf("expected prompt template %q, got %q", expectedPrompt, wsDef.PromptTemplate)
	}
}

func TestLoadWorkersFromConfig_PromptTemplateFromFile(t *testing.T) {
	dir := t.TempDir()

	expectedPrompt := "Custom prompt loaded from file: {{ .WorkID }}"
	writeWorkstationAgentsMDWithPromptFile(t, dir, "review", "prompt.md", expectedPrompt)
	writeWorkerAgentsMD(t, dir, "worker-a")

	factoryCfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{
			{Name: "review"},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "worker-a"},
		},
	}

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, factoryCfg,
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	opts, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), cfg, logging.NoopLogger{}, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}

	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	wsDef, ok := wsExec.RuntimeConfig.Workstation("review")
	if !ok {
		t.Fatal("expected 'review' workstation in runtime config")
	}

	if wsDef.PromptTemplate != expectedPrompt {
		t.Errorf("expected prompt template %q, got %q", expectedPrompt, wsDef.PromptTemplate)
	}
}

func TestLoadWorkersFromConfig_ModelWorkerWithCanonicalExecutorProviderUsesAgentExecutorPath(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
executorProvider: script_wrap
modelProvider: codex
stopToken: COMPLETE
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	opts, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), cfg, logging.NoopLogger{}, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}

	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	if _, ok := wsExec.Executor.(*workers.AgentExecutor); !ok {
		t.Fatalf("expected wrapped executor to be *workers.AgentExecutor, got %T", wsExec.Executor)
	}

	workerDef, ok := wsExec.RuntimeConfig.Worker("worker-a")
	if !ok {
		t.Fatal("expected worker-a in runtime config")
	}
	if workerDef.ExecutorProvider != "script_wrap" {
		t.Fatalf("executor provider = %q, want script_wrap", workerDef.ExecutorProvider)
	}
	if workerDef.ModelProvider != "codex" {
		t.Fatalf("model provider = %q, want codex", workerDef.ModelProvider)
	}
}

func TestLoadWorkersFromConfig_ReplayEmbeddedRuntimeUsesCanonicalLookup(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	generated, err := replay.GeneratedFactoryFromRuntimeConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}

	opts, err := loadWorkersFromConfig(runtimeCfg.FactoryDir(), runtimeCfg.Factory, runtimeCfg, logging.NoopLogger{}, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}

	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	if got := wsExec.RuntimeConfig.FactoryDir(); got != dir {
		t.Fatalf("embedded runtime FactoryDir = %q, want %q", got, dir)
	}
	if got := wsExec.RuntimeConfig.RuntimeBaseDir(); got != dir {
		t.Fatalf("embedded runtime RuntimeBaseDir = %q, want %q", got, dir)
	}
	if _, ok := wsExec.RuntimeConfig.Worker("worker-a"); !ok {
		t.Fatal("expected replay runtime worker lookup for worker-a")
	}
	if _, ok := wsExec.RuntimeConfig.Workstation("review"); !ok {
		t.Fatal("expected replay runtime workstation lookup for review")
	}
}

func TestLoadWorkersFromConfig_LoadedRuntimeBaseDirOverrideFlowsThroughCanonicalLookup(t *testing.T) {
	dir := t.TempDir()
	runtimeBaseDir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)
	loaded.SetRuntimeBaseDir(runtimeBaseDir)

	opts, err := loadWorkersFromConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded, logging.NoopLogger{}, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["worker-a"]
	if !ok {
		t.Fatal("expected worker-a executor to be registered")
	}

	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	if got := wsExec.RuntimeConfig.FactoryDir(); got != dir {
		t.Fatalf("loaded runtime FactoryDir = %q, want %q", got, dir)
	}
	if got := wsExec.RuntimeConfig.RuntimeBaseDir(); got != runtimeBaseDir {
		t.Fatalf("loaded runtime RuntimeBaseDir = %q, want %q", got, runtimeBaseDir)
	}
}

func TestLoadWorkersFromConfig_CanonicalRuntimeLookupDrivesScriptExecutionWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	runtimeBaseDir := t.TempDir()

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeRuntimeLookupWorkstationAgentsMD(t, dir, "run-script")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "run-script",
			WorkerTypeName: "script-worker",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)
	loaded.SetRuntimeBaseDir(runtimeBaseDir)

	runner := &capturingCommandRunner{}
	opts, err := loadWorkersFromConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded, logging.NoopLogger{}, nil, nil, runner, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}

	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-runtime-lookup-script",
		TransitionID:    "t-runtime-lookup-script",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-runtime-lookup-script",
			Color: interfaces.TokenColor{
				WorkID: "work-runtime-lookup-script",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if got := runner.request.WorkDir; got != filepath.Join(runtimeBaseDir, "workspace") {
		t.Fatalf("command working directory = %q, want %q", got, filepath.Join(runtimeBaseDir, "workspace"))
	}
}

func TestLoadWorkersFromConfig_ReplayRuntimeLookupDrivesScriptExecutionWorkingDirectory(t *testing.T) {
	dir := t.TempDir()

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeRuntimeLookupWorkstationAgentsMD(t, dir, "run-script")

	loaded := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "run-script",
			WorkerTypeName: "script-worker",
		}},
		Workers: []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)

	generated, err := replay.GeneratedFactoryFromRuntimeConfig(loaded.FactoryDir(), loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}

	runner := &capturingCommandRunner{}
	opts, err := loadWorkersFromConfig(runtimeCfg.FactoryDir(), runtimeCfg.Factory, runtimeCfg, logging.NoopLogger{}, nil, nil, runner, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}

	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	result, err := wsExec.Execute(context.Background(), interfaces.WorkDispatch{
		DispatchID:      "d-replay-runtime-lookup-script",
		TransitionID:    "t-replay-runtime-lookup-script",
		WorkerType:      "script-worker",
		WorkstationName: "run-script",
		ProjectID:       "agent-factory",
		InputTokens: workers.InputTokens(interfaces.Token{
			ID: "tok-replay-runtime-lookup-script",
			Color: interfaces.TokenColor{
				WorkID: "work-replay-runtime-lookup-script",
			},
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("Outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
	}
	if got := runner.request.WorkDir; got != filepath.Join(dir, "workspace") {
		t.Fatalf("command working directory = %q, want %q", got, filepath.Join(dir, "workspace"))
	}
}

func TestLoadWorkersFromConfig_ScriptWorkerUsesWorkstationExecutor(t *testing.T) {
	dir := t.TempDir()
	scriptRecorder := func(factoryapi.FactoryEvent) {}

	writeScriptWorkerAgentsMD(t, dir, "script-worker")
	writeWorkstationAgentsMD(t, dir, "run-script")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "run-script"}},
		Workers:      []interfaces.WorkerConfig{{Name: "script-worker"}},
	},
		map[string]*interfaces.WorkerConfig{
			"script-worker": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "script-worker")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"run-script": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "run-script")),
		},
	)

	opts, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), cfg, logging.NoopLogger{}, nil, nil, &stubCommandRunner{}, scriptRecorder, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["script-worker"]
	if !ok {
		t.Fatal("expected script-worker executor to be registered")
	}

	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	scriptExec, ok := wsExec.Executor.(*workers.ScriptExecutor)
	if !ok {
		t.Fatalf("expected wrapped executor to be *workers.ScriptExecutor, got %T", wsExec.Executor)
	}
	if recorder := reflect.ValueOf(scriptExec).Elem().FieldByName("recorder"); !recorder.IsValid() || recorder.IsNil() {
		t.Fatal("expected script executor to receive canonical script event recorder")
	}
}

func TestLoadWorkersFromConfig_RegistersWorkerlessLogicalWorkstationByName(t *testing.T) {
	cfg := newLoadedFactoryConfigForServiceTest(t, "", &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "review-loop-breaker",
			Type: interfaces.WorkstationTypeLogical,
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "failed",
			}},
		}},
	}, nil, map[string]*interfaces.FactoryWorkstationConfig{
		"review-loop-breaker": {
			Name: "review-loop-breaker",
			Type: interfaces.WorkstationTypeLogical,
			Inputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "init",
			}},
			Outputs: []interfaces.IOConfig{{
				WorkTypeName: "story",
				StateName:    "failed",
			}},
		},
	})

	opts, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), cfg, logging.NoopLogger{}, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors["review-loop-breaker"]
	if !ok {
		t.Fatal("expected workerless logical workstation executor to be registered by workstation name")
	}
	if _, ok := exec.(*workers.WorkstationExecutor); !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
}

type stubCommandRunner struct{}

func (stubCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte("ok")}, nil
}

type capturingCommandRunner struct {
	request workers.CommandRequest
}

func (r *capturingCommandRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.request = workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req))
	return workers.CommandResult{Stdout: []byte("ok")}, nil
}

func mustLoadWorkerConfig(t *testing.T, dir string) *interfaces.WorkerConfig {
	t.Helper()
	def, err := config.LoadWorkerConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkerConfig(%s): %v", dir, err)
	}
	return def
}

func mustLoadWorkstationConfig(t *testing.T, dir string) *interfaces.FactoryWorkstationConfig {
	t.Helper()
	def, err := config.LoadWorkstationConfig(dir)
	if err != nil {
		t.Fatalf("LoadWorkstationConfig(%s): %v", dir, err)
	}
	return def
}

type blockingInferenceProvider struct {
	releaseCh <-chan struct{}
	content   string
}

func (p *blockingInferenceProvider) Infer(context.Context, interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	<-p.releaseCh
	return interfaces.InferenceResponse{Content: p.content}, nil
}

type dashboardWorldViewProvider struct {
	requests  chan interfaces.ProviderInferenceRequest
	responses chan dashboardWorldViewProviderResponse
}

type dashboardWorldViewProviderResponse struct {
	response interfaces.InferenceResponse
	err      error
}

func newDashboardWorldViewProvider() *dashboardWorldViewProvider {
	return &dashboardWorldViewProvider{
		requests:  make(chan interfaces.ProviderInferenceRequest, 2),
		responses: make(chan dashboardWorldViewProviderResponse, 2),
	}
}

func (p *dashboardWorldViewProvider) Infer(ctx context.Context, request interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	select {
	case p.requests <- request:
	case <-ctx.Done():
		return interfaces.InferenceResponse{}, ctx.Err()
	}
	select {
	case response := <-p.responses:
		return response.response, response.err
	case <-ctx.Done():
		return interfaces.InferenceResponse{}, ctx.Err()
	}
}

func (p *dashboardWorldViewProvider) nextDispatch(t *testing.T) interfaces.ProviderInferenceRequest {
	t.Helper()
	select {
	case request := <-p.requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider dispatch")
		return interfaces.ProviderInferenceRequest{}
	}
}

func (p *dashboardWorldViewProvider) respond(response interfaces.InferenceResponse, err error) {
	p.responses <- dashboardWorldViewProviderResponse{response: response, err: err}
}

func submitDashboardWorldViewWork(t *testing.T, svc *FactoryService, workID, traceID string) {
	t.Helper()
	err := submitWorkRequestsToService(context.Background(), svc, []interfaces.SubmitRequest{{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    json.RawMessage(`{"title":"dashboard world view"}`),
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
}

func renderSimpleDashboardForTest(
	t *testing.T,
	svc *FactoryService,
	rendered <-chan SimpleDashboardRenderInput,
) SimpleDashboardRenderInput {
	t.Helper()
	svc.renderDashboard(context.Background())
	select {
	case input := <-rendered:
		return input
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for dashboard renderer input")
		return SimpleDashboardRenderInput{}
	}
}

func waitForTokenInPlaceByWorkID(t *testing.T, svc *FactoryService, placeID, workID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot output token: %v", err)
		}
		for _, token := range snap.Marking.TokensInPlace(placeID) {
			if token.Color.WorkID == workID || token.Color.ParentID == workID {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work %q in %s", workID, placeID)
}

func assertDashboardRenderDataActive(t *testing.T, renderData dashboardrender.SimpleDashboardRenderData, workID string) {
	t.Helper()
	if renderData.InFlightDispatchCount != 1 || len(renderData.ActiveExecutionsByDispatchID) != 1 {
		t.Fatalf("active executions = %#v, in-flight=%d, want one active dispatch",
			renderData.ActiveExecutionsByDispatchID,
			renderData.InFlightDispatchCount,
		)
	}
	if renderData.Session.DispatchedCount != 1 {
		t.Fatalf("dispatched count = %d, want 1", renderData.Session.DispatchedCount)
	}
	for _, execution := range renderData.ActiveExecutionsByDispatchID {
		for _, item := range execution.WorkItems {
			if item.WorkID == workID {
				if got := len(renderData.Session.DispatchHistory); got != 0 {
					t.Fatalf("dispatch history length = %d, want no completed dispatches during request-only tick", got)
				}
				return
			}
		}
	}
	t.Fatalf("active execution did not include work %q: %#v", workID, renderData.ActiveExecutionsByDispatchID)
}

func assertDashboardRenderDataCompleted(t *testing.T, renderData dashboardrender.SimpleDashboardRenderData, providerSessionID string) {
	t.Helper()
	session := renderData.Session
	if session.CompletedCount != 1 || session.DispatchedCount != 1 {
		t.Fatalf("session counts after completion = %#v, want dispatched=1 completed=1", session)
	}
	if len(session.ProviderSessions) != 1 || session.ProviderSessions[0].ProviderSession.ID != providerSessionID {
		t.Fatalf("provider sessions = %#v, want %q", session.ProviderSessions, providerSessionID)
	}
	assertRenderDataPlaceOccupancyContainsWork(t, renderData, "task:complete", "dashboard-world-active")
	if len(session.DispatchHistory) != 1 || session.DispatchHistory[0].Result.Outcome != string(interfaces.OutcomeAccepted) {
		t.Fatalf("dispatch history = %#v, want one accepted completion", session.DispatchHistory)
	}
	if !dispatchHistoryContainsWork(t, session.DispatchHistory, "dashboard-world-active") {
		t.Fatalf("dispatch history = %#v, want completed work dashboard-world-active", session.DispatchHistory)
	}
}

func assertDashboardRenderDataFailed(t *testing.T, renderData dashboardrender.SimpleDashboardRenderData, workID string) {
	t.Helper()
	session := renderData.Session
	if session.DispatchedCount != 2 || session.CompletedCount != 1 || session.FailedCount != 1 {
		t.Fatalf("session counts after failure = %#v, want dispatched=2 completed=1 failed=1", session)
	}
	assertRenderDataPlaceOccupancyContainsWork(t, renderData, "task:failed", workID)
	if len(session.DispatchHistory) != 2 {
		t.Fatalf("dispatch history = %#v, want both successful and failed completions", session.DispatchHistory)
	}
	if !dispatchHistoryContainsWork(t, session.DispatchHistory, workID) {
		t.Fatalf("dispatch history = %#v, want failed work %q", session.DispatchHistory, workID)
	}
	if !providerSessionsContainID(session.ProviderSessions, "sess-dashboard-failed") {
		t.Fatalf("provider sessions = %#v, want retained failed provider session", session.ProviderSessions)
	}
}

func assertRenderDataPlaceOccupancyContainsWork(
	t *testing.T,
	renderData dashboardrender.SimpleDashboardRenderData,
	placeID, workID string,
) {
	t.Helper()

	for _, item := range renderData.PlaceOccupancyWorkItemsByPlaceID[placeID] {
		if item.WorkID == workID {
			return
		}
	}
	t.Fatalf("place occupancy[%s] = %#v, want work %q", placeID, renderData.PlaceOccupancyWorkItemsByPlaceID[placeID], workID)
}

func dispatchHistoryContainsWork(t *testing.T, history []interfaces.FactoryWorldDispatchCompletion, workID string) bool {
	t.Helper()

	for _, dispatch := range history {
		for _, item := range dispatch.InputWorkItems {
			if item.ID == workID {
				return true
			}
		}
		for _, item := range dispatch.OutputWorkItems {
			if item.ID == workID {
				return true
			}
		}
		if dispatch.TerminalWork != nil && dispatch.TerminalWork.WorkItem.ID == workID {
			return true
		}
		for _, itemID := range dispatch.WorkItemIDs {
			if itemID == workID {
				return true
			}
		}
	}
	return false
}

func providerSessionsContainID(sessions []interfaces.FactoryWorldProviderSessionRecord, sessionID string) bool {
	for _, session := range sessions {
		if session.ProviderSession.ID == sessionID {
			return true
		}
	}
	return false
}

func assertSimpleDashboardOutputContains(t *testing.T, input SimpleDashboardRenderInput, wants []string) {
	t.Helper()
	output := dashboard.FormatSimpleDashboardWithRenderData(
		input.EngineState,
		input.RenderData,
		time.Now(),
	)
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("simple dashboard output missing %q:\n%s", want, output)
		}
	}
}

func stopServiceModeRun(t *testing.T, cancel context.CancelFunc, errCh <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("service-mode run error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode run to stop")
	}
}

type aggregateSnapshotFactory struct {
	engineState              *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	engineStateErr           error
	engineStateSnapshotCalls int
	factoryEvents            []factoryapi.FactoryEvent
	factoryEventsErr         error
	factoryEventsCalls       int
	submitFunc               func(context.Context, interfaces.WorkRequest) error
	submitCalls              int
	submissions              []interfaces.WorkRequest
}

func (f *aggregateSnapshotFactory) Run(context.Context) error { return nil }
func (f *aggregateSnapshotFactory) SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	normalized, err := factory.NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{})
	if err != nil {
		return interfaces.WorkRequestSubmitResult{}, err
	}
	result := interfaces.WorkRequestSubmitResult{RequestID: request.RequestID, Accepted: true}
	if len(normalized) > 0 {
		result.TraceID = normalized[0].TraceID
	}
	f.submitCalls++
	f.submissions = append(f.submissions, request)
	if f.submitFunc != nil {
		return result, f.submitFunc(ctx, request)
	}
	return result, nil
}
func (f *aggregateSnapshotFactory) SubscribeFactoryEvents(context.Context) (*interfaces.FactoryEventStream, error) {
	return &interfaces.FactoryEventStream{Events: make(chan factoryapi.FactoryEvent)}, nil
}
func (f *aggregateSnapshotFactory) Pause(context.Context) error { return nil }
func (f *aggregateSnapshotFactory) GetEngineStateSnapshot(context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	f.engineStateSnapshotCalls++
	if f.engineStateErr != nil {
		return nil, f.engineStateErr
	}
	return f.engineState, nil
}
func (f *aggregateSnapshotFactory) GetFactoryEvents(context.Context) ([]factoryapi.FactoryEvent, error) {
	f.factoryEventsCalls++
	if f.factoryEventsErr != nil {
		return nil, f.factoryEventsErr
	}
	return append([]factoryapi.FactoryEvent(nil), f.factoryEvents...), nil
}
func (f *aggregateSnapshotFactory) WaitToComplete() <-chan struct{} {
	ch := make(chan struct{})
	return ch
}
