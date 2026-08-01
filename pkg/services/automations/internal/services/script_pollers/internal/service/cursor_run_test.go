package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRunScriptPoller_CommitsCursorAfterSuccessfulAdvance(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	stdout := []byte(`{
		"requestId":"linear-issue-batch-cursor",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-cursor","workTypeName":"task","payload":{"id":"ISSUE-CURSOR"}}],
		"cursor":"opaque-cursor-2",
		"checkpoint":"checkpoint-2"
	}`)
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: workers.CommandResult{Stdout: stdout}}},
	}
	submitted := &recordingSubmitter{}
	recorder := newMemoryCursorRecorder()
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner:         runner,
		cursorRecorder: recorder,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)
	supervision := scriptPollerSupervision{
		automationID: "workflow-cursor",
		sourceID:     "source-cursor",
		instanceID:   "instance-cursor-1",
	}

	err := svc.runScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		supervision,
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller error = %v, want unexpected exit after successful submit", err)
	}
	if submitted.calls != 1 {
		t.Fatalf("submit calls = %d, want 1", submitted.calls)
	}

	cursor, err := svc.GetCursor(context.Background(), automations.GetCursorRequest{
		InstanceID: supervision.instanceID,
	})
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	if cursor.AutomationID != supervision.automationID ||
		cursor.InstanceID != supervision.instanceID ||
		cursor.Cursor != "opaque-cursor-2" ||
		cursor.Checkpoint != "checkpoint-2" {
		t.Fatalf("GetCursor() = %+v, want committed opaque recovery facts", cursor)
	}
}

func TestRunScriptPoller_ResumesWithCompatibleCursorInCommandEnv(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	recorder := newMemoryCursorRecorder()
	ctx := context.Background()
	const instanceID = "instance-resume"
	if err := recorder.CommitCursor(ctx, commitCursorRequest{
		automationID: "workflow-resume",
		instanceID:   instanceID,
		cursor:       "opaque-cursor-resume",
		checkpoint:   "checkpoint-resume",
	}); err != nil {
		t.Fatalf("CommitCursor() error = %v", err)
	}

	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: workers.CommandResult{Stdout: []byte(`{"requestId":"noop","type":"FACTORY_REQUEST_BATCH","works":[{"name":"noop","workTypeName":"task"}]}`)}}},
	}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner:         runner,
		cursorRecorder: recorder,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)
	supervision := scriptPollerSupervision{
		automationID:   "workflow-resume",
		sourceID:       "source-resume",
		instanceID:     instanceID,
		expectedCursor: "opaque-cursor-resume",
	}

	err := svc.runScriptPoller(
		ctx,
		runner,
		runtimeCfg,
		poller,
		worker,
		supervision,
		func(context.Context, work.WorkRequest) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("RunScriptPoller error = %v", err)
	}
	if runner.callCount() != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.callCount())
	}
	runner.mu.Lock()
	req := runner.reqs[0]
	runner.mu.Unlock()
	if !containsEnv(req.Env, scriptPollerCursorEnvVar+"=opaque-cursor-resume") ||
		!containsEnv(req.Env, scriptPollerCheckpointEnvVar+"=checkpoint-resume") {
		t.Fatalf("command env = %#v, want resume cursor/checkpoint injected", req.Env)
	}
}

func TestRunScriptPoller_RejectsStaleCursorWithoutSubmit(t *testing.T) {
	t.Parallel()

	recorder := newMemoryCursorRecorder()
	ctx := context.Background()
	const instanceID = "instance-stale-run"
	if err := recorder.CommitCursor(ctx, commitCursorRequest{
		automationID: "workflow-stale-run",
		instanceID:   instanceID,
		cursor:       "cursor-current",
	}); err != nil {
		t.Fatalf("CommitCursor() error = %v", err)
	}

	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: workers.CommandResult{Stdout: []byte(`{"requestId":"stale","type":"FACTORY_REQUEST_BATCH","works":[{"name":"stale","workTypeName":"task"}]}`)}}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner:         runner,
		cursorRecorder: recorder,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	err := svc.runScriptPoller(
		ctx,
		runner,
		runtimeCfg,
		poller,
		worker,
		scriptPollerSupervision{
			automationID:   "workflow-stale-run",
			instanceID:     instanceID,
			expectedCursor: "cursor-stale",
		},
		submitted.submit,
	)
	assertAutomationsConflict(t, err, getCursorOperation)
	if submitted.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 on stale cursor conflict", submitted.calls)
	}
	if runner.callCount() != 0 {
		t.Fatalf("runner calls = %d, want 0 before stale cursor rejection", runner.callCount())
	}

	cursor, err := svc.GetCursor(ctx, automations.GetCursorRequest{InstanceID: instanceID})
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	if cursor.Cursor != "cursor-current" {
		t.Fatalf("cursor after stale run = %q, want authoritative cursor-current", cursor.Cursor)
	}
}

func TestRunScriptPoller_CursorPersistFailureDoesNotReportSuccess(t *testing.T) {
	t.Parallel()

	stdout := []byte(`{
		"requestId":"linear-issue-batch-persist-fail",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-persist","workTypeName":"task"}],
		"cursor":"opaque-cursor-next"
	}`)
	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: workers.CommandResult{Stdout: stdout}}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner: runner,
		cursorRecorder: failingCursorRecorder{
			commitErr: errors.New("disk unavailable"),
		},
	})

	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	err := svc.runScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		scriptPollerSupervision{
			automationID: "workflow-persist-fail",
			instanceID:   "instance-persist-fail",
		},
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "cursor persistence failed") {
		t.Fatalf("RunScriptPoller error = %v, want cursor persistence failure", err)
	}
	if submitted.calls != 1 {
		t.Fatalf("submit calls = %d, want 1 before persistence failure surfaces", submitted.calls)
	}
}

func TestRunScriptPoller_RejectsCheckpointOnlyRecoveryBeforeSubmit(t *testing.T) {
	t.Parallel()

	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: workers.CommandResult{Stdout: []byte(`{
			"requestId":"checkpoint-only-run",
			"type":"FACTORY_REQUEST_BATCH",
			"works":[{"name":"checkpoint-only","workTypeName":"task"}],
			"checkpoint":"checkpoint-only"
		}`)}}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner:         runner,
		cursorRecorder: newMemoryCursorRecorder(),
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	err := svc.runScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		scriptPollerSupervision{automationID: "workflow-checkpoint-only", instanceID: "instance-checkpoint-only"},
		submitted.submit,
	)
	if err == nil || !strings.Contains(err.Error(), "checkpoint requires cursor") {
		t.Fatalf("RunScriptPoller error = %v, want checkpoint/cursor validation failure", err)
	}
	if submitted.calls != 0 {
		t.Fatalf("submit calls = %d, want 0 for invalid recovery facts", submitted.calls)
	}
}

type failingCursorRecorder struct {
	commitErr error
}

func (f failingCursorRecorder) GetCursor(
	_ context.Context,
	request automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	return newMemoryCursorRecorder().GetCursor(context.Background(), request)
}

func (f failingCursorRecorder) CommitCursor(context.Context, commitCursorRequest) error {
	return f.commitErr
}

func assertAutomationsConflict(t *testing.T, err error, op string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s error = nil, want conflict", op)
	}
	typed, ok := err.(*automations.Error)
	if !ok {
		t.Fatalf("%s error type = %T, want *automations.Error", op, err)
	}
	if typed.Op != op || typed.Code != automations.ErrorCodeConflict {
		t.Fatalf("%s error = %+v, want op=%q code=%q", op, typed, op, automations.ErrorCodeConflict)
	}
	if !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("%s error = %v, want errors.Is ErrConflict", op, err)
	}
}
