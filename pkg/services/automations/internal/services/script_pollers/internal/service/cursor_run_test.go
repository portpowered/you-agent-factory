package service_test

import (
	"context"
	"errors"
	cursorscopeswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes/wire"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	"github.com/portpowered/infinite-you/pkg/services/work"
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
		outcomes: []runOutcome{{result: platformprocess.CommandResult{Stdout: stdout}}},
	}
	submitted := &recordingSubmitter{}
	recorder := cursorscopeswire.NewService(nil)
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner:  runner,
		cursors: recorder,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)
	supervision := scriptpollers.ScriptPollerSupervision{
		AutomationID: "workflow-cursor",
		SourceID:     "source-cursor",
		InstanceID:   "instance-cursor-1",
	}

	err := svc.RunScriptPoller(
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
		InstanceID: supervision.InstanceID,
	})
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	if cursor.AutomationID != supervision.AutomationID ||
		cursor.InstanceID != supervision.InstanceID ||
		cursor.Cursor != "opaque-cursor-2" ||
		cursor.Checkpoint != "checkpoint-2" {
		t.Fatalf("GetCursor() = %+v, want committed opaque recovery facts", cursor)
	}
}

func TestRunScriptPoller_ResumesWithCompatibleCursorInCommandEnv(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	recorder := cursorscopeswire.NewService(nil)
	ctx := context.Background()
	const instanceID = "instance-resume"
	if err := recorder.CommitCursor(ctx, scriptpollers.CursorScope{}, scriptpollers.CommitCursorRequest{
		AutomationID: "workflow-resume",
		InstanceID:   instanceID,
		Cursor:       "opaque-cursor-resume",
		Checkpoint:   "checkpoint-resume",
	}); err != nil {
		t.Fatalf("CommitCursor() error = %v", err)
	}

	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: platformprocess.CommandResult{Stdout: []byte(`{"requestId":"noop","type":"FACTORY_REQUEST_BATCH","works":[{"name":"noop","workTypeName":"task"}]}`)}}},
	}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner:  runner,
		cursors: recorder,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)
	supervision := scriptpollers.ScriptPollerSupervision{
		AutomationID:   "workflow-resume",
		SourceID:       "source-resume",
		InstanceID:     instanceID,
		ExpectedCursor: "opaque-cursor-resume",
	}

	err := svc.RunScriptPoller(
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
	if !containsEnv(req.Env, scriptpollers.ScriptPollerCursorEnvVar+"=opaque-cursor-resume") ||
		!containsEnv(req.Env, scriptpollers.ScriptPollerCheckpointEnvVar+"=checkpoint-resume") {
		t.Fatalf("command env = %#v, want resume cursor/checkpoint injected", req.Env)
	}
}

func TestRunScriptPoller_RejectsStaleCursorWithoutSubmit(t *testing.T) {
	t.Parallel()

	recorder := cursorscopeswire.NewService(nil)
	ctx := context.Background()
	const instanceID = "instance-stale-run"
	if err := recorder.CommitCursor(ctx, scriptpollers.CursorScope{}, scriptpollers.CommitCursorRequest{
		AutomationID: "workflow-stale-run",
		InstanceID:   instanceID,
		Cursor:       "cursor-current",
	}); err != nil {
		t.Fatalf("CommitCursor() error = %v", err)
	}

	runner := &sequenceCommandRunner{
		outcomes: []runOutcome{{result: platformprocess.CommandResult{Stdout: []byte(`{"requestId":"stale","type":"FACTORY_REQUEST_BATCH","works":[{"name":"stale","workTypeName":"task"}]}`)}}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner:  runner,
		cursors: recorder,
	})
	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	err := svc.RunScriptPoller(
		ctx,
		runner,
		runtimeCfg,
		poller,
		worker,
		scriptpollers.ScriptPollerSupervision{
			AutomationID:   "workflow-stale-run",
			InstanceID:     instanceID,
			ExpectedCursor: "cursor-stale",
		},
		submitted.submit,
	)
	assertAutomationsConflict(t, err, scriptpollers.GetCursorOperation)
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
		outcomes: []runOutcome{{result: platformprocess.CommandResult{Stdout: stdout}}},
	}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{
		runner: runner,
		cursors: failingCursorRecorder{
			commitErr: errors.New("disk unavailable"),
		},
	})

	poller := newCanonicalScriptPollerWorkstation()
	worker := newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)

	err := svc.RunScriptPoller(
		context.Background(),
		runner,
		runtimeCfg,
		poller,
		worker,
		scriptpollers.ScriptPollerSupervision{
			AutomationID: "workflow-persist-fail",
			InstanceID:   "instance-persist-fail",
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

type failingCursorRecorder struct {
	commitErr error
}

func TestRunScriptPoller_FailedDurableReplacementResumesPriorCommit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	factoryDir := t.TempDir()
	supervision := scriptpollers.ScriptPollerSupervision{
		AutomationID: "durable-workflow", InstanceID: "durable-instance",
		CursorScope: scriptpollers.CursorScope{RuntimeID: "runtime-durable", BaseDir: factoryDir},
	}
	recorder := cursorscopeswire.NewService(platformfilesystem.Local{})
	if err := recorder.CommitCursor(ctx, supervision.CursorScope, scriptpollers.CommitCursorRequest{
		AutomationID: supervision.AutomationID, InstanceID: supervision.InstanceID,
		Cursor: "cursor-prior", Checkpoint: "checkpoint-prior",
	}); err != nil {
		t.Fatalf("seed prior commit: %v", err)
	}
	persistErr := errors.New("cursor destination unavailable")
	failing := cursorscopeswire.NewService(cursorRenameFailure{err: persistErr})
	runner := &sequenceCommandRunner{outcomes: []runOutcome{{result: platformprocess.CommandResult{Stdout: []byte(
		`{"requestId":"admitted-before-failure","type":"FACTORY_REQUEST_BATCH","works":[{"name":"retained","workTypeName":"task"}],"cursor":"cursor-next","checkpoint":"checkpoint-next"}`),
	}}}}
	submitted := &recordingSubmitter{}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{runner: runner, cursors: failing})
	poller, worker := newCanonicalScriptPollerWorkstation(), newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)
	err := svc.RunScriptPoller(ctx, runner, runtimeCfg, poller, worker, supervision, submitted.submit)
	var typed *automations.Error
	if !errors.As(err, &typed) || typed.Op != scriptpollers.CommitCursorOperation || typed.Code != automations.ErrorCodeFailed || !errors.Is(err, persistErr) {
		t.Fatalf("poll error = %v, want classified durable replacement failure", err)
	}
	if submitted.calls != 1 || submitted.submissions[0].RequestID != "admitted-before-failure" {
		t.Fatalf("admissions = %+v, want retained Work before failure", submitted)
	}
	got, err := svc.GetCursorForScope(ctx, supervision.CursorScope, automations.GetCursorRequest{InstanceID: supervision.InstanceID})
	if err != nil || got.Cursor != "cursor-prior" || got.Checkpoint != "checkpoint-prior" {
		t.Fatalf("cursor after failed poll = %+v, %v, want prior commit", got, err)
	}
	assertDurablePollRecovery(t, factoryDir, supervision, runner, submitted)
}

func assertDurablePollRecovery(t *testing.T, factoryDir string, supervision scriptpollers.ScriptPollerSupervision, runner *sequenceCommandRunner, submitted *recordingSubmitter) {
	t.Helper()
	ctx := context.Background()
	poller, worker := newCanonicalScriptPollerWorkstation(), newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, factoryDir, poller, worker)
	// A new scoped owner reads durable facts without prior in-memory state.
	recovered := cursorscopeswire.NewService(platformfilesystem.Local{})
	next := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{runner: runner, cursors: recovered})
	err := next.RunScriptPoller(ctx, runner, runtimeCfg, poller, worker, supervision, submitted.submit)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") || submitted.calls != 2 {
		t.Fatalf("recovery poll = %v, admissions = %d, want successful submit and terminal exit", err, submitted.calls)
	}
	if len(runner.reqs) != 2 || !containsEnv(runner.reqs[1].Env, scriptpollers.ScriptPollerCursorEnvVar+"=cursor-prior") ||
		!containsEnv(runner.reqs[1].Env, scriptpollers.ScriptPollerCheckpointEnvVar+"=checkpoint-prior") {
		t.Fatalf("recovery command = %+v, want prior committed env facts", runner.reqs)
	}
	got, err := next.GetCursorForScope(ctx, supervision.CursorScope, automations.GetCursorRequest{InstanceID: supervision.InstanceID})
	if err != nil || got.Cursor != "cursor-next" || got.Checkpoint != "checkpoint-next" {
		t.Fatalf("cursor after recovery = %+v, %v, want next commit", got, err)
	}
}

type cursorRenameFailure struct {
	platformfilesystem.Local
	err error
}

func (f cursorRenameFailure) Rename(string, string) error { return f.err }

func (f failingCursorRecorder) GetCursor(
	_ context.Context,
	_ scriptpollers.CursorScope,
	request automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	return automations.GetCursorResult{}, &automations.Error{Op: scriptpollers.GetCursorOperation, Code: automations.ErrorCodeNotFound, Err: automations.ErrNotFound}
}

func (f failingCursorRecorder) CommitCursor(context.Context, scriptpollers.CursorScope, scriptpollers.CommitCursorRequest) error {
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

func TestRunScriptPoller_SharedInstanceUsesItsOwnScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cursors := cursorscopeswire.NewService(nil)
	runner := &sequenceCommandRunner{outcomes: []runOutcome{{result: platformprocess.CommandResult{Stdout: []byte(
		`{"requestId":"scoped-work","type":"FACTORY_REQUEST_BATCH","works":[{"name":"task","workTypeName":"task"}],"cursor":"advanced","checkpoint":"advanced-checkpoint"}`),
	}}}}
	svc := newScriptPollersServiceWithOptions(scriptPollersServiceOptions{runner: runner, cursors: cursors})
	poller, worker := newCanonicalScriptPollerWorkstation(), newCanonicalScriptPollerWorker()
	runtimeCfg := newScriptPollerLoadedRuntimeConfig(t, t.TempDir(), poller, worker)
	for _, runtimeID := range []string{"", "runtime-a", "runtime-b"} {
		scope := scriptpollers.CursorScope{RuntimeID: runtimeID}
		if err := cursors.CommitCursor(ctx, scope, scriptpollers.CommitCursorRequest{
			AutomationID: "workflow", InstanceID: "shared", Cursor: automations.Cursor("cursor-" + runtimeID), Checkpoint: "checkpoint-" + runtimeID,
		}); err != nil {
			t.Fatal(err)
		}
	}
	submitted := &recordingSubmitter{}
	supervision := scriptpollers.ScriptPollerSupervision{
		AutomationID: "workflow", InstanceID: "shared", ExpectedCursor: "cursor-runtime-a",
		CursorScope: scriptpollers.CursorScope{RuntimeID: "runtime-a"},
	}
	err := svc.RunScriptPoller(ctx, runner, runtimeCfg, poller, worker, supervision, submitted.submit)
	if err == nil || !strings.Contains(err.Error(), "exited unexpectedly") || submitted.calls != 1 {
		t.Fatalf("scoped poll = %v, admissions = %d, want committed admission", err, submitted.calls)
	}
	if !containsEnv(runner.reqs[0].Env, scriptpollers.ScriptPollerCursorEnvVar+"=cursor-runtime-a") ||
		!containsEnv(runner.reqs[0].Env, scriptpollers.ScriptPollerCheckpointEnvVar+"=checkpoint-runtime-a") {
		t.Fatalf("command env = %v, want runtime A recovery facts", runner.reqs[0].Env)
	}
	assertIsolatedPollerRecovery(t, svc)
	err = svc.RunScriptPoller(ctx, runner, runtimeCfg, poller, worker, supervision, submitted.submit)
	assertAutomationsConflict(t, err, scriptpollers.GetCursorOperation)
	if runner.callCount() != 1 || submitted.calls != 1 {
		t.Fatalf("stale poll executed: commands=%d admissions=%d", runner.callCount(), submitted.calls)
	}
	got, err := svc.GetCursor(ctx, automations.GetCursorRequest{InstanceID: "shared"})
	if err != nil || got.Cursor != "cursor-" {
		t.Fatalf("detached cursor = %+v, %v, want unchanged empty scope", got, err)
	}
}

func assertIsolatedPollerRecovery(t *testing.T, svc scriptpollers.Service) {
	t.Helper()
	ctx := context.Background()
	for _, runtimeID := range []string{"", "runtime-a", "runtime-b"} {
		wantCursor, wantCheckpoint := automations.Cursor("cursor-"+runtimeID), "checkpoint-"+runtimeID
		if runtimeID == "runtime-a" {
			wantCursor, wantCheckpoint = "advanced", "advanced-checkpoint"
		}
		got, readErr := svc.GetCursorForScope(ctx, scriptpollers.CursorScope{RuntimeID: runtimeID}, automations.GetCursorRequest{InstanceID: "shared"})
		if readErr != nil || got.Cursor != wantCursor || got.Checkpoint != wantCheckpoint {
			t.Fatalf("scope %q recovery = %+v, %v, want %s/%s", runtimeID, got, readErr, wantCursor, wantCheckpoint)
		}
	}
}

func (failingCursorRecorder) ReleaseScope(scriptpollers.CursorScope) {
	panic("unexpected scope release")
}
