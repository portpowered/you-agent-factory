import { describe, expect, it } from "vitest";

import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../api/events";
import { buildWorkOutcomeTimelineSamplesFromEvents } from "./useWorkOutcomeChart";

const eventTime = "2026-04-29T12:00:00Z";

describe("buildWorkOutcomeTimelineSamplesFromEvents", () => {
  it("derives compact throughput samples directly from timeline events", () => {
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
            ],
            workers: [],
            workstations: [],
          },
          recordedAt: eventTime,
        }),
        event("work-request", 1, FACTORY_EVENT_TYPES.workRequest, {
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: "Story One",
              trace_id: "trace-1",
              work_id: "work-1",
              work_type_name: "story",
            },
          ],
        }),
        event("dispatch-request", 2, FACTORY_EVENT_TYPES.dispatchRequest, {
          inputs: [{ workId: "work-1" }],
          transitionId: "review",
        }, {
          dispatchId: "dispatch-1",
        }),
        event("dispatch-response", 3, FACTORY_EVENT_TYPES.dispatchResponse, {
          durationMillis: 100,
          outcome: "ACCEPTED",
          outputWork: [
            {
              name: "Story One",
              state: "done",
              trace_id: "trace-1",
              work_id: "work-1",
              work_type_name: "story",
            },
          ],
          transitionId: "review",
        }, {
          dispatchId: "dispatch-1",
        }),
        event("work-request-2", 4, FACTORY_EVENT_TYPES.workRequest, {
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: "Story Two",
              trace_id: "trace-2",
              work_id: "work-2",
              work_type_name: "story",
            },
          ],
        }),
        event("dispatch-request-2", 5, FACTORY_EVENT_TYPES.dispatchRequest, {
          inputs: [{ workId: "work-2" }],
          transitionId: "review",
        }, {
          dispatchId: "dispatch-2",
        }),
        event("dispatch-response-2", 6, FACTORY_EVENT_TYPES.dispatchResponse, {
          durationMillis: 100,
          failureMessage: "Rejected",
          failureReason: "review failed",
          outcome: "FAILED",
          outputWork: [
            {
              name: "Story Two",
              state: "failed",
              trace_id: "trace-2",
              work_id: "work-2",
              work_type_name: "story",
            },
          ],
          transitionId: "review",
        }, {
          dispatchId: "dispatch-2",
        }),
      ],
      6,
    );

    expect(samples).toMatchObject([
      {
        completedCount: 0,
        dispatchedCount: 0,
        failedCount: 0,
        inFlightCount: 0,
        queuedCount: 0,
        tick: 0,
      },
      {
        completedCount: 0,
        dispatchedCount: 0,
        failedCount: 0,
        inFlightCount: 0,
        queuedCount: 1,
        tick: 1,
      },
      {
        completedCount: 0,
        dispatchedCount: 1,
        failedCount: 0,
        inFlightCount: 1,
        queuedCount: 0,
        tick: 2,
      },
      {
        completedCount: 1,
        dispatchedCount: 1,
        failedCount: 0,
        inFlightCount: 0,
        queuedCount: 0,
        tick: 3,
      },
      {
        completedCount: 1,
        dispatchedCount: 1,
        failedCount: 0,
        inFlightCount: 0,
        queuedCount: 1,
        tick: 4,
      },
      {
        completedCount: 1,
        dispatchedCount: 2,
        failedCount: 0,
        inFlightCount: 1,
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

