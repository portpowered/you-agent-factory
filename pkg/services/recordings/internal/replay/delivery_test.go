package replay

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordingcontracts "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestSubmissionHook_ReplaysWorkRequestEventsByTick(t *testing.T) {
	hook, err := NewSubmissionHook(testFactorySnapshotDecoder, testRuntimeConfigDecoder, testReplayArtifact(t, replayWorkRequestEvent(t, "request-1", 2, "api", []factoryapi.Work{{
		Name:         "work-1",
		WorkId:       stringPtrIfNotEmpty("work-1"),
		RequestId:    stringPtrIfNotEmpty("request-1"),
		WorkTypeName: stringPtrIfNotEmpty("task"),
		TraceId:      stringPtrIfNotEmpty("trace-1"),
		Content: replayWorkContentForDeliveryTest(t, []work.WorkContentPart{
			{Type: work.WorkContentPartTypeText, Text: "alpha"},
			{Type: work.WorkContentPartTypeImage, URL: "file://fixtures/alpha.png"},
		}),
	}, {
		Name:         "work-2",
		WorkId:       stringPtrIfNotEmpty("work-2"),
		RequestId:    stringPtrIfNotEmpty("request-1"),
		WorkTypeName: stringPtrIfNotEmpty("task"),
		TraceId:      stringPtrIfNotEmpty("trace-1"),
	}}, []factoryapi.Relation{{
		Type:           factoryapi.RelationTypeDependsOn,
		SourceWorkName: "work-2",
		TargetWorkName: "work-1",
	}})))
	if err != nil {
		t.Fatalf("NewSubmissionHook: %v", err)
	}

	before, err := hook.OnTick(context.Background(), replaySubmissionHookContext(1))
	if err != nil {
		t.Fatalf("OnTick before due tick: %v", err)
	}
	assertSubmissionHookBeforeDueTick(t, before)

	due, err := hook.OnTick(context.Background(), replaySubmissionHookContext(2))
	if err != nil {
		t.Fatalf("OnTick at due tick: %v", err)
	}
	assertSubmissionHookDueTickBatchCount(t, due, 1)
	batch := due.GeneratedBatches[0]
	assertSubmissionHookReplayedRequestID(t, batch, "request-1")
	assertSubmissionHookReplayedSourceMetadata(t, batch, "api")
	assertSubmissionHookReplayedGeneratedWorkCount(t, batch, 2, "work-1")
	assertSubmissionHookReplayedOrderedContent(t, batch.Request.Works[0], "alpha", "file://fixtures/alpha.png")
	assertSubmissionHookReplayedDependencyRelation(t, batch, "work-2", "work-1")
	assertSubmissionHookFinalKeepAliveShutdown(t, due)
}

func TestApplyReplaySubmissionDefaultsRecoversLongestWorkTypeFromWorkID(t *testing.T) {
	submissions := []replaySubmission{{request: work.WorkRequest{Works: []work.Work{
		{WorkID: "work-research-task-42"},
		{WorkID: "work-task-43"},
		{WorkID: "unrecognized-44"},
	}}}}
	factoryConfig := &interfaces.FactoryConfig{WorkTypes: []interfaces.WorkTypeConfig{
		{Name: "task"},
		{Name: "research-task"},
	}}

	applyReplaySubmissionDefaults(submissions, factoryConfig)

	got := submissions[0].request.Works
	if got[0].WorkTypeID != "research-task" || got[1].WorkTypeID != "task" {
		t.Fatalf("recovered work types = %q, %q, want research-task, task", got[0].WorkTypeID, got[1].WorkTypeID)
	}
	if got[2].WorkTypeID != "" {
		t.Fatalf("unrecognized work type = %q, want empty", got[2].WorkTypeID)
	}
}

func assertSubmissionHookBeforeDueTick(t *testing.T, got recordingcontracts.ReplayHookResult) {
	t.Helper()
	if len(got.GeneratedBatches) != 0 {
		t.Fatalf("before-due batch count = %d, want 0", len(got.GeneratedBatches))
	}
	if !got.KeepAlive {
		t.Fatal("before-due keep-alive = false, want true while future submissions remain")
	}
}

func assertSubmissionHookDueTickBatchCount(t *testing.T, got recordingcontracts.ReplayHookResult, want int) {
	t.Helper()
	if len(got.GeneratedBatches) != want {
		t.Fatalf("due-tick batch count = %d, want %d", len(got.GeneratedBatches), want)
	}
}

func assertSubmissionHookFinalKeepAliveShutdown(t *testing.T, got recordingcontracts.ReplayHookResult) {
	t.Helper()
	if got.KeepAlive {
		t.Fatal("final keep-alive shutdown = true, want false after last submission is emitted")
	}
}

func assertSubmissionHookReplayedRequestID(t *testing.T, batch work.GeneratedSubmissionBatch, want string) {
	t.Helper()
	if batch.Request.RequestID != want {
		t.Fatalf("replayed request ID = %q, want %q", batch.Request.RequestID, want)
	}
}

func assertSubmissionHookReplayedSourceMetadata(t *testing.T, batch work.GeneratedSubmissionBatch, want string) {
	t.Helper()
	if batch.Metadata.Source != want {
		t.Fatalf("replayed source metadata = %q, want %q", batch.Metadata.Source, want)
	}
}

func assertSubmissionHookReplayedGeneratedWorkCount(t *testing.T, batch work.GeneratedSubmissionBatch, wantCount int, firstWorkID string) {
	t.Helper()
	if len(batch.Request.Works) != wantCount {
		t.Fatalf("replayed generated work count = %d, want %d", len(batch.Request.Works), wantCount)
	}
	if batch.Request.Works[0].WorkID != firstWorkID {
		t.Fatalf("replayed first work ID = %q, want %q", batch.Request.Works[0].WorkID, firstWorkID)
	}
}

func assertSubmissionHookReplayedOrderedContent(t *testing.T, work work.Work, wantText, wantImageURL string) {
	t.Helper()
	got := work.Content
	if len(got) != 2 {
		t.Fatalf("replayed content part count = %d, want 2", len(got))
	}
	if got[0].Text != wantText {
		t.Fatalf("replayed first content text = %q, want %q", got[0].Text, wantText)
	}
	if got[1].URL != wantImageURL {
		t.Fatalf("replayed second content image URL = %q, want %q", got[1].URL, wantImageURL)
	}
}

func assertSubmissionHookReplayedDependencyRelation(t *testing.T, batch work.GeneratedSubmissionBatch, sourceWork, targetWork string) {
	t.Helper()
	if len(batch.Request.Relations) != 1 {
		t.Fatalf("replayed relation count = %d, want 1", len(batch.Request.Relations))
	}
	relation := batch.Request.Relations[0]
	if relation.SourceWorkName != sourceWork || relation.TargetWorkName != targetWork {
		t.Fatalf("replayed dependency relation = %q -> %q, want %q -> %q (source depends on target)",
			relation.SourceWorkName, relation.TargetWorkName, sourceWork, targetWork)
	}
}

func replayWorkContentForDeliveryTest(t *testing.T, parts []work.WorkContentPart) *factoryapi.WorkContent {
	t.Helper()
	content := make(factoryapi.WorkContent, 0, len(parts))
	for _, part := range parts {
		var generated factoryapi.WorkContentPart
		switch part.Type {
		case work.WorkContentPartTypeText:
			if err := generated.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
				Type: factoryapi.WorkContentPartTypeText,
				Text: part.Text,
			}); err != nil {
				t.Fatalf("encode text content: %v", err)
			}
		case work.WorkContentPartTypeImage:
			if err := generated.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
				Type: factoryapi.WorkContentPartTypeImage,
				Url:  factoryapi.WorkContentURLProperty(contentURLForTestPath(part.URL, part.File)),
			}); err != nil {
				t.Fatalf("encode image content: %v", err)
			}
		}
		content = append(content, generated)
	}
	return &content
}

func TestSubmissionHook_ReplaysCronTimeWorkRequestWithPendingTargetState(t *testing.T) {
	hook, err := NewSubmissionHook(testFactorySnapshotDecoder, testRuntimeConfigDecoder, testReplayArtifact(t, replayWorkRequestEvent(t, "request-cron", 1, "external-submit", []factoryapi.Work{{
		Name:         "cron:poll-for-work",
		WorkId:       stringPtrIfNotEmpty("time-cron"),
		WorkTypeName: stringPtrIfNotEmpty(interfaces.SystemTimeWorkTypeID),
		TraceId:      stringPtrIfNotEmpty("trace-cron"),
		Tags: &factoryapi.StringMap{
			interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
			interfaces.TimeWorkTagKeyCronWorkstation: "poll-for-work",
		},
	}}, nil)))
	if err != nil {
		t.Fatalf("NewSubmissionHook: %v", err)
	}

	due, err := hook.OnTick(context.Background(), replaySubmissionHookContext(1))
	if err != nil {
		t.Fatalf("OnTick at due tick: %v", err)
	}
	if len(due.GeneratedBatches) != 1 || len(due.GeneratedBatches[0].Request.Works) != 1 {
		t.Fatalf("generated batches = %#v, want one cron time work request", due.GeneratedBatches)
	}
	work := due.GeneratedBatches[0].Request.Works[0]
	if work.WorkTypeID != interfaces.SystemTimeWorkTypeID || work.State != interfaces.SystemTimePendingState {
		t.Fatalf("replayed cron work = %#v, want system time pending target", work)
	}
	if due.KeepAlive {
		t.Fatal("cron due tick KeepAlive = true, want false after submission is emitted")
	}
}

func TestSubmissionHook_KeepAliveUntilFutureSubmissionTick(t *testing.T) {
	hook, err := NewSubmissionHook(testFactorySnapshotDecoder, testRuntimeConfigDecoder, testReplayArtifact(t,
		replayWorkRequestEvent(t, "request-early", 2, "api", []factoryapi.Work{{
			Name:         "work-early",
			WorkId:       stringPtrIfNotEmpty("work-early"),
			RequestId:    stringPtrIfNotEmpty("request-early"),
			WorkTypeName: stringPtrIfNotEmpty("task"),
			TraceId:      stringPtrIfNotEmpty("trace-1"),
		}}, nil),
		replayWorkRequestEvent(t, "request-late", 5, "api", []factoryapi.Work{{
			Name:         "work-late",
			WorkId:       stringPtrIfNotEmpty("work-late"),
			RequestId:    stringPtrIfNotEmpty("request-late"),
			WorkTypeName: stringPtrIfNotEmpty("task"),
			TraceId:      stringPtrIfNotEmpty("trace-2"),
		}}, nil),
	))
	if err != nil {
		t.Fatalf("NewSubmissionHook: %v", err)
	}

	beforeLate, err := hook.OnTick(context.Background(), replaySubmissionHookContext(2))
	if err != nil {
		t.Fatalf("OnTick at first due tick: %v", err)
	}
	if len(beforeLate.GeneratedBatches) != 1 {
		t.Fatalf("generated batches at first due tick = %d, want 1", len(beforeLate.GeneratedBatches))
	}
	if !beforeLate.KeepAlive {
		t.Fatal("first due tick KeepAlive = false, want true while a later submission remains")
	}

	waiting, err := hook.OnTick(context.Background(), replaySubmissionHookContext(4))
	if err != nil {
		t.Fatalf("OnTick before second due tick: %v", err)
	}
	if len(waiting.GeneratedBatches) != 0 {
		t.Fatalf("generated batches before second due tick = %d, want 0", len(waiting.GeneratedBatches))
	}
	if !waiting.KeepAlive {
		t.Fatal("before second due tick KeepAlive = false, want true")
	}

	final, err := hook.OnTick(context.Background(), replaySubmissionHookContext(5))
	if err != nil {
		t.Fatalf("OnTick at second due tick: %v", err)
	}
	if len(final.GeneratedBatches) != 1 {
		t.Fatalf("generated batches at second due tick = %d, want 1", len(final.GeneratedBatches))
	}
	if final.KeepAlive {
		t.Fatal("final due tick KeepAlive = true, want false after all submissions are emitted")
	}
}

func TestCompletionDeliveryPlan_UnobservedDispatchDoesNotDivergeOnTickAlone(t *testing.T) {
	plan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, deliveryArtifact(t,
		replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1"),
		replayTestCompletion("completion-1", "dispatch-1", "process", 3),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	err = plan.ValidateReplayTick(3)
	if err != nil {
		t.Fatalf("tick-only dispatch drift should not force replay divergence: %v", err)
	}
}

func TestCompletionDeliveryPlan_MissingCompletionDispatchCanBeSkippedAfterRepair(t *testing.T) {
	plan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, testReplayArtifact(t,
		replayDispatchCreatedEvent(t, replayTestDispatch("dispatch-no-completion", "process", 2, "trace-1", "work-1", "tok-1"), 2),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	if err := plan.ValidateReplayTick(3); err != nil {
		t.Fatalf("missing-completion dispatch should not force replay divergence after repair: %v", err)
	}
}

func TestCompletionDeliveryPlan_ReplaysIgnoredCanceledDispatchAsCanceledResult(t *testing.T) {
	dispatch := replayTestDispatch("dispatch-ignored", "process", 2, "trace-ignored", "work-ignored", "tok-ignored")
	plan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, testReplayArtifact(
		t,
		replayDispatchCreatedEvent(t, dispatch, 2),
		replayDispatchResultIgnoredEvent(t, dispatch.DispatchID, dispatch.Execution.WorkIDs, workerexecution.OutcomeCanceled, 5),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	deliveryTick, ok, err := plan.DeliveryTickForDispatch(dispatch)
	if err != nil {
		t.Fatalf("DeliveryTickForDispatch: %v", err)
	}
	if !ok || deliveryTick != 5 {
		t.Fatalf("delivery match = (%t, %d), want (true, 5)", ok, deliveryTick)
	}

	planned, ok, err := plan.PlannedResultForDispatch(dispatch)
	if err != nil {
		t.Fatalf("PlannedResultForDispatch: %v", err)
	}
	if !ok {
		t.Fatal("expected planned canceled result for ignored dispatch")
	}
	if planned.Outcome != workerexecution.OutcomeCanceled || planned.DispatchID != dispatch.DispatchID {
		t.Fatalf("planned result = %#v, want canceled result for %q", planned, dispatch.DispatchID)
	}
	if planned.Cancellation == nil || planned.Cancellation.Reason != workerexecution.DispatchCancellationReasonCanceled {
		t.Fatalf("planned cancellation = %#v, want CANCELED", planned.Cancellation)
	}
}

func TestCompletionDeliveryPlan_DispatchIdentityMismatchReportsDivergence(t *testing.T) {
	plan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, deliveryArtifact(t,
		replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1"),
		replayTestCompletion("completion-1", "dispatch-1", "process", 3),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	_, _, err = plan.DeliveryTickForDispatch(work.WorkDispatch{
		DispatchID:      "observed-dispatch",
		TransitionID:    "review",
		WorkstationName: "review",
		InputTokens:     workers.InputTokens(workers.Token{ID: "tok-1"}),
		Execution: work.ExecutionMetadata{
			DispatchCreatedTick: 2,
			ReplayKey:           "review/trace-1/work-1",
			TraceID:             "trace-1",
			WorkIDs:             []string{"work-1"},
		},
	})
	if err == nil {
		t.Fatal("expected dispatch mismatch divergence")
	}
	report := requireDivergence(t, err)
	if report.Category != DivergenceCategoryDispatchMismatch {
		t.Fatalf("category = %q, want %q", report.Category, DivergenceCategoryDispatchMismatch)
	}
	if report.Tick != 2 {
		t.Fatalf("tick = %d, want 2", report.Tick)
	}
	if !strings.Contains(report.Expected, "transition=process") {
		t.Fatalf("expected summary missing recorded transition: %q", report.Expected)
	}
	if !strings.Contains(report.Observed, "transition=review") {
		t.Fatalf("observed summary missing observed transition: %q", report.Observed)
	}
	if report.ExpectedEventID == "" {
		t.Fatal("expected divergence report to include expected event id")
	}
}

func TestCompletionDeliveryPlan_EarlyDispatchCreatedTickDoesNotDeliverBeforeRecordedTick(t *testing.T) {
	plan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, deliveryArtifact(t,
		replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1"),
		replayTestCompletion("completion-1", "dispatch-1", "process", 3),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	earlyDispatch := replayTestDispatch("observed-dispatch", "process", 1, "trace-1", "work-1", "tok-1")
	earlyDispatch.Execution.ReplayKey = "process/trace-1/work-1"

	deliveryTick, ok, err := plan.DeliveryTickForDispatch(earlyDispatch)
	if err != nil {
		t.Fatalf("tick-drifted dispatch should still match by logical identity: %v", err)
	}
	if !ok {
		t.Fatal("expected delivery tick for tick-drifted dispatch")
	}
	if deliveryTick != 3 {
		t.Fatalf("delivery tick = %d, want recorded completion tick floor 3", deliveryTick)
	}
}

func TestCompletionDeliveryPlan_LateDispatchCreatedTickKeepsRelativeDelay(t *testing.T) {
	plan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, deliveryArtifact(t,
		replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1"),
		replayTestCompletion("completion-1", "dispatch-1", "process", 3),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	lateDispatch := replayTestDispatch("observed-dispatch", "process", 4, "trace-1", "work-1", "tok-1")
	lateDispatch.Execution.ReplayKey = "process/trace-1/work-1"

	deliveryTick, ok, err := plan.DeliveryTickForDispatch(lateDispatch)
	if err != nil {
		t.Fatalf("late tick-drifted dispatch should still match by logical identity: %v", err)
	}
	if !ok {
		t.Fatal("expected delivery tick for late tick-drifted dispatch")
	}
	if deliveryTick != 5 {
		t.Fatalf("delivery tick = %d, want observed dispatch tick plus recorded delay 5", deliveryTick)
	}
}

func TestCompletionDeliveryPlan_PlannedResultClonesFailureMetadataOnlyInput(t *testing.T) {
	completion := replayTestCompletion("completion-1", "dispatch-1", "process", 3)
	completion.FailureMetadata = &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyRetryable,
		Type:   workerexecution.WorkFailureTypeInternalServerError,
	}

	plan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, deliveryArtifact(t,
		replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1"),
		completion,
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	observed := replayTestDispatch("observed-dispatch", "process", 2, "trace-1", "work-1", "tok-1")
	deliveryTick, ok, err := plan.DeliveryTickForDispatch(observed)
	if err != nil {
		t.Fatalf("DeliveryTickForDispatch: %v", err)
	}
	if !ok || deliveryTick != 3 {
		t.Fatalf("delivery match = (%t, %d), want (true, 3)", ok, deliveryTick)
	}

	completion.FailureMetadata.Type = workerexecution.WorkFailureTypeAuthFailure

	planned, ok, err := plan.PlannedResultForDispatch(observed)
	if err != nil {
		t.Fatalf("PlannedResultForDispatch: %v", err)
	}
	if !ok {
		t.Fatal("expected planned result for observed dispatch")
	}
	if planned.FailureMetadata == nil || planned.FailureMetadata.Type != workerexecution.WorkFailureTypeInternalServerError {
		t.Fatalf("planned failure metadata = %#v, want detached internal_server_error", planned.FailureMetadata)
	}
}

func TestCompletionDeliveryPlan_LineageMismatchReportsDivergence(t *testing.T) {
	plan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, deliveryArtifact(t,
		replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1"),
		replayTestCompletion("completion-1", "dispatch-1", "process", 3),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	_, _, err = plan.DeliveryTickForDispatch(work.WorkDispatch{
		DispatchID:      "observed-dispatch",
		TransitionID:    "process",
		WorkstationName: "process",
		InputTokens:     workers.InputTokens(workers.Token{ID: "tok-different"}),
		Execution: work.ExecutionMetadata{
			DispatchCreatedTick: 2,
			ReplayKey:           "process/trace-1/work-1",
			TraceID:             "trace-1",
			WorkIDs:             []string{"work-1"},
		},
	})
	if err == nil {
		t.Fatal("expected lineage mismatch divergence")
	}
	report := requireDivergence(t, err)
	if report.Category != DivergenceCategoryDispatchMismatch {
		t.Fatalf("category = %q, want %q", report.Category, DivergenceCategoryDispatchMismatch)
	}
	if !strings.Contains(report.Expected, "tok-1") {
		t.Fatalf("expected summary missing recorded token: %q", report.Expected)
	}
	if !strings.Contains(report.Observed, "tok-different") {
		t.Fatalf("observed summary missing observed token: %q", report.Observed)
	}
}

func TestCompletionDeliveryPlan_ResourceTokenIDChangesDoNotDiverge(t *testing.T) {
	dispatch := replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1")
	dispatch.InputTokens = workers.InputTokens(
		resourceReplayToken("executor-slot:resource:7"),
		workers.Token{ID: "tok-1"},
	)
	plan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, deliveryArtifact(t,
		dispatch,
		replayTestCompletion("completion-1", "dispatch-1", "process", 3),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	observed := dispatch
	observed.DispatchID = "observed-dispatch"
	observed.InputTokens = workers.InputTokens(
		resourceReplayToken("executor-slot:resource:4"),
		workers.Token{ID: "tok-1"},
	)
	deliveryTick, ok, err := plan.DeliveryTickForDispatch(observed)
	if err != nil {
		t.Fatalf("resource token ID difference should not diverge: %v", err)
	}
	if !ok {
		t.Fatal("expected delivery tick for resource-equivalent dispatch")
	}
	if deliveryTick != 3 {
		t.Fatalf("delivery tick = %d, want 3", deliveryTick)
	}
}

func TestCompletionDeliveryPlan_UnknownCompletionReportsDivergence(t *testing.T) {
	_, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, deliveryArtifact(t,
		replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1"),
		replayTestCompletion("completion-1", "unknown-dispatch", "process", 3),
	))
	if err == nil {
		t.Fatal("expected unknown completion divergence")
	}
	report := requireDivergence(t, err)
	if report.Category != DivergenceCategoryUnknownCompletion {
		t.Fatalf("category = %q, want %q", report.Category, DivergenceCategoryUnknownCompletion)
	}
	if report.DispatchID != "unknown-dispatch" {
		t.Fatalf("dispatch ID = %q, want unknown-dispatch", report.DispatchID)
	}
}

func TestFactoryMetadataWarnings_ReportsConfigHashMismatch(t *testing.T) {
	artifactConfig := factoryapi.Factory{Metadata: generatedStringMapPtr(map[string]string{metadataFactoryHash: "sha256:recorded"})}
	currentConfig := factoryapi.Factory{Metadata: generatedStringMapPtr(map[string]string{metadataFactoryHash: "sha256:current"})}

	warnings := FactoryMetadataWarnings(mustFactorySnapshot(t, artifactConfig), mustFactorySnapshot(t, currentConfig))
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Key != metadataFactoryHash {
		t.Fatalf("warning key = %q, want %q", warnings[0].Key, metadataFactoryHash)
	}
}

func requireDivergence(t *testing.T, err error) DivergenceReport {
	t.Helper()
	var divergence *DivergenceError
	if !errors.As(err, &divergence) {
		t.Fatalf("error %T is not DivergenceError: %v", err, err)
	}
	return divergence.Report
}

func deliveryArtifact(t *testing.T, dispatch work.WorkDispatch, completion workerexecution.WorkResult) *interfaces.ReplayArtifact {
	t.Helper()
	return testReplayArtifact(
		t,
		replayDispatchCreatedEvent(t, dispatch, dispatch.Execution.DispatchCreatedTick),
		replayDispatchCompletedEvent(t, "completion-1", completion, 3),
	)
}

func replaySubmissionHookContext(tick int) recordingcontracts.ReplaySnapshot {
	return recordingcontracts.ReplaySnapshot{Tick: tick}
}

func replaySubmissionHookContextWithWorkToken(tick int, workID, tokenID, placeID string) recordingcontracts.ReplaySnapshot {
	ctx := replaySubmissionHookContext(tick)
	ctx.TokenByWorkID = map[string]recordingcontracts.ReplayWorkToken{
		workID: {TokenID: tokenID, PlaceID: placeID},
	}
	return ctx
}

func TestWorkStateChangeHook_ReplaysOperatorMoveAtRecordedTick(t *testing.T) {
	hook, err := NewWorkStateChangeHook(testFactorySnapshotDecoder, testRuntimeConfigDecoder, testReplayArtifact(
		t,
		replayWorkStateChangeEvent(t, "work-recover", "failed", "init", "task:failed", "task:init", factoryapi.WorkStateChangeSourceAPI, 4),
	))
	if err != nil {
		t.Fatalf("NewWorkStateChangeHook: %v", err)
	}

	before, err := hook.OnTick(context.Background(), replaySubmissionHookContextWithWorkToken(3, "work-recover", "token-work-recover", "task:failed"))
	if err != nil {
		t.Fatalf("OnTick before move tick: %v", err)
	}
	if len(before.MarkingMutations) != 0 {
		t.Fatalf("marking mutations before due tick = %#v, want none", before.MarkingMutations)
	}
	if !before.KeepAlive {
		t.Fatal("expected hook to stay alive before due operator move tick")
	}

	due, err := hook.OnTick(context.Background(), replaySubmissionHookContextWithWorkToken(4, "work-recover", "token-work-recover", "task:failed"))
	if err != nil {
		t.Fatalf("OnTick at move tick: %v", err)
	}
	if len(due.MarkingMutations) != 1 {
		t.Fatalf("marking mutations = %#v, want one operator move", due.MarkingMutations)
	}
	mutation := due.MarkingMutations[0]
	if mutation.Type != interfaces.MutationMove || mutation.TokenID != "token-work-recover" {
		t.Fatalf("mutation = %#v, want MOVE for token-work-recover", mutation)
	}
	if mutation.FromPlace != "task:failed" || mutation.ToPlace != "task:init" {
		t.Fatalf("mutation places = %q -> %q, want task:failed -> task:init", mutation.FromPlace, mutation.ToPlace)
	}
}

func TestWorkStateChangeHook_ReplaysMoveFromConsumedDispatchToken(t *testing.T) {
	hook, err := NewWorkStateChangeHook(testFactorySnapshotDecoder, testRuntimeConfigDecoder, testReplayArtifact(
		t,
		replayWorkStateChangeEvent(t, "work-in-flight", "init", "complete", "task:init", "task:complete", factoryapi.WorkStateChangeSourceAPI, 4),
	))
	if err != nil {
		t.Fatalf("NewWorkStateChangeHook: %v", err)
	}

	input := replaySubmissionHookContext(4)
	input.ConsumedTokenByWorkID = map[string]recordingcontracts.ReplayWorkToken{
		"work-in-flight": {TokenID: "token-in-flight", PlaceID: "task:init"},
	}
	got, err := hook.OnTick(context.Background(), input)
	if err != nil {
		t.Fatalf("OnTick: %v", err)
	}
	if len(got.MarkingMutations) != 1 {
		t.Fatalf("marking mutations = %#v, want one operator move", got.MarkingMutations)
	}
	mutation := got.MarkingMutations[0]
	if mutation.Type != interfaces.MutationMove || mutation.TokenID != "token-in-flight" ||
		mutation.FromPlace != "task:init" || mutation.ToPlace != "task:complete" {
		t.Fatalf("mutation = %#v, want in-flight token move task:init -> task:complete", mutation)
	}
}

func replayTestDispatch(dispatchID, transitionID string, tick int, traceID, workID, tokenID string) work.WorkDispatch {
	return work.WorkDispatch{
		DispatchID:      dispatchID,
		TransitionID:    transitionID,
		WorkstationName: transitionID,
		InputTokens:     workers.InputTokens(workers.Token{ID: tokenID}),
		Execution: work.ExecutionMetadata{
			DispatchCreatedTick: tick,
			ReplayKey:           transitionID + "/" + traceID + "/" + workID,
			TraceID:             traceID,
			WorkIDs:             []string{workID},
		},
	}
}

func replayTestCompletion(_ string, dispatchID string, transitionID string, _ int) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}
}

func contentURLForTestPath(url string, file string) string {
	if strings.TrimSpace(url) != "" {
		return url
	}
	return "file://" + file
}

func resourceReplayToken(id string) workers.Token {
	return workers.Token{
		ID:    id,
		State: "available",
		Color: workers.Color{
			DataType: workers.DataTypeResource,
			Name:     "executor-slot",
		},
	}
}

func TestSubmissionHook_CancellationDoesNotConsumeDueSubmission(t *testing.T) {
	hook, err := NewSubmissionHook(testFactorySnapshotDecoder, testRuntimeConfigDecoder, testReplayArtifact(t, replayWorkRequestEvent(t, "request-canceled", 1, "api", []factoryapi.Work{{
		Name:         "work-canceled",
		WorkId:       stringPtrIfNotEmpty("work-canceled"),
		RequestId:    stringPtrIfNotEmpty("request-canceled"),
		WorkTypeName: stringPtrIfNotEmpty("task"),
	}}, nil)))
	if err != nil {
		t.Fatalf("NewSubmissionHook: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hook.OnTick(canceled, replaySubmissionHookContext(1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("OnTick canceled error = %v, want context.Canceled", err)
	}

	got, err := hook.OnTick(context.Background(), replaySubmissionHookContext(1))
	if err != nil {
		t.Fatalf("OnTick after cancellation: %v", err)
	}
	assertSubmissionHookDueTickBatchCount(t, got, 1)
	assertSubmissionHookReplayedRequestID(t, got.GeneratedBatches[0], "request-canceled")
}

func TestWorkStateChangeHook_CancellationDoesNotConsumeDueMove(t *testing.T) {
	hook, err := NewWorkStateChangeHook(testFactorySnapshotDecoder, testRuntimeConfigDecoder, testReplayArtifact(
		t,
		replayWorkStateChangeEvent(t, "work-canceled", "failed", "init", "task:failed", "task:init", factoryapi.WorkStateChangeSourceAPI, 1),
	))
	if err != nil {
		t.Fatalf("NewWorkStateChangeHook: %v", err)
	}
	input := replaySubmissionHookContextWithWorkToken(1, "work-canceled", "token-work-canceled", "task:failed")

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hook.OnTick(canceled, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("OnTick canceled error = %v, want context.Canceled", err)
	}

	got, err := hook.OnTick(context.Background(), input)
	if err != nil {
		t.Fatalf("OnTick after cancellation: %v", err)
	}
	if len(got.MarkingMutations) != 1 || got.MarkingMutations[0].TokenID != "token-work-canceled" {
		t.Fatalf("marking mutations after cancellation = %#v, want retained move", got.MarkingMutations)
	}
}
