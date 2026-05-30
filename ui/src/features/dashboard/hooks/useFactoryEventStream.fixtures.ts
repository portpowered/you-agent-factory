import { QueryClient } from "@tanstack/react-query";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import {
  type WorldState,
} from "../../timeline/state/factoryTimelineStore";

export const SEEDED_SNAPSHOT: DashboardSnapshot = {
  factory_state: "IDLE",
  runtime: {
    in_flight_dispatch_count: 0,
    session: {
      completed_count: 0,
      dispatched_count: 0,
      failed_count: 0,
      has_data: true,
    },
  },
  tick_count: 3,
  topology: {
    edges: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  },
  uptime_seconds: 12,
};

export function timelineSnapshot(snapshot: DashboardSnapshot): WorldState {
  return {
    ...snapshot,
    relationsByWorkID: {},
    tracesByWorkID: {},
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

export const CANONICAL_SELECTED_TICK_EVENTS = [
  {
    context: {
      eventTime: "2026-04-25T20:00:01Z",
      sequence: 1,
      tick: 1,
    },
    id: "event-1",
    payload: {
      factory: {
        workTypes: [{
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        }],
        workstations: [
          {
            id: "review",
            inputs: [{ state: "new", workType: "story" }],
            name: "Review",
            outputs: [{ state: "done", workType: "story" }],
            worker: "reviewer",
          },
        ],
        workers: [
          {
            model: "gpt-5.4",
            modelProvider: "codex",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
      },
    },
    type: FACTORY_EVENT_TYPES.initialStructureRequest,
  },
  {
    context: {
      eventTime: "2026-04-25T20:00:02Z",
      requestId: "request-story-1",
      sequence: 2,
      tick: 2,
      traceIds: ["trace-story-1"],
      workIds: ["work-story-1"],
    },
    id: "event-2",
    payload: {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: "Canonical Story",
          trace_id: "trace-story-1",
          work_id: "work-story-1",
          work_type_name: "story",
        },
      ],
    },
    type: FACTORY_EVENT_TYPES.workRequest,
  },
];

export function createFactoryEventStreamQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  });
}
