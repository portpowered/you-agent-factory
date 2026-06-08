import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { reconstructWorldState } from "./replayWorldState";

function lifecycleEvent(
  type: string,
  id: string,
  sequence: number,
  tick: number,
  payload: Record<string, unknown>,
): Parameters<typeof reconstructWorldState>[0][number] {
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
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionResultUpdated, "partial", 2, 2, {
        artifactIds: ["artifact-partial"],
        resultStatus: "PARTIAL",
      }),
      lifecycleEvent(FACTORY_EVENT_TYPES.sessionCompleted, "completed", 3, 3, {
        artifactIds: ["artifact-final"],
        completedAt: "2026-06-09T12:00:05Z",
        dispatchCounts: { completed: 2, queued: 0, running: 0 },
        durationMillis: 5000,
        finalStatus: "FINISHED",
        resultStatus: "FINAL",
      }),
    ];

    const state = reconstructWorldState(events, 3);
    expect(state.sessionBracket).toMatchObject({
      factory_id: "factory-alpha",
      result_status: "FINAL",
      session_id: "session-alpha",
      source_ref: "workflow/main.js",
      terminal: true,
    });
    expect(state.sessionBracket?.artifact_ids).toEqual(["artifact-final"]);
  });

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
          artifactId: "artifact-child",
          kind: "CHILD_RESULT",
          visibility: "CUSTOMER",
        },
        type: FACTORY_EVENT_TYPES.artifactCreated,
      },
    ];

    const state = reconstructWorldState(events, 4);
    expect(state.javascriptRuntime?.phase).toBe("review");
    expect(state.javascriptRuntime?.dispatches[0]).toMatchObject({
      id: "dispatch-1",
      status: "RECONCILED",
    });
    expect(state.sessionArtifacts).toEqual([
      expect.objectContaining({ id: "artifact-child", kind: "CHILD_RESULT" }),
    ]);
  });
});
