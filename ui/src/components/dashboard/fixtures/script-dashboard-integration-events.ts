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
      eventTime: `2026-04-19T12:00:${String(tick).padStart(2, "0")}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

export const scriptDashboardIntegrationFixtureIDs = {
  failedDispatchID: "dispatch-script-dashboard-failed",
  failedFailureMessage: "Script timed out while reviewing the dashboard story.",
  failedFailureReason: "script_timeout",
  failedTraceID: "trace-script-dashboard-failed",
  failedWorkID: "work-script-dashboard-failed",
  failedWorkLabel: "Script Failed Story",
  inferenceDispatchID: "dispatch-script-dashboard-inference",
  inferenceDurationMillis: 740,
  inferencePromptSource: "factory-renderer",
  inferenceProviderSessionID: "sess-script-dashboard-inference",
  inferenceResponseText: "The inference-backed dashboard story is ready for the next workstation.",
  inferenceTraceID: "trace-script-dashboard-inference",
  inferenceWorkID: "work-script-dashboard-inference",
  inferenceWorkLabel: "Inference Story",
  scriptSuccessDispatchID: "dispatch-script-dashboard-success",
  scriptSuccessTraceID: "trace-script-dashboard-success",
  scriptSuccessWorkID: "work-script-dashboard-success",
  scriptSuccessWorkLabel: "Script Success Story",
} as const;

const reviewWorkstation = {
  id: "review",
  inputs: [{ state: "new", workType: "story" }],
  name: "Review",
  onFailure: { state: "failed", workType: "story" },
  outputs: [{ state: "done", workType: "story" }],
  worker: "script-reviewer",
};

export const scriptDashboardIntegrationTimelineEvents: FactoryEvent[] = [
  factoryEvent(
    "script-dashboard-integration-1",
    1,
    FACTORY_EVENT_TYPES.initialStructureRequest,
    {
      factory: {
        workers: [
          {
            executorProvider: "script_wrap",
            name: "script-reviewer",
            type: "SCRIPT_WORKER",
          },
          {
            modelProvider: "codex",
            name: "model-reviewer",
          },
        ],
        workTypes: [{
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "done", type: "TERMINAL" },
            { name: "failed", type: "FAILED" },
          ],
        }],
        workstations: [reviewWorkstation],
      },
    },
  ),
  factoryEvent(
    "script-dashboard-integration-2",
    2,
    FACTORY_EVENT_TYPES.workRequest,
    {
      type: "FACTORY_REQUEST_BATCH",
      works: [{
        name: scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkLabel,
        trace_id: scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID,
        work_id: scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID,
        work_type_name: "story",
      }],
    },
  ),
  factoryEvent(
    "script-dashboard-integration-3",
    2,
    FACTORY_EVENT_TYPES.workRequest,
    {
      type: "FACTORY_REQUEST_BATCH",
      works: [{
        name: scriptDashboardIntegrationFixtureIDs.failedWorkLabel,
        trace_id: scriptDashboardIntegrationFixtureIDs.failedTraceID,
        work_id: scriptDashboardIntegrationFixtureIDs.failedWorkID,
        work_type_name: "story",
      }],
    },
  ),
  factoryEvent(
    "script-dashboard-integration-4",
    2,
    FACTORY_EVENT_TYPES.workRequest,
    {
      type: "FACTORY_REQUEST_BATCH",
      works: [{
        name: scriptDashboardIntegrationFixtureIDs.inferenceWorkLabel,
        trace_id: scriptDashboardIntegrationFixtureIDs.inferenceTraceID,
        work_id: scriptDashboardIntegrationFixtureIDs.inferenceWorkID,
        work_type_name: "story",
      }],
    },
  ),
  factoryEvent(
    "script-dashboard-integration-5",
    3,
    FACTORY_EVENT_TYPES.dispatchRequest,
    {
      dispatchId: scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID,
      inputs: [
        {
          name: scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkLabel,
          trace_id: scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID,
          work_id: scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID,
          work_type_name: "story",
        },
      ],
      transitionId: "review",
      worker: {
        executorProvider: "script_wrap",
        name: "script-reviewer",
        type: "SCRIPT_WORKER",
      },
      workstation: reviewWorkstation,
    },
  ),
  factoryEvent(
    "script-dashboard-integration-6",
    4,
    FACTORY_EVENT_TYPES.scriptRequest,
    {
      args: ["--work", scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID],
      attempt: 1,
      command: "script-tool",
      dispatchId: scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID,
      scriptRequestId: `${scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID}/script-request/1`,
      transitionId: "review",
    },
  ),
  factoryEvent(
    "script-dashboard-integration-7",
    5,
    FACTORY_EVENT_TYPES.scriptResponse,
    {
      attempt: 1,
      dispatchId: scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID,
      durationMillis: 222,
      outcome: "SUCCEEDED",
      scriptRequestId: `${scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID}/script-request/1`,
      stderr: "",
      stdout: "script success stdout\n",
      transitionId: "review",
    },
  ),
  factoryEvent(
    "script-dashboard-integration-8",
    6,
    FACTORY_EVENT_TYPES.dispatchResponse,
    {
      dispatchId: scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID,
      durationMillis: 222,
      outcome: "ACCEPTED",
      output: "legacy script success output",
      outputWork: [
        {
          name: scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkLabel,
          trace_id: scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID,
          work_id: scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID,
          work_type_name: "story",
        },
      ],
      transitionId: "review",
      workstation: reviewWorkstation,
    },
  ),
  factoryEvent(
    "script-dashboard-integration-9",
    7,
    FACTORY_EVENT_TYPES.dispatchRequest,
    {
      dispatchId: scriptDashboardIntegrationFixtureIDs.failedDispatchID,
      inputs: [
        {
          name: scriptDashboardIntegrationFixtureIDs.failedWorkLabel,
          trace_id: scriptDashboardIntegrationFixtureIDs.failedTraceID,
          work_id: scriptDashboardIntegrationFixtureIDs.failedWorkID,
          work_type_name: "story",
        },
      ],
      transitionId: "review",
      worker: {
        executorProvider: "script_wrap",
        name: "script-reviewer",
        type: "SCRIPT_WORKER",
      },
      workstation: reviewWorkstation,
    },
  ),
  factoryEvent(
    "script-dashboard-integration-10",
    8,
    FACTORY_EVENT_TYPES.scriptRequest,
    {
      args: ["--work", scriptDashboardIntegrationFixtureIDs.failedWorkID],
      attempt: 1,
      command: "script-tool",
      dispatchId: scriptDashboardIntegrationFixtureIDs.failedDispatchID,
      scriptRequestId: `${scriptDashboardIntegrationFixtureIDs.failedDispatchID}/script-request/1`,
      transitionId: "review",
    },
  ),
  factoryEvent(
    "script-dashboard-integration-11",
    9,
    FACTORY_EVENT_TYPES.scriptResponse,
    {
      attempt: 1,
      dispatchId: scriptDashboardIntegrationFixtureIDs.failedDispatchID,
      durationMillis: 500,
      failureType: "TIMEOUT",
      outcome: "TIMED_OUT",
      scriptRequestId: `${scriptDashboardIntegrationFixtureIDs.failedDispatchID}/script-request/1`,
      stderr: "script timed out\n",
      stdout: "",
      transitionId: "review",
    },
  ),
  factoryEvent(
    "script-dashboard-integration-12",
    10,
    FACTORY_EVENT_TYPES.dispatchResponse,
    {
      dispatchId: scriptDashboardIntegrationFixtureIDs.failedDispatchID,
      durationMillis: 500,
      failureMessage: scriptDashboardIntegrationFixtureIDs.failedFailureMessage,
      failureReason: scriptDashboardIntegrationFixtureIDs.failedFailureReason,
      outcome: "FAILED",
      output: "legacy script failure output",
      outputWork: [
        {
          name: scriptDashboardIntegrationFixtureIDs.failedWorkLabel,
          trace_id: scriptDashboardIntegrationFixtureIDs.failedTraceID,
          work_id: scriptDashboardIntegrationFixtureIDs.failedWorkID,
          work_type_name: "story",
        },
      ],
      transitionId: "review",
      workstation: reviewWorkstation,
    },
  ),
  factoryEvent(
    "script-dashboard-integration-13",
    11,
    FACTORY_EVENT_TYPES.dispatchRequest,
    {
      dispatchId: scriptDashboardIntegrationFixtureIDs.inferenceDispatchID,
      inputs: [
        {
          name: scriptDashboardIntegrationFixtureIDs.inferenceWorkLabel,
          trace_id: scriptDashboardIntegrationFixtureIDs.inferenceTraceID,
          work_id: scriptDashboardIntegrationFixtureIDs.inferenceWorkID,
          work_type_name: "story",
        },
      ],
      transitionId: "review",
      worker: {
        model: "gpt-5.4",
        modelProvider: "codex",
        name: "model-reviewer",
      },
      workstation: {
        ...reviewWorkstation,
        worker: "model-reviewer",
      },
    },
  ),
  factoryEvent(
    "script-dashboard-integration-14",
    12,
    FACTORY_EVENT_TYPES.inferenceRequest,
    {
      attempt: 1,
      dispatchId: scriptDashboardIntegrationFixtureIDs.inferenceDispatchID,
      inferenceRequestId: `${scriptDashboardIntegrationFixtureIDs.inferenceDispatchID}/inference-request/1`,
      prompt: "Review the inference-backed dashboard story.",
      transitionId: "review",
      workingDirectory: "/work/inference-dashboard",
      worktree: "/work/inference-dashboard/.worktrees/story",
    },
  ),
  factoryEvent(
    "script-dashboard-integration-15",
    13,
    FACTORY_EVENT_TYPES.inferenceResponse,
    {
      attempt: 1,
      dispatchId: scriptDashboardIntegrationFixtureIDs.inferenceDispatchID,
      durationMillis: scriptDashboardIntegrationFixtureIDs.inferenceDurationMillis,
      inferenceRequestId: `${scriptDashboardIntegrationFixtureIDs.inferenceDispatchID}/inference-request/1`,
      outcome: "SUCCEEDED",
      response: scriptDashboardIntegrationFixtureIDs.inferenceResponseText,
      transitionId: "review",
    },
  ),
  factoryEvent(
    "script-dashboard-integration-16",
    14,
    FACTORY_EVENT_TYPES.dispatchResponse,
    {
      diagnostics: {
        provider: {
          model: "gpt-5.4",
          provider: "codex",
          requestMetadata: {
            prompt_source: scriptDashboardIntegrationFixtureIDs.inferencePromptSource,
            source: "script-dashboard-integration-fixture",
          },
          responseMetadata: {
            provider_session_id: scriptDashboardIntegrationFixtureIDs.inferenceProviderSessionID,
            retry_count: "0",
          },
        },
      },
      dispatchId: scriptDashboardIntegrationFixtureIDs.inferenceDispatchID,
      durationMillis: scriptDashboardIntegrationFixtureIDs.inferenceDurationMillis,
      outcome: "ACCEPTED",
      output: scriptDashboardIntegrationFixtureIDs.inferenceResponseText,
      outputWork: [
        {
          name: scriptDashboardIntegrationFixtureIDs.inferenceWorkLabel,
          trace_id: scriptDashboardIntegrationFixtureIDs.inferenceTraceID,
          work_id: scriptDashboardIntegrationFixtureIDs.inferenceWorkID,
          work_type_name: "story",
        },
      ],
      providerSession: {
        id: scriptDashboardIntegrationFixtureIDs.inferenceProviderSessionID,
        kind: "session_id",
        provider: "codex",
      },
      transitionId: "review",
      workstation: {
        ...reviewWorkstation,
        worker: "model-reviewer",
      },
    },
  ),
];

scriptDashboardIntegrationTimelineEvents[1].context.requestId = "request-script-dashboard-success";
scriptDashboardIntegrationTimelineEvents[1].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID,
];
scriptDashboardIntegrationTimelineEvents[1].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID,
];
scriptDashboardIntegrationTimelineEvents[2].context.requestId = "request-script-dashboard-failed";
scriptDashboardIntegrationTimelineEvents[2].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.failedTraceID,
];
scriptDashboardIntegrationTimelineEvents[2].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.failedWorkID,
];
scriptDashboardIntegrationTimelineEvents[3].context.requestId =
  "request-script-dashboard-inference";
scriptDashboardIntegrationTimelineEvents[3].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceTraceID,
];
scriptDashboardIntegrationTimelineEvents[3].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceWorkID,
];
scriptDashboardIntegrationTimelineEvents[4].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID;
scriptDashboardIntegrationTimelineEvents[4].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID,
];
scriptDashboardIntegrationTimelineEvents[4].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID,
];
scriptDashboardIntegrationTimelineEvents[5].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID;
scriptDashboardIntegrationTimelineEvents[5].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID,
];
scriptDashboardIntegrationTimelineEvents[5].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID,
];
scriptDashboardIntegrationTimelineEvents[6].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID;
scriptDashboardIntegrationTimelineEvents[6].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID,
];
scriptDashboardIntegrationTimelineEvents[6].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID,
];
scriptDashboardIntegrationTimelineEvents[7].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID;
scriptDashboardIntegrationTimelineEvents[7].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID,
];
scriptDashboardIntegrationTimelineEvents[7].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID,
];
scriptDashboardIntegrationTimelineEvents[8].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.failedDispatchID;
scriptDashboardIntegrationTimelineEvents[8].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.failedTraceID,
];
scriptDashboardIntegrationTimelineEvents[8].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.failedWorkID,
];
scriptDashboardIntegrationTimelineEvents[9].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.failedDispatchID;
scriptDashboardIntegrationTimelineEvents[9].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.failedTraceID,
];
scriptDashboardIntegrationTimelineEvents[9].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.failedWorkID,
];
scriptDashboardIntegrationTimelineEvents[10].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.failedDispatchID;
scriptDashboardIntegrationTimelineEvents[10].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.failedTraceID,
];
scriptDashboardIntegrationTimelineEvents[10].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.failedWorkID,
];
scriptDashboardIntegrationTimelineEvents[11].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.failedDispatchID;
scriptDashboardIntegrationTimelineEvents[11].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.failedTraceID,
];
scriptDashboardIntegrationTimelineEvents[11].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.failedWorkID,
];
scriptDashboardIntegrationTimelineEvents[12].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.inferenceDispatchID;
scriptDashboardIntegrationTimelineEvents[12].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceTraceID,
];
scriptDashboardIntegrationTimelineEvents[12].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceWorkID,
];
scriptDashboardIntegrationTimelineEvents[13].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.inferenceDispatchID;
scriptDashboardIntegrationTimelineEvents[13].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceTraceID,
];
scriptDashboardIntegrationTimelineEvents[13].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceWorkID,
];
scriptDashboardIntegrationTimelineEvents[14].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.inferenceDispatchID;
scriptDashboardIntegrationTimelineEvents[14].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceTraceID,
];
scriptDashboardIntegrationTimelineEvents[14].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceWorkID,
];
scriptDashboardIntegrationTimelineEvents[15].context.dispatchId =
  scriptDashboardIntegrationFixtureIDs.inferenceDispatchID;
scriptDashboardIntegrationTimelineEvents[15].context.traceIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceTraceID,
];
scriptDashboardIntegrationTimelineEvents[15].context.workIds = [
  scriptDashboardIntegrationFixtureIDs.inferenceWorkID,
];
