import { describe, expect, it } from "vitest";

import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { projectSnapshot } from "./projectSnapshot";
import {
  advanceWorldStateFromCheckpoint,
  reconstructWorldState,
} from "./replayWorldState";

const eventTime = "2026-05-30T12:00:00.000Z";

function factoryEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime,
      sequence: tick,
      tick,
      ...context,
    },
    id,
    payload,
    type,
  };
}

const initialStructureRequest = factoryEvent(
  "event-structure",
  0,
  FACTORY_EVENT_TYPES.initialStructureRequest,
  {
    factory: {
      workers: [
        {
          model: "gpt-5.4",
          modelProvider: "openai",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", workType: "story" }],
          name: "Review",
          onFailure: [{ state: "failed", workType: "story" }],
          outputs: [{ state: "review", workType: "story" }],
          worker: "reviewer",
        },
      ],
    },
  },
);

describe("reconstructWorldState inference success", () => {
  it("records inference attempts for an active dispatch", () => {
    const workID = "work-inference";
    const dispatchID = "dispatch-inference";
    const inferenceRequestID = `${dispatchID}/inference-request/1`;
    const events: FactoryEvent[] = [
      initialStructureRequest,
      factoryEvent(
        "event-work",
        1,
        FACTORY_EVENT_TYPES.workRequest,
        {
          source: "external-submit",
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: workID,
              traceId: `trace-${workID}`,
              workId: workID,
              workTypeName: "story",
            },
          ],
        },
        {
          requestId: `request-${workID}`,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
      factoryEvent(
        "event-dispatch-request",
        2,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: workID }],
          transitionId: "review",
        },
        {
          dispatchId: dispatchID,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
      factoryEvent(
        "event-inference-request",
        3,
        FACTORY_EVENT_TYPES.inferenceRequest,
        {
          attempt: 1,
          inferenceRequestId: inferenceRequestID,
          prompt: "Review the story.",
          workingDirectory: "/work/project",
          worktree: "/work/project/.worktrees/story",
        },
        {
          dispatchId: dispatchID,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
      factoryEvent(
        "event-inference-response",
        4,
        FACTORY_EVENT_TYPES.inferenceResponse,
        {
          attempt: 1,
          durationMillis: 500,
          inferenceRequestId: inferenceRequestID,
          outcome: "SUCCEEDED",
          response: "Looks good.",
        },
        {
          dispatchId: dispatchID,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
    ];

    const state = reconstructWorldState(events, 4);
    const attempt =
      state.inferenceAttemptsByDispatchID[dispatchID]?.[inferenceRequestID];
    expect(attempt).toEqual(
      expect.objectContaining({
        dispatch_id: dispatchID,
        inference_request_id: inferenceRequestID,
        outcome: "SUCCEEDED",
        prompt: "",
        promptTextBlobID: `inference:${inferenceRequestID}:prompt`,
        response: undefined,
        responseTextBlobID: `inference:${inferenceRequestID}:response`,
        transition_id: "review",
      }),
    );
    expect(state.textBlobsByID[`inference:${inferenceRequestID}:prompt`]).toBe(
      "Review the story.",
    );
    expect(
      state.textBlobsByID[`inference:${inferenceRequestID}:response`],
    ).toBe("Looks good.");
  });
});

describe("reconstructWorldState inference guards", () => {
  it("ignores inference events without dispatch context", () => {
    const events: FactoryEvent[] = [
      initialStructureRequest,
      factoryEvent(
        "event-inference-request",
        1,
        FACTORY_EVENT_TYPES.inferenceRequest,
        {
          attempt: 1,
          inferenceRequestId: "orphan/inference-request/1",
          prompt: "ignored",
          workingDirectory: "/tmp",
          worktree: "/tmp",
        },
      ),
      factoryEvent(
        "event-inference-response",
        2,
        FACTORY_EVENT_TYPES.inferenceResponse,
        {
          attempt: 1,
          durationMillis: 1,
          inferenceRequestId: "orphan/inference-request/1",
          outcome: "FAILED",
          response: "",
        },
      ),
    ];

    const state = reconstructWorldState(events, 2);
    expect(state.inferenceAttemptsByDispatchID).toEqual({});
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: Failure replay scenarios keep the canonical event sequence and cross-projection assertions together.
describe("reconstructWorldState inference failures", () => {
  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: This regression is clearest with its complete live/replay event sequence inline.
  it("projects canonical Codex failure detail through live and replayed selections", () => {
    const workID = "work-failed-inference";
    const dispatchID = "dispatch-failed-inference";
    const inferenceRequestID = `${dispatchID}/inference-request/1`;
    const events: FactoryEvent[] = [
      initialStructureRequest,
      factoryEvent(
        "event-work",
        1,
        FACTORY_EVENT_TYPES.workRequest,
        {
          source: "external-submit",
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: workID,
              traceId: `trace-${workID}`,
              workId: workID,
              workTypeName: "story",
            },
          ],
        },
        {
          requestId: `request-${workID}`,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
      factoryEvent(
        "event-dispatch-request",
        2,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: workID }],
          transitionId: "review",
        },
        {
          dispatchId: dispatchID,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
      factoryEvent(
        "event-inference-request",
        3,
        FACTORY_EVENT_TYPES.inferenceRequest,
        {
          attempt: 1,
          inferenceRequestId: inferenceRequestID,
          prompt: "Retry.",
          workingDirectory: "/work/project",
          worktree: "/work/project/.worktrees/story",
        },
        {
          dispatchId: dispatchID,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
      factoryEvent(
        "event-inference-response",
        4,
        FACTORY_EVENT_TYPES.inferenceResponse,
        {
          attempt: 1,
          diagnostics: { provider: "openai" },
          durationMillis: 250,
          exitCode: 1,
          failureDetail: {
            message:
              "Model gpt-5.6-sol requires a newer Codex version. Upgrade Codex and retry.",
            reason: "permanent_bad_request",
          },
          inferenceRequestId: inferenceRequestID,
          outcome: "FAILED",
          providerSession: {
            id: "codex-session-1",
            kind: "session_id",
            provider: "codex",
          },
          response: "",
        },
        {
          dispatchId: dispatchID,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
      factoryEvent(
        "event-dispatch-response",
        5,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          durationMillis: 300,
          outcome: "FAILED",
          outputWork: [
            {
              name: workID,
              state: "failed",
              traceId: `trace-${workID}`,
              workId: workID,
              workTypeName: "story",
            },
          ],
          transitionId: "review",
        },
        {
          dispatchId: dispatchID,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
    ];

    const state = reconstructWorldState(events, 5);
    const liveState = advanceWorldStateFromCheckpoint(
      reconstructWorldState(events, 3),
      events,
      5,
    );
    expect(
      state.inferenceAttemptsByDispatchID[dispatchID]?.[inferenceRequestID],
    ).toEqual(
      expect.objectContaining({
        error_class: "permanent_bad_request",
        exit_code: 1,
        failure_detail: {
          message:
            "Model gpt-5.6-sol requires a newer Codex version. Upgrade Codex and retry.",
          reason: "permanent_bad_request",
        },
        outcome: "FAILED",
      }),
    );
    expect(state.completedDispatches[0]).toMatchObject({
      failureMessage:
        "Model gpt-5.6-sol requires a newer Codex version. Upgrade Codex and retry.",
      failureReason: "permanent_bad_request",
    });
    expect(state.failedWorkDetailsByWorkID[workID]).toMatchObject({
      failure_message:
        "Model gpt-5.6-sol requires a newer Codex version. Upgrade Codex and retry.",
      failure_reason: "permanent_bad_request",
    });
    expect(state.providerSessions[0]).toMatchObject({
      failure_message:
        "Model gpt-5.6-sol requires a newer Codex version. Upgrade Codex and retry.",
      failure_reason: "permanent_bad_request",
    });
    expect(liveState).toEqual(state);

    const snapshot = projectSnapshot(state);
    const expectedFailureDetail = {
      message:
        "Model gpt-5.6-sol requires a newer Codex version. Upgrade Codex and retry.",
      reason: "permanent_bad_request",
    };
    expect(
      snapshot.runtime.inference_attempts_by_dispatch_id?.[dispatchID]?.[
        inferenceRequestID
      ]?.failure_detail,
    ).toEqual(expectedFailureDetail);
    expect(
      snapshot.runtime.workstation_requests_by_dispatch_id?.[dispatchID]
        ?.response?.failureDetail,
    ).toEqual(expectedFailureDetail);
    expect(snapshot.workstationRequestsByDispatchID[dispatchID]).toMatchObject({
      failure_message: expectedFailureDetail.message,
      failure_reason: expectedFailureDetail.reason,
    });
    expect(
      snapshot.runtime.session.failed_work_details_by_work_id?.[workID],
    ).toMatchObject({
      failure_message: expectedFailureDetail.message,
      failure_reason: expectedFailureDetail.reason,
    });
    expect(snapshot.runtime.session.provider_sessions?.[0]).toMatchObject({
      failure_message: expectedFailureDetail.message,
      failure_reason: expectedFailureDetail.reason,
    });
  });

  it("retains a historical failure reason without synthesizing a message", () => {
    const response = factoryEvent(
      "event-inference-response-history",
      4,
      FACTORY_EVENT_TYPES.inferenceResponse,
      {
        attempt: 1,
        durationMillis: 1,
        failureDetail: { reason: "unknown" },
        inferenceRequestId: "missing/inference-request/1",
        outcome: "FAILED",
      },
      { dispatchId: "missing" },
    );
    const state = reconstructWorldState(
      [
        initialStructureRequest,
        factoryEvent(
          "event-dispatch-request-history",
          2,
          FACTORY_EVENT_TYPES.dispatchRequest,
          { inputs: [], transitionId: "review" },
          { dispatchId: "missing" },
        ),
        response,
        factoryEvent(
          "event-dispatch-response-history",
          5,
          FACTORY_EVENT_TYPES.dispatchResponse,
          {
            durationMillis: 2,
            outcome: "FAILED",
            outputWork: [],
            transitionId: "review",
          },
          { dispatchId: "missing" },
        ),
      ],
      5,
    );
    expect(
      state.inferenceAttemptsByDispatchID.missing?.[
        "missing/inference-request/1"
      ]?.failure_detail,
    ).toEqual({ reason: "unknown" });
    const snapshot = projectSnapshot(state);
    expect(
      snapshot.runtime.workstation_requests_by_dispatch_id?.missing?.response
        ?.failureDetail,
    ).toEqual({ reason: "unknown" });
    expect(snapshot.workstationRequestsByDispatchID.missing).toMatchObject({
      failure_message: undefined,
      failure_reason: "unknown",
    });
  });
});
