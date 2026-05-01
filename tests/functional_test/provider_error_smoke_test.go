package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/factory"
	"github.com/portpowered/agent-factory/pkg/factory/state"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/petri"
	"github.com/portpowered/agent-factory/pkg/service"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
)

// portos:func-length-exception owner=agent-factory reason=legacy-provider-throttle-observability-smoke review=2026-07-19 removal=split-runtime-snapshot-dashboard-and-event-assertions-before-next-provider-error-smoke-change
func TestProviderErrorSmoke_ThrottlePauseObservabilityFlowsThroughRuntimeSnapshotAndDashboard(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow throttle-pause observability smoke")
	pauseDuration := 2 * time.Second
	pauseHarness := testutil.NewProviderErrorSmokePauseIsolationHarness(
		t,
		testutil.ProviderErrorSmokeLane{
			WorkTypeID:      "claude-task",
			WorkerName:      "claude-worker",
			WorkstationName: "process-claude",
			Provider:        workers.ModelProviderClaude,
			Model:           "claude-sonnet-4-5-20250514",
			PromptBody:      "Process the Claude lane task.\n",
		},
		testutil.ProviderErrorSmokeLane{
			WorkTypeID:      "codex-task",
			WorkerName:      "codex-worker",
			WorkstationName: "process-codex",
			Provider:        workers.ModelProviderCodex,
			Model:           "gpt-5-codex",
			PromptBody:      "Process the Codex lane task.\n",
		},
	)
	runner := pauseHarness.ProviderRunner()
	pauseHarness.QueueProviderResults(
		providerErrorCorpusEntryForTest(t, "claude_rate_limit_error").RepeatedCommandResults(3)...,
	)
	pauseHarness.QueueProviderResults(
		workers.CommandResult{Stdout: []byte("codex lane completed while claude was paused. COMPLETE")},
		workers.CommandResult{Stdout: []byte("claude lane recovered after pause expiry. COMPLETE")},
		workers.CommandResult{Stdout: []byte("codex reconciliation lane completed. COMPLETE")},
	)

	throttledWork := testutil.ProviderErrorSmokeWork{
		Name:       "claude-observable-throttle-lane",
		WorkID:     "work-claude-observable-throttle-lane",
		WorkTypeID: "claude-task",
		TraceID:    "trace-claude-observable-throttle-lane",
		Payload:    []byte("claude observable throttle payload"),
	}
	unaffectedWork := testutil.ProviderErrorSmokeWork{
		Name:       "codex-observable-healthy-lane",
		WorkID:     "work-codex-observable-healthy-lane",
		WorkTypeID: "codex-task",
		TraceID:    "trace-codex-observable-healthy-lane",
		Payload:    []byte("codex observable healthy payload"),
	}
	reconcileWork := testutil.ProviderErrorSmokeWork{
		Name:       "codex-reconcile-after-pause-expiry",
		WorkID:     "work-codex-reconcile-after-pause-expiry",
		WorkTypeID: "codex-task",
		TraceID:    "trace-codex-reconcile-after-pause-expiry",
		Payload:    []byte("codex reconciliation payload"),
	}
	pauseHarness.SeedWork(t, throttledWork)

	server := StartFunctionalServerWithConfig(
		t,
		pauseHarness.Dir,
		false,
		func(cfg *service.FactoryServiceConfig) {
			cfg.ProviderCommandRunnerOverride = runner
		},
		factory.WithServiceMode(),
		factory.WithProviderThrottlePauseDuration(pauseDuration),
	)

	activeEngineState := waitForEngineStateSnapshot(
		t,
		server,
		10*time.Second,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			return len(snapshot.ActiveThrottlePauses) == 1 &&
				hasWorkTokenInPlace(snapshot.Marking, throttledWork.WorkTypeID+":init", throttledWork.WorkID)
		},
	)
	assertActiveThrottlePause(t, activeEngineState, workers.ModelProviderClaude, "claude-sonnet-4-5-20250514")

	activeDashboard := server.GetDashboard(t)
	assertDashboardThrottlePausesMatchEngineState(t, "active pause dashboard", activeEngineState, activeDashboard)

	server.SubmitRuntimeWork(t, interfaces.SubmitRequest{
		Name:       unaffectedWork.Name,
		WorkID:     unaffectedWork.WorkID,
		WorkTypeID: unaffectedWork.WorkTypeID,
		TraceID:    unaffectedWork.TraceID,
		Payload:    unaffectedWork.Payload,
	})

	isolatedEngineState := waitForEngineStateSnapshot(
		t,
		server,
		5*time.Second,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			return len(snapshot.ActiveThrottlePauses) == 1 &&
				hasWorkTokenInPlace(snapshot.Marking, throttledWork.WorkTypeID+":init", throttledWork.WorkID) &&
				hasWorkTokenInPlace(snapshot.Marking, unaffectedWork.WorkTypeID+":complete", unaffectedWork.WorkID)
		},
	)
	assertDashboardThrottlePausesMatchEngineState(t, "pause isolation dashboard", isolatedEngineState, server.GetDashboard(t))

	if wait := time.Until(activeEngineState.ActiveThrottlePauses[0].PausedUntil.Add(100 * time.Millisecond)); wait > 0 {
		time.Sleep(wait)
	}
	server.SubmitRuntimeWork(t, interfaces.SubmitRequest{
		Name:       reconcileWork.Name,
		WorkID:     reconcileWork.WorkID,
		WorkTypeID: reconcileWork.WorkTypeID,
		TraceID:    reconcileWork.TraceID,
		Payload:    reconcileWork.Payload,
	})

	recoveredEngineState := waitForEngineStateSnapshot(
		t,
		server,
		10*time.Second,
		func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
			return len(snapshot.ActiveThrottlePauses) == 0 &&
				hasWorkTokenInPlace(snapshot.Marking, throttledWork.WorkTypeID+":complete", throttledWork.WorkID) &&
				hasWorkTokenInPlace(snapshot.Marking, unaffectedWork.WorkTypeID+":complete", unaffectedWork.WorkID) &&
				hasWorkTokenInPlace(snapshot.Marking, reconcileWork.WorkTypeID+":complete", reconcileWork.WorkID)
		},
	)
	recoveredDashboard := waitForDashboardSnapshot(
		t,
		5*time.Second,
		func() (DashboardResponse, bool) {
			dashboard := server.GetDashboard(t)
			return dashboard, len(sliceValue(dashboard.Runtime.ActiveThrottlePauses)) == 0 &&
				dashboard.Runtime.InFlightDispatchCount == 0 &&
				dashboard.Runtime.Session.CompletedCount >= 3
		},
	)
	assertDashboardMatchesEngineState(t, "recovered dashboard", recoveredEngineState, recoveredDashboard)

	requests := runner.Requests()
	if len(requests) < 4 {
		t.Fatalf("provider command count = %d, want at least 4", len(requests))
	}
	for i := 0; i < 3; i++ {
		if requests[i].Command != string(workers.ModelProviderClaude) {
			t.Fatalf("request %d command = %q, want %q", i, requests[i].Command, workers.ModelProviderClaude)
		}
	}
	if requests[3].Command != string(workers.ModelProviderCodex) {
		t.Fatalf("request 3 command = %q, want %q", requests[3].Command, workers.ModelProviderCodex)
	}

	throttledDispatches := dispatchesForProviderSmokeWork(recoveredEngineState.DispatchHistory, throttledWork)
	unaffectedDispatches := dispatchesForProviderSmokeWork(recoveredEngineState.DispatchHistory, unaffectedWork)
	if len(throttledDispatches) == 0 {
		t.Fatal("throttled lane dispatch count = 0, want at least one failed dispatch")
	}
	if len(unaffectedDispatches) != 1 {
		t.Fatalf("unaffected lane dispatch count = %d, want 1", len(unaffectedDispatches))
	}
	if throttledDispatches[0].Outcome != interfaces.OutcomeFailed {
		t.Fatalf("first throttled dispatch outcome = %s, want %s", throttledDispatches[0].Outcome, interfaces.OutcomeFailed)
	}
	if len(throttledDispatches) > 1 && throttledDispatches[1].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("second throttled dispatch outcome = %s, want %s", throttledDispatches[1].Outcome, interfaces.OutcomeAccepted)
	}
	if unaffectedDispatches[0].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("unaffected dispatch outcome = %s, want %s", unaffectedDispatches[0].Outcome, interfaces.OutcomeAccepted)
	}
}

func assertActiveThrottlePause(
	t *testing.T,
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	provider workers.ModelProvider,
	model string,
) {
	t.Helper()

	if engineState == nil {
		t.Fatal("engine state is nil")
	}
	if len(engineState.ActiveThrottlePauses) != 1 {
		t.Fatalf("active throttle pauses = %d, want 1", len(engineState.ActiveThrottlePauses))
	}
	pause := engineState.ActiveThrottlePauses[0]
	if pause.Provider != string(provider) || pause.Model != model {
		t.Fatalf("active throttle pause lane = %s/%s, want %s/%s", pause.Provider, pause.Model, provider, model)
	}
	if pause.LaneID != string(provider)+"/"+model {
		t.Fatalf("active throttle pause LaneID = %q, want %q", pause.LaneID, string(provider)+"/"+model)
	}
	if pause.PausedAt.IsZero() {
		t.Fatal("active throttle pause PausedAt is zero")
	}
	if !pause.PausedUntil.After(pause.PausedAt) {
		t.Fatalf("active throttle pause PausedUntil = %s, want after PausedAt %s", pause.PausedUntil, pause.PausedAt)
	}
}

func dispatchesForProviderSmokeWork(
	history []interfaces.CompletedDispatch,
	work testutil.ProviderErrorSmokeWork,
) []interfaces.CompletedDispatch {
	dispatches := make([]interfaces.CompletedDispatch, 0, len(history))
	for _, dispatch := range history {
		for _, token := range dispatch.ConsumedTokens {
			if token.Color.WorkID == work.WorkID {
				dispatches = append(dispatches, dispatch)
				break
			}
		}
	}
	return dispatches
}

func assertDashboardThrottlePausesMatchEngineState(
	t *testing.T,
	label string,
	engineState *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
	dashboard DashboardResponse,
) {
	t.Helper()

	pauses := sliceValue(dashboard.Runtime.ActiveThrottlePauses)
	if len(pauses) != len(engineState.ActiveThrottlePauses) {
		t.Fatalf(
			"%s active throttle pause count = %d, want engine-state count %d",
			label,
			len(pauses),
			len(engineState.ActiveThrottlePauses),
		)
	}
	for i, pause := range pauses {
		enginePause := engineState.ActiveThrottlePauses[i]
		if pause.LaneId != enginePause.LaneID || pause.Provider != enginePause.Provider || pause.Model != enginePause.Model {
			t.Fatalf("%s pause[%d] identity = %#v, want engine pause %#v", label, i, pause, enginePause)
		}
		if pause.PausedAt == nil || !pause.PausedAt.Equal(enginePause.PausedAt) {
			t.Fatalf("%s pause[%d] PausedAt = %#v, want %s", label, i, pause.PausedAt, enginePause.PausedAt)
		}
		if !pause.PausedUntil.Equal(enginePause.PausedUntil) || !pause.RecoverAt.Equal(enginePause.PausedUntil) {
			t.Fatalf("%s pause[%d] window = %#v, want paused-until/recover-at %s", label, i, pause, enginePause.PausedUntil)
		}
		if pause.AffectedTransitionIds == nil || len(*pause.AffectedTransitionIds) == 0 {
			t.Fatalf("%s pause[%d] affected transitions = %#v, want non-empty projection", label, i, pause.AffectedTransitionIds)
		}
		if pause.AffectedWorkstationNames == nil || len(*pause.AffectedWorkstationNames) == 0 {
			t.Fatalf("%s pause[%d] affected workstation names = %#v, want non-empty projection", label, i, pause.AffectedWorkstationNames)
		}
		if pause.AffectedWorkerTypes == nil || len(*pause.AffectedWorkerTypes) == 0 {
			t.Fatalf("%s pause[%d] affected worker types = %#v, want non-empty projection", label, i, pause.AffectedWorkerTypes)
		}
		if pause.AffectedWorkTypeIds == nil || len(*pause.AffectedWorkTypeIds) == 0 {
			t.Fatalf("%s pause[%d] affected work type IDs = %#v, want non-empty projection", label, i, pause.AffectedWorkTypeIds)
		}
	}
}
