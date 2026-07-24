import { describe, expect, it } from "vitest";

import type { FactoryEvent, FactoryWorkItem } from "../../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../../api/events";
import { reconstructFactoryReplayState } from "./buildSnapshot";
import { projectRuntime } from "./projectRuntime";

const eventTime = "2026-06-01T12:00:00.000Z";

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

function workItem(id: string, text: string): FactoryWorkItem {
  return {
    id,
    display_name: id,
    trace_id: `trace-${id}`,
    work_type_id: "task",
    content: [{ type: "text", text }],
  };
}

describe("projectRuntime payload lineage place occupancy", () => {
  it("projects selected payload into current_work_items_by_place_id", () => {
    const item = workItem("work-1", "hello from sse");
    const events: FactoryEvent[] = [
      factoryEvent(
        "event-structure",
        0,
        FACTORY_EVENT_TYPES.initialStructureRequest,
        {
          factory: {
            workTypes: [
              {
                name: "task",
                states: [
                  { name: "init", type: "INITIAL" },
                  { name: "review", type: "PROCESSING" },
                ],
              },
            ],
            workstations: [
              {
                id: "t-review",
                inputs: [{ state: "init", workType: "task" }],
                name: "Review",
                outputs: [{ state: "review", workType: "task" }],
                worker: "reviewer",
              },
            ],
            workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
          },
        },
      ),
      factoryEvent(
        "event-work-request",
        1,
        FACTORY_EVENT_TYPES.workRequest,
        {
          source: "external-submit",
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              content: item.content,
              name: item.display_name,
              traceId: item.trace_id,
              workId: item.id,
              workTypeName: item.work_type_id,
            },
          ],
        },
        {
          requestId: "request/work-1",
          traceIds: item.trace_id ? [item.trace_id] : undefined,
          workIds: [item.id],
        },
      ),
    ];

    const state = reconstructFactoryReplayState(events, 1);
    const runtime = projectRuntime(state);
    const placeID = state.workItemsByID["work-1"]?.place_id;
    const refs = placeID
      ? runtime.current_work_items_by_place_id?.[placeID]
      : undefined;

    expect(refs).toEqual([
      expect.objectContaining({
        content: [{ type: "text", text: "hello from sse" }],
        payload_status: "RESOLVED",
        work_id: "work-1",
      }),
    ]);
  });
});

describe("projectRuntime payload lineage active execution", () => {
  it("projects selected payload into active execution work_items", () => {
    const item = workItem("work-1", "active execution body");
    const events: FactoryEvent[] = [
      factoryEvent(
        "event-structure",
        0,
        FACTORY_EVENT_TYPES.initialStructureRequest,
        {
          factory: {
            workTypes: [
              {
                name: "task",
                states: [
                  { name: "init", type: "INITIAL" },
                  { name: "review", type: "PROCESSING" },
                ],
              },
            ],
            workstations: [
              {
                id: "t-review",
                inputs: [{ state: "init", workType: "task" }],
                name: "Review",
                outputs: [{ state: "review", workType: "task" }],
                worker: "reviewer",
              },
            ],
            workers: [{ name: "reviewer", type: "MODEL_WORKER" }],
          },
        },
      ),
      factoryEvent(
        "event-work-request",
        1,
        FACTORY_EVENT_TYPES.workRequest,
        {
          source: "external-submit",
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              content: item.content,
              name: item.display_name,
              traceId: item.trace_id,
              workId: item.id,
              workTypeName: item.work_type_id,
            },
          ],
        },
        {
          requestId: "request/work-1",
          traceIds: item.trace_id ? [item.trace_id] : undefined,
          workIds: [item.id],
        },
      ),
      factoryEvent(
        "event-dispatch-request",
        2,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [
            {
              name: item.display_name,
              traceId: item.trace_id,
              workId: item.id,
              workTypeName: item.work_type_id,
            },
          ],
          resources: [],
          transitionId: "t-review",
        },
        {
          dispatchId: "dispatch-1",
          traceIds: item.trace_id ? [item.trace_id] : undefined,
          workIds: [item.id],
        },
      ),
    ];

    const state = reconstructFactoryReplayState(events, 2);
    const runtime = projectRuntime(state);
    const execution = runtime.active_executions_by_dispatch_id?.["dispatch-1"];

    expect(execution?.work_items).toEqual([
      expect.objectContaining({
        content: [{ type: "text", text: "active execution body" }],
        payload_status: "RESOLVED",
        work_id: "work-1",
      }),
    ]);
  });
});
