import type { DashboardSnapshot } from "../api/dashboard";
import type { FactoryEvent } from "../api/events";
import { FACTORY_EVENT_TYPES } from "../api/events";
import {
  buildDashboardSnapshotFixture,
  mediumBranchingDashboardTopology,
} from "../components/dashboard/fixtures";

const baselineSnapshot = buildDashboardSnapshotFixture(
  mediumBranchingDashboardTopology,
);

export const historicalTimelineSnapshot = {
  ...baselineSnapshot,
  tick_count: 1,
} satisfies DashboardSnapshot;

const eventTimelineWorkID = "work-event-story";
const eventTimelineTraceID = "trace-event-story";
const eventTimelineDispatchID = "dispatch-event-story";

function factoryEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-04-16T12:00:0${tick}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

export const selectedTickTimelineEvents: FactoryEvent[] = [
  factoryEvent("timeline-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
    factory: {
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "done", type: "TERMINAL" },
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
  }),
  factoryEvent("timeline-2", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [
      {
        name: "Event Story",
        trace_id: eventTimelineTraceID,
        work_id: eventTimelineWorkID,
        work_type_id: "story",
      },
    ],
  }),
  factoryEvent("timeline-3", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: eventTimelineDispatchID,
    inputs: [
      {
        name: "Event Story",
        trace_id: eventTimelineTraceID,
        work_id: eventTimelineWorkID,
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      outputs: [{ state: "review", workType: "story" }],
      worker: "reviewer",
    },
  }),
  factoryEvent("timeline-4", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
    dispatchId: eventTimelineDispatchID,
    durationMillis: 1500,
    outcome: "ACCEPTED",
    outputWork: [
      {
        name: "Event Story",
        trace_id: eventTimelineTraceID,
        work_id: eventTimelineWorkID,
        work_type_id: "story",
      },
    ],
    providerSession: {
      id: "sess-event-story",
      kind: "session_id",
      provider: "codex",
    },
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      outputs: [{ state: "done", workType: "story" }],
      worker: "reviewer",
    },
  }),
];

selectedTickTimelineEvents[1].context.requestId = "request-event-story";
selectedTickTimelineEvents[1].context.traceIds = [eventTimelineTraceID];
selectedTickTimelineEvents[1].context.workIds = [eventTimelineWorkID];
selectedTickTimelineEvents[2].context.dispatchId = eventTimelineDispatchID;
selectedTickTimelineEvents[2].context.traceIds = [eventTimelineTraceID];
selectedTickTimelineEvents[2].context.workIds = [eventTimelineWorkID];
selectedTickTimelineEvents[3].context.dispatchId = eventTimelineDispatchID;
selectedTickTimelineEvents[3].context.traceIds = [eventTimelineTraceID];
selectedTickTimelineEvents[3].context.workIds = [eventTimelineWorkID];
