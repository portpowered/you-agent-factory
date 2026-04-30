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
      eventTime: `2026-04-18T12:00:${String(tick).padStart(2, "0")}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

export const runtimeDetailsFixtureIDs = {
  activeDispatchID: "dispatch-runtime-pending",
  activeTraceID: "trace-runtime-pending",
  activeWorkID: "work-runtime-pending",
  activeWorkLabel: "Pending Runtime Story",
  completedDispatchID: "dispatch-runtime-completed",
  completedDurationMillis: 875,
  completedPromptSource: "factory-renderer",
  completedProviderSessionID: "sess-runtime-completed",
  completedResponseText: "The completed runtime story is ready for review.",
  completedSystemPromptHash: "sha256:runtime-system",
  completedTraceID: "trace-runtime-completed",
  completedUserMessageHash: "sha256:runtime-user",
  completedWorkID: "work-runtime-completed",
  completedWorkLabel: "Completed Runtime Story",
  failedDispatchID: "dispatch-runtime-failed",
  failedDurationMillis: 600,
  failedErrorClass: "rate_limited",
  failedFailureMessage: "Provider rate limit exceeded while reviewing the failed runtime story.",
  failedFailureReason: "provider_rate_limit",
  failedPromptSource: "retry-renderer",
  failedProviderSessionID: "sess-runtime-failed",
  failedTraceID: "trace-runtime-failed",
  failedWorkID: "work-runtime-failed",
  failedWorkLabel: "Failed Runtime Story",
  unsafeSystemPromptBody: "Do not render this raw runtime system prompt.",
  unsafeUserMessageBody: "Do not render this raw runtime user message.",
} as const;

const reviewWorkstation = {
  id: "review",
  inputs: [{ state: "new", workType: "story" }],
  name: "Review",
  onFailure: { state: "failed", workType: "story" },
  outputs: [{ state: "done", workType: "story" }],
  worker: "runtime-reviewer",
};

export const runtimeDetailsTimelineEvents: FactoryEvent[] = [
  factoryEvent("runtime-details-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
    factory: {
      workers: [
        {
          executorProvider: "script_wrap",
          modelProvider: "codex",
          name: "runtime-reviewer",
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
  }),
  factoryEvent("runtime-details-2", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: runtimeDetailsFixtureIDs.activeWorkLabel,
      trace_id: runtimeDetailsFixtureIDs.activeTraceID,
      work_id: runtimeDetailsFixtureIDs.activeWorkID,
      work_type_name: "story",
    }],
  }),
  factoryEvent("runtime-details-3", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: runtimeDetailsFixtureIDs.completedWorkLabel,
      trace_id: runtimeDetailsFixtureIDs.completedTraceID,
      work_id: runtimeDetailsFixtureIDs.completedWorkID,
      work_type_name: "story",
    }],
  }),
  factoryEvent("runtime-details-4", 2, FACTORY_EVENT_TYPES.workRequest, {
    type: "FACTORY_REQUEST_BATCH",
    works: [{
      name: runtimeDetailsFixtureIDs.failedWorkLabel,
      trace_id: runtimeDetailsFixtureIDs.failedTraceID,
      work_id: runtimeDetailsFixtureIDs.failedWorkID,
      work_type_name: "story",
    }],
  }),
  factoryEvent("runtime-details-5", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: runtimeDetailsFixtureIDs.activeDispatchID,
    inputs: [
      {
        name: runtimeDetailsFixtureIDs.activeWorkLabel,
        trace_id: runtimeDetailsFixtureIDs.activeTraceID,
        work_id: runtimeDetailsFixtureIDs.activeWorkID,
        work_type_name: "story",
      },
    ],
    transitionId: "review",
    worker: {
      modelProvider: "codex",
      name: "runtime-reviewer",
    },
    workstation: reviewWorkstation,
  }),
  factoryEvent("runtime-details-6", 4, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: runtimeDetailsFixtureIDs.completedDispatchID,
    inputs: [
      {
        name: runtimeDetailsFixtureIDs.completedWorkLabel,
        trace_id: runtimeDetailsFixtureIDs.completedTraceID,
        work_id: runtimeDetailsFixtureIDs.completedWorkID,
        work_type_name: "story",
      },
    ],
    transitionId: "review",
    worker: {
      model: "gpt-5.4",
      modelProvider: "codex",
      name: "runtime-reviewer",
    },
    workstation: reviewWorkstation,
  }),
  factoryEvent("runtime-details-7", 5, FACTORY_EVENT_TYPES.inferenceRequest, {
    attempt: 1,
    dispatchId: runtimeDetailsFixtureIDs.completedDispatchID,
    inferenceRequestId: `${runtimeDetailsFixtureIDs.completedDispatchID}/inference-request/1`,
    prompt: "Review the completed runtime story.",
    transitionId: "review",
    workingDirectory: "/work/completed-runtime",
    worktree: "/work/completed-runtime/.worktrees/runtime",
  }),
  factoryEvent("runtime-details-8", 6, FACTORY_EVENT_TYPES.inferenceResponse, {
    attempt: 1,
    dispatchId: runtimeDetailsFixtureIDs.completedDispatchID,
    durationMillis: runtimeDetailsFixtureIDs.completedDurationMillis,
    inferenceRequestId: `${runtimeDetailsFixtureIDs.completedDispatchID}/inference-request/1`,
    outcome: "SUCCEEDED",
    response: runtimeDetailsFixtureIDs.completedResponseText,
    transitionId: "review",
  }),
  factoryEvent("runtime-details-9", 7, FACTORY_EVENT_TYPES.dispatchResponse, {
    diagnostics: {
      provider: {
        model: "gpt-5.4",
        provider: "codex",
        requestMetadata: {
          prompt_source: runtimeDetailsFixtureIDs.completedPromptSource,
          source: "runtime-details-fixture",
        },
        responseMetadata: {
          provider_session_id: runtimeDetailsFixtureIDs.completedProviderSessionID,
          retry_count: "0",
        },
      },
      renderedPrompt: {
        systemPromptHash: runtimeDetailsFixtureIDs.completedSystemPromptHash,
        userMessageHash: runtimeDetailsFixtureIDs.completedUserMessageHash,
        variables: {
          prompt_source: runtimeDetailsFixtureIDs.completedPromptSource,
          system_prompt: runtimeDetailsFixtureIDs.unsafeSystemPromptBody,
          user_message: runtimeDetailsFixtureIDs.unsafeUserMessageBody,
        },
      },
    },
    dispatchId: runtimeDetailsFixtureIDs.completedDispatchID,
    durationMillis: runtimeDetailsFixtureIDs.completedDurationMillis,
    outcome: "ACCEPTED",
    output: runtimeDetailsFixtureIDs.completedResponseText,
    outputWork: [
      {
        name: runtimeDetailsFixtureIDs.completedWorkLabel,
        trace_id: runtimeDetailsFixtureIDs.completedTraceID,
        work_id: runtimeDetailsFixtureIDs.completedWorkID,
        work_type_name: "story",
      },
    ],
    providerSession: {
      id: runtimeDetailsFixtureIDs.completedProviderSessionID,
      kind: "session_id",
      provider: "codex",
    },
    transitionId: "review",
    workstation: reviewWorkstation,
  }),
  factoryEvent("runtime-details-10", 8, FACTORY_EVENT_TYPES.dispatchRequest, {
    dispatchId: runtimeDetailsFixtureIDs.failedDispatchID,
    inputs: [
      {
        name: runtimeDetailsFixtureIDs.failedWorkLabel,
        trace_id: runtimeDetailsFixtureIDs.failedTraceID,
        work_id: runtimeDetailsFixtureIDs.failedWorkID,
        work_type_name: "story",
      },
    ],
    transitionId: "review",
    worker: {
      modelProvider: "codex",
      name: "runtime-reviewer",
    },
    workstation: reviewWorkstation,
  }),
  factoryEvent("runtime-details-11", 9, FACTORY_EVENT_TYPES.inferenceRequest, {
    attempt: 1,
    dispatchId: runtimeDetailsFixtureIDs.failedDispatchID,
    inferenceRequestId: `${runtimeDetailsFixtureIDs.failedDispatchID}/inference-request/1`,
    prompt: "Retry the failed runtime story.",
    transitionId: "review",
    workingDirectory: "/work/failed-runtime",
    worktree: "/work/failed-runtime/.worktrees/runtime",
  }),
  factoryEvent("runtime-details-12", 10, FACTORY_EVENT_TYPES.inferenceResponse, {
    attempt: 1,
    dispatchId: runtimeDetailsFixtureIDs.failedDispatchID,
    durationMillis: runtimeDetailsFixtureIDs.failedDurationMillis,
    errorClass: runtimeDetailsFixtureIDs.failedErrorClass,
    inferenceRequestId: `${runtimeDetailsFixtureIDs.failedDispatchID}/inference-request/1`,
    outcome: "FAILED",
    transitionId: "review",
  }),
  factoryEvent("runtime-details-13", 11, FACTORY_EVENT_TYPES.dispatchResponse, {
    diagnostics: {
      provider: {
        model: "claude-3.7",
        provider: "anthropic",
        requestMetadata: {
          prompt_source: runtimeDetailsFixtureIDs.failedPromptSource,
          source: "runtime-details-fixture",
        },
        responseMetadata: {
          provider_session_id: runtimeDetailsFixtureIDs.failedProviderSessionID,
          retry_count: "1",
        },
      },
    },
    dispatchId: runtimeDetailsFixtureIDs.failedDispatchID,
    durationMillis: runtimeDetailsFixtureIDs.failedDurationMillis,
    failureMessage: runtimeDetailsFixtureIDs.failedFailureMessage,
    failureReason: runtimeDetailsFixtureIDs.failedFailureReason,
    outcome: "FAILED",
    outputWork: [
      {
        name: runtimeDetailsFixtureIDs.failedWorkLabel,
        trace_id: runtimeDetailsFixtureIDs.failedTraceID,
        work_id: runtimeDetailsFixtureIDs.failedWorkID,
        work_type_name: "story",
      },
    ],
    providerSession: {
      id: runtimeDetailsFixtureIDs.failedProviderSessionID,
      kind: "session_id",
      provider: "anthropic",
    },
    transitionId: "review",
    workstation: reviewWorkstation,
  }),
];

runtimeDetailsTimelineEvents[1].context.requestId = "request-runtime-pending";
runtimeDetailsTimelineEvents[1].context.traceIds = [runtimeDetailsFixtureIDs.activeTraceID];
runtimeDetailsTimelineEvents[1].context.workIds = [runtimeDetailsFixtureIDs.activeWorkID];
runtimeDetailsTimelineEvents[2].context.requestId = "request-runtime-completed";
runtimeDetailsTimelineEvents[2].context.traceIds = [runtimeDetailsFixtureIDs.completedTraceID];
runtimeDetailsTimelineEvents[2].context.workIds = [runtimeDetailsFixtureIDs.completedWorkID];
runtimeDetailsTimelineEvents[3].context.requestId = "request-runtime-failed";
runtimeDetailsTimelineEvents[3].context.traceIds = [runtimeDetailsFixtureIDs.failedTraceID];
runtimeDetailsTimelineEvents[3].context.workIds = [runtimeDetailsFixtureIDs.failedWorkID];
runtimeDetailsTimelineEvents[4].context.dispatchId = runtimeDetailsFixtureIDs.activeDispatchID;
runtimeDetailsTimelineEvents[4].context.traceIds = [runtimeDetailsFixtureIDs.activeTraceID];
runtimeDetailsTimelineEvents[4].context.workIds = [runtimeDetailsFixtureIDs.activeWorkID];
runtimeDetailsTimelineEvents[5].context.dispatchId = runtimeDetailsFixtureIDs.completedDispatchID;
runtimeDetailsTimelineEvents[5].context.traceIds = [runtimeDetailsFixtureIDs.completedTraceID];
runtimeDetailsTimelineEvents[5].context.workIds = [runtimeDetailsFixtureIDs.completedWorkID];
runtimeDetailsTimelineEvents[6].context.dispatchId = runtimeDetailsFixtureIDs.completedDispatchID;
runtimeDetailsTimelineEvents[6].context.traceIds = [runtimeDetailsFixtureIDs.completedTraceID];
runtimeDetailsTimelineEvents[6].context.workIds = [runtimeDetailsFixtureIDs.completedWorkID];
runtimeDetailsTimelineEvents[7].context.dispatchId = runtimeDetailsFixtureIDs.completedDispatchID;
runtimeDetailsTimelineEvents[7].context.traceIds = [runtimeDetailsFixtureIDs.completedTraceID];
runtimeDetailsTimelineEvents[7].context.workIds = [runtimeDetailsFixtureIDs.completedWorkID];
runtimeDetailsTimelineEvents[8].context.dispatchId = runtimeDetailsFixtureIDs.completedDispatchID;
runtimeDetailsTimelineEvents[8].context.traceIds = [runtimeDetailsFixtureIDs.completedTraceID];
runtimeDetailsTimelineEvents[8].context.workIds = [runtimeDetailsFixtureIDs.completedWorkID];
runtimeDetailsTimelineEvents[9].context.dispatchId = runtimeDetailsFixtureIDs.failedDispatchID;
runtimeDetailsTimelineEvents[9].context.traceIds = [runtimeDetailsFixtureIDs.failedTraceID];
runtimeDetailsTimelineEvents[9].context.workIds = [runtimeDetailsFixtureIDs.failedWorkID];
runtimeDetailsTimelineEvents[10].context.dispatchId = runtimeDetailsFixtureIDs.failedDispatchID;
runtimeDetailsTimelineEvents[10].context.traceIds = [runtimeDetailsFixtureIDs.failedTraceID];
runtimeDetailsTimelineEvents[10].context.workIds = [runtimeDetailsFixtureIDs.failedWorkID];
runtimeDetailsTimelineEvents[11].context.dispatchId = runtimeDetailsFixtureIDs.failedDispatchID;
runtimeDetailsTimelineEvents[11].context.traceIds = [runtimeDetailsFixtureIDs.failedTraceID];
runtimeDetailsTimelineEvents[11].context.workIds = [runtimeDetailsFixtureIDs.failedWorkID];
runtimeDetailsTimelineEvents[12].context.dispatchId = runtimeDetailsFixtureIDs.failedDispatchID;
runtimeDetailsTimelineEvents[12].context.traceIds = [runtimeDetailsFixtureIDs.failedTraceID];
runtimeDetailsTimelineEvents[12].context.workIds = [runtimeDetailsFixtureIDs.failedWorkID];
