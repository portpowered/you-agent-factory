package replay

import (
	"context"
	"errors"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// pkgmaintcheck:ignore-cyclomatic-complexity this replay hook test keeps the tick-ordered work request event contract readable in one end-to-end flow.
func TestSubmissionHook_ReplaysWorkRequestEventsByTick(t *testing.T) {
	hook, err := NewSubmissionHook(testReplayArtifact(t, replayWorkRequestEvent(t, "request-1", 2, "api", []factoryapi.Work{{
		Name:         "work-1",
		WorkId:       stringPtrIfNotEmpty("work-1"),
		RequestId:    stringPtrIfNotEmpty("request-1"),
		WorkTypeName: stringPtrIfNotEmpty("task"),
		TraceId:      stringPtrIfNotEmpty("trace-1"),
		Content: replayWorkContentForDeliveryTest(t, []interfaces.WorkContentPart{
			{Type: interfaces.WorkContentPartTypeText, Text: "alpha"},
			{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/alpha.png"},
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
	if len(before.GeneratedBatches) != 0 {
		t.Fatalf("generated batches before due tick = %d, want 0", len(before.GeneratedBatches))
	}
	if !before.KeepAlive {
		t.Fatal("before due tick KeepAlive = false, want true while future submissions remain")
	}

	due, err := hook.OnTick(context.Background(), replaySubmissionHookContext(2))
	if err != nil {
		t.Fatalf("OnTick at due tick: %v", err)
	}
	if len(due.GeneratedBatches) != 1 {
		t.Fatalf("generated batches at due tick = %d, want 1", len(due.GeneratedBatches))
	}
	batch := due.GeneratedBatches[0]
	if batch.Request.RequestID != "request-1" {
		t.Fatalf("replayed request ID = %q, want request-1", batch.Request.RequestID)
	}
	if len(batch.Request.Works) != 2 {
		t.Fatalf("replayed batch works = %d, want 2", len(batch.Request.Works))
	}
	if batch.Request.Works[0].WorkID != "work-1" || batch.Metadata.Source != "api" {
		t.Fatalf("replayed generated batch = %#v, want work-1 from api", batch)
	}
	if got := batch.Request.Works[0].Content; len(got) != 2 || got[0].Text != "alpha" || got[1].File != "fixtures/alpha.png" {
		t.Fatalf("replayed content = %#v, want ordered text and image parts", got)
	}
	if len(batch.Request.Relations) != 1 || batch.Request.Relations[0].SourceWorkName != "work-2" || batch.Request.Relations[0].TargetWorkName != "work-1" {
		t.Fatalf("replayed relations = %#v, want work-2 depends on work-1", batch.Request.Relations)
	}
	if due.KeepAlive {
		t.Fatal("due tick KeepAlive = true, want false after last submission is emitted")
	}
}

func replayWorkContentForDeliveryTest(t *testing.T, parts []interfaces.WorkContentPart) *factoryapi.WorkContent {
	t.Helper()
	content := make(factoryapi.WorkContent, 0, len(parts))
	for _, part := range parts {
		var generated factoryapi.WorkContentPart
		switch part.Type {
		case interfaces.WorkContentPartTypeText:
			if err := generated.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
				Type: factoryapi.WorkContentPartTypeText,
				Text: part.Text,
			}); err != nil {
				t.Fatalf("encode text content: %v", err)
			}
		case interfaces.WorkContentPartTypeImage:
			if err := generated.FromWorkImageContentPart(factoryapi.WorkImageContentPart{
				Type: factoryapi.WorkContentPartTypeImage,
				File: part.File,
			}); err != nil {
				t.Fatalf("encode image content: %v", err)
			}
		}
		content = append(content, generated)
	}
	return &content
}

func TestSubmissionHook_ReplaysCronTimeWorkRequestWithPendingTargetState(t *testing.T) {
	hook, err := NewSubmissionHook(testReplayArtifact(t, replayWorkRequestEvent(t, "request-cron", 1, "external-submit", []factoryapi.Work{{
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
	hook, err := NewSubmissionHook(testReplayArtifact(t,
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
	plan, err := NewCompletionDeliveryPlan(deliveryArtifact(t,
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
	plan, err := NewCompletionDeliveryPlan(testReplayArtifact(t,
		replayDispatchCreatedEvent(t, replayTestDispatch("dispatch-no-completion", "process", 2, "trace-1", "work-1", "tok-1"), 2),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	if err := plan.ValidateReplayTick(3); err != nil {
		t.Fatalf("missing-completion dispatch should not force replay divergence after repair: %v", err)
	}
}

func TestCompletionDeliveryPlan_DispatchIdentityMismatchReportsDivergence(t *testing.T) {
	plan, err := NewCompletionDeliveryPlan(deliveryArtifact(t,
		replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1"),
		replayTestCompletion("completion-1", "dispatch-1", "process", 3),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	_, _, err = plan.DeliveryTickForDispatch(interfaces.WorkDispatch{
		DispatchID:      "observed-dispatch",
		TransitionID:    "review",
		WorkstationName: "review",
		InputTokens:     workers.InputTokens(interfaces.Token{ID: "tok-1"}),
		Execution: interfaces.ExecutionMetadata{
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
	plan, err := NewCompletionDeliveryPlan(deliveryArtifact(t,
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
	plan, err := NewCompletionDeliveryPlan(deliveryArtifact(t,
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

func TestCompletionDeliveryPlan_PlannedResultClonesProviderMetadata(t *testing.T) {
	completion := replayTestCompletion("completion-1", "dispatch-1", "process", 3)
	completion.ProviderFailure = &interfaces.ProviderFailureMetadata{
		Family: interfaces.ProviderErrorFamilyRetryable,
		Type:   interfaces.ProviderErrorTypeInternalServerError,
	}

	plan, err := NewCompletionDeliveryPlan(deliveryArtifact(t,
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

	completion.ProviderFailure.Type = interfaces.ProviderErrorTypeAuthFailure

	planned, ok, err := plan.PlannedResultForDispatch(observed)
	if err != nil {
		t.Fatalf("PlannedResultForDispatch: %v", err)
	}
	if !ok {
		t.Fatal("expected planned result for observed dispatch")
	}
	if planned.ProviderFailure == nil || planned.ProviderFailure.Type != interfaces.ProviderErrorTypeInternalServerError {
		t.Fatalf("planned provider failure = %#v, want detached internal_server_error metadata", planned.ProviderFailure)
	}
}

func TestCompletionDeliveryPlan_LineageMismatchReportsDivergence(t *testing.T) {
	plan, err := NewCompletionDeliveryPlan(deliveryArtifact(t,
		replayTestDispatch("dispatch-1", "process", 2, "trace-1", "work-1", "tok-1"),
		replayTestCompletion("completion-1", "dispatch-1", "process", 3),
	))
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}

	_, _, err = plan.DeliveryTickForDispatch(interfaces.WorkDispatch{
		DispatchID:      "observed-dispatch",
		TransitionID:    "process",
		WorkstationName: "process",
		InputTokens:     workers.InputTokens(interfaces.Token{ID: "tok-different"}),
		Execution: interfaces.ExecutionMetadata{
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
		interfaces.Token{ID: "tok-1"},
	)
	plan, err := NewCompletionDeliveryPlan(deliveryArtifact(t,
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
		interfaces.Token{ID: "tok-1"},
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
	_, err := NewCompletionDeliveryPlan(deliveryArtifact(t,
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

	warnings := FactoryMetadataWarnings(artifactConfig, currentConfig)
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

func deliveryArtifact(t *testing.T, dispatch interfaces.WorkDispatch, completion interfaces.WorkResult) *interfaces.ReplayArtifact {
	t.Helper()
	return testReplayArtifact(
		t,
		replayDispatchCreatedEvent(t, dispatch, dispatch.Execution.DispatchCreatedTick),
		replayDispatchCompletedEvent(t, "completion-1", completion, 3),
	)
}

func replaySubmissionHookContext(tick int) interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]] {
	return interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]{
		Snapshot: interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			TickCount: tick,
		},
	}
}

func replaySubmissionHookContextWithWorkToken(tick int, workID, tokenID, placeID string) interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]] {
	ctx := replaySubmissionHookContext(tick)
	ctx.Snapshot.Marking.Tokens = map[string]*interfaces.Token{
		tokenID: {
			ID:      tokenID,
			PlaceID: placeID,
			Color: interfaces.TokenColor{
				WorkID:     workID,
				WorkTypeID: "task",
				DataType:   interfaces.DataTypeWork,
			},
		},
	}
	return ctx
}

func TestWorkStateChangeHook_ReplaysOperatorMoveAtRecordedTick(t *testing.T) {
	hook, err := NewWorkStateChangeHook(testReplayArtifact(
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

func replayTestDispatch(dispatchID, transitionID string, tick int, traceID, workID, tokenID string) interfaces.WorkDispatch {
	return interfaces.WorkDispatch{
		DispatchID:      dispatchID,
		TransitionID:    transitionID,
		WorkstationName: transitionID,
		InputTokens:     workers.InputTokens(interfaces.Token{ID: tokenID}),
		Execution: interfaces.ExecutionMetadata{
			DispatchCreatedTick: tick,
			ReplayKey:           transitionID + "/" + traceID + "/" + workID,
			TraceID:             traceID,
			WorkIDs:             []string{workID},
		},
	}
}

func replayTestCompletion(_ string, dispatchID string, transitionID string, _ int) interfaces.WorkResult {
	return interfaces.WorkResult{
		DispatchID:   dispatchID,
		TransitionID: transitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}
}

func resourceReplayToken(id string) interfaces.Token {
	return interfaces.Token{
		ID:      id,
		PlaceID: "executor-slot:available",
		Color: interfaces.TokenColor{
			DataType: interfaces.DataTypeResource,
			Name:     "executor-slot",
		},
	}
}
