import { describe, expect, it } from "vitest";

import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { reconstructFactoryReplayState } from "./buildSnapshot";

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
          ],
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", workType: "story" }],
          name: "Review",
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

    const state = reconstructFactoryReplayState(events, 4);
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

    const state = reconstructFactoryReplayState(events, 2);
    expect(state.inferenceAttemptsByDispatchID).toEqual({});
  });
});

describe("reconstructWorldState inference failures", () => {
  it("records failed inference outcomes with error metadata", () => {
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
          failureDetail: {
            message: "Provider rate limit exceeded.",
            reason: "throttled",
          },
          exitCode: 1,
          inferenceRequestId: inferenceRequestID,
          outcome: "FAILED",
          response: "",
        },
        {
          dispatchId: dispatchID,
          traceIds: [`trace-${workID}`],
          workIds: [workID],
        },
      ),
    ];

    const state = reconstructFactoryReplayState(events, 4);
    expect(
      state.inferenceAttemptsByDispatchID[dispatchID]?.[inferenceRequestID],
    ).toEqual(
      expect.objectContaining({
        error_class: "throttled",
        exit_code: 1,
        outcome: "FAILED",
      }),
    );
  });
});
