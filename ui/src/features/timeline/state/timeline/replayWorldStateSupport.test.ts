import { describe, expect, it } from "vitest";

import type { DashboardInferenceAttempt } from "../../../../api/dashboard";
import {
  applyScriptRequest,
  applyScriptResponse,
  inferenceAttemptsForDispatch,
  resolveDispatchTransitionID,
  scriptRequestsForDispatch,
  scriptResponsesForDispatch,
  syncCompletedDispatchAttempt,
} from "./replayWorldStateSupport";
import { emptyWorldRuntime, type ReplayWorldState } from "./types";

function emptyState(): ReplayWorldState {
  return {
    activeDispatches: {},
    completedDispatches: [],
    factory_state: "RUNNING",
    failedWorkDetailsByWorkID: {},
    failedWorkItemsByID: {},
    inferenceAttemptsByDispatchID: {},
    occupancyByID: {},
    providerSessions: [],
    relationsByWorkID: {},
    runtime: emptyWorldRuntime(),
    scriptRequestsByDispatchID: {},
    scriptResponsesByDispatchID: {},
    terminalWorkByID: {},
    tick_count: 0,
    topology: {},
    tracesByID: {},
    tracesByWorkID: {},
    uptime_seconds: 0,
    workItemsByID: {},
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

describe("replayWorldStateSupport inference helpers", () => {
  it("creates and reuses inference attempt maps per dispatch", () => {
    const state = emptyState();
    const first = inferenceAttemptsForDispatch(state, "dispatch-1");
    const second = inferenceAttemptsForDispatch(state, "dispatch-1");
    expect(first).toBe(second);
    first["attempt-1"] = {
      dispatch_id: "dispatch-1",
      inference_request_id: "attempt-1",
      prompt: "hello",
      request_time: "2026-05-30T12:00:00.000Z",
      transition_id: "review",
    };
    expect(state.inferenceAttemptsByDispatchID["dispatch-1"]).toBe(first);
  });

  it("resolves transition ids from active and completed dispatches", () => {
    const state = emptyState();
    state.activeDispatches["dispatch-active"] = {
      dispatchID: "dispatch-active",
      transitionID: "active-transition",
      traceIDs: [],
      workIDs: [],
    };
    state.completedDispatches.push({
      dispatchID: "dispatch-completed",
      transitionID: "completed-transition",
      traceIDs: ["trace-1"],
      workIDs: ["work-1"],
    });
    expect(resolveDispatchTransitionID(state, "dispatch-active")).toBe(
      "active-transition",
    );
    expect(resolveDispatchTransitionID(state, "dispatch-completed")).toBe(
      "completed-transition",
    );
    expect(resolveDispatchTransitionID(state, "missing")).toBeUndefined();
  });

  it("skips non-matching completed dispatches when syncing inference", () => {
    const state = emptyState();
    state.completedDispatches.push(
      {
        consumedTokens: [],
        dispatchID: "dispatch-other",
        diagnostics: {},
        durationMillis: 1,
        endTime: "2026-05-30T12:00:01.000Z",
        inputItems: [],
        outcome: "SUCCEEDED",
        outputItems: [],
        outputMutations: [],
        resources: [],
        startedAt: "2026-05-30T12:00:00.000Z",
        systemOnly: false,
        traceIDs: [],
        transitionID: "other",
        workIDs: [],
        workItems: [],
      },
      {
        consumedTokens: [],
        dispatchID: "dispatch-target",
        diagnostics: { model: "before" },
        durationMillis: 1,
        endTime: "2026-05-30T12:00:02.000Z",
        inputItems: [],
        outcome: "SUCCEEDED",
        outputItems: [],
        outputMutations: [],
        resources: [],
        startedAt: "2026-05-30T12:00:01.000Z",
        systemOnly: false,
        traceIDs: [],
        transitionID: "review",
        workIDs: [],
        workItems: [],
      },
    );
    syncCompletedDispatchAttempt(state, "dispatch-target", {
      diagnostics: { model: "after" },
      dispatch_id: "dispatch-target",
      inference_request_id: "attempt-1",
      prompt: "p",
      request_time: "2026-05-30T12:00:00.000Z",
      transition_id: "review",
    });
    expect(state.completedDispatches[0]?.diagnostics).toEqual({});
    expect(state.completedDispatches[1]?.diagnostics).toEqual({ model: "after" });
  });

  it("does not duplicate provider sessions already recorded on the state", () => {
    const state = emptyState();
    state.completedDispatches.push({
      consumedTokens: [],
      dispatchID: "dispatch-1",
      diagnostics: {},
      durationMillis: 1,
      endTime: "2026-05-30T12:00:01.000Z",
      inputItems: [],
      outcome: "SUCCEEDED",
      outputItems: [],
      outputMutations: [],
      providerSession: {
        id: "session-1",
        kind: "session_id",
        provider: "openai",
      },
      resources: [],
      startedAt: "2026-05-30T12:00:00.000Z",
      systemOnly: false,
      traceIDs: [],
      transitionID: "review",
      workIDs: [],
      workItems: [],
    });
    state.providerSessions = [
      {
        dispatch_id: "dispatch-1",
        provider_session: {
          id: "session-1",
          kind: "session_id",
          provider: "openai",
        },
      },
    ];
    syncCompletedDispatchAttempt(state, "dispatch-1", {
      dispatch_id: "dispatch-1",
      inference_request_id: "attempt-1",
      prompt: "p",
      provider_session: {
        id: "session-1",
        kind: "session_id",
        provider: "openai",
      },
      request_time: "2026-05-30T12:00:00.000Z",
      transition_id: "review",
    });
    expect(state.providerSessions).toHaveLength(1);
  });

  it("syncs completed dispatch diagnostics and provider sessions from inference", () => {
    const state = emptyState();
    state.completedDispatches.push({
      consumedTokens: [],
      dispatchID: "dispatch-1",
      diagnostics: { model: "gpt-5.4" },
      durationMillis: 100,
      endTime: "2026-05-30T12:00:01.000Z",
      inputItems: [],
      outcome: "SUCCEEDED",
      outputItems: [],
      outputMutations: [],
      providerSession: {
        id: "session-1",
        kind: "session_id",
        provider: "openai",
      },
      resources: [],
      startedAt: "2026-05-30T12:00:00.000Z",
      systemOnly: false,
      traceIDs: ["trace-1"],
      transitionID: "review",
      workIDs: ["work-1"],
      workItems: [],
    });
    state.tracesByID["trace-1"] = {
      dispatches: [
        {
          dispatch_id: "dispatch-1",
          transition_id: "review",
        },
      ],
      trace_id: "trace-1",
    };
    const attempt: DashboardInferenceAttempt = {
      diagnostics: { tokens: 42 },
      dispatch_id: "dispatch-1",
      inference_request_id: "dispatch-1/inference-request/1",
      prompt: "Review",
      provider_session: {
        id: "session-1",
        kind: "session_id",
        provider: "openai",
      },
      request_time: "2026-05-30T12:00:00.000Z",
      transition_id: "review",
    };
    syncCompletedDispatchAttempt(state, "dispatch-1", attempt);
    expect(state.completedDispatches[0]?.diagnostics).toEqual({ tokens: 42 });
    expect(state.tracesByID["trace-1"]?.dispatches[0]).toMatchObject({
      dispatch_id: "dispatch-1",
      transition_id: "review",
    });
  });

  it("ignores missing traces when syncing completed dispatch attempts", () => {
    const state = emptyState();
    state.completedDispatches.push({
      consumedTokens: [],
      dispatchID: "dispatch-1",
      diagnostics: {},
      durationMillis: 1,
      endTime: "2026-05-30T12:00:01.000Z",
      inputItems: [],
      outcome: "SUCCEEDED",
      outputItems: [],
      outputMutations: [],
      resources: [],
      startedAt: "2026-05-30T12:00:00.000Z",
      systemOnly: false,
      traceIDs: ["missing-trace"],
      transitionID: "review",
      workIDs: [],
      workItems: [],
    });
    syncCompletedDispatchAttempt(state, "dispatch-1", {
      diagnostics: { tokens: 1 },
      dispatch_id: "dispatch-1",
      inference_request_id: "attempt-1",
      prompt: "p",
      request_time: "2026-05-30T12:00:00.000Z",
      transition_id: "review",
    });
    expect(state.completedDispatches[0]?.diagnostics).toEqual({ tokens: 1 });
  });
});

describe("replayWorldStateSupport script helpers", () => {
  it("reuses existing script response maps per dispatch", () => {
    const state = emptyState();
    const first = scriptResponsesForDispatch(state, "dispatch-script");
    const second = scriptResponsesForDispatch(state, "dispatch-script");
    expect(first).toBe(second);
  });

  it("records script requests and responses per dispatch", () => {
    const state = emptyState();
    applyScriptRequest(state, {
      context: { eventTime: "2026-05-30T12:00:00.000Z" },
      payload: {
        args: ["--flag"],
        attempt: 1,
        command: "tool",
        dispatchId: "dispatch-script",
        scriptRequestId: "dispatch-script/script-request/1",
        transitionId: "review",
      },
    });
    applyScriptResponse(state, {
      context: { eventTime: "2026-05-30T12:00:01.000Z" },
      payload: {
        attempt: 1,
        dispatchId: "dispatch-script",
        durationMillis: 120,
        exitCode: 0,
        outcome: "SUCCEEDED",
        scriptRequestId: "dispatch-script/script-request/1",
        stderr: "",
        stdout: "ok\n",
        transitionId: "review",
      },
    });
    const requests = scriptRequestsForDispatch(state, "dispatch-script");
    expect(requests["dispatch-script/script-request/1"]).toMatchObject({
      command: "tool",
      dispatch_id: "dispatch-script",
    });
    expect(
      state.scriptResponsesByDispatchID["dispatch-script"]?.[
        "dispatch-script/script-request/1"
      ],
    ).toMatchObject({
      outcome: "SUCCEEDED",
      stdout: "ok\n",
    });
  });

  it("ignores script events without dispatch or request identifiers", () => {
    const state = emptyState();
    applyScriptRequest(state, {
      context: { eventTime: "2026-05-30T12:00:00.000Z" },
      payload: {
        args: [],
        attempt: 1,
        command: "tool",
        transitionId: "review",
      },
    });
    applyScriptResponse(state, {
      context: { eventTime: "2026-05-30T12:00:01.000Z" },
      payload: {
        attempt: 1,
        durationMillis: 1,
        outcome: "FAILED",
        stderr: "err",
        stdout: "",
        transitionId: "review",
      },
    });
    expect(state.scriptRequestsByDispatchID).toEqual({});
    expect(state.scriptResponsesByDispatchID).toEqual({});
  });
});
