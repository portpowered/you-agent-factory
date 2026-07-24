// biome-ignore lint/style/noExcessiveLinesPerFile: session lifecycle replay cases share lifecycleEvent helper and replay harness.
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { reconstructFactoryReplayState } from "./buildSnapshot";
import { applyDispatchLifecycleEvent } from "./replayDispatchLifecycle";
import { applyOrchestratorProgressEvent } from "./replayOrchestratorProgress";
import { emptyReplayWorldState } from "./replayWorldStateSupport";

function lifecycleEvent(
  type: string,
  id: string,
  sequence: number,
  tick: number,
  payload: Record<string, unknown>,
): Parameters<typeof reconstructFactoryReplayState>[0][number] {
  return {
    context: {
      eventTime: "2026-06-09T12:00:00Z",
      orchestratorKind: "JAVASCRIPT",
      sequence,
      sessionId: "session-alpha",
      sessionSequence: sequence,
      tick,
    },
    id,
    payload,
    type,
  };
}

describe("reconstructWorldState session lifecycle replay", () => {
  it("reconstructs started, partial, and terminal session bracket state", () => {
    const events = [
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionStarted, "started", 1, 1, {
        factoryId: "factory-alpha",
        sourceRef: "workflow/main.js",
        startedAt: "2026-06-09T12:00:00Z",
      }),
      lifecycleEvent(
        FACTORY_EVENT_TYPES.sessionResultUpdated,
        "partial",
        2,
        2,
        {
          artifactIds: ["artifact-partial"],
          resultStatus: "PARTIAL",
        },
      ),
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionCompleted, "completed", 3, 3, {
        artifactIds: ["artifact-final"],
        completedAt: "2026-06-09T12:00:05Z",
        dispatchCounts: { completed: 2, queued: 0, running: 0 },
        durationMillis: 5000,
        finalStatus: "SUCCEEDED",
        resultStatus: "FINAL",
      }),
    ];

    const state = reconstructFactoryReplayState(events, 3);
    expect(state.sessionBracket).toMatchObject({
      factory_id: "factory-alpha",
      result_status: "FINAL",
      session_id: "session-alpha",
      source_ref: "workflow/main.js",
      terminal: true,
    });
    expect(state.sessionBracket?.artifact_ids).toEqual(["artifact-final"]);
  });

  it("reconstructs paused and resumed lifecycle control status from canonical events", () => {
    const events = [
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionStarted, "started", 1, 1, {
        factoryId: "factory-alpha",
        sourceRef: "workflow/main.js",
        startedAt: "2026-06-09T12:00:00Z",
      }),
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionPaused, "paused", 2, 2, {
        pausedAt: "2026-06-09T12:00:02Z",
        status: "PAUSED",
      }),
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionResumed, "resumed", 3, 3, {
        resumedAt: "2026-06-09T12:00:04Z",
        status: "RUNNING",
      }),
    ];

    const pausedState = reconstructFactoryReplayState(events, 2);
    expect(pausedState.sessionBracket).toMatchObject({
      lifecycle_control_status: "PAUSED",
      paused_at: "2026-06-09T12:00:02Z",
      session_id: "session-alpha",
    });

    const runningState = reconstructFactoryReplayState(events, 3);
    expect(runningState.sessionBracket).toMatchObject({
      lifecycle_control_status: "RUNNING",
      resumed_at: "2026-06-09T12:00:04Z",
      session_id: "session-alpha",
    });
  });
});

describe("reconstructWorldState SESSION_LIFECYCLE_CONTROL replay", () => {
  it("reconstructs paused and resumed lifecycle control status from SESSION_LIFECYCLE_CONTROL events", () => {
    const events = [
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionStarted, "started", 1, 1, {
        factoryId: "factory-alpha",
        sourceRef: "workflow/main.js",
        startedAt: "2026-06-09T12:00:00Z",
      }),
      lifecycleEvent(
        FACTORY_EVENT_TYPES.sessionLifecycleControl,
        "session-lifecycle-control/session-alpha/2",
        2,
        2,
        {
          newStatus: "PAUSED",
          occurredAt: "2026-06-09T12:00:02Z",
          operation: "PAUSE",
          outcome: "ACCEPTED",
          previousStatus: "RUNNING",
        },
      ),
      lifecycleEvent(
        FACTORY_EVENT_TYPES.sessionLifecycleControl,
        "session-lifecycle-control/session-alpha/3",
        3,
        3,
        {
          newStatus: "RUNNING",
          occurredAt: "2026-06-09T12:00:04Z",
          operation: "RESUME",
          outcome: "ACCEPTED",
          previousStatus: "PAUSED",
        },
      ),
    ];

    const pausedState = reconstructFactoryReplayState(events, 2);
    expect(pausedState.sessionBracket).toMatchObject({
      lifecycle_control_status: "PAUSED",
      paused_at: "2026-06-09T12:00:02Z",
      session_id: "session-alpha",
    });

    const runningState = reconstructFactoryReplayState(events, 3);
    expect(runningState.sessionBracket).toMatchObject({
      lifecycle_control_status: "RUNNING",
      resumed_at: "2026-06-09T12:00:04Z",
      session_id: "session-alpha",
    });
  });

  it("ignores non-accepted SESSION_LIFECYCLE_CONTROL outcomes", () => {
    const events = [
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionStarted, "started", 1, 1, {
        factoryId: "factory-alpha",
        startedAt: "2026-06-09T12:00:00Z",
      }),
      lifecycleEvent(
        FACTORY_EVENT_TYPES.sessionLifecycleControl,
        "session-lifecycle-control/session-alpha/2",
        2,
        2,
        {
          newStatus: "PAUSED",
          occurredAt: "2026-06-09T12:00:02Z",
          operation: "PAUSE",
          outcome: "NO_OP",
          previousStatus: "PAUSED",
        },
      ),
    ];

    const state = reconstructFactoryReplayState(events, 2);
    expect(state.sessionBracket).toMatchObject({
      session_id: "session-alpha",
    });
    expect(state.sessionBracket?.lifecycle_control_status).toBeUndefined();
    expect(state.sessionBracket?.paused_at).toBeUndefined();
  });
});

describe("reconstructWorldState dispatch and artifact replay", () => {
  it("reconstructs orchestrator phase, dispatch lifecycle, and artifact events", () => {
    const events = [
      {
        context: {
          eventTime: "2026-06-09T12:00:01Z",
          phaseName: "review",
          sequence: 1,
          sessionSequence: 1,
          tick: 1,
        },
        id: "phase",
        payload: {
          phaseStatus: "ACTIVE",
          previousPhaseName: "plan",
        },
        type: FACTORY_EVENT_TYPES.orchestratorPhaseChanged,
      },
      {
        context: {
          dispatchId: "dispatch-1",
          eventTime: "2026-06-09T12:00:02Z",
          phaseName: "review",
          sequence: 2,
          sessionSequence: 2,
          tick: 2,
        },
        id: "queued",
        payload: {
          dispatchKind: "JAVASCRIPT_AGENT",
          label: "review child",
        },
        type: FACTORY_EVENT_TYPES.dispatchQueued,
      },
      {
        context: {
          dispatchId: "dispatch-1",
          eventTime: "2026-06-09T12:00:03Z",
          sequence: 3,
          sessionSequence: 3,
          tick: 3,
        },
        id: "reconciled",
        payload: {
          artifactIds: ["artifact-child"],
          reconciledStatus: "RECONCILED",
        },
        type: FACTORY_EVENT_TYPES.dispatchReconciled,
      },
      {
        context: {
          eventTime: "2026-06-09T12:00:04Z",
          sequence: 4,
          sessionSequence: 4,
          tick: 4,
        },
        id: "artifact",
        payload: {
          artifact: {
            captureMetadata: {
              mimeType: "application/json",
            },
            id: "artifact-child",
            kind: "CHILD_RESULT",
            label: "Review summary",
            visibility: "CUSTOMER",
          },
          capturedAt: "2026-06-09T12:00:04Z",
        },
        type: FACTORY_EVENT_TYPES.artifactCreated,
      },
    ];

    const state = reconstructFactoryReplayState(events, 4);
    expect(state.javascriptRuntime?.phase).toBe("review");
    expect(state.javascriptRuntime?.dispatches[0]).toMatchObject({
      id: "dispatch-1",
      status: "RECONCILED",
    });
    expect(state.sessionArtifacts).toEqual([
      expect.objectContaining({
        content_type: "application/json",
        id: "artifact-child",
        kind: "CHILD_RESULT",
        label: "Review summary",
        visibility: "CUSTOMER",
      }),
    ]);
  });

  it("maps artifact content type from contentType when captureMetadata mimeType is absent", () => {
    const events = [
      {
        context: {
          eventTime: "2026-06-09T12:00:05Z",
          sequence: 5,
          sessionSequence: 5,
          tick: 5,
        },
        id: "artifact-legacy-content-type",
        payload: {
          artifact: {
            contentType: "text/plain",
            id: "artifact-legacy-content-type",
            kind: "LOG",
            visibility: "OPERATOR",
          },
          capturedAt: "2026-06-09T12:00:05Z",
        },
        type: FACTORY_EVENT_TYPES.artifactCreated,
      },
    ];

    const state = reconstructFactoryReplayState(events, 5);
    expect(state.sessionArtifacts).toEqual([
      expect.objectContaining({
        content_type: "text/plain",
        id: "artifact-legacy-content-type",
        kind: "LOG",
        visibility: "OPERATOR",
      }),
    ]);
  });
});

describe("reconstructWorldState dispatch lifecycle bootstrap", () => {
  it("initializes javascript runtime when dispatch events arrive first", () => {
    const events = [
      {
        context: {
          dispatchId: "dispatch-bootstrap",
          eventTime: "2026-06-09T12:00:01Z",
          phaseId: "phase-1",
          sequence: 1,
          sessionSequence: 1,
          tick: 1,
        },
        id: "queued-bootstrap",
        payload: {
          dispatchKind: "JAVASCRIPT_AGENT",
          label: "bootstrap dispatch",
        },
        type: FACTORY_EVENT_TYPES.dispatchQueued,
      },
    ];

    const state = reconstructFactoryReplayState(events, 1);
    expect(state.javascriptRuntime?.dispatches).toEqual([
      expect.objectContaining({
        dispatch_kind: "JAVASCRIPT_AGENT",
        id: "dispatch-bootstrap",
        label: "bootstrap dispatch",
        phase: "phase-1",
        status: "QUEUED",
      }),
    ]);
    expect(state.javascriptRuntime?.child_dispatch_counts).toEqual({
      completed: 0,
      queued: 1,
      running: 0,
    });
  });
});

describe("reconstructWorldState dispatch interruption replay", () => {
  it("reconstructs checkpoint, interrupted dispatch, and skipped phase script status", () => {
    const events = [
      {
        context: {
          checkpointId: "checkpoint-1",
          eventTime: "2026-06-09T12:00:01Z",
          phaseName: "plan",
          sequence: 1,
          sessionSequence: 1,
          tick: 1,
        },
        id: "checkpoint",
        payload: {
          label: "plan checkpoint",
          phaseSummary: "planned work",
        },
        type: FACTORY_EVENT_TYPES.orchestratorCheckpointWritten,
      },
      {
        context: {
          eventTime: "2026-06-09T12:00:02Z",
          phaseId: "phase-skipped",
          sequence: 2,
          sessionSequence: 2,
          tick: 2,
        },
        id: "phase-skipped",
        payload: {
          phaseStatus: "SKIPPED",
          previousPhaseId: "phase-plan",
        },
        type: FACTORY_EVENT_TYPES.orchestratorPhaseChanged,
      },
      {
        context: {
          dispatchId: "dispatch-2",
          eventTime: "2026-06-09T12:00:03Z",
          phaseId: "phase-skipped",
          sequence: 3,
          sessionSequence: 3,
          tick: 3,
        },
        id: "queued-2",
        payload: {
          dispatchKind: "JAVASCRIPT_AGENT",
          label: "child dispatch",
        },
        type: FACTORY_EVENT_TYPES.dispatchQueued,
      },
      {
        context: {
          dispatchId: "dispatch-2",
          eventTime: "2026-06-09T12:00:04Z",
          sequence: 4,
          sessionSequence: 4,
          tick: 4,
        },
        id: "interrupted-2",
        payload: {
          observedStatus: "RUNNING",
        },
        type: FACTORY_EVENT_TYPES.dispatchInterrupted,
      },
      {
        context: {
          dispatchId: "dispatch-2",
          eventTime: "2026-06-09T12:00:05Z",
          sequence: 5,
          sessionSequence: 5,
          tick: 5,
        },
        id: "reconciled-2",
        payload: {
          reconciledStatus: "COMPLETED",
          resultArtifactRef: { id: "artifact-result-ref" },
        },
        type: FACTORY_EVENT_TYPES.dispatchReconciled,
      },
    ];

    const state = reconstructFactoryReplayState(events, 5);
    expect(state.javascriptRuntime?.checkpoints).toEqual([
      expect.objectContaining({
        id: "checkpoint-1",
        label: "plan checkpoint",
        summary: "planned work",
      }),
    ]);
    expect(state.javascriptRuntime?.script_status).toBe("SKIPPED");
    expect(state.javascriptRuntime?.child_dispatch_counts).toEqual({
      completed: 1,
      queued: 0,
      running: 0,
    });
    expect(state.javascriptRuntime?.dispatches[0]).toMatchObject({
      artifact_ids: ["artifact-result-ref"],
      id: "dispatch-2",
      status: "COMPLETED",
    });
  });
});

describe("lifecycle replay edge cases", () => {
  it("ignores artifact created events without nested artifact payloads", () => {
    const state = emptyReplayWorldState(1);
    const handled = applyDispatchLifecycleEvent(state, {
      context: {
        eventTime: "2026-06-09T12:00:01Z",
        sequence: 1,
        tick: 1,
      },
      id: "artifact-empty",
      payload: {},
      type: FACTORY_EVENT_TYPES.artifactCreated,
    });
    expect(handled).toBe(true);
    expect(state.sessionArtifacts).toEqual([]);
  });

  it("deduplicates repeated orchestrator phase history entries", () => {
    const events = [
      {
        context: {
          eventTime: "2026-06-09T12:00:01Z",
          phaseName: "review",
          sequence: 1,
          sessionSequence: 1,
          tick: 1,
        },
        id: "phase-review-1",
        payload: { phaseStatus: "ACTIVE" },
        type: FACTORY_EVENT_TYPES.orchestratorPhaseChanged,
      },
      {
        context: {
          eventTime: "2026-06-09T12:00:02Z",
          phaseName: "review",
          sequence: 2,
          sessionSequence: 2,
          tick: 2,
        },
        id: "phase-review-2",
        payload: { phaseStatus: "ACTIVE" },
        type: FACTORY_EVENT_TYPES.orchestratorPhaseChanged,
      },
    ];

    const state = reconstructFactoryReplayState(events, 2);
    expect(state.javascriptRuntime?.phases).toEqual(["review"]);
  });

  it("returns false for unrecognized lifecycle reducer events", () => {
    const state = emptyReplayWorldState(1);
    const unknownEvent = {
      context: {
        eventTime: "2026-06-09T12:00:01Z",
        sequence: 1,
        tick: 1,
      },
      id: "unknown",
      payload: {},
      type: FACTORY_EVENT_TYPES.runRequest,
    };
    expect(applyDispatchLifecycleEvent(state, unknownEvent)).toBe(false);
    expect(applyOrchestratorProgressEvent(state, unknownEvent)).toBe(false);
  });
});

describe("reconstructWorldState failed session replay", () => {
  it("reconstructs failed session bracket details and partial result summaries", () => {
    const events = [
      {
        ...lifecycleEvent(FACTORY_EVENT_TYPES.sessionStarted, "started", 1, 1, {
          factoryId: "factory-alpha",
          sourceRef: "workflow/main.js",
          startedAt: "2026-06-09T12:00:00Z",
        }),
        context: {
          eventTime: "2026-06-09T12:00:00Z",
          orchestratorDialect: "petri-js",
          orchestratorKind: "JAVASCRIPT",
          sequence: 1,
          sessionId: "session-alpha",
          sessionSequence: 1,
          tick: 1,
        },
      },
      lifecycleEvent(
        FACTORY_EVENT_TYPES.sessionResultUpdated,
        "partial",
        2,
        2,
        {
          artifactIds: ["artifact-partial"],
          resultStatus: "FAILED_WITH_PARTIAL",
          resultSummary: [{ text: "partial output", type: "TEXT" }],
        },
      ),
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionCompleted, "completed", 3, 3, {
        artifactIds: ["artifact-partial"],
        completedAt: "2026-06-09T12:00:05Z",
        durationMillis: 5000,
        failureDetail: {
          message: "child dispatch failed",
          reason: "DISPATCH_FAILED",
        },
        finalStatus: "SUCCEEDED",
        resultStatus: "FAILED_WITH_PARTIAL",
      }),
    ];

    const state = reconstructFactoryReplayState(events, 3);
    expect(state.sessionBracket).toMatchObject({
      failure_message: "child dispatch failed",
      failure_reason: "DISPATCH_FAILED",
      orchestrator_dialect: "petri-js",
      result_status: "FAILED_WITH_PARTIAL",
      result_summary: [{ text: "partial output", type: "TEXT" }],
      terminal: true,
    });
  });
});
