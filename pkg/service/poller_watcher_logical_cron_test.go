package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

type rejectingWorkerExecutor struct{}

func (rejectingWorkerExecutor) Execute(context.Context, interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	return interfaces.WorkResult{}, errors.New("worker executor must not be invoked for workerless cron logical move")
}

func TestFactoryService_LogicalMoveCronTickConsumesTimeWorkWithoutWorkerExecutor(t *testing.T) {
	start := time.Date(2026, time.June, 3, 12, 0, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 16)

	dir := t.TempDir()
	writeFactoryJSON(t, dir, logicalMoveCronFactoryConfig("* * * * *"))
	writeLogicalMoveCronWorkstationAgentsMD(t, dir, "scheduled-route")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		Clock:             fakeClock,
		ExtraOptions: []factory.FactoryOption{
			factory.WithSubmissionRecorder(nonBlockingSubmissionRecorder(observedSubmissions)),
			factory.WithWorkerExecutor("cron-worker", rejectingWorkerExecutor{}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()
	waitForCronServiceStartup(t, svc)

	ws := configuredCronWorkstationForServiceTest(t, svc, "scheduled-route")
	if ws.Type != interfaces.WorkstationTypeLogical {
		t.Fatalf("workstation type = %q, want %q", ws.Type, interfaces.WorkstationTypeLogical)
	}
	if err := svc.submitCronTick(runCtx, ws, start); err != nil {
		t.Fatalf("submitCronTick: %v", err)
	}

	record := waitForCronSubmission(t, observedSubmissions, time.Second)
	assertCronSubmissionRecord(t, record, "scheduled-route")
	assertLogicalMoveCronDispatchAndOutput(t, svc, record.Request.WorkID, "scheduled-route", "task:init")
	stopServiceModeRun(t, cancelRun, errCh)
}

func logicalMoveCronFactoryConfig(schedule string) map[string]any {
	return map[string]any{
		"name": "factory",
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
		"workstations": []map[string]any{
			{
				"name":     "scheduled-route",
				"type":     "LOGICAL_MOVE",
				"behavior": "CRON",
				"cron":     map[string]string{"schedule": schedule, "expiryWindow": "500ms"},
				"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func writeLogicalMoveCronWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	wsDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	agentsMD := "---\ntype: LOGICAL_MOVE\n---\n"
	if err := os.WriteFile(filepath.Join(wsDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func waitForCronServiceStartup(t *testing.T, svc *FactoryService) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		handle := svc.currentLiveRuntime()
		if handle != nil {
			startCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := svc.waitForLiveRuntimeStart(startCtx, handle)
			cancel()
			if err != nil {
				t.Fatalf("wait for cron service startup: %v", err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for cron service runtime handle")
}

func assertLogicalMoveCronDispatchAndOutput(
	t *testing.T,
	svc *FactoryService,
	workID string,
	workstation string,
	outputPlace string,
) {
	t.Helper()
	dispatch := waitForCompletedDispatchConsumingWorkID(t, svc, workID, time.Second)
	if dispatch.WorkstationName != workstation {
		t.Fatalf("completed dispatch workstation = %q, want %q", dispatch.WorkstationName, workstation)
	}
	matched := consumedTokenWithWorkID(dispatch.ConsumedTokens, workID)
	if matched == nil {
		t.Fatalf("completed cron dispatch did not retain consumed time token %q: %#v", workID, dispatch.ConsumedTokens)
	}
	if matched.Color.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron token work type = %q, want %q", matched.Color.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}

	output := waitForTokenInPlaceByParent(t, svc, outputPlace, workID, time.Second)
	if output.Color.WorkTypeID != "task" {
		t.Fatalf("cron logical move output work type = %q, want task", output.Color.WorkTypeID)
	}
}
