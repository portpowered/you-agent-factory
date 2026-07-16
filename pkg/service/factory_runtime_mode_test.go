// backendsizecheck:ignore-file consolidated runtime-mode and session-registry tests remain together until dedicated service test seams split.
// pkgmaintcheck:ignore-file-lines consolidated runtime-mode and session-registry tests remain together until dedicated service test seams split.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/operatordefaultsruntime"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factorysessionservice "github.com/portpowered/infinite-you/pkg/factory/sessions/service"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/service/factorysave"
	"github.com/portpowered/infinite-you/pkg/service/runtimebuild"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

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
// pkgmaintcheck:ignore-cyclomatic-complexity this service-mode runtime test keeps idle-startup and late-submission assertions together on the public seam.
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

	err = submitWorkRequestsToService(context.Background(), svc, []work.SubmitRequest{{
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

func TestBuildFactoryService_ServiceModeRuntimeMetricsCaptureLifecycleAndStateTransitions(t *testing.T) {
	dir := t.TempDir()
	metricsDir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	releaseProvider := make(chan struct{})
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		RuntimeMetricsDir: metricsDir,
		ProviderOverride:  &blockingInferenceProvider{releaseCh: releaseProvider, content: "ok"},
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

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "service runtime idle startup")
	session := svc.sessionByID(defaultFactorySessionID)
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil || liveSessionHandle(session).Bundle.MetricsSink == nil {
		t.Fatal("default session runtime metrics sink is unavailable")
	}
	metricsPath := liveSessionHandle(session).Bundle.MetricsSink.Path()
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricLifecycleStarted, 1)
	}, "runtime start")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricStateIdle, 1)
	}, "idle state")

	err = submitWorkRequestsToService(context.Background(), svc, []work.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-runtime-metrics-active",
		Payload:    json.RawMessage(`{"title":"runtime metrics active"}`),
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricQueueSubmissionCount, 1) &&
			metricRecordString(record, "trace_id") == "trace-runtime-metrics-active"
	}, "submission count")

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusActive, time.Second, "service runtime active work")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricStateActive, 1)
	}, "active state")

	close(releaseProvider)
	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "service runtime idle after work")
	if err := svc.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	waitForSessionFactoryState(t, svc, defaultFactorySessionID, interfaces.FactoryStatePaused, time.Second, "service runtime paused")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricStatePaused, 1)
	}, "paused state")

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service runtime shutdown")
	}

	records := waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricNameAndValue(record, runtimeMetricLifecycleStopped, 1) &&
			metricRecordString(record, "outcome") == "canceled"
	}, "canceled stop")
	if len(records) == 0 {
		t.Fatal("runtime metrics records should not be empty")
	}
}

// portos:func-length-exception owner=agent-factory reason=dispatch-metrics-observable-fixture review=2026-07-18 removal=split-runtime-metrics-submission-cases-before-next-dispatch-metrics-change
// pkgmaintcheck:ignore-function-lines this dispatch-metrics runtime test keeps accepted, rejected, and failed observable assertions together on one service seam.
func TestBuildFactoryService_ServiceModeRuntimeMetricsCaptureDispatchOutcomes(t *testing.T) {
	dir := t.TempDir()
	metricsDir := t.TempDir()
	cfg := minimalFactoryConfig()
	cfg["workTypes"] = []map[string]any{{
		"name": "task",
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "complete", "type": "TERMINAL"},
			{"name": "rejected", "type": "REJECTED"},
			{"name": "failed", "type": "FAILED"},
		},
	}}
	cfg["workstations"] = []map[string]any{{
		"name":        "process",
		"worker":      "worker-a",
		"inputs":      []map[string]string{{"workType": "task", "state": "init"}},
		"outputs":     []map[string]string{{"workType": "task", "state": "complete"}},
		"onRejection": []map[string]string{{"workType": "task", "state": "rejected"}},
		"onFailure":   []map[string]string{{"workType": "task", "state": "failed"}},
	}}
	writeFactoryJSON(t, dir, cfg)
	writeWorkerAgentsMD(t, dir, "worker-a")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMode:                             interfaces.RuntimeModeService,
		RuntimeMetricsDir:                       metricsDir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
		ExtraOptions: []factory.FactoryOption{
			factory.WithWorkerExecutor("worker-a", dispatchMetricsWorkerExecutor{}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- svc.Run(runCtx) }()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "service runtime idle startup")
	session := svc.sessionByID(defaultFactorySessionID)
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil || liveSessionHandle(session).Bundle.MetricsSink == nil {
		t.Fatal("default session runtime metrics sink is unavailable")
	}
	metricsPath := liveSessionHandle(session).Bundle.MetricsSink.Path()
	submissions := []struct {
		workID   string
		traceID  string
		placeID  string
		outcome  string
		duration float64
		retries  float64
		cost     float64
	}{
		{workID: "work-dispatch-accepted", traceID: "trace-dispatch-accepted", placeID: "task:complete", outcome: string(workerexecution.OutcomeAccepted), duration: 250, retries: 2, cost: 1.25},
		{workID: "work-dispatch-rejected", traceID: "trace-dispatch-rejected", placeID: "task:rejected", outcome: string(workerexecution.OutcomeRejected), duration: 125, retries: 1},
		{workID: "work-dispatch-failed", traceID: "trace-dispatch-failed", placeID: "task:failed", outcome: string(workerexecution.OutcomeFailed), duration: 500, retries: 3},
	}

	for _, submission := range submissions {
		err = submitWorkRequestsToService(context.Background(), svc, []work.SubmitRequest{{
			WorkID:     submission.workID,
			Name:       submission.workID,
			WorkTypeID: "task",
			TraceID:    submission.traceID,
			Payload:    []byte(`{"title":"` + submission.workID + `"}`),
		}})
		if err != nil {
			t.Fatalf("SubmitWorkRequest(%s): %v", submission.workID, err)
		}
		waitForTokenInPlaceByWorkID(t, svc, submission.placeID, submission.workID, time.Second)
		waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
			return runtimeMetricMatchesWork(record, runtimeMetricDispatchStarted, submission.workID, submission.traceID, "")
		}, "dispatch start "+submission.workID)
		waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
			return runtimeMetricMatchesWork(record, runtimeMetricDispatchComplete, submission.workID, submission.traceID, submission.outcome)
		}, "dispatch completion "+submission.workID)
		waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
			return runtimeMetricMatchesValue(record, runtimeMetricDispatchDuration, submission.workID, submission.outcome, submission.duration, "ms")
		}, "dispatch duration "+submission.workID)
		waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
			return runtimeMetricMatchesValue(record, runtimeMetricDispatchRetries, submission.workID, submission.outcome, submission.retries, "")
		}, "dispatch retries "+submission.workID)
		if submission.cost > 0 {
			waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
				return runtimeMetricMatchesValue(record, runtimeMetricDispatchCost, submission.workID, submission.outcome, submission.cost, "usd")
			}, "dispatch cost "+submission.workID)
		}
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service runtime shutdown")
	}
}

// portos:func-length-exception owner=agent-factory reason=provider-and-script-metrics-observable-fixture review=2026-07-18 removal=split-provider-and-script-runtime-metrics-cases-before-next-boundary-metrics-change
// pkgmaintcheck:ignore-function-lines this runtime metrics test keeps provider and script observable assertions on one service seam.
// pkgmaintcheck:ignore-cyclomatic-complexity this runtime metrics test keeps provider and script observable assertions on one service seam.
func TestBuildFactoryService_ServiceModeRuntimeMetricsCaptureProviderAndScriptDiagnostics(t *testing.T) {
	dir := t.TempDir()
	metricsDir := t.TempDir()
	writeFactoryJSON(t, dir, map[string]any{
		"name": "runtime-metrics-provider-script",
		"workTypes": []map[string]any{
			{
				"name": "model-task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "script-task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name":          "model-worker",
				"type":          interfaces.WorkerTypeModel,
				"modelProvider": "CODEX",
				"model":         "gpt-5-codex",
				"body":          "Process model work.",
			},
			{
				"name":    "script-worker",
				"type":    interfaces.WorkerTypeScript,
				"command": "echo",
				"args":    []string{"ignored"},
				"body":    "Run script work.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":      "review",
				"worker":    "model-worker",
				"inputs":    []map[string]string{{"workType": "model-task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "model-task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "model-task", "state": "failed"}},
			},
			{
				"name":      "run-script",
				"worker":    "script-worker",
				"inputs":    []map[string]string{{"workType": "script-task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "script-task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "script-task", "state": "failed"}},
			},
		},
	})
	writeWorkstationAgentsMD(t, dir, "review")
	writeWorkstationAgentsMD(t, dir, "run-script")

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMode:                             interfaces.RuntimeModeService,
		RuntimeMetricsDir:                       metricsDir,
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		ExtraOptions: []factory.FactoryOption{
			factory.WithWorkerExecutor("model-worker", providerMetricsWorkerExecutor{}),
			factory.WithWorkerExecutor("script-worker", scriptMetricsWorkerExecutor{}),
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- svc.Run(runCtx) }()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "service runtime idle startup")
	session := svc.sessionByID(defaultFactorySessionID)
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil || liveSessionHandle(session).Bundle.MetricsSink == nil {
		t.Fatal("default session runtime metrics sink is unavailable")
	}
	metricsPath := liveSessionHandle(session).Bundle.MetricsSink.Path()

	err = submitWorkRequestsToService(context.Background(), svc, []work.SubmitRequest{{
		WorkID:     "work-provider-metrics",
		Name:       "work-provider-metrics",
		WorkTypeID: "model-task",
		TraceID:    "trace-provider-metrics",
		Payload:    []byte(`{"title":"provider metrics"}`),
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest(provider): %v", err)
	}
	waitForTokenInPlaceByWorkID(t, svc, "model-task:complete", "work-provider-metrics", time.Second)
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesProvider(record, runtimeMetricProviderRequest, "work-provider-metrics", string(modelprovider.Codex), "")
	}, "provider request")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesProvider(record, runtimeMetricProviderComplete, "work-provider-metrics", string(modelprovider.Codex), string(workerexecution.OutcomeAccepted))
	}, "provider completion")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesProviderValue(record, runtimeMetricProviderDuration, "work-provider-metrics", float64(240), "ms")
	}, "provider duration")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesProviderValue(record, runtimeMetricProviderInputTok, "work-provider-metrics", float64(13), "tokens")
	}, "provider input tokens")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesProviderValue(record, runtimeMetricProviderOutputTok, "work-provider-metrics", float64(7), "tokens")
	}, "provider output tokens")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesProviderValue(record, runtimeMetricProviderCost, "work-provider-metrics", 2.75, "usd")
	}, "provider cost")

	err = submitWorkRequestsToService(context.Background(), svc, []work.SubmitRequest{{
		WorkID:     "work-provider-failed",
		Name:       "work-provider-failed",
		WorkTypeID: "model-task",
		TraceID:    "trace-provider-failed",
		Payload:    []byte(`{"title":"provider failure"}`),
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest(provider failure): %v", err)
	}
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesProvider(record, runtimeMetricProviderFailed, "work-provider-failed", string(modelprovider.Codex), string(workerexecution.OutcomeFailed)) &&
			metricRecordString(record, "reason") == string(workerexecution.WorkFailureTypeInternalServerError)
	}, "provider failure")

	err = submitWorkRequestsToService(context.Background(), svc, []work.SubmitRequest{{
		WorkID:     "work-script-metrics",
		Name:       "work-script-metrics",
		WorkTypeID: "script-task",
		TraceID:    "trace-script-metrics",
		Payload:    []byte(`{"title":"script metrics"}`),
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest(script): %v", err)
	}
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesWork(record, runtimeMetricScriptStarted, "work-script-metrics", "trace-script-metrics", "")
	}, "script start")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesWork(record, runtimeMetricScriptComplete, "work-script-metrics", "trace-script-metrics", string(workerexecution.OutcomeFailed)) &&
			metricRecordString(record, "reason") == "timeout"
	}, "script completion")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesValue(record, runtimeMetricScriptDuration, "work-script-metrics", string(workerexecution.OutcomeFailed), float64(875), "ms")
	}, "script duration")
	waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesValue(record, runtimeMetricScriptTimedOut, "work-script-metrics", string(workerexecution.OutcomeFailed), 1, "")
	}, "script timeout")

	records := waitForRuntimeMetricsRecord(t, metricsPath, time.Second, func(record map[string]any) bool {
		return runtimeMetricMatchesValue(record, runtimeMetricScriptTimedOut, "work-script-metrics", string(workerexecution.OutcomeFailed), 1, "")
	}, "all provider and script metrics")
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("Marshal(metric record): %v", err)
		}
		text := string(encoded)
		if strings.Contains(text, "provider stdout secret") || strings.Contains(text, "provider stderr secret") ||
			strings.Contains(text, "script stdout secret") || strings.Contains(text, "script stderr secret") {
			t.Fatalf("metrics record leaked command output: %s", text)
		}
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service runtime shutdown")
	}
}

func TestBuildFactoryService_ServiceModeContinuesWhenRuntimeMetricsSinkUnavailable(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	metricsRootFile := filepath.Join(t.TempDir(), "metrics-root-file")
	if err := os.WriteFile(metricsRootFile, []byte("not-a-directory"), 0o644); err != nil {
		t.Fatalf("write metrics root file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		RuntimeMetricsDir: metricsRootFile,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- svc.Run(runCtx) }()

	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "service runtime idle startup without metrics sink")
	session := svc.sessionByID(defaultFactorySessionID)
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil {
		t.Fatal("default session runtime is unavailable")
	}
	if liveSessionHandle(session).Bundle.MetricsSink != nil {
		t.Fatal("runtime metrics sink should be nil when metrics root is unavailable")
	}
	logPath := liveSessionHandle(session).Bundle.LogSink.Path()
	err = submitWorkRequestsToService(context.Background(), svc, []work.SubmitRequest{{
		WorkID:     "work-no-metrics-sink",
		Name:       "work-no-metrics-sink",
		WorkTypeID: "task",
		TraceID:    "trace-no-metrics-sink",
		Payload:    []byte(`{"title":"continue without metrics sink"}`),
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	waitForTokenInPlaceByWorkID(t, svc, "task:complete", "work-no-metrics-sink", time.Second)

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service runtime shutdown")
	}

	assertRuntimeLogContainsMetricsSinkWarning(t, logPath)
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

	err = submitWorkRequestsToService(context.Background(), svc, []work.SubmitRequest{{
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

func TestFactoryService_ServiceModeAPISurfaceStartsBeforeStartupWorkFileSubmission(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "startup-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "requestId": "request-startup-before-workfile",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "startup-item",
      "workId": "work-startup-before-workfile",
      "workTypeName": "task",
      "traceId": "trace-startup-before-workfile",
      "payload": {"title": "startup work"}
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	type starterObservation struct {
		snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
		err      error
	}

	observedCh := make(chan starterObservation, 1)
	apiReady := make(chan struct{})
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		WorkFile:          workFile,
		Port:              1,
		Logger:            zap.NewNop(),
		APIServerReady:    apiReady,
		APIServerStarter: func(ctx context.Context, runtime apisurface.APISurface, _ int, _ *zap.Logger) error {
			snapshot, err := runtime.GetEngineStateSnapshot(ctx)
			if err != nil {
				observedCh <- starterObservation{err: err}
			} else {
				observedCh <- starterObservation{snapshot: snapshot}
			}
			close(apiReady)
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	var observation starterObservation
	select {
	case observation = <-observedCh:
	case err := <-errCh:
		t.Fatalf("Run returned before API starter observation: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for API starter observation")
	}
	if observation.err != nil {
		t.Fatalf("APIServerStarter GetEngineStateSnapshot: %v", observation.err)
	}
	if observation.snapshot == nil {
		t.Fatal("APIServerStarter snapshot = nil, want idle runtime snapshot")
	}
	if observation.snapshot.RuntimeStatus != interfaces.RuntimeStatusIdle {
		t.Fatalf("startup runtime status = %q, want %q", observation.snapshot.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}
	if len(observation.snapshot.Marking.Tokens) != 0 {
		t.Fatalf("startup tokens = %#v, want no startup-work tokens before work-file submission", observation.snapshot.Marking.Tokens)
	}

	waitForTokenInPlaceByWorkID(t, svc, "task:complete", "work-startup-before-workfile", time.Second)
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

func TestFactoryService_ServiceModeStartupWorkReadabilityFailsWhenAPIServerStartFails(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "startup-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "requestId": "request-startup-api-failure",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "startup-item",
      "workId": "work-startup-api-failure",
      "workTypeName": "task",
      "traceId": "trace-startup-api-failure",
      "payload": {"title": "startup work"}
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	apiStartErr := errors.New("listen tcp 127.0.0.1:7777: bind: address already in use")
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		WorkFile:          workFile,
		Port:              7777,
		Logger:            zap.NewNop(),
		APIServerReady:    make(chan struct{}),
		APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
			return apiStartErr
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(context.Background())
	}()

	select {
	case runErr := <-runErrCh:
		if runErr == nil {
			t.Fatal("Run error = nil, want API startup failure")
		}
		if !strings.Contains(runErr.Error(), "wait for service-mode startup work readiness") {
			t.Fatalf("Run error = %q, want startup readability context", runErr.Error())
		}
		if !strings.Contains(runErr.Error(), apiStartErr.Error()) {
			t.Fatalf("Run error = %q, want API startup failure detail %q", runErr.Error(), apiStartErr.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode API startup failure to return")
	}
}

func TestFactoryService_ServiceModeStartupWorkSkipsAPIReadinessWaitWhenPortDisabled(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "startup-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "requestId": "request-service-port-disabled",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {
      "name": "startup-item",
      "workId": "work-service-port-disabled",
      "workTypeName": "task",
      "traceId": "trace-service-port-disabled",
      "payload": {"title": "startup work"}
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write work file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		WorkFile:          workFile,
		Logger:            zap.NewNop(),
		Port:              0,
		APIServerReady:    make(chan struct{}),
		APIServerStarter: func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
			return errors.New("API server should not start when port is disabled")
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- svc.Run(runCtx)
	}()

	waitForTokenInPlaceByWorkID(t, svc, "task:complete", "work-service-port-disabled", time.Second)
	cancel()
	select {
	case runErr := <-runErrCh:
		if runErr != nil {
			t.Fatalf("Run after cancellation: %v", runErr)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode run with disabled API port to stop")
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime observability test keeps snapshot and event-stream assertions together in one service flow.
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
	want := "works[0].target_state is not supported; use state"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

func TestBuildFactoryService_WorkFileRejectsConflictingTraceAliases(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	if err := os.WriteFile(workFile, []byte(`{
  "requestId": "request-service-trace-conflict",
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "draft", "workTypeName": "task", "currentChainingTraceId": "chain-a", "traceId": "trace-b"}
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
		t.Fatal("expected conflicting trace aliases to fail")
	}
	if !strings.Contains(err.Error(), "currentChainingTraceId and traceId must match") {
		t.Fatalf("error = %q, want conflicting trace alias rejection", err.Error())
	}
}

func TestBuildReplacementFactoryRuntime_ServiceModeStaysRunningUntilCanceled(t *testing.T) {
	rootDir := t.TempDir()
	runtimeLogDir := filepath.Join(rootDir, "runtime-logs")
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
		RuntimeLogDir:     runtimeLogDir,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.coordinatorPolicy().runtimeMode != interfaces.RuntimeModeService {
		t.Fatalf("service runtime mode = %q, want %q", svc.coordinatorPolicy().runtimeMode, interfaces.RuntimeModeService)
	}
	if svc.coordinatorPolicy().dir != alphaDir {
		t.Fatalf("service dir = %q, want %q", svc.coordinatorPolicy().dir, alphaDir)
	}
	t.Cleanup(func() {
		if startup := svc.startupRuntimeBundle(); startup != nil {
			if err := closeRuntimeBundleSinks(startup.LogSink, startup.MetricsSink); err != nil {
				t.Errorf("close startup runtime sinks: %v", err)
			}
		}
	})

	createReplacementWatchChannel(t, betaDir, "task", "activated")
	replacement, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, defaultFactorySessionID)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if replacement.Dir != betaDir {
		t.Fatalf("replacement dir = %q, want %q", replacement.Dir, betaDir)
	}
	t.Cleanup(func() {
		if err := closeRuntimeBundleSinks(replacement.LogSink, replacement.MetricsSink); err != nil {
			t.Errorf("close replacement runtime sinks: %v", err)
		}
	})

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- replacement.Factory.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("replacement runtime returned before cancellation: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("replacement runtime after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replacement runtime to stop")
	}
}

func TestBuildReplacementFactoryRuntime_WiresLocalModelDelegationSeam(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		LocalModelRuntimeOverride: &fakeLocalModelRuntime{
			response: workerexecution.InferenceResponse{Content: "ok"},
		},
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	replacement, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, defaultFactorySessionID)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if replacement.LocalModels == nil {
		t.Fatal("runtime bundle localModels = nil, want managed localmodels.Manager from buildRuntimeBundle seam")
	}
	if replacement.ModelAssets == nil {
		t.Fatal("runtime bundle modelAssets = nil, want localmodels.AssetPuller from buildRuntimeBundle seam")
	}
	if replacement.ModelResources == nil {
		t.Fatal("runtime bundle modelResources = nil, want localmodels.ResourceLimiter from buildRuntimeBundle seam")
	}
	if replacement.LogSink == nil {
		t.Fatal("runtime bundle logSink = nil, want runtime log sink from buildRuntimeBundle seam")
	}
	if replacement.Logger == nil {
		t.Fatal("runtime bundle logger = nil, want session logger from buildRuntimeBundle seam")
	}
}

func TestBuildFactoryService_StartupRuntimeBundleMatchesLiveHandleShape(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
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
	bundle := svc.currentRuntimeBundle()
	if bundle == nil {
		t.Fatal("currentRuntimeBundle = nil, want startup bundle before Run")
	}
	if bundle.LogSink == nil {
		t.Fatal("startup bundle logSink = nil, want runtime log sink")
	}
	if bundle.Factory == nil {
		t.Fatal("startup bundle factory = nil")
	}
	if bundle == nil {
		t.Fatal("currentRuntimeBundle should resolve through the bound default session runtime")
	}
}

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
	runFactoryServiceWithCleanup(t, svc)
	waitForSessionRuntimeStatus(t, svc, defaultFactorySessionID, interfaces.RuntimeStatusIdle, time.Second, "default runtime")
	defaultHandle := liveSessionHandle(svc.defaultSession())
	if bundle := svc.currentRuntimeBundle(); bundle == nil || defaultHandle == nil || bundle != defaultHandle.Bundle {
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
	if svc.runtimeBuild == nil {
		t.Fatal("expected runtimebuild.Service on FactoryService")
	}
	registryBefore := svc.sessions

	if _, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, defaultFactorySessionID); err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if svc.sessions != registryBefore {
		t.Fatal("buildReplacementFactoryRuntime replaced sessions registry; session ownership should stay on FactoryService")
	}
	if svc.coordinatorPolicy().dir != alphaDir {
		t.Fatalf("service dir = %q, want unchanged %q until activation", svc.coordinatorPolicy().dir, alphaDir)
	}
}

func TestBuildFactoryService_ConstructsExplicitCollaborators(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	models := &stubModelService{}
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		ModelAPI:          models,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	assertExplicitFactoryServiceCollaborators(t, svc, alphaDir, models)
}

func TestBuildFactoryServiceWithoutModelAPIDoesNotConstructFallback(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
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
	if _, err := svc.ListModels(context.Background()); !errors.Is(err, errModelServiceUnavailable) {
		t.Fatalf("ListModels error = %v, want explicit unavailable model boundary", err)
	}
}

func assertExplicitFactoryServiceCollaborators(
	t *testing.T,
	svc *FactoryService,
	alphaDir string,
	models apisurface.ModelAPI,
) {
	t.Helper()

	if svc.sessions == nil {
		t.Fatal("expected explicit factorysessions.Registry collaborator")
	}
	if svc.runtimeBuild == nil {
		t.Fatal("expected explicit runtimebuild.Service collaborator")
	}
	if svc.modelService == nil {
		t.Fatal("expected explicit model service collaborator")
	}
	if svc.modelService != models {
		t.Fatalf("modelService = %T, want exact injected collaborator %T", svc.modelService, models)
	}
	if _, err := svc.ListModels(context.Background()); err != nil {
		t.Fatalf("ListModels through injected compatibility service: %v", err)
	}
	if svc.factorySave == nil {
		t.Fatal("expected explicit factorysave collaborator")
	}
	if svc.definitions == nil {
		t.Fatal("expected explicit factory definition collaborator")
	}
	if _, ok := svc.factorySave.(*factorysave.Service); !ok {
		t.Fatalf("factorySave type = %T, want *factorysave.Service for production wiring", svc.factorySave)
	}
	if svc.hostedWorkers.Logger == nil {
		t.Fatal("expected explicit hostedworkers.Config collaborator with logger")
	}
	if svc.modelAssets == nil {
		t.Fatal("expected explicit localmodels asset puller collaborator")
	}
	defaultSessionSpec := liveSessionBuildSpec(svc.defaultSession())
	if defaultSessionSpec == nil {
		t.Fatal("expected default session build spec")
	}
	if defaultSessionSpec.LoadedFactoryCfg == nil {
		t.Fatal("default session build spec loaded config = nil")
	}
	if svc.coordinatorPolicy().dir != alphaDir {
		t.Fatalf("service dir = %q, want %q", svc.coordinatorPolicy().dir, alphaDir)
	}
}

func TestBuildFactoryService_WiresSessionGatewayCollaborator(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.sessionGateway == nil {
		t.Fatal("expected session gateway collaborator on FactoryService")
	}
	if _, ok := svc.sessionGateway.(*factorysessionservice.Service); !ok {
		t.Fatalf("session gateway type = %T, want *factorysessionservice.Service", svc.sessionGateway)
	}
}

func TestFactoryService_SaveFactoryForSession_DelegatesToInjectedFactorySave(t *testing.T) {
	t.Parallel()

	stub := &recordingFactorySaveSaver{}
	svc := &FactoryService{factorySave: stub}
	request := factoryapi.Factory{
		Name: factoryapi.FactoryName("story-save"),
	}
	mode := factoryapi.FactorySaveModeReplaceCurrent
	sessionID := "session-collaborator-proof"

	got, err := svc.SaveFactoryForSession(context.Background(), sessionID, mode, request)
	if err != nil {
		t.Fatalf("SaveFactoryForSession: %v", err)
	}
	if got.Name != request.Name {
		t.Fatalf("saved factory name = %q, want %q", got.Name, request.Name)
	}
	if stub.calls != 1 {
		t.Fatalf("factory save calls = %d, want 1", stub.calls)
	}
	if stub.sessionID != sessionID {
		t.Fatalf("factory save sessionID = %q, want %q", stub.sessionID, sessionID)
	}
	if stub.mode != mode {
		t.Fatalf("factory save mode = %q, want %q", stub.mode, mode)
	}
	if stub.request.Name != request.Name {
		t.Fatalf("factory save request name = %q, want %q", stub.request.Name, request.Name)
	}
}

type recordingFactorySaveSaver struct {
	sessionID string
	mode      factoryapi.FactorySaveMode
	request   factoryapi.Factory
	calls     int
}

func (s *recordingFactorySaveSaver) Save(
	_ context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	s.calls++
	s.sessionID = sessionID
	s.mode = mode
	s.request = request
	return request, nil
}

func TestFactoryService_GetCurrentFactory_DelegatesToInjectedDefinitions(t *testing.T) {
	t.Parallel()

	stub := &recordingFactoryDefinitions{
		namedFactory: factoryapi.Factory{Name: factoryapi.FactoryName("delegated-current")},
	}
	svc := &FactoryService{definitions: stub}

	got, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if got.Name != stub.namedFactory.Name {
		t.Fatalf("current factory name = %q, want %q", got.Name, stub.namedFactory.Name)
	}
	if stub.namedCalls != 1 {
		t.Fatalf("named definition calls = %d, want 1", stub.namedCalls)
	}
}

func TestFactoryService_GetCurrentFactoryForSession_DelegatesToInjectedDefinitions(t *testing.T) {
	t.Parallel()

	const sessionID = "session-definition-proof"
	stub := &recordingFactoryDefinitions{
		sessionFactory: factoryapi.Factory{Name: factoryapi.FactoryName("delegated-session")},
	}
	svc := &FactoryService{definitions: stub}

	got, err := svc.GetCurrentFactoryForSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession: %v", err)
	}
	if got.Name != stub.sessionFactory.Name {
		t.Fatalf("session factory name = %q, want %q", got.Name, stub.sessionFactory.Name)
	}
	if stub.sessionCalls != 1 {
		t.Fatalf("session definition calls = %d, want 1", stub.sessionCalls)
	}
	if stub.sessionID != sessionID {
		t.Fatalf("session definition sessionID = %q, want %q", stub.sessionID, sessionID)
	}
}

type recordingFactoryDefinitions struct {
	namedFactory   factoryapi.Factory
	sessionFactory factoryapi.Factory
	sessionID      string
	namedCalls     int
	sessionCalls   int
}

func (s *recordingFactoryDefinitions) GetCurrentNamedFactory(context.Context) (factoryapi.Factory, error) {
	s.namedCalls++
	return s.namedFactory, nil
}

func (s *recordingFactoryDefinitions) GetCurrentFactoryForSession(_ context.Context, sessionID string) (factoryapi.Factory, error) {
	s.sessionCalls++
	s.sessionID = sessionID
	return s.sessionFactory, nil
}

func TestFactoryService_ListModels_DelegatesToLocalmodelsCatalog(t *testing.T) {
	t.Parallel()

	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", localModelFactoryConfig(), localModelRuntimeWorkers(), nil)
	svc := newModelCatalogServiceForTest(runtimeCfg, nil)
	attachModelServiceForTest(t, svc)

	got, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want, err := localmodels.ListModels(runtimeCfg)
	if err != nil {
		t.Fatalf("localmodels.ListModels: %v", err)
	}
	if len(got.Results) != len(want.Results) {
		t.Fatalf("ListModels results = %d, want %d from localmodels owner", len(got.Results), len(want.Results))
	}
	if len(got.Results) != 1 || got.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("ListModels results = %#v, want one OMNIVOICE model", got.Results)
	}
}

func TestFactoryService_GetModel_DelegatesToLocalmodelsCatalog(t *testing.T) {
	t.Parallel()

	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", localModelFactoryConfig(), localModelRuntimeWorkers(), nil)
	svc := newModelCatalogServiceForTest(runtimeCfg, nil)
	attachModelServiceForTest(t, svc)

	got, err := svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	want, err := localmodels.GetModel(runtimeCfg, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("localmodels.GetModel: %v", err)
	}
	if got.Name != want.Name || got.Status != want.Status {
		t.Fatalf("GetModel detail = (%s, %s), want (%s, %s) from localmodels owner", got.Name, got.Status, want.Name, want.Status)
	}
}

func TestFactoryService_PullModel_DelegatesToInjectedModelAssets(t *testing.T) {
	t.Parallel()

	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", localModelFactoryConfig(), localModelRuntimeWorkers(), nil)
	puller := &recordingServiceModelAssetPuller{}
	svc := newModelCatalogServiceForTest(runtimeCfg, puller)
	attachModelServiceForTest(t, svc)

	if _, err := svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M"); err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if puller.calls != 1 || puller.modelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model asset puller calls = %d modelName = %q, want 1 and OMNIVOICE_Q4_K_M", puller.calls, puller.modelName)
	}
}

func newModelCatalogServiceForTest(runtimeCfg *factoryconfig.LoadedFactoryConfig, puller modelAssetPuller) *FactoryService {
	svc := &FactoryService{
		sessions:    factorysessions.NewRegistry(),
		modelAssets: puller,
	}
	svc.sessions.Upsert(factorysessions.NewLiveSession(
		defaultFactorySessionID,
		"",
		"",
		"",
		FactorySessionTargetRef{},
		&liveSessionState{spec: &runtimebuild.SessionBuildSpec{LoadedFactoryCfg: runtimeCfg}},
		true,
		"",
	), true)
	return svc
}

type recordingServiceModelAssetPuller struct {
	calls     int
	modelName string
}

func (p *recordingServiceModelAssetPuller) PullModel(_ context.Context, _ *config.LoadedFactoryConfig, modelName string) (apisurface.ModelPullResult, error) {
	p.calls++
	p.modelName = modelName
	return apisurface.ModelPullResult{ModelName: modelName}, nil
}

func (p *recordingServiceModelAssetPuller) EnsureModelAvailable(context.Context, *config.LoadedFactoryConfig, *workerconfig.Config) error {
	return nil
}

func (p *recordingServiceModelAssetPuller) ResolveModelCache(context.Context, *config.LoadedFactoryConfig, *workerconfig.Config) (localModelCacheLayout, error) {
	return localModelCacheLayout{}, nil
}

func (p *recordingServiceModelAssetPuller) InspectRuntimeCache(context.Context, *config.LoadedFactoryConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{}, nil
}

type stubModelService struct {
	listResult   factoryapi.ListModelsResponse
	listErr      error
	gotModel     factoryapi.ModelDetail
	getErr       error
	pullResult   apisurface.ModelPullResult
	pullErr      error
	invokeResult apisurface.ModelInvocationResult
	invokeErr    error
	calls        []string
	modelNames   []string
	requests     []factoryapi.ModelInvocationRequest
	contexts     []context.Context
}

func (s *stubModelService) ListModels(ctx context.Context) (factoryapi.ListModelsResponse, error) {
	s.calls = append(s.calls, "list")
	s.contexts = append(s.contexts, ctx)
	return s.listResult, s.listErr
}

func (s *stubModelService) GetModel(ctx context.Context, modelName string) (factoryapi.ModelDetail, error) {
	s.calls = append(s.calls, "get")
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	return s.gotModel, s.getErr
}

func (s *stubModelService) PullModel(ctx context.Context, modelName string) (apisurface.ModelPullResult, error) {
	s.calls = append(s.calls, "pull")
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	return s.pullResult, s.pullErr
}

func (s *stubModelService) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	s.calls = append(s.calls, "invoke")
	s.contexts = append(s.contexts, ctx)
	s.modelNames = append(s.modelNames, modelName)
	s.requests = append(s.requests, request)
	return s.invokeResult, s.invokeErr
}

type stubSessionGateway struct {
	openResult          *FactorySessionOpenResult
	openFromFolder      *FactorySessionOpenResult
	listSessionsResult  []factorysessions.ReadProjection
	getSessionResult    factorysessions.ProjectionContext
	pauseResult         factorysessionexecution.LifecycleControlResult
	resumeResult        factorysessionexecution.LifecycleControlResult
	durablePauseResult  factorysessionexecution.LifecycleControlResult
	durableCancelResult factorysessionexecution.LifecycleControlResult
	calls               []string
	folderPaths         []string
	sessionIDs          []string
}

func (s *stubSessionGateway) OpenFactorySession(_ context.Context, request factorysessions.OpenRequest) (*FactorySessionOpenResult, error) {
	s.calls = append(s.calls, "open-session")
	if request.FolderPath != "" {
		s.folderPaths = append(s.folderPaths, request.FolderPath)
	}
	return s.openResult, nil
}

func (s *stubSessionGateway) OpenFactorySessionFromFolder(_ context.Context, folderPath string, _ *FactorySessionTargetRef, _ bool, _ bool) (*FactorySessionOpenResult, error) {
	s.calls = append(s.calls, "open-session-from-folder")
	s.folderPaths = append(s.folderPaths, folderPath)
	if s.openFromFolder != nil {
		return s.openFromFolder, nil
	}
	return &FactorySessionOpenResult{SessionID: "session-from-folder"}, nil
}

func (s *stubSessionGateway) ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error) {
	s.calls = append(s.calls, "list-sessions")
	return s.listSessionsResult, nil
}

func (s *stubSessionGateway) GetFactorySession(_ context.Context, sessionID string) (factorysessions.ProjectionContext, error) {
	s.calls = append(s.calls, "get-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.getSessionResult, nil
}

func (s *stubSessionGateway) GetFactorySessionSyncPreflight(
	_ context.Context,
	sessionID string,
	_ *interfaces.FactoryEventReconnectCursor,
	_ *interfaces.FactorySessionLogicalResolveHint,
) (factorysessions.SyncPreflightResult, error) {
	s.calls = append(s.calls, "get-session-sync-preflight")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return factorysessions.SyncPreflightResult{}, nil
}

func (s *stubSessionGateway) GetFactorySessionResult(_ context.Context, sessionID string) (workflowresult.LiveSessionResult, error) {
	s.calls = append(s.calls, "get-session-result")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return workflowresult.LiveSessionResult{}, nil
}

func (s *stubSessionGateway) GetFactorySessionPartialResult(_ context.Context, sessionID string) (workflowresult.PartialSessionResult, error) {
	s.calls = append(s.calls, "get-session-partial-result")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return workflowresult.PartialSessionResult{}, nil
}

func (s *stubSessionGateway) PauseLiveFactorySession(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.calls = append(s.calls, "pause-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.pauseResult, nil
}

func (s *stubSessionGateway) ResumeLiveFactorySession(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.calls = append(s.calls, "resume-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.resumeResult, nil
}

func (s *stubSessionGateway) CloseFactorySession(_ context.Context, sessionID string) error {
	s.calls = append(s.calls, "close-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return nil
}

func (s *stubSessionGateway) PauseDurableFactorySession(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.calls = append(s.calls, "pause-durable-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.durablePauseResult, nil
}

func (s *stubSessionGateway) ResumeDurableFactorySession(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.calls = append(s.calls, "resume-durable-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.durablePauseResult, nil
}

func (s *stubSessionGateway) CancelDurableFactorySession(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.calls = append(s.calls, "cancel-durable-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.durableCancelResult, nil
}

func (s *stubSessionGateway) TerminateDurableFactorySession(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.calls = append(s.calls, "terminate-durable-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.durableCancelResult, nil
}

func (s *stubSessionGateway) ApproveDurableFactorySession(_ context.Context, sessionID string, _ factorysessionexecution.ApproveRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.calls = append(s.calls, "approve-durable-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.durablePauseResult, nil
}

func (s *stubSessionGateway) RetryDurableFactorySessionDispatch(_ context.Context, sessionID string, _ factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.calls = append(s.calls, "retry-durable-dispatch")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.durablePauseResult, nil
}

func (s *stubSessionGateway) InterruptDurableFactorySessionDispatch(_ context.Context, sessionID string, _ factorysessionexecution.InterruptDispatchRequest) (factorysessionexecution.LifecycleControlResult, error) {
	s.calls = append(s.calls, "interrupt-durable-dispatch")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.durablePauseResult, nil
}

func (s *stubSessionGateway) SubscribeSessionResponseStream(string, string, int64) (*factorysessions.SessionResponseStreamSubscription, error) {
	s.calls = append(s.calls, "subscribe-response-stream")
	return nil, nil
}

func (s *stubSessionGateway) SessionResponseStreamDispatchIDs(string) ([]string, error) {
	s.calls = append(s.calls, "response-stream-dispatch-ids")
	return nil, nil
}

func (s *stubSessionGateway) CloseSessionResponseStreams(*factorysessions.LiveSession) {
	s.calls = append(s.calls, "close-response-streams")
}

func (s *stubSessionGateway) JavaScriptCheckpointStore(*factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore {
	s.calls = append(s.calls, "javascript-checkpoint-store")
	return nil
}

func (s *stubSessionGateway) InferenceProgressPublisherFactory(*zap.Logger) func(string) workerprovider.InferenceProgressPublisher {
	s.calls = append(s.calls, "inference-progress-publisher-factory")
	return nil
}

func (s *stubSessionGateway) DispatchCompletionObserverFactory() func(string) func(string) {
	s.calls = append(s.calls, "dispatch-completion-observer-factory")
	return nil
}

type stubFactoryCoordinator struct {
	listSessionsResult factoryapi.ListFactorySessionsResponse
	getSessionResult   factoryapi.FactorySession
	openResult         factoryapi.OpenFactorySessionResponse
	currentFactory     factoryapi.Factory
	workSubmitResult   work.WorkRequestSubmitResult
	moveResult         work.OperatorMoveResult
	eventStream        *interfaces.FactoryEventStream
	engineSnapshot     *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	calls              []string
	sessionIDs         []string
	folderPaths        []string
	runtimeNames       []string
}

func (s *stubFactoryCoordinator) ActivateNamedFactory(_ context.Context, name string) error {
	s.calls = append(s.calls, "activate")
	s.runtimeNames = append(s.runtimeNames, name)
	return nil
}

func (s *stubFactoryCoordinator) ListFactorySessions(context.Context) (factoryapi.ListFactorySessionsResponse, error) {
	s.calls = append(s.calls, "list-sessions")
	return s.listSessionsResult, nil
}

func (s *stubFactoryCoordinator) GetFactorySession(_ context.Context, sessionID string) (factoryapi.FactorySession, error) {
	s.calls = append(s.calls, "get-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.getSessionResult, nil
}

func (s *stubFactoryCoordinator) GetFactorySessionSyncPreflight(
	_ context.Context,
	sessionID string,
	_ interfaces.FactorySessionSyncPreflightOptions,
) (factoryapi.FactorySessionSyncPreflightResponse, error) {
	s.calls = append(s.calls, "get-session-sync-preflight")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return factoryapi.FactorySessionSyncPreflightResponse{}, nil
}

func (s *stubFactoryCoordinator) GetFactorySessionResult(context.Context, string) (factoryapi.FactorySessionLiveResult, error) {
	s.calls = append(s.calls, "get-session-result")
	return factoryapi.FactorySessionLiveResult{}, nil
}

func (s *stubFactoryCoordinator) GetFactorySessionPartialResult(context.Context, string) (factoryapi.FactorySessionPartialResult, error) {
	s.calls = append(s.calls, "get-session-partial-result")
	return factoryapi.FactorySessionPartialResult{}, nil
}

func (s *stubFactoryCoordinator) OpenFactorySession(_ context.Context, request factoryapi.OpenFactorySessionRequest) (factoryapi.OpenFactorySessionResponse, error) {
	s.calls = append(s.calls, "open-session")
	if request.FolderPath != "" {
		s.folderPaths = append(s.folderPaths, request.FolderPath)
	}
	return s.openResult, nil
}

func (s *stubFactoryCoordinator) CloseFactorySession(_ context.Context, sessionID string) error {
	s.calls = append(s.calls, "close-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return nil
}

func (s *stubFactoryCoordinator) OpenFactorySessionFromFolder(_ context.Context, folderPath string, _ *FactorySessionTargetRef, _ bool, _ bool) (*FactorySessionOpenResult, error) {
	s.calls = append(s.calls, "open-session-from-folder")
	s.folderPaths = append(s.folderPaths, folderPath)
	return &FactorySessionOpenResult{SessionID: "session-from-folder"}, nil
}

func (s *stubFactoryCoordinator) SubmitWorkRequestForSession(_ context.Context, sessionID string, _ work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	s.calls = append(s.calls, "submit-session-work")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.workSubmitResult, nil
}

func (s *stubFactoryCoordinator) MoveWorkForSession(_ context.Context, sessionID, _, _, _ string) (work.OperatorMoveResult, error) {
	s.calls = append(s.calls, "move-session-work")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.moveResult, nil
}

func (s *stubFactoryCoordinator) SubscribeFactoryEventsForSession(_ context.Context, sessionID string, _ *interfaces.FactoryEventReconnectCursor) (*interfaces.FactoryEventStream, error) {
	s.calls = append(s.calls, "subscribe-session-events")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.eventStream, nil
}

func (s *stubFactoryCoordinator) GetEngineStateSnapshotForSession(_ context.Context, sessionID string) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	s.calls = append(s.calls, "snapshot-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.engineSnapshot, nil
}

func (s *stubFactoryCoordinator) GetCurrentFactoryForSession(_ context.Context, sessionID string) (factoryapi.Factory, error) {
	s.calls = append(s.calls, "current-factory-session")
	s.sessionIDs = append(s.sessionIDs, sessionID)
	return s.currentFactory, nil
}

func (s *stubFactoryCoordinator) startDefaultRuntime(context.Context, context.Context, bool) (*liveRuntimeHandle, error) {
	s.calls = append(s.calls, "start-default-runtime")
	return &liveRuntimeHandle{}, nil
}

func (s *stubFactoryCoordinator) startBackgroundSessionWithMetadata(context.Context, string, *factoryRuntimeBundle, FactorySessionTarget) error {
	s.calls = append(s.calls, "start-background-session")
	return nil
}

func (s *stubFactoryCoordinator) startLiveRuntimeSidecars(context.Context, *liveRuntimeHandle) error {
	s.calls = append(s.calls, "start-sidecars")
	return nil
}

func (s *stubFactoryCoordinator) stopLiveRuntimeSidecars(*liveRuntimeHandle) {
	s.calls = append(s.calls, "stop-sidecars")
}

func (s *stubFactoryCoordinator) stopLiveRuntime(*liveRuntimeHandle) error {
	s.calls = append(s.calls, "stop-runtime")
	return nil
}

func (s *stubFactoryCoordinator) shutdownOtherLiveSessions(*liveRuntimeHandle) error {
	s.calls = append(s.calls, "shutdown-other-sessions")
	return nil
}

func (s *stubFactoryCoordinator) replaceSessionRuntime(context.Context, *factorysessions.LiveSession, string, *factoryRuntimeBundle) error {
	s.calls = append(s.calls, "replace-session-runtime")
	return nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity this delegation test keeps the public model facade call sequence and payload assertions together on one compatibility seam.
func TestFactoryService_ModelMethodsDelegateToModelService(t *testing.T) {
	t.Parallel()

	stub := &stubModelService{
		listResult:   factoryapi.ListModelsResponse{Results: []factoryapi.ModelSummary{{Name: "catalog-model"}}},
		gotModel:     factoryapi.ModelDetail{Name: "detail-model"},
		pullResult:   apisurface.ModelPullResult{ModelName: "pull-model"},
		invokeResult: apisurface.ModelInvocationResult{ModelName: "invoke-model"},
	}
	svc := &FactoryService{modelService: stub}

	listed, err := svc.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	got, err := svc.GetModel(context.Background(), "detail-model")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	pulled, err := svc.PullModel(context.Background(), "pull-model")
	if err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	invoked, err := svc.InvokeModel(context.Background(), "invoke-model", factoryapi.ModelInvocationRequest{Operation: "TTS"})
	if err != nil {
		t.Fatalf("InvokeModel: %v", err)
	}

	if len(listed.Results) != 1 || listed.Results[0].Name != "catalog-model" {
		t.Fatalf("ListModels result = %#v, want delegated catalog-model", listed)
	}
	if got.Name != "detail-model" {
		t.Fatalf("GetModel result = %#v, want delegated detail-model", got)
	}
	if pulled.ModelName != "pull-model" {
		t.Fatalf("PullModel result = %#v, want delegated pull-model", pulled)
	}
	if invoked.ModelName != "invoke-model" {
		t.Fatalf("InvokeModel result = %#v, want delegated invoke-model", invoked)
	}
	if strings.Join(stub.calls, ",") != "list,get,pull,invoke" {
		t.Fatalf("model service calls = %#v, want list,get,pull,invoke", stub.calls)
	}
	if len(stub.modelNames) != 3 || stub.modelNames[0] != "detail-model" || stub.modelNames[1] != "pull-model" || stub.modelNames[2] != "invoke-model" {
		t.Fatalf("model names = %#v, want delegated model-name sequence", stub.modelNames)
	}
	if len(stub.requests) != 1 || stub.requests[0].Operation != "TTS" {
		t.Fatalf("invoke requests = %#v, want delegated TTS request", stub.requests)
	}
}

func TestFactoryService_ModelMethodsForwardContextResultsAndErrorsUnchanged(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "catalog-request")
	listErr := errors.New("list sentinel")
	getErr := fmt.Errorf("requested model: %w", apisurface.ErrModelNotFound)
	pullErr := &apisurface.ManagedRuntimePullError{
		Result: apisurface.ModelPullResult{ModelName: "pull-result", ManagedPullOutcome: "TIMED_OUT"},
		Cause:  errors.New("pull sentinel"),
	}
	stub := &stubModelService{
		listResult: factoryapi.ListModelsResponse{Results: []factoryapi.ModelSummary{{Name: "list-result"}}},
		listErr:    listErr,
		gotModel:   factoryapi.ModelDetail{Name: "detail-result"},
		getErr:     getErr,
		pullResult: apisurface.ModelPullResult{ModelName: "pull-result", ManagedPullOutcome: "TIMED_OUT"},
		pullErr:    pullErr,
	}
	svc := &FactoryService{modelService: stub}

	listed, gotListErr := svc.ListModels(ctx)
	detail, gotGetErr := svc.GetModel(ctx, "requested-model")
	pulled, gotPullErr := svc.PullModel(ctx, "pull-model")

	if !reflect.DeepEqual(listed, stub.listResult) || gotListErr != listErr {
		t.Fatalf("ListModels = (%#v, %v), want exact result and sentinel error", listed, gotListErr)
	}
	if detail.Name != "detail-result" || gotGetErr != getErr {
		t.Fatalf("GetModel = (%#v, %v), want exact result and sentinel error", detail, gotGetErr)
	}
	if pulled.ModelName != "pull-result" || pulled.ManagedPullOutcome != "TIMED_OUT" || gotPullErr != pullErr {
		t.Fatalf("PullModel = (%#v, %v), want exact result and sentinel error", pulled, gotPullErr)
	}
	if !errors.Is(gotGetErr, apisurface.ErrModelNotFound) || !apisurface.IsManagedRuntimePullError(gotPullErr) {
		t.Fatalf("typed errors = (%v, %v), want model-not-found and managed-runtime-pull errors", gotGetErr, gotPullErr)
	}
	assertModelCatalogCallsForwardedOnce(t, stub, ctx)
}

func assertModelCatalogCallsForwardedOnce(t *testing.T, stub *stubModelService, ctx context.Context) {
	t.Helper()
	if len(stub.contexts) != 3 || stub.contexts[0] != ctx || stub.contexts[1] != ctx || stub.contexts[2] != ctx {
		t.Fatalf("model contexts = %#v, want original context three times", stub.contexts)
	}
	if len(stub.modelNames) != 2 || stub.modelNames[0] != "requested-model" || stub.modelNames[1] != "pull-model" {
		t.Fatalf("model names = %#v, want requested-model then pull-model", stub.modelNames)
	}
	if !reflect.DeepEqual(stub.calls, []string{"list", "get", "pull"}) {
		t.Fatalf("model calls = %#v, want each operation exactly once", stub.calls)
	}
}

func TestFactoryService_InvokeModelForwardsContextRequestResultAndErrorUnchanged(t *testing.T) {
	t.Parallel()

	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("request"), "invoke-request")
	invokeErr := &apisurface.ManagedRuntimeInvocationError{
		Identity:       "invoke-model",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
		Cause:          apisurface.ErrManagedRuntimeMissing,
	}
	request := factoryapi.ModelInvocationRequest{Operation: "TTS"}
	stub := &stubModelService{
		invokeResult: apisurface.ModelInvocationResult{ModelName: "invoke-result", Operation: "TTS"},
		invokeErr:    invokeErr,
	}

	result, err := (&FactoryService{modelService: stub}).InvokeModel(ctx, "invoke-model", request)
	if result.ModelName != "invoke-result" || result.Operation != "TTS" || err != invokeErr {
		t.Fatalf("InvokeModel = (%#v, %v), want exact result and sentinel error", result, err)
	}
	if !apisurface.IsManagedRuntimeMissing(err) {
		t.Fatalf("InvokeModel error = %v, want typed unavailable-runtime error", err)
	}
	if len(stub.contexts) != 1 || stub.contexts[0] != ctx || len(stub.modelNames) != 1 || stub.modelNames[0] != "invoke-model" {
		t.Fatalf("forwarded context/model = (%#v, %#v), want original context and invoke-model", stub.contexts, stub.modelNames)
	}
	if len(stub.requests) != 1 || !reflect.DeepEqual(stub.requests[0], request) {
		t.Fatalf("invoke requests = %#v, want exact request", stub.requests)
	}
	if !reflect.DeepEqual(stub.calls, []string{"invoke"}) {
		t.Fatalf("model calls = %#v, want invoke exactly once", stub.calls)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this delegation test keeps the session-lifecycle facade sequence and collaborator assertions together on one compatibility seam.
func TestFactoryService_LifecycleMethodsDelegateToCoordinator(t *testing.T) {
	t.Parallel()

	stub := &stubFactoryCoordinator{
		workSubmitResult: work.WorkRequestSubmitResult{RequestID: "request-1"},
		moveResult:       work.OperatorMoveResult{WorkID: "move-1"},
		eventStream:      &interfaces.FactoryEventStream{},
		engineSnapshot:   &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{RuntimeStatus: interfaces.RuntimeStatusIdle},
	}
	gatewayStub := &stubSessionGateway{
		listSessionsResult: []factorysessions.ReadProjection{{
			Context: factorysessions.ProjectionContext{Session: &factorysessions.LiveSession{ID: "session-a"}},
		}},
	}
	definitions := &recordingFactoryDefinitions{
		sessionFactory: factoryapi.Factory{Name: "beta"},
	}
	svc := &FactoryService{coordinator: stub, definitions: definitions, sessionGateway: gatewayStub}

	listed, err := svc.ListFactorySessions(context.Background())
	if err != nil {
		t.Fatalf("ListFactorySessions: %v", err)
	}
	folderPath := "/tmp/factory"
	if _, err := svc.OpenFactorySession(context.Background(), factoryapi.OpenFactorySessionRequest{FolderPath: folderPath}); err != nil {
		t.Fatalf("OpenFactorySession: %v", err)
	}
	if _, err := svc.OpenFactorySessionFromFolder(context.Background(), folderPath, nil, false, false); err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}
	if err := svc.CloseFactorySession(context.Background(), "session-a"); err != nil {
		t.Fatalf("CloseFactorySession: %v", err)
	}
	if _, err := svc.PauseLiveFactorySession(context.Background(), "session-a", factoryapi.FactorySessionLifecycleControlRequest{}); err != nil {
		t.Fatalf("PauseLiveFactorySession: %v", err)
	}
	if _, err := svc.ResumeLiveFactorySession(context.Background(), "session-a", factoryapi.FactorySessionLifecycleControlRequest{}); err != nil {
		t.Fatalf("ResumeLiveFactorySession: %v", err)
	}
	if _, err := svc.PauseDurableFactorySession(context.Background(), "dur-sess-a", factoryapi.FactorySessionLifecycleControlRequest{}); err != nil {
		t.Fatalf("PauseDurableFactorySession: %v", err)
	}
	if _, err := svc.CancelDurableFactorySession(context.Background(), "dur-sess-a", factoryapi.FactorySessionLifecycleControlRequest{}); err != nil {
		t.Fatalf("CancelDurableFactorySession: %v", err)
	}
	if _, err := svc.GetFactorySessionSyncPreflight(context.Background(), "session-a", interfaces.FactorySessionSyncPreflightOptions{}); err != nil {
		t.Fatalf("GetFactorySessionSyncPreflight: %v", err)
	}
	if _, err := svc.GetFactorySessionResult(context.Background(), "session-a"); err != nil {
		t.Fatalf("GetFactorySessionResult: %v", err)
	}
	if _, err := svc.GetFactorySessionPartialResult(context.Background(), "session-a"); err != nil {
		t.Fatalf("GetFactorySessionPartialResult: %v", err)
	}
	if _, err := svc.SubscribeSessionResponseStream("session-a", "dispatch-1", 0); err != nil {
		t.Fatalf("SubscribeSessionResponseStream: %v", err)
	}
	if err := svc.ActivateNamedFactory(context.Background(), "gamma"); err != nil {
		t.Fatalf("ActivateNamedFactory: %v", err)
	}
	current, err := svc.GetCurrentFactoryForSession(context.Background(), "session-a")
	if err != nil {
		t.Fatalf("GetCurrentFactoryForSession: %v", err)
	}
	if _, err := svc.SubmitWorkRequestForSession(context.Background(), "session-a", work.WorkRequest{}); err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if _, err := svc.MoveWorkForSession(context.Background(), "session-a", "work-1", "done", "request-2"); err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if _, err := svc.SubscribeFactoryEventsForSession(context.Background(), "session-a", nil); err != nil {
		t.Fatalf("SubscribeFactoryEventsForSession: %v", err)
	}
	snapshot, err := svc.GetEngineStateSnapshotForSession(context.Background(), "session-a")
	if err != nil {
		t.Fatalf("GetEngineStateSnapshotForSession: %v", err)
	}

	if len(listed.Sessions) != 1 || listed.Sessions[0].Id != "session-a" {
		t.Fatalf("ListFactorySessions result = %#v, want delegated session summary", listed)
	}
	if current.Name != "beta" {
		t.Fatalf("GetCurrentFactoryForSession result = %#v, want delegated beta factory", current)
	}
	if snapshot == nil || snapshot.RuntimeStatus != interfaces.RuntimeStatusIdle {
		t.Fatalf("GetEngineStateSnapshotForSession result = %#v, want delegated idle snapshot", snapshot)
	}
	if strings.Join(stub.calls, ",") != "get-session-sync-preflight,activate,submit-session-work,move-session-work,subscribe-session-events,snapshot-session" {
		t.Fatalf("coordinator calls = %#v, want delegated lifecycle sequence without open, read, or close methods", stub.calls)
	}
	if strings.Join(gatewayStub.calls, ",") != "list-sessions,open-session,open-session-from-folder,close-session,pause-session,resume-session,pause-durable-session,cancel-durable-session,get-session-result,get-session-partial-result,subscribe-response-stream" {
		t.Fatalf("session gateway calls = %#v, want delegated read, open, lifecycle, preflight, result, and stream sequence", gatewayStub.calls)
	}
	if len(stub.runtimeNames) != 1 || stub.runtimeNames[0] != "gamma" {
		t.Fatalf("activation targets = %#v, want gamma", stub.runtimeNames)
	}
	if definitions.sessionCalls != 1 || definitions.sessionID != "session-a" {
		t.Fatalf("definition collaborator session calls = %d sessionID = %q, want 1 and session-a", definitions.sessionCalls, definitions.sessionID)
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
	if svc.coordinatorPolicy().dir != alphaDir {
		t.Fatalf("service dir = %q, want %q", svc.coordinatorPolicy().dir, alphaDir)
	}
	if svc.sessions.Count() != 1 {
		t.Fatalf("sessions.Count() = %d before Run, want seeded default session", svc.sessions.Count())
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
	if svc.sessions.Count() != 1 {
		t.Fatalf("sessions.Count() = %d before Run, want seeded default session", svc.sessions.Count())
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
	if defaultHandle == nil || defaultHandle.Bundle == nil {
		t.Fatal("default session live handle is required after Run")
	}
	if got := cleanResolvedPath(defaultHandle.Bundle.Dir); got != cleanResolvedPath(alphaDir) {
		t.Fatalf("default live handle runtime dir = %q, want %q", defaultHandle.Bundle.Dir, alphaDir)
	}

	runState := svc.currentRunState()
	if runState == nil {
		t.Fatal("runState = nil after Run, want default session run state")
	}
	if runState.sessionID != defaultFactorySessionID {
		t.Fatalf("runState.sessionID = %q, want %q", runState.sessionID, defaultFactorySessionID)
	}
	if current := svc.currentSession(); current == nil || current.ID != defaultFactorySessionID {
		t.Fatalf("currentSession = %#v, want selected %q", current, defaultFactorySessionID)
	}
	if bundle := svc.currentRuntimeBundle(); bundle != defaultHandle.Bundle {
		t.Fatal("currentRuntimeBundle should resolve through the default session registry handle after Run")
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

	if snap.FactoryState != string(interfaces.FactoryStateIdle) {
		t.Errorf("expected FactoryState=IDLE, got %s", snap.FactoryState)
	}
	if snap.RuntimeStatus != interfaces.RuntimeStatusIdle {
		t.Errorf("expected RuntimeStatus=IDLE, got %s", snap.RuntimeStatus)
	}
	if snap.TickCount != 0 {
		t.Errorf("expected TickCount=0, got %d", snap.TickCount)
	}
	if snap.Topology == nil {
		t.Fatal("expected aggregate snapshot topology")
	}
	if _, ok := snap.Topology.WorkTypes["task"]; !ok {
		t.Fatalf("expected topology to include task work type, got %#v", snap.Topology.WorkTypes)
	}
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
	svc := &FactoryService{}
	bindServiceStartupRuntime(svc, &factoryRuntimeBundle{Factory: mock})

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

func TestFactoryService_GetEngineStateSnapshot_ReportsIdleActiveAndFinishedStates(t *testing.T) {
	svc, releaseCh := buildServiceModeSnapshotFixture(t)
	runCtx, cancelRun := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRun()

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Run(runCtx)
	}()

	waitForSnapshotMatch(t, svc, time.Second, "idle startup", func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snap.RuntimeStatus == interfaces.RuntimeStatusIdle
	})
	submitSnapshotStatusWork(t, svc)
	waitForSnapshotMatch(t, svc, time.Second, "active work", func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snap.RuntimeStatus == interfaces.RuntimeStatusActive && snap.InFlightCount > 0
	})

	close(releaseCh)
	waitForSnapshotMatch(t, svc, time.Second, "idle after completion", func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshotHasCompletedTaskToken(snap)
	})

	cancelRun()
	if err := <-errCh; err != nil {
		t.Fatalf("service-mode run error: %v", err)
	}

	batchSvc := buildBatchSnapshotFixture(t)
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

func buildServiceModeSnapshotFixture(t *testing.T) (*FactoryService, chan struct{}) {
	t.Helper()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	writeWorkerAgentsMD(t, dir, "worker-a")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	releaseCh := make(chan struct{})
	provider := &blockingInferenceProvider{releaseCh: releaseCh, content: "COMPLETE"}
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:              dir,
		RuntimeMode:      interfaces.RuntimeModeService,
		Logger:           zap.NewNop(),
		ProviderOverride: provider,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	return svc, releaseCh
}

func buildBatchSnapshotFixture(t *testing.T) *FactoryService {
	t.Helper()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName), 0o755); err != nil {
		t.Fatalf("create batch inputs dir: %v", err)
	}
	workFile := filepath.Join(dir, interfaces.InputsDir, "task", interfaces.DefaultChannelName, "seed.json")
	if err := os.WriteFile(workFile, []byte(`{"title":"terminal-status"}`), 0o644); err != nil {
		t.Fatalf("write seed work file: %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService batch: %v", err)
	}
	return svc
}

func submitSnapshotStatusWork(t *testing.T, svc *FactoryService) {
	t.Helper()

	if err := submitWorkRequestsToService(context.Background(), svc, []work.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-engine-state-statuses",
		Payload:    json.RawMessage(`{"title":"runtime-statuses"}`),
	}}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
}

func waitForSnapshotMatch(
	t *testing.T,
	svc *FactoryService,
	timeout time.Duration,
	phase string,
	match func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool,
) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
	for time.Now().Before(deadline) {
		snap, err := svc.GetEngineStateSnapshot(context.Background())
		if err != nil {
			if strings.Contains(err.Error(), "runtime is not available") {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			t.Fatalf("GetEngineStateSnapshot during %s: %v", phase, err)
		}
		last = snap
		if match(snap) {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last == nil {
		t.Fatalf("timed out waiting for %s snapshot", phase)
	}
	t.Fatalf("timed out waiting for %s, last status=%q inflight=%d tokens=%d", phase, last.RuntimeStatus, last.InFlightCount, len(last.Marking.Tokens))
	return nil
}

func snapshotHasCompletedTaskToken(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
	if snap.RuntimeStatus != interfaces.RuntimeStatusIdle || len(snap.Marking.Tokens) != 1 {
		return false
	}
	for _, token := range snap.Marking.Tokens {
		return token.PlaceID == "task:complete"
	}
	return false
}

type dispatchMetricsWorkerExecutor struct{}

func (dispatchMetricsWorkerExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	result := workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
		Metrics:      workerexecution.WorkMetrics{Duration: 250 * time.Millisecond, RetryCount: 2, Cost: 1.25},
	}
	if len(dispatch.Execution.WorkIDs) == 0 {
		return result, nil
	}
	switch dispatch.Execution.WorkIDs[0] {
	case "work-dispatch-rejected":
		result.Outcome = workerexecution.OutcomeRejected
		result.Feedback = "needs changes"
		result.Metrics = workerexecution.WorkMetrics{Duration: 125 * time.Millisecond, RetryCount: 1}
	case "work-dispatch-failed":
		result.Outcome = workerexecution.OutcomeFailed
		result.Error = "worker crashed"
		result.Metrics = workerexecution.WorkMetrics{Duration: 500 * time.Millisecond, RetryCount: 3}
	}
	return result, nil
}

func runtimeMetricMatchesWork(record map[string]any, metricName, workID, traceID, outcome string) bool {
	if strings.TrimSpace(metricRecordString(record, "metric_name")) != metricName ||
		metricRecordString(record, "work_id") != workID {
		return false
	}
	if traceID != "" && metricRecordString(record, "trace_id") != traceID {
		return false
	}
	if outcome == "" {
		return true
	}
	return metricRecordString(record, "outcome") == outcome
}

func runtimeMetricMatchesValue(record map[string]any, metricName, workID, outcome string, value float64, unit string) bool {
	if !runtimeMetricMatchesWork(record, metricName, workID, "", outcome) {
		return false
	}
	got, ok := record["value"].(float64)
	return ok && got == value && metricRecordString(record, "unit") == unit
}

type providerMetricsWorkerExecutor struct{}

func (providerMetricsWorkerExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	if len(dispatch.Execution.WorkIDs) > 0 && dispatch.Execution.WorkIDs[0] == "work-provider-failed" {
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        "provider 500",
			FailureMetadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyRetryable,
				Type:   workerexecution.WorkFailureTypeInternalServerError,
			},
			Diagnostics: &workerexecution.WorkDiagnostics{
				Provider: &workerexecution.ProviderDiagnostic{
					Provider: string(modelprovider.Codex),
					Model:    "gpt-5-codex",
					ResponseMetadata: map[string]string{
						"duration_api_ms": "125",
					},
				},
				Command: &workerexecution.CommandDiagnostic{
					Stdout: "provider stdout secret",
					Stderr: "provider stderr secret",
				},
			},
		}, nil
	}
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "provider done",
		Metrics:      workerexecution.WorkMetrics{Duration: 300 * time.Millisecond, Cost: 2.75},
		Diagnostics: &workerexecution.WorkDiagnostics{
			Provider: &workerexecution.ProviderDiagnostic{
				Provider: string(modelprovider.Codex),
				Model:    "gpt-5-codex",
				ResponseMetadata: map[string]string{
					"duration_api_ms": "240",
					"input_tokens":    "13",
					"output_tokens":   "7",
				},
			},
			Command: &workerexecution.CommandDiagnostic{
				Stdout: "provider stdout secret",
				Stderr: "provider stderr secret",
			},
		},
	}, nil
}

type scriptMetricsWorkerExecutor struct{}

func (scriptMetricsWorkerExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        "script timed out",
		Metrics:      workerexecution.WorkMetrics{Duration: 900 * time.Millisecond},
		FailureMetadata: &workerexecution.WorkFailureMetadata{
			Family: workerexecution.WorkFailureFamilyRetryable,
			Type:   workerexecution.WorkFailureTypeTimeout,
		},
		Diagnostics: &workerexecution.WorkDiagnostics{
			Command: &workerexecution.CommandDiagnostic{
				Duration: 875 * time.Millisecond,
				TimedOut: true,
				Stdout:   "script stdout secret",
				Stderr:   "script stderr secret",
			},
		},
	}, nil
}

func runtimeMetricMatchesProvider(record map[string]any, metricName, workID, provider, outcome string) bool {
	if !runtimeMetricMatchesWork(record, metricName, workID, "", outcome) {
		return false
	}
	return metricRecordString(record, "provider") == provider
}

func runtimeMetricMatchesProviderValue(record map[string]any, metricName, workID string, value float64, unit string) bool {
	if !runtimeMetricMatchesProvider(record, metricName, workID, string(modelprovider.Codex), string(workerexecution.OutcomeAccepted)) {
		return false
	}
	got, ok := record["value"].(float64)
	return ok && got == value && metricRecordString(record, "unit") == unit
}

func assertRuntimeLogContainsMetricsSinkWarning(t *testing.T, logPath string) {
	t.Helper()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %q: %v", logPath, err)
	}
	records := parseRuntimeLogRecords(t, string(data))
	for _, record := range records {
		if strings.TrimSpace(metricRecordString(record, "msg")) != "runtime metrics sink unavailable; continuing without metrics" {
			continue
		}
		if !strings.Contains(strings.TrimSpace(metricRecordString(record, "error")), "build runtime metrics sink") {
			t.Fatalf("runtime metrics warning error = %q, want wrapped sink build failure", metricRecordString(record, "error"))
		}
		return
	}
	t.Fatalf("runtime log did not contain degraded metrics warning:\n%s", string(data))
}

func TestBuildFactoryService_AppliesOperatorDefaultsToOmittedModelWorkerFields(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "executor",
			"type": "MODEL_WORKER",
			"body": "You are the executor.",
		}},
		"workstations": []map[string]any{{
			"name":      "execute-task",
			"worker":    "executor",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Implement {{ .WorkID }}.",
		}},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir: dir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider: "CODEX",
			WorkerModel:         "gpt-5-codex",
		},
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	runtimeCfg := svc.currentRuntimeConfig()
	if runtimeCfg == nil {
		t.Fatal("expected current runtime config")
	}
	worker, ok := runtimeCfg.Worker("executor")
	if !ok {
		t.Fatal("expected executor worker")
	}
	if worker.ModelProvider != string(modelprovider.Codex) {
		t.Fatalf("modelProvider = %q, want %q", worker.ModelProvider, modelprovider.Codex)
	}
	if worker.Model != "gpt-5-codex" {
		t.Fatalf("model = %q, want gpt-5-codex", worker.Model)
	}
}

func TestBuildFactoryService_PreservesAuthoredModelWorkerFieldsOverOperatorDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "executor",
			"type":          "MODEL_WORKER",
			"modelProvider": "CLAUDE",
			"model":         "claude-sonnet-4-20250514",
			"body":          "You are the executor.",
		}},
		"workstations": []map[string]any{{
			"name":      "execute-task",
			"worker":    "executor",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Implement {{ .WorkID }}.",
		}},
	})

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir: dir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider: "CODEX",
			WorkerModel:         "gpt-5-codex",
		},
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	worker, ok := svc.currentRuntimeConfig().Worker("executor")
	if !ok {
		t.Fatal("expected executor worker")
	}
	if worker.ModelProvider != string(modelprovider.Claude) {
		t.Fatalf("modelProvider = %q, want %q", worker.ModelProvider, modelprovider.Claude)
	}
	if worker.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("model = %q, want authored model", worker.Model)
	}
}

func TestBuildReplacementFactoryRuntime_AppliesOperatorDefaults(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir: rootDir,
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider: "CODEX",
			WorkerModel:         "gpt-5-codex",
		},
		RuntimeMode:                             interfaces.RuntimeModeService,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
		Logger:                                  zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	alphaWorker, ok := svc.currentRuntimeConfig().Worker("executor")
	if !ok {
		t.Fatal("expected alpha executor worker")
	}
	if alphaWorker.ModelProvider != string(modelprovider.Codex) {
		t.Fatalf("alpha modelProvider = %q, want codex", alphaWorker.ModelProvider)
	}

	replacement, err := svc.buildReplacementFactoryRuntime(context.Background(), rootDir, betaDir, defaultFactorySessionID)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if replacement.Dir != betaDir {
		t.Fatalf("replacement dir = %q, want %q", replacement.Dir, betaDir)
	}
	betaWorker, ok := replacement.RuntimeCfg.Worker("executor")
	if !ok {
		t.Fatal("expected beta executor worker")
	}
	if betaWorker.ModelProvider != string(modelprovider.Codex) {
		t.Fatalf("beta modelProvider = %q, want codex", betaWorker.ModelProvider)
	}
	if betaWorker.Model != "gpt-5-codex" {
		t.Fatalf("beta model = %q, want gpt-5-codex", betaWorker.Model)
	}
	_ = alphaDir
}

func TestGeneratedFactoryFromRuntimeConfig_CapturesOperatorDefaultedModelWorkerFields(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "executor",
			"type": "MODEL_WORKER",
			"body": "You are the executor.",
		}},
		"workstations": []map[string]any{{
			"name":      "execute-task",
			"worker":    "executor",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Implement {{ .WorkID }}.",
		}},
	})

	loaded, err := config.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if err := operatordefaultsruntime.ApplyToLoadedConfig(loaded, operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CODEX",
		WorkerModel:         "gpt-5-codex",
	}); err != nil {
		t.Fatalf("ApplyToLoadedConfig: %v", err)
	}

	generated, err := generatedFactoryFromRuntimeConfigForTest(dir, loaded.FactoryConfig(), loaded)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("generated workers = %#v, want one worker", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.ModelProvider == nil || string(*worker.ModelProvider) != "CODEX" {
		t.Fatalf("generated modelProvider = %#v, want CODEX", worker.ModelProvider)
	}
	if worker.Model == nil || *worker.Model != "gpt-5-codex" {
		t.Fatalf("generated model = %#v, want gpt-5-codex", worker.Model)
	}
}

func TestModelOwnedLocalDomain_WiresProcessWideModelHost(t *testing.T) {
	domain, err := modelhost.NewLocalDomain(LocalModelDomainDependencies(&FactoryServiceConfig{
		ModelCacheDir: t.TempDir(),
	}))
	if err != nil {
		t.Fatalf("modelhost.NewLocalDomain: %v", err)
	}
	if domain.Host == nil {
		t.Fatal("local model domain host = nil, want process-wide modelhost.Host")
	}
	if _, ok := domain.Host.(*modelhost.CatalogHost); !ok {
		t.Fatalf("host type = %T, want *modelhost.CatalogHost", domain.Host)
	}
}

func TestBuildFactoryService_StartupModelHostMatchesRuntimeBundle(t *testing.T) {
	rootDir := t.TempDir()
	writeNamedFactoryFixture(t, rootDir, "alpha")
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
	startupHost := svc.core.ModelHost()
	if startupHost == nil {
		t.Fatal("startup model host = nil")
	}
	if svc.startupBundle != nil && svc.startupBundle.ModelHost != startupHost {
		t.Fatal("startup bundle model host does not match service collaborator host")
	}
}

func TestFactoryService_PausedSessionBufferedSubmission_DoesNotAffectOtherSessions(t *testing.T) {
	harness := startRunningSessionService(t, runningSessionServiceOptions{
		defaultFactory: "alpha",
		namedFactories: []string{"alpha", "beta"},
	})
	defer harness.stop(t)

	betaSessionID := harness.openFactorySession(t, "beta")
	alphaSession := harness.requireSession(t, defaultFactorySessionID)
	betaSession := harness.requireSession(t, betaSessionID)

	pauseSessionFactory(t, betaSession)
	waitForSessionFactoryState(t, harness.svc, betaSession.ID, interfaces.FactoryStatePaused, time.Second, "beta session paused")
	waitForSessionFactoryState(t, harness.svc, alphaSession.ID, interfaces.FactoryStateRunning, time.Second, "alpha session still running")

	submitSessionWork(t, betaSession, "beta-paused-submit-work", "trace-beta-paused-submit")
	submitSessionWork(t, alphaSession, "alpha-running-submit-work", "trace-alpha-running-submit")

	waitForSessionEventsToContain(t, alphaSession, "alpha-running-submit-work", time.Second)
	assertSessionEventsDoNotContain(t, betaSession, "beta-paused-submit-work")

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		betaSnap := sessionEngineSnapshot(t, betaSession)
		if betaSnap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("beta factory state = %q, want PAUSED", betaSnap.FactoryState)
		}
		if snapshotHasTokenAtPlace(betaSnap, "task:complete") || snapshotHasTokenAtPlace(betaSnap, "task:init") {
			t.Fatalf("paused beta submission applied to marking = %#v", betaSnap.Marking.Tokens)
		}
		if betaSnap.InFlightCount > 0 || len(betaSnap.Dispatches) > 0 {
			t.Fatalf("beta dispatch started while paused inFlight=%d dispatches=%d", betaSnap.InFlightCount, len(betaSnap.Dispatches))
		}
		time.Sleep(20 * time.Millisecond)
	}

	resumeSessionFactory(t, betaSession)
	waitForSessionFactoryState(t, harness.svc, betaSession.ID, interfaces.FactoryStateRunning, time.Second, "beta session resumed")
	waitForSessionEventsToContain(t, betaSession, "beta-paused-submit-work", time.Second)
	assertSessionEventsDoNotContain(t, betaSession, "alpha-running-submit-work")
}

func TestFactoryService_PausedSessionBufferedWorkerResult_DoesNotAffectOtherSessions(t *testing.T) {
	blocking := &prefixBlockingExecutor{
		blockPrefix: "beta-blocked-",
		started:     make(chan struct{}),
		release:     make(chan struct{}),
	}

	secondDir := t.TempDir()
	writeFactoryJSON(t, secondDir, minimalFactoryConfig())

	harness := startRunningSessionService(t, runningSessionServiceOptions{
		rootConfig: minimalFactoryConfig(),
		extraOptions: []factory.FactoryOption{
			factory.WithWorkerExecutor("worker-a", blocking),
		},
	})
	svc := harness.svc
	openResult, err := svc.OpenFactorySessionFromFolder(context.Background(), secondDir, nil, false, false)
	if err != nil {
		t.Fatalf("OpenFactorySessionFromFolder: %v", err)
	}

	alphaSession := requireLiveSession(t, svc, defaultFactorySessionID)
	betaSession := requireLiveSession(t, svc, openResult.SessionID)

	submitSessionWork(t, betaSession, "beta-blocked-result-work", "trace-beta-blocked-result")
	waitForSessionInFlight(t, betaSession, time.Second)

	pauseSessionFactory(t, betaSession)
	waitForSessionFactoryState(t, svc, betaSession.ID, interfaces.FactoryStatePaused, time.Second, "beta session paused")
	close(blocking.release)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		betaSnap := sessionEngineSnapshot(t, betaSession)
		if betaSnap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("beta factory state = %q, want PAUSED", betaSnap.FactoryState)
		}
		if snapshotHasTokenAtPlace(betaSnap, "task:complete") {
			t.Fatalf("beta worker result applied while paused")
		}
		if betaSnap.InFlightCount == 0 {
			t.Fatalf("beta dispatch completed while paused")
		}
		time.Sleep(20 * time.Millisecond)
	}

	submitSessionWork(t, alphaSession, "alpha-running-result-work", "trace-alpha-running-result")
	waitForSessionEventsToContain(t, alphaSession, "alpha-running-result-work", time.Second)
	assertSessionEventsDoNotContain(t, betaSession, "alpha-running-result-work")
	betaSnap := sessionEngineSnapshot(t, betaSession)
	if snapshotHasTokenAtPlace(betaSnap, "task:complete") {
		t.Fatalf("beta worker result applied while alpha session processed normally")
	}

	resumeSessionFactory(t, betaSession)
	waitForSessionFactoryState(t, svc, betaSession.ID, interfaces.FactoryStateRunning, time.Second, "beta session resumed")
	waitForSessionEventsToContain(t, betaSession, "beta-blocked-result-work", time.Second)
	assertSessionEventsDoNotContain(t, betaSession, "alpha-running-result-work")
}
