package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type recordingLogger struct {
	entries []logEntry
}

type logEntry struct {
	level   string
	message string
	fields  map[string]any
}

func (l *recordingLogger) Debug(msg string, keysAndValues ...any) {
	l.record("debug", msg, keysAndValues...)
}

func (l *recordingLogger) Info(msg string, keysAndValues ...any) {
	l.record("info", msg, keysAndValues...)
}

func (l *recordingLogger) Warn(msg string, keysAndValues ...any) {
	l.record("warn", msg, keysAndValues...)
}

func (l *recordingLogger) Error(msg string, keysAndValues ...any) {
	l.record("error", msg, keysAndValues...)
}

func (l *recordingLogger) Verbose(msg string, keysAndValues ...any) {
	l.record("verbose", msg, keysAndValues...)
}

func (l *recordingLogger) record(level, msg string, keysAndValues ...any) {
	fields := map[string]any{}
	for index := 0; index+1 < len(keysAndValues); index += 2 {
		key, ok := keysAndValues[index].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[index+1]
	}
	l.entries = append(l.entries, logEntry{
		level:   level,
		message: msg,
		fields:  fields,
	})
}

var _ logging.Logger = (*recordingLogger)(nil)

func TestPauseResume_EmitCanonicalSessionLifecycleEvents(t *testing.T) {
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
		withWorkflowContext(&factory_context.FactoryContext{SessionID: "session-pause-resume"}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if history.CallCount("RecordSessionPaused") != 1 || history.CallCount("RecordSessionResumed") != 1 {
		t.Fatalf("lifecycle calls = %v, want one pause and one resume", history.CallsSnapshot())
	}
	if len(history.LifecycleControls) < 2 ||
		history.LifecycleControls[0].SessionID != "session-pause-resume" ||
		history.LifecycleControls[len(history.LifecycleControls)-1].SessionID != "session-pause-resume" {
		t.Fatalf("lifecycle inputs = %#v, want session-pause-resume", history.LifecycleControls)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factoryState = %q, want RUNNING", snapshot.FactoryState)
	}
	// Recordings owns replay reduction of these emitted events. The named owner
	// invariant is TestReconstructFactoryWorldState_PauseResumeHistoryReconstructsLifecycleControlStatus.
}

func TestPauseResume_NoOpDoesNotEmitAdditionalLifecycleEvents(t *testing.T) {
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("first Resume: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("second Resume: %v", err)
	}

	pauseCount := history.CallCount("RecordSessionPaused")
	resumeCount := history.CallCount("RecordSessionResumed")
	if pauseCount != 1 || resumeCount != 1 {
		t.Fatalf("lifecycle event counts = pause %d resume %d, want one each", pauseCount, resumeCount)
	}
}

func TestPauseResume_ReplayPreservesFinalPausedStatus(t *testing.T) {
	t0 := time.Date(2026, 6, 20, 11, 0, 0, 0, time.UTC)
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
		withClock(platformclock.NewDeterministic(t0, time.Second)),
		withWorkflowContext(&factory_context.FactoryContext{SessionID: "session-paused-only"}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after pause: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStatePaused) {
		t.Fatalf("factoryState = %q, want PAUSED", snapshot.FactoryState)
	}

	if history.CallCount("RecordSessionPaused") != 1 {
		t.Fatalf("pause record calls = %d, want one", history.CallCount("RecordSessionPaused"))
	}
	// Recordings replay of the canonical pause event is covered owner-locally by
	// TestReconstructFactoryWorldState_PauseResumeHistoryReconstructsLifecycleControlStatus.
}

func TestPauseResume_DiagnosticsLogAcceptedTransitions(t *testing.T) {
	logger := &recordingLogger{}
	f, err := newTestFactory(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logger),
		withWorkflowContext(&factory_context.FactoryContext{SessionID: "session-diagnostics"}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	pauseEntry := findLogEntry(t, logger.entries, "factory runtime lifecycle control", "operation", "PAUSE")
	if pauseEntry.fields["outcome"] != "ACCEPTED" {
		t.Fatalf("pause outcome = %#v, want ACCEPTED", pauseEntry.fields["outcome"])
	}
	if pauseEntry.fields["session_id"] != "session-diagnostics" {
		t.Fatalf("pause session_id = %#v, want session-diagnostics", pauseEntry.fields["session_id"])
	}

	resumeEntry := findLogEntry(t, logger.entries, "factory runtime lifecycle control", "operation", "RESUME")
	if resumeEntry.fields["outcome"] != "ACCEPTED" {
		t.Fatalf("resume outcome = %#v, want ACCEPTED", resumeEntry.fields["outcome"])
	}
}

func TestResume_DiagnosticsLogPostResumeBufferedDrain(t *testing.T) {
	logger := &recordingLogger{}
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withLogger(logger),
		withWorkflowContext(&factory_context.FactoryContext{SessionID: "session-drain"}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	impl := f.(*factoryImpl)
	if !impl.resultBuffer.Write(ctx, workerexecution.WorkResult{
		DispatchID: "dispatch-drain",
		Outcome:    workerexecution.OutcomeAccepted,
	}) {
		t.Fatal("buffered result write failed")
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	findLogEntry(t, logger.entries, "factory runtime resume buffered results pending drain", "buffered_result_count", 1)

	impl.observePostResumeBufferedDrain(1)
	drainEntry := findLogEntry(t, logger.entries, "factory runtime resume buffered results drained", "drained_result_count", 1)
	if drainEntry.fields["session_id"] != "session-drain" {
		t.Fatalf("drain session_id = %#v, want session-drain", drainEntry.fields["session_id"])
	}
}

func TestObservePostResumeBufferedDrain_IgnoresDrainWhenResumeWasNotPending(t *testing.T) {
	logger := &recordingLogger{}
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withLogger(logger),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	impl := f.(*factoryImpl)
	impl.observePostResumeBufferedDrain(2)
	for _, entry := range logger.entries {
		if entry.message == "factory runtime resume buffered results drained" {
			t.Fatalf("unexpected drain log without pending resume: %#v", entry)
		}
	}
}

func TestPauseResume_DiagnosticsAvoidPayloadAndPathFields(t *testing.T) {
	logger := &recordingLogger{}
	f, err := newTestFactory(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logger),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	for _, entry := range logger.entries {
		for key, value := range entry.fields {
			text, ok := value.(string)
			if !ok {
				continue
			}
			if strings.Contains(text, "/Users/") || strings.Contains(strings.ToLower(key), "payload") {
				t.Fatalf("diagnostic log leaked sensitive field %q=%q in %q", key, text, entry.message)
			}
		}
	}
}

func findLogEntry(t *testing.T, entries []logEntry, message, fieldKey string, want any) logEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.message != message {
			continue
		}
		if entry.fields[fieldKey] == want {
			return entry
		}
	}
	t.Fatalf("log entry %q with %q=%#v not found in %#v", message, fieldKey, want, entries)
	return logEntry{}
}

func TestTickWhilePaused_SkipsCascadeButOperatorMoveUpdatesMarking(t *testing.T) {
	f, history, ctx := setupPausedParentFailedChildInit(t)
	assertChildRemainsInInitAfterPausedTick(t, f, ctx)

	result, err := f.MoveWork(ctx, "child-work", "complete", work.WorkStateChangeSourceCLI, "")
	if err != nil {
		t.Fatalf("MoveWork while paused: %v", err)
	}
	if result.FromState != "init" || result.ToState != "complete" {
		t.Fatalf("move result = %#v, want init -> complete", result)
	}
	assertOperatorWorkStateChangeRecord(t, history, "child-work", "init", "complete", work.WorkStateChangeSourceCLI)

	afterMove, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after move: %v", err)
	}
	if !markingContainsWorkAtPlace(&afterMove.Marking, "child-work", "task:complete") {
		t.Fatalf("marking = %#v, want child-work at task:complete after operator move", afterMove.Marking.Tokens)
	}
}

func setupPausedParentFailedChildInit(
	t *testing.T,
) (factory.Factory, *recordingfixtures.ScriptedRuntimeLedger, context.Context) {
	t.Helper()
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildMoveControlNet()),
		withInlineDispatch(),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, []work.SubmitRequest{
		{WorkID: "parent-work", WorkTypeID: "task", TraceID: "trace-parent"},
		{
			WorkID:     "child-work",
			WorkTypeID: "task",
			TraceID:    "trace-child",
			Relations: []work.Relation{{
				Type:          work.RelationDependsOn,
				TargetWorkID:  "parent-work",
				RequiredState: "complete",
			}},
		},
	}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick inject: %v", err)
	}

	if _, err := f.MoveWork(ctx, "parent-work", "failed", work.WorkStateChangeSourceCLI, ""); err != nil {
		t.Fatalf("MoveWork parent to failed: %v", err)
	}
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	return f, history, ctx
}

func assertChildRemainsInInitAfterPausedTick(t *testing.T, f factory.Factory, ctx context.Context) {
	t.Helper()
	before, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before tick: %v", err)
	}
	if err := tickableFactory(t, f).Tick(ctx); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	afterTick, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after tick: %v", err)
	}
	if !markingContainsWorkAtPlace(&before.Marking, "child-work", "task:init") {
		t.Fatalf("pre-tick marking = %#v, want child-work in task:init", before.Marking.Tokens)
	}
	if !markingContainsWorkAtPlace(&afterTick.Marking, "child-work", "task:init") {
		t.Fatalf("post-tick marking = %#v, want child-work still in task:init (no cascade)", afterTick.Marking.Tokens)
	}
}
