import { describe, expect, it } from "vitest";

import type { FactoryEvent } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { projectRuntime } from "./projectRuntime";
import { reconstructWorldState } from "./replayWorldState";

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
          name: "task",
          states: [
            { name: "init", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
      workstations: [
        {
          id: "t-review",
          inputs: [{ state: "init", workType: "task" }],
          name: "Review",
          onFailure: [{ state: "failed", workType: "task" }],
          outputs: [{ state: "review", workType: "task" }],
          worker: "reviewer",
        },
      ],
    },
  },
);

function workRequestEvent(tick: number, workID: string): FactoryEvent {
  return factoryEvent(
    `event-work-${workID}`,
    tick,
    FACTORY_EVENT_TYPES.workRequest,
    {
      source: "external-submit",
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: workID,
          traceId: `trace-${workID}`,
          workId: workID,
          workTypeName: "task",
        },
      ],
    },
    {
      requestId: `request-${workID}`,
      traceIds: [`trace-${workID}`],
      workIds: [workID],
    },
  );
}

function failedDispatchEvents(tick: number, workID: string): FactoryEvent[] {
  const dispatchID = `dispatch-failed-${workID}`;
  return [
    factoryEvent(
      `event-dispatch-request-${workID}`,
      tick,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        inputs: [{ workId: workID }],
        resources: [],
        transitionId: "t-review",
      },
      {
        dispatchId: dispatchID,
        traceIds: [`trace-${workID}`],
        workIds: [workID],
      },
    ),
    factoryEvent(
      `event-dispatch-response-${workID}`,
      tick + 1,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        durationMillis: 1,
        failureMessage: "boom",
        failureReason: "worker_error",
        outcome: "FAILED",
        outputWork: [
          {
            name: workID,
            traceId: `trace-${workID}`,
            workId: workID,
            workTypeName: "task",
            state: "failed",
          },
        ],
        transitionId: "t-review",
      },
      {
        dispatchId: dispatchID,
        traceIds: [`trace-${workID}`],
        workIds: [workID],
      },
    ),
  ];
}

function workStateChangeEvent(
  tick: number,
  workID: string,
  fromState: string,
  toState: string,
  fromPlaceID: string,
  toPlaceID: string,
  source: "api" | "cli",
): FactoryEvent {
  return factoryEvent(
    `event-work-state-change-${workID}-${tick}`,
    tick,
    FACTORY_EVENT_TYPES.workStateChange,
    {
      fromPlaceId: fromPlaceID,
      fromState,
      source,
      toPlaceId: toPlaceID,
      toState,
      workId: workID,
      workTypeName: "task",
    },
    { workIds: [workID] },
  );
}

describe("reconstructWorldState WORK_STATE_CHANGE", () => {
  it("moves work from failed to in-progress at a later tick while retaining failure details", () => {
    const workID = "work-recover";
    const events: FactoryEvent[] = [
      initialStructureRequest,
      workRequestEvent(1, workID),
      ...failedDispatchEvents(2, workID),
      workStateChangeEvent(4, workID, "failed", "review", "task:failed", "task:review", "cli"),
    ];

    const failedTick = reconstructWorldState(events, 3);
    expect(failedTick.failedWorkItemsByID[workID]).toBeDefined();
    expect(failedTick.occupancyByID["task:failed"]?.workItemIDs).toEqual([workID]);

    const recoveredTick = reconstructWorldState(events, 4);
    expect(recoveredTick.failedWorkItemsByID[workID]).toBeUndefined();
    expect(recoveredTick.occupancyByID["task:failed"]?.workItemIDs ?? []).toEqual([]);
    expect(recoveredTick.occupancyByID["task:review"]?.workItemIDs).toEqual([workID]);
    expect(recoveredTick.workItemsByID[workID]?.place_id).toBe("task:review");
    expect(recoveredTick.failedWorkDetailsByWorkID[workID]?.failure_reason).toBeDefined();

    const runtime = projectRuntime(recoveredTick);
    expect(runtime.current_work_items_by_place_id["task:review"]).toEqual([
      expect.objectContaining({ work_id: workID }),
    ]);
    expect(runtime.place_occupancy_work_items_by_place_id["task:review"]).toEqual([
      expect.objectContaining({ work_id: workID }),
    ]);
  });

  it("moves work from initial to an arbitrary processing place", () => {
    const workID = "work-bootstrap";
    const events: FactoryEvent[] = [
      initialStructureRequest,
      workRequestEvent(1, workID),
      workStateChangeEvent(2, workID, "init", "review", "task:init", "task:review", "api"),
    ];

    const state = reconstructWorldState(events, 2);
    expect(state.occupancyByID["task:init"]?.workItemIDs ?? []).toEqual([]);
    expect(state.occupancyByID["task:review"]?.workItemIDs).toEqual([workID]);
    expect(state.workItemsByID[workID]?.place_id).toBe("task:review");

    const runtime = projectRuntime(state);
    expect(runtime.current_work_items_by_place_id["task:review"]).toEqual([
      expect.objectContaining({ work_id: workID }),
    ]);
  });
});
