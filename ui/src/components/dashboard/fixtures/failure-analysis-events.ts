import { FACTORY_EVENT_TYPES } from "../../../api/events";
import type { FactoryEvent } from "../../../api/events";

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

export const failureAnalysisTimelineEvents: FactoryEvent[] = [
  factoryEvent("failure-analysis-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
    factory: {
      workTypes: [{
        name: "story",
        states: [
          { name: "new", type: "INITIAL" },
          { name: "review", type: "PROCESSING" },
          { name: "done", type: "TERMINAL" },
          { name: "failed", type: "FAILED" },
        ],
      }],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", workType: "story" }],
          name: "Review",
          onFailure: { state: "failed", workType: "story" },
          outputs: [{ state: "review", workType: "story" }],
          worker: "reviewer",
        },
      ],
    },
  }),
  factoryEvent("failure-analysis-2", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: "Queued Analysis Story",
      trace_id: "trace-queued-analysis",
      work_id: "work-queued-analysis",
      work_type_name: "story",
    }],
  }),
  factoryEvent("failure-analysis-3", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: "Blocked Analysis Story",
      trace_id: "trace-blocked-analysis",
      work_id: "work-blocked-analysis",
      work_type_name: "story",
    }],
  }),
  factoryEvent("failure-analysis-4", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: "dispatch-blocked-analysis",
    inputs: [
      {
        name: "Blocked Analysis Story",
        trace_id: "trace-blocked-analysis",
        work_id: "work-blocked-analysis",
        work_type_name: "story",
      },
    ],
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onFailure: { state: "failed", workType: "story" },
      outputs: [{ state: "review", workType: "story" }],
      worker: "reviewer",
    },
  }),
  factoryEvent("failure-analysis-5", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
    dispatchId: "dispatch-blocked-analysis",
    durationMillis: 900,
    failureMessage: "Provider rate limit exceeded while generating the analysis.",
    failureReason: "provider_rate_limit",
    outcome: "FAILED",
    outputWork: [{
      name: "Blocked Analysis Story",
      trace_id: "trace-blocked-analysis",
      work_id: "work-blocked-analysis",
      work_type_name: "story",
    }],
    providerSession: {
      id: "sess-blocked-analysis",
      kind: "session_id",
      provider: "codex",
    },
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onFailure: { state: "failed", workType: "story" },
      outputs: [{ state: "review", workType: "story" }],
      worker: "reviewer",
    },
  }),
];

failureAnalysisTimelineEvents[1].context.requestId = "request-queued-analysis";
failureAnalysisTimelineEvents[1].context.traceIds = ["trace-queued-analysis"];
failureAnalysisTimelineEvents[1].context.workIds = ["work-queued-analysis"];
failureAnalysisTimelineEvents[2].context.requestId = "request-blocked-analysis";
failureAnalysisTimelineEvents[2].context.traceIds = ["trace-blocked-analysis"];
failureAnalysisTimelineEvents[2].context.workIds = ["work-blocked-analysis"];
failureAnalysisTimelineEvents[3].context.dispatchId = "dispatch-blocked-analysis";
failureAnalysisTimelineEvents[3].context.traceIds = ["trace-blocked-analysis"];
failureAnalysisTimelineEvents[3].context.workIds = ["work-blocked-analysis"];
failureAnalysisTimelineEvents[4].context.dispatchId = "dispatch-blocked-analysis";
failureAnalysisTimelineEvents[4].context.traceIds = ["trace-blocked-analysis"];
failureAnalysisTimelineEvents[4].context.workIds = ["work-blocked-analysis"];

