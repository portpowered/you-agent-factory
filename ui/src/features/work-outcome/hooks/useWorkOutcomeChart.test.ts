import { describe, expect, it } from "vitest";

import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../api/events";
import { buildWorkOutcomeTimelineSamplesFromEvents } from "./useWorkOutcomeChart";

describe("buildWorkOutcomeTimelineSamplesFromEvents", () => {
  it("projects the exact uninterrupted customer work-outcome series", () => {
    const samples = buildWorkOutcomeTimelineSamplesFromEvents(
      [
        event("run-started", 0, FACTORY_EVENT_TYPES.runRequest, {
          factory: {
            resources: [],
            workTypes: [
              {
                name: "story",
                states: [
                  { name: "init", type: "INITIAL" },
                  { name: "done", type: "TERMINAL" },
                  { name: "failed", type: "FAILED" },
                ],
              },
              {
                name: "__system_time",
                states: [
                  { name: "pending", type: "INITIAL" },
                  { name: "expired", type: "TERMINAL" },
                ],
              },
            ],
            workers: [],
            workstations: [],
          },
          recordedAt: "2026-04-29T12:00:00Z",
        }),
        event("work-request", 1, FACTORY_EVENT_TYPES.workRequest, {
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: "Story One",
              traceId: "trace-1",
              workId: "work-1",
              workTypeName: "story",
            },
          ],
        }),
        event("system-time-work-request", 1, FACTORY_EVENT_TYPES.workRequest, {
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: "System clock",
              traceId: "trace-system-time",
              workId: "work-system-time",
              workTypeName: "__system_time",
            },
          ],
        }),
        event(
          "dispatch-request",
          2,
          FACTORY_EVENT_TYPES.dispatchRequest,
          {
            inputs: [{ workId: "work-1" }],
            transitionId: "review",
          },
          {
            dispatchId: "dispatch-1",
          },
        ),
        event(
          "system-time-dispatch-request",
          2,
          FACTORY_EVENT_TYPES.dispatchRequest,
          {
            inputs: [{ workId: "work-system-time" }],
            transitionId: "__system_time:expire",
          },
          {
            dispatchId: "dispatch-system-time",
          },
        ),
        event(
          "dispatch-response",
          3,
          FACTORY_EVENT_TYPES.dispatchResponse,
          {
            durationMillis: 100,
            outcome: "ACCEPTED",
            outputWork: [
              {
                name: "Story One",
                state: "done",
                traceId: "trace-1",
                workId: "work-1",
                workTypeName: "story",
              },
            ],
            transitionId: "review",
          },
          {
            dispatchId: "dispatch-1",
          },
        ),
        event(
          "system-time-dispatch-response",
          3,
          FACTORY_EVENT_TYPES.dispatchResponse,
          {
            durationMillis: 10,
            outcome: "ACCEPTED",
            outputWork: [
              {
                name: "System clock",
                state: "expired",
                traceId: "trace-system-time",
                workId: "work-system-time",
                workTypeName: "__system_time",
              },
            ],
            transitionId: "__system_time:expire",
          },
          {
            dispatchId: "dispatch-system-time",
          },
        ),
        event("work-request-2", 4, FACTORY_EVENT_TYPES.workRequest, {
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: "Story Two",
              traceId: "trace-2",
              workId: "work-2",
              workTypeName: "story",
            },
          ],
        }),
        event(
          "dispatch-request-2",
          5,
          FACTORY_EVENT_TYPES.dispatchRequest,
          {
            inputs: [{ workId: "work-2" }],
            transitionId: "review",
          },
          {
            dispatchId: "dispatch-2",
          },
        ),
        event(
          "dispatch-response-2",
          6,
          FACTORY_EVENT_TYPES.dispatchResponse,
          {
            durationMillis: 100,
            failureMessage: "Rejected",
            failureReason: "review failed",
            outcome: "FAILED",
            outputWork: [
              {
                name: "Story Two",
                state: "failed",
                traceId: "trace-2",
                workId: "work-2",
                workTypeName: "story",
              },
            ],
            transitionId: "review",
          },
          {
            dispatchId: "dispatch-2",
          },
        ),
      ],
      6,
    );

    expect(samples).toEqual([
      {
        completedCount: 0,
        dispatchedCount: 0,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 0,
        observedAt: 1777464000000,
        queuedCount: 0,
        tick: 0,
      },
      {
        completedCount: 0,
        dispatchedCount: 0,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 0,
        observedAt: 1777464001000,
        queuedCount: 1,
        tick: 1,
      },
      {
        completedCount: 0,
        dispatchedCount: 1,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 1,
        observedAt: 1777464002000,
        queuedCount: 0,
        tick: 2,
      },
      {
        completedCount: 1,
        dispatchedCount: 1,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 0,
        observedAt: 1777464003000,
        queuedCount: 0,
        tick: 3,
      },
      {
        completedCount: 1,
        dispatchedCount: 1,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 0,
        observedAt: 1777464004000,
        queuedCount: 1,
        tick: 4,
      },
      {
        completedCount: 1,
        dispatchedCount: 2,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 1,
        observedAt: 1777464005000,
        queuedCount: 0,
        tick: 5,
      },
      {
        completedCount: 1,
        dispatchedCount: 2,
        failedByWorkType: { story: 1 },
        failedCount: 1,
        failedWorkLabels: ["Story Two"],
        inFlightCount: 0,
        observedAt: 1777464006000,
        queuedCount: 0,
        tick: 6,
      },
    ]);
  });
});

function event(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-04-29T12:00:0${tick}Z`,
      sequence: tick,
      tick,
      ...context,
    },
    id,
    payload,
    type,
  };
}
