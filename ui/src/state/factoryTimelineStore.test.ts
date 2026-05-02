import type { FactoryEvent } from "../api/events";
import { FACTORY_EVENT_TYPES } from "../api/events";
import {
  failureAnalysisTimelineEvents,
  resourceCountAvailablePlaceID,
  resourceCountTimelineEvents,
} from "../components/dashboard/fixtures";

import {
  buildFactoryTimelineSnapshot,
  resolveConfiguredWorkTypeName,
  useFactoryTimelineStore,
} from "./factoryTimelineStore";

const eventTime = "2026-04-16T12:00:00Z";

function event(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

const initialStructureRequest = event(
  "event-1",
  1,
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
        {
          model: "gpt-5.4-mini",
          modelProvider: "openai",
          name: "completer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "done", type: "TERMINAL" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", workType: "story" }],
          name: "Review",
          onFailure: { state: "failed", workType: "story" },
          outputs: [{ state: "review", workType: "story" }],
          worker: "reviewer",
        },
        {
          id: "complete",
          inputs: [{ state: "review", workType: "story" }],
          name: "Complete",
          outputs: [{ state: "done", workType: "story" }],
          worker: "completer",
        },
      ],
    },
  },
);

const runRequest = event(
  "event-run-started",
  0,
  FACTORY_EVENT_TYPES.runRequest,
  {
    factory: {
      workers: [
        {
          model: "gpt-5.4",
          modelProvider: "openai",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
        {
          model: "gpt-5.4-mini",
          modelProvider: "openai",
          name: "completer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "done", type: "TERMINAL" },
            { name: "failed", type: "FAILED" },
          ],
        },
      ],
      workstations: [
        {
          id: "review",
          inputs: [{ state: "new", workType: "story" }],
          name: "Review",
          onFailure: { state: "failed", workType: "story" },
          outputs: [{ state: "review", workType: "story" }],
          worker: "reviewer",
        },
        {
          id: "complete",
          inputs: [{ state: "review", workType: "story" }],
          name: "Complete",
          outputs: [{ state: "done", workType: "story" }],
          worker: "completer",
        },
      ],
    },
    recordedAt: eventTime,
  },
);

const workInput = event("event-2", 2, FACTORY_EVENT_TYPES.workRequest, {
  type: "FACTORY_REQUEST_BATCH",
  works: [
    {
      name: "Timeline Story",
      trace_id: "trace-1",
      work_id: "work-1",
      work_type_id: "story",
    },
  ],
});

const workRequest = event("event-batch-1", 2, FACTORY_EVENT_TYPES.workRequest, {
  source: "api",
  type: "FACTORY_REQUEST_BATCH",
  works: [
    {
      name: "First Batch Story",
      trace_id: "trace-batch",
      work_id: "work-batch-first",
      work_type_id: "story",
    },
    {
      name: "Second Batch Story",
      trace_id: "trace-batch",
      work_id: "work-batch-second",
      work_type_id: "story",
    },
  ],
});

const relationshipChangeRequest = event(
  "event-batch-2",
  2,
  FACTORY_EVENT_TYPES.relationshipChangeRequest,
  {
    relation: {
      request_id: "request-batch-1",
      required_state: "done",
      source_work_id: "work-batch-second",
      source_work_name: "second",
      target_work_id: "work-batch-first",
      target_work_name: "first",
      trace_id: "trace-batch",
      type: "DEPENDS_ON",
    },
  },
);

workRequest.context.requestId = "request-batch-1";
workRequest.context.traceIds = ["trace-batch"];
workRequest.context.workIds = ["work-batch-first", "work-batch-second"];
relationshipChangeRequest.context.requestId = "request-batch-1";
relationshipChangeRequest.context.traceIds = ["trace-batch"];
relationshipChangeRequest.context.workIds = ["work-batch-second", "work-batch-first"];
workInput.context.requestId = "request-work-1";
workInput.context.traceIds = ["trace-1"];
workInput.context.workIds = ["work-1"];

const request = event("event-3", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
  inputs: [
    {
      workId: "work-1",
    },
  ],
  transitionId: "review",
});

const response = event("event-4", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
  durationMillis: 1250,
  outcome: "ACCEPTED",
  output: "Worker output fallback",
  outputWork: [
    {
      name: "Timeline Story",
      state: "done",
      trace_id: "trace-1",
      work_id: "work-1",
      work_type_id: "story",
    },
  ],
  transitionId: "review",
});
request.context.dispatchId = "dispatch-1";
request.context.traceIds = ["trace-1"];
request.context.workIds = ["work-1"];
response.context.dispatchId = "dispatch-1";
response.context.traceIds = ["trace-1"];
response.context.workIds = ["work-1"];

const inferenceRequest = event(
  "event-inference-request-1",
  4,
  FACTORY_EVENT_TYPES.inferenceRequest,
  {
    attempt: 1,
    inferenceRequestId: "dispatch-1/inference-request/1",
    prompt: "Review this timeline story.",
    workingDirectory: "/work/project",
    worktree: "/work/project/.worktrees/story",
  },
);
inferenceRequest.context.dispatchId = "dispatch-1";
inferenceRequest.context.traceIds = ["trace-1"];
inferenceRequest.context.workIds = ["work-1"];
inferenceRequest.context.eventTime = "2026-04-16T12:00:04Z";

const inferenceResponse = event(
  "event-inference-response-1",
  5,
  FACTORY_EVENT_TYPES.inferenceResponse,
  {
    attempt: 1,
    diagnostics: {
      provider: {
        model: "gpt-5.4",
        provider: "openai",
        requestMetadata: {
          prompt_source: "factory-renderer",
          session_id: "session-1",
        },
        responseMetadata: {
          provider_session_id: "session-1",
          retry_count: "0",
        },
      },
    },
    durationMillis: 1250,
    inferenceRequestId: "dispatch-1/inference-request/1",
    outcome: "SUCCEEDED",
    providerSession: {
      id: "session-1",
      kind: "session_id",
      provider: "codex",
    },
    response: "The story is ready for review.",
  },
);
inferenceResponse.context.dispatchId = "dispatch-1";
inferenceResponse.context.traceIds = ["trace-1"];
inferenceResponse.context.workIds = ["work-1"];
inferenceResponse.context.eventTime = "2026-04-16T12:00:05Z";

const failedInferenceRequest = event(
  "event-inference-request-2",
  6,
  FACTORY_EVENT_TYPES.inferenceRequest,
  {
    attempt: 2,
    inferenceRequestId: "dispatch-1/inference-request/2",
    prompt: "Retry the timeline story.",
    workingDirectory: "/work/project",
    worktree: "/work/project/.worktrees/story",
  },
);
failedInferenceRequest.context.dispatchId = "dispatch-1";
failedInferenceRequest.context.traceIds = ["trace-1"];
failedInferenceRequest.context.workIds = ["work-1"];
failedInferenceRequest.context.eventTime = "2026-04-16T12:00:06Z";

const failedInferenceResponse = event(
  "event-inference-response-2",
  7,
  FACTORY_EVENT_TYPES.inferenceResponse,
  {
    attempt: 2,
    durationMillis: 875,
    errorClass: "rate_limited",
    exitCode: 1,
    inferenceRequestId: "dispatch-1/inference-request/2",
    outcome: "FAILED",
  },
);
failedInferenceResponse.context.dispatchId = "dispatch-1";
failedInferenceResponse.context.traceIds = ["trace-1"];
failedInferenceResponse.context.workIds = ["work-1"];
failedInferenceResponse.context.eventTime = "2026-04-16T12:00:07Z";

const lifecycle = event("event-5", 5, FACTORY_EVENT_TYPES.factoryStateResponse, {
  previousState: "RUNNING",
  reason: "test",
  state: "PAUSED",
});

const resourceInitialStructure = event(
  "event-resource-1",
  1,
  FACTORY_EVENT_TYPES.initialStructureRequest,
  {
    factory: {
      resources: [
        { capacity: 2, name: "agent-slot" },
        { capacity: 1, name: "gpu" },
        { capacity: 0, name: "empty-slot" },
      ],
    },
  },
);

const resourceRequest = event(
  "event-resource-2",
  2,
  FACTORY_EVENT_TYPES.dispatchRequest,
  {
    dispatchId: "dispatch-resource",
    inputs: [],
    resources: [{ capacity: 2, name: "agent-slot" }],
    transitionId: "implement",
    workstation: {
      id: "implement",
      inputs: [{ state: "available", workType: "agent-slot" }],
      name: "Implement",
      outputs: [],
      worker: "agent",
    },
  },
);
resourceRequest.context.dispatchId = "dispatch-resource";

const resourceResponse = event(
  "event-resource-3",
  3,
  FACTORY_EVENT_TYPES.dispatchResponse,
  {
    dispatchId: "dispatch-resource",
    outcome: "ACCEPTED",
    outputResources: [{ capacity: 2, name: "agent-slot" }],
    transitionId: "implement",
    workstation: {
      id: "implement",
      inputs: [{ state: "available", workType: "agent-slot" }],
      name: "Implement",
      outputs: [],
      worker: "agent",
    },
  },
);
resourceResponse.context.dispatchId = "dispatch-resource";

const failedWorkInput = event(
  "event-failed-1",
  2,
  FACTORY_EVENT_TYPES.workRequest,
  {
    type: "FACTORY_REQUEST_BATCH",
  works: [
    {
      name: "Blocked Timeline Story",
      trace_id: "trace-failed",
      work_id: "work-failed",
      work_type_id: "story",
      },
    ],
  },
);
failedWorkInput.context.requestId = "request-work-failed";
failedWorkInput.context.traceIds = ["trace-failed"];
failedWorkInput.context.workIds = ["work-failed"];

const failedRequest = event(
  "event-failed-2",
  3,
  FACTORY_EVENT_TYPES.dispatchRequest,
  {
    dispatchId: "dispatch-failed",
    inputs: [
      {
        name: "Blocked Timeline Story",
        trace_id: "trace-failed",
        work_id: "work-failed",
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onFailure: { state: "failed", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "reviewer",
    },
  },
);
failedRequest.context.dispatchId = "dispatch-failed";
failedRequest.context.traceIds = ["trace-failed"];
failedRequest.context.workIds = ["work-failed"];

const failedResponse = event(
  "event-failed-3",
  4,
  FACTORY_EVENT_TYPES.dispatchResponse,
  {
    dispatchId: "dispatch-failed",
    diagnostics: {
      provider: {
        model: "claude-3.7",
        provider: "anthropic",
        requestMetadata: {
          prompt_source: "factory-renderer",
        },
        responseMetadata: {
          retry_count: "1",
        },
      },
    },
    durationMillis: 600,
    failureMessage: "Provider rate limit exceeded.",
    failureReason: "throttled",
    outcome: "FAILED",
    outputWork: [
      {
        name: "Blocked Timeline Story",
        state: "failed",
        trace_id: "trace-failed",
        work_id: "work-failed",
        work_type_id: "story",
      },
    ],
    providerSession: {
      id: "session-failed",
      kind: "session_id",
      provider: "codex",
    },
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onFailure: { state: "failed", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "reviewer",
    },
  },
);
failedResponse.context.dispatchId = "dispatch-failed";
failedResponse.context.traceIds = ["trace-failed"];
failedResponse.context.workIds = ["work-failed"];

const rejectedWorkInput = event(
  "event-rejected-1",
  2,
  FACTORY_EVENT_TYPES.workRequest,
  {
    type: "FACTORY_REQUEST_BATCH",
    works: [
      {
        name: "Rejected Timeline Story",
        trace_id: "trace-rejected",
        work_id: "work-rejected",
        work_type_id: "story",
      },
    ],
  },
);
rejectedWorkInput.context.requestId = "request-work-rejected";
rejectedWorkInput.context.traceIds = ["trace-rejected"];
rejectedWorkInput.context.workIds = ["work-rejected"];

const rejectedRequest = event(
  "event-rejected-2",
  3,
  FACTORY_EVENT_TYPES.dispatchRequest,
  {
    dispatchId: "dispatch-rejected",
    inputs: [
      {
        name: "Rejected Timeline Story",
        trace_id: "trace-rejected",
        work_id: "work-rejected",
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onRejection: { state: "new", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "reviewer",
    },
  },
);
rejectedRequest.context.dispatchId = "dispatch-rejected";
rejectedRequest.context.traceIds = ["trace-rejected"];
rejectedRequest.context.workIds = ["work-rejected"];

const rejectedInferenceRequest = event(
  "event-rejected-inference-request",
  4,
  FACTORY_EVENT_TYPES.inferenceRequest,
  {
    attempt: 1,
    dispatchId: "dispatch-rejected",
    inferenceRequestId: "dispatch-rejected/inference-request/1",
    prompt: "Review the story and explain why it needs more work.",
    transitionId: "review",
    workingDirectory: "/work/rejected",
    worktree: "/work/rejected/.worktrees/story",
  },
);
rejectedInferenceRequest.context.dispatchId = "dispatch-rejected";
rejectedInferenceRequest.context.traceIds = ["trace-rejected"];
rejectedInferenceRequest.context.workIds = ["work-rejected"];
rejectedInferenceRequest.context.eventTime = "2026-04-16T12:00:04Z";

const rejectedInferenceResponse = event(
  "event-rejected-inference-response",
  5,
  FACTORY_EVENT_TYPES.inferenceResponse,
  {
    attempt: 1,
    dispatchId: "dispatch-rejected",
    durationMillis: 540,
    inferenceRequestId: "dispatch-rejected/inference-request/1",
    outcome: "SUCCEEDED",
    response: "The story needs another pass before approval.",
    transitionId: "review",
  },
);
rejectedInferenceResponse.context.dispatchId = "dispatch-rejected";
rejectedInferenceResponse.context.traceIds = ["trace-rejected"];
rejectedInferenceResponse.context.workIds = ["work-rejected"];
rejectedInferenceResponse.context.eventTime = "2026-04-16T12:00:05Z";

const rejectedResponse = event(
  "event-rejected-3",
  6,
  FACTORY_EVENT_TYPES.dispatchResponse,
  {
    dispatchId: "dispatch-rejected",
    diagnostics: {
        provider: {
          model: "gpt-5.4-mini",
          provider: "codex",
          requestMetadata: {
            prompt_source: "factory-renderer",
        },
        responseMetadata: {
          retry_count: "0",
        },
      },
    },
    durationMillis: 540,
    feedback: "Please fix the missing acceptance test.",
    outcome: "REJECTED",
    output: "Fallback rejection summary",
    outputWork: [
      {
        name: "Rejected Timeline Story",
        state: "new",
        trace_id: "trace-rejected",
        work_id: "work-rejected",
        work_type_id: "story",
      },
    ],
    providerSession: {
      id: "session-rejected",
      kind: "session_id",
      provider: "codex",
    },
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onRejection: { state: "new", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "reviewer",
    },
  },
);
rejectedResponse.context.dispatchId = "dispatch-rejected";
rejectedResponse.context.traceIds = ["trace-rejected"];
rejectedResponse.context.workIds = ["work-rejected"];

const scriptPendingWorkInput = event(
  "event-script-pending-1",
  8,
  FACTORY_EVENT_TYPES.workRequest,
  {
    type: "FACTORY_REQUEST_BATCH",
    works: [
      {
        name: "Pending Script Story",
        trace_id: "trace-script-pending",
        work_id: "work-script-pending",
        work_type_id: "story",
      },
    ],
  },
);
scriptPendingWorkInput.context.requestId = "request-script-pending";
scriptPendingWorkInput.context.traceIds = ["trace-script-pending"];
scriptPendingWorkInput.context.workIds = ["work-script-pending"];

const scriptPendingDispatchRequest = event(
  "event-script-pending-2",
  9,
  FACTORY_EVENT_TYPES.dispatchRequest,
  {
    dispatchId: "dispatch-script-pending",
    inputs: [
      {
        name: "Pending Script Story",
        trace_id: "trace-script-pending",
        work_id: "work-script-pending",
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    worker: {
      name: "script-reviewer",
      executorProvider: "SCRIPT_WRAP",
      type: "SCRIPT_WORKER",
    },
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onFailure: { state: "failed", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "script-reviewer",
    },
  },
);
scriptPendingDispatchRequest.context.dispatchId = "dispatch-script-pending";
scriptPendingDispatchRequest.context.traceIds = ["trace-script-pending"];
scriptPendingDispatchRequest.context.workIds = ["work-script-pending"];

const scriptPendingRequest = event(
  "event-script-pending-3",
  10,
  FACTORY_EVENT_TYPES.scriptRequest,
  {
    args: ["--work", "work-script-pending"],
    attempt: 1,
    command: "script-tool",
    dispatchId: "dispatch-script-pending",
    scriptRequestId: "dispatch-script-pending/script-request/1",
    transitionId: "review",
  },
);
scriptPendingRequest.context.dispatchId = "dispatch-script-pending";
scriptPendingRequest.context.traceIds = ["trace-script-pending"];
scriptPendingRequest.context.workIds = ["work-script-pending"];
scriptPendingRequest.context.eventTime = "2026-04-16T12:00:10Z";

const scriptSuccessWorkInput = event(
  "event-script-success-1",
  11,
  FACTORY_EVENT_TYPES.workRequest,
  {
    type: "FACTORY_REQUEST_BATCH",
    works: [
      {
        name: "Successful Script Story",
        trace_id: "trace-script-success",
        work_id: "work-script-success",
        work_type_id: "story",
      },
    ],
  },
);
scriptSuccessWorkInput.context.requestId = "request-script-success";
scriptSuccessWorkInput.context.traceIds = ["trace-script-success"];
scriptSuccessWorkInput.context.workIds = ["work-script-success"];

const scriptSuccessDispatchRequest = event(
  "event-script-success-2",
  12,
  FACTORY_EVENT_TYPES.dispatchRequest,
  {
    dispatchId: "dispatch-script-success",
    inputs: [
      {
        name: "Successful Script Story",
        trace_id: "trace-script-success",
        work_id: "work-script-success",
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    worker: {
      name: "script-reviewer",
      executorProvider: "SCRIPT_WRAP",
      type: "SCRIPT_WORKER",
    },
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onFailure: { state: "failed", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "script-reviewer",
    },
  },
);
scriptSuccessDispatchRequest.context.dispatchId = "dispatch-script-success";
scriptSuccessDispatchRequest.context.traceIds = ["trace-script-success"];
scriptSuccessDispatchRequest.context.workIds = ["work-script-success"];

const scriptSuccessRequest = event(
  "event-script-success-3",
  13,
  FACTORY_EVENT_TYPES.scriptRequest,
  {
    args: ["--work", "work-script-success"],
    attempt: 1,
    command: "script-tool",
    dispatchId: "dispatch-script-success",
    scriptRequestId: "dispatch-script-success/script-request/1",
    transitionId: "review",
  },
);
scriptSuccessRequest.context.dispatchId = "dispatch-script-success";
scriptSuccessRequest.context.traceIds = ["trace-script-success"];
scriptSuccessRequest.context.workIds = ["work-script-success"];
scriptSuccessRequest.context.eventTime = "2026-04-16T12:00:13Z";

const scriptSuccessResponse = event(
  "event-script-success-4",
  14,
  FACTORY_EVENT_TYPES.scriptResponse,
  {
    attempt: 1,
    dispatchId: "dispatch-script-success",
    durationMillis: 222,
    outcome: "SUCCEEDED",
    scriptRequestId: "dispatch-script-success/script-request/1",
    stderr: "",
    stdout: "script success stdout\n",
    transitionId: "review",
  },
);
scriptSuccessResponse.context.dispatchId = "dispatch-script-success";
scriptSuccessResponse.context.traceIds = ["trace-script-success"];
scriptSuccessResponse.context.workIds = ["work-script-success"];
scriptSuccessResponse.context.eventTime = "2026-04-16T12:00:14Z";

const scriptSuccessDispatchResponse = event(
  "event-script-success-5",
  15,
  FACTORY_EVENT_TYPES.dispatchResponse,
  {
    dispatchId: "dispatch-script-success",
    durationMillis: 222,
    outcome: "ACCEPTED",
    output: "legacy script success output",
    outputWork: [
      {
        name: "Successful Script Story",
        trace_id: "trace-script-success",
        work_id: "work-script-success",
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      outputs: [{ state: "done", workType: "story" }],
      worker: "script-reviewer",
    },
  },
);
scriptSuccessDispatchResponse.context.dispatchId = "dispatch-script-success";
scriptSuccessDispatchResponse.context.traceIds = ["trace-script-success"];
scriptSuccessDispatchResponse.context.workIds = ["work-script-success"];

const scriptFailedWorkInput = event(
  "event-script-failed-1",
  16,
  FACTORY_EVENT_TYPES.workRequest,
  {
    type: "FACTORY_REQUEST_BATCH",
    works: [
      {
        name: "Failed Script Story",
        trace_id: "trace-script-failed",
        work_id: "work-script-failed",
        work_type_id: "story",
      },
    ],
  },
);
scriptFailedWorkInput.context.requestId = "request-script-failed";
scriptFailedWorkInput.context.traceIds = ["trace-script-failed"];
scriptFailedWorkInput.context.workIds = ["work-script-failed"];

const scriptFailedDispatchRequest = event(
  "event-script-failed-2",
  17,
  FACTORY_EVENT_TYPES.dispatchRequest,
  {
    dispatchId: "dispatch-script-failed",
    inputs: [
      {
        name: "Failed Script Story",
        trace_id: "trace-script-failed",
        work_id: "work-script-failed",
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    worker: {
      name: "script-reviewer",
      executorProvider: "SCRIPT_WRAP",
      type: "SCRIPT_WORKER",
    },
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onFailure: { state: "failed", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "script-reviewer",
    },
  },
);
scriptFailedDispatchRequest.context.dispatchId = "dispatch-script-failed";
scriptFailedDispatchRequest.context.traceIds = ["trace-script-failed"];
scriptFailedDispatchRequest.context.workIds = ["work-script-failed"];

const scriptFailedRequest = event(
  "event-script-failed-3",
  18,
  FACTORY_EVENT_TYPES.scriptRequest,
  {
    args: ["--work", "work-script-failed"],
    attempt: 1,
    command: "script-tool",
    dispatchId: "dispatch-script-failed",
    scriptRequestId: "dispatch-script-failed/script-request/1",
    transitionId: "review",
  },
);
scriptFailedRequest.context.dispatchId = "dispatch-script-failed";
scriptFailedRequest.context.traceIds = ["trace-script-failed"];
scriptFailedRequest.context.workIds = ["work-script-failed"];
scriptFailedRequest.context.eventTime = "2026-04-16T12:00:18Z";

const scriptFailedResponse = event(
  "event-script-failed-4",
  19,
  FACTORY_EVENT_TYPES.scriptResponse,
  {
    attempt: 1,
    dispatchId: "dispatch-script-failed",
    durationMillis: 500,
    failureType: "TIMEOUT",
    outcome: "TIMED_OUT",
    scriptRequestId: "dispatch-script-failed/script-request/1",
    stderr: "script timed out\n",
    stdout: "",
    transitionId: "review",
  },
);
scriptFailedResponse.context.dispatchId = "dispatch-script-failed";
scriptFailedResponse.context.traceIds = ["trace-script-failed"];
scriptFailedResponse.context.workIds = ["work-script-failed"];
scriptFailedResponse.context.eventTime = "2026-04-16T12:00:19Z";

const scriptFailedDispatchResponse = event(
  "event-script-failed-5",
  20,
  FACTORY_EVENT_TYPES.dispatchResponse,
  {
    dispatchId: "dispatch-script-failed",
    durationMillis: 500,
    failureMessage: "Script timed out.",
    failureReason: "script_timeout",
    outcome: "FAILED",
    output: "legacy script failure output",
    outputWork: [
      {
        name: "Failed Script Story",
        trace_id: "trace-script-failed",
        work_id: "work-script-failed",
        work_type_id: "story",
      },
    ],
    transitionId: "review",
    workstation: {
      id: "review",
      inputs: [{ state: "new", workType: "story" }],
      name: "Review",
      onFailure: { state: "failed", workType: "story" },
      outputs: [{ state: "done", workType: "story" }],
      worker: "script-reviewer",
    },
  },
);
scriptFailedDispatchResponse.context.dispatchId = "dispatch-script-failed";
scriptFailedDispatchResponse.context.traceIds = ["trace-script-failed"];
scriptFailedDispatchResponse.context.workIds = ["work-script-failed"];

describe("factory timeline reconstruction", () => {
  afterEach(() => {
    useFactoryTimelineStore.getState().reset();
  });

  it("prefers configured work-type names for submit-work options", () => {
    expect(resolveConfiguredWorkTypeName("story-internal", "story")).toBe(
      "story",
    );
    expect(resolveConfiguredWorkTypeName("story-fallback")).toBe(
      "story-fallback",
    );
  });

  it("builds dashboard topology from the run-started generated factory config", () => {
    const snapshot = buildFactoryTimelineSnapshot([runRequest, workInput], 2);

    expect(JSON.stringify(runRequest.payload)).not.toContain("effectiveConfig");
    expect(snapshot.dashboard.topology.submit_work_types).toEqual([
      { work_type_name: "story" },
    ]);
    expect(snapshot.dashboard.topology.workstation_node_ids).toEqual([
      "complete",
      "review",
    ]);
    expect(snapshot.dashboard.runtime.place_token_counts?.["story:new"]).toBe(
      1,
    );
    expect(
      snapshot.dashboard.runtime.current_work_items_by_place_id?.["story:new"],
    ).toEqual([
      {
        display_name: "Timeline Story",
        trace_id: "trace-1",
        work_id: "work-1",
        work_type_id: "story",
      },
    ]);
  });

  it("reconstructs graph, totals, trace, terminal work, and provider sessions at discrete ticks", () => {
    const tickTwo = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workInput, request, response],
      2,
    );
    const tickThree = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workInput, request, response],
      3,
    );
    const tickFour = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        workInput,
        request,
        response,
        inferenceRequest,
        inferenceResponse,
      ],
      5,
    );

    expect(tickTwo.dashboard.runtime.place_token_counts?.["story:new"]).toBe(1);
    expect(
      tickTwo.dashboard.runtime.current_work_items_by_place_id?.["story:new"],
    ).toEqual([
      {
        display_name: "Timeline Story",
        trace_id: "trace-1",
        work_id: "work-1",
        work_type_id: "story",
      },
    ]);
    expect(
      tickTwo.dashboard.runtime.current_work_items_by_place_id?.[
        "story:review"
      ],
    ).toEqual([]);
    expect(tickTwo.dashboard.topology.edges).toEqual([
      {
        edge_id: "review:complete:story:review:accepted",
        from_node_id: "review",
        outcome_kind: "accepted",
        state_category: "PROCESSING",
        state_value: "review",
        to_node_id: "complete",
        via_place_id: "story:review",
        work_type_id: "story",
      },
    ]);
    expect(tickTwo.dashboard.runtime.in_flight_dispatch_count).toBe(0);
    expect(tickThree.dashboard.runtime.in_flight_dispatch_count).toBe(1);
    expect(
      tickThree.dashboard.runtime.place_token_counts?.["story:new"],
    ).toBeUndefined();
    expect(
      tickThree.dashboard.runtime.current_work_items_by_place_id?.["story:new"],
    ).toEqual([]);
    expect(tickThree.dashboard.runtime.active_workstation_node_ids).toEqual([
      "review",
    ]);
    expect(tickFour.dashboard.runtime.session.completed_count).toBe(1);
    expect(tickFour.dashboard.runtime.session.completed_work_labels).toEqual([
      "Timeline Story",
    ]);
    expect(
      tickFour.dashboard.runtime.session.provider_sessions?.[0]
        ?.provider_session?.id,
    ).toBe("session-1");
    expect(
      tickFour.dashboard.runtime.current_work_items_by_place_id?.["story:done"],
    ).toBeUndefined();
    expect(tickFour.tracesByWorkID["work-1"].dispatches[0].dispatch_id).toBe(
      "dispatch-1",
    );
  });

  it("removes submitted occupancy when request consumes a distinct runtime token", () => {
    const tickTwo = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workInput, request],
      2,
    );
    const tickThree = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workInput, request],
      3,
    );

    expect(tickTwo.dashboard.runtime.place_token_counts?.["story:new"]).toBe(1);
    expect(
      tickTwo.dashboard.runtime.current_work_items_by_place_id?.["story:new"],
    ).toEqual([
      {
        display_name: "Timeline Story",
        trace_id: "trace-1",
        work_id: "work-1",
        work_type_id: "story",
      },
    ]);
    expect(
      tickTwo.dashboard.runtime.current_work_items_by_place_id?.[
        "story:review"
      ],
    ).toEqual([]);
    expect(
      tickThree.dashboard.runtime.place_token_counts?.["story:new"],
    ).toBeUndefined();
    expect(
      tickThree.dashboard.runtime.current_work_items_by_place_id?.["story:new"],
    ).toEqual([]);
    expect(
      tickThree.dashboard.runtime.current_work_items_by_place_id?.[
        "story:review"
      ],
    ).toEqual([]);
    expect(
      tickThree.dashboard.runtime.current_work_items_by_place_id?.[
        "story:done"
      ],
    ).toBeUndefined();
    expect(tickThree.dashboard.runtime.in_flight_dispatch_count).toBe(1);
  });

  it("replays resource availability from initial capacity through dispatch consume and release", () => {
    const events = [
      resourceInitialStructure,
      resourceRequest,
      resourceResponse,
    ];
    const idle = buildFactoryTimelineSnapshot(events, 1);
    const active = buildFactoryTimelineSnapshot(events, 2);
    const released = buildFactoryTimelineSnapshot(events, 3);

    expect(
      idle.dashboard.runtime.place_token_counts?.["agent-slot:available"],
    ).toBe(2);
    expect(idle.dashboard.runtime.place_token_counts?.["gpu:available"]).toBe(
      1,
    );
    expect(
      idle.dashboard.runtime.place_token_counts?.["empty-slot:available"],
    ).toBeUndefined();
    expect(
      active.dashboard.runtime.place_token_counts?.["agent-slot:available"],
    ).toBe(1);
    expect(active.dashboard.runtime.in_flight_dispatch_count).toBe(1);
    expect(
      released.dashboard.runtime.place_token_counts?.["agent-slot:available"],
    ).toBe(2);
    expect(released.dashboard.runtime.in_flight_dispatch_count).toBe(0);
  });

  it("accepts canonical camelCase factory events from the live SSE stream", () => {
    const canonicalInitialStructure = event(
      "event-camel-1",
      1,
      FACTORY_EVENT_TYPES.initialStructureRequest,
      {
        factory: {
          resources: [{ capacity: 10, name: "executor-slot" }],
          workers: [
            {
              executorProvider: "SCRIPT_WRAP",
              modelProvider: "CODEX",
              name: "processor",
              type: "MODEL_WORKER",
            },
          ],
          workTypes: [
            {
              name: "thoughts",
              states: [
                { name: "init", type: "INITIAL" },
                { name: "complete", type: "TERMINAL" },
                { name: "failed", type: "FAILED" },
              ],
            },
          ],
          workstations: [
            {
              id: "ideafy",
              inputs: [
                { state: "available", workType: "executor-slot" },
                { state: "init", workType: "thoughts" },
              ],
              behavior: "STANDARD",
              name: "ideafy",
              onFailure: { state: "failed", workType: "thoughts" },
              onRejection: { state: "failed", workType: "thoughts" },
              outputs: [
                { state: "available", workType: "executor-slot" },
                { state: "complete", workType: "thoughts" },
              ],
              worker: "processor",
            },
          ],
        },
      },
    );

    const canonicalWorkRequest = event(
      "event-camel-2",
      2,
      FACTORY_EVENT_TYPES.workRequest,
      {
        source: "external-submit",
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "agents-04-21-2026",
            state: "complete",
            trace_id: "trace-camel-1",
            work_id: "work-camel-1",
            work_type_name: "thoughts",
          },
        ],
      },
    );
    canonicalWorkRequest.context.requestId = "request-camel-1";
    canonicalWorkRequest.context.traceIds = ["trace-camel-1"];
    canonicalWorkRequest.context.workIds = ["work-camel-1"];

    const canonicalDispatchRequest = event(
      "event-camel-3",
      3,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        dispatchId: "dispatch-camel-1",
        inputs: [
          {
            name: "agents-04-21-2026",
            trace_id: "trace-camel-1",
            work_id: "work-camel-1",
            work_type_name: "thoughts",
          },
        ],
        resources: [{ capacity: 10, name: "executor-slot" }],
        transitionId: "ideafy",
        worker: {
          executorProvider: "SCRIPT_WRAP",
          modelProvider: "CODEX",
          name: "processor",
        },
        workstation: {
          id: "ideafy",
          inputs: [
            { state: "available", workType: "executor-slot" },
            { state: "init", workType: "thoughts" },
          ],
          behavior: "STANDARD",
          name: "ideafy",
          onFailure: { state: "failed", workType: "thoughts" },
          onRejection: { state: "failed", workType: "thoughts" },
          outputs: [
            { state: "available", workType: "executor-slot" },
            { state: "complete", workType: "thoughts" },
          ],
          worker: "processor",
        },
      },
    );
    canonicalDispatchRequest.context.dispatchId = "dispatch-camel-1";
    canonicalDispatchRequest.context.traceIds = ["trace-camel-1"];
    canonicalDispatchRequest.context.workIds = ["work-camel-1"];

    const canonicalDispatchResponse = event(
      "event-camel-4",
      4,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        diagnostics: {
          provider: {
            model: "gpt-5.4",
            provider: "codex",
            requestMetadata: {
              prompt_source: "factory-renderer",
            },
            responseMetadata: {
              provider_session_id: "session-camel-1",
            },
          },
        },
        dispatchId: "dispatch-camel-1",
        durationMillis: 900,
        outcome: "ACCEPTED",
        outputResources: [{ capacity: 10, name: "executor-slot" }],
        outputWork: [
          {
            name: "agents-04-21-2026",
            trace_id: "trace-camel-1",
            work_id: "work-camel-1",
            work_type_name: "thoughts",
          },
        ],
        providerSession: {
          id: "session-camel-1",
          kind: "session_id",
          provider: "codex",
        },
        transitionId: "ideafy",
        workstation: {
          id: "ideafy",
          inputs: [
            { state: "available", workType: "executor-slot" },
            { state: "init", workType: "thoughts" },
          ],
          behavior: "STANDARD",
          name: "ideafy",
          onFailure: { state: "failed", workType: "thoughts" },
          onRejection: { state: "failed", workType: "thoughts" },
          outputs: [
            { state: "available", workType: "executor-slot" },
            { state: "complete", workType: "thoughts" },
          ],
          worker: "processor",
        },
      },
    );
    canonicalDispatchResponse.context.dispatchId = "dispatch-camel-1";
    canonicalDispatchResponse.context.traceIds = ["trace-camel-1"];
    canonicalDispatchResponse.context.workIds = ["work-camel-1"];

    const events = [
      canonicalInitialStructure,
      canonicalWorkRequest,
      canonicalDispatchRequest,
      canonicalDispatchResponse,
    ];
    const queued = buildFactoryTimelineSnapshot(events, 2);
    const active = buildFactoryTimelineSnapshot(events, 3);
    const completed = buildFactoryTimelineSnapshot(events, 4);

    expect(queued.dashboard.topology.workstation_node_ids).toEqual(["ideafy"]);
    expect(queued.dashboard.runtime.place_token_counts?.["executor-slot:available"]).toBe(10);
    expect(queued.dashboard.runtime.current_work_items_by_place_id?.["thoughts:init"]).toEqual([
      {
        display_name: "agents-04-21-2026",
        trace_id: "trace-camel-1",
        work_id: "work-camel-1",
        work_type_id: "thoughts",
      },
    ]);

    expect(active.dashboard.runtime.in_flight_dispatch_count).toBe(1);
    expect(active.dashboard.runtime.place_token_counts?.["executor-slot:available"]).toBe(9);
    expect(
      active.dashboard.runtime.active_executions_by_dispatch_id?.["dispatch-camel-1"]
        ?.model_provider,
    ).toBe("CODEX");
    expect(
      active.dashboard.runtime.active_executions_by_dispatch_id?.["dispatch-camel-1"]
        ?.provider,
    ).toBe("SCRIPT_WRAP");
    expect(active.dashboard.runtime.current_work_items_by_place_id?.["thoughts:init"]).toEqual(
      [],
    );

    expect(completed.dashboard.runtime.in_flight_dispatch_count).toBe(0);
    expect(completed.dashboard.runtime.place_token_counts?.["executor-slot:available"]).toBe(10);
    expect(
      completed.dashboard.runtime.current_work_items_by_place_id?.["thoughts:complete"],
    ).toBeUndefined();
    expect(
      completed.dashboard.runtime.place_occupancy_work_items_by_place_id?.[
        "thoughts:complete"
      ],
    ).toEqual([
      {
        display_name: "agents-04-21-2026",
        trace_id: "trace-camel-1",
        work_id: "work-camel-1",
        work_type_id: "thoughts",
      },
    ]);
    expect(
      completed.dashboard.runtime.session.provider_sessions?.[0]?.provider_session?.id,
    ).toBe("session-camel-1");
  });

  it("does not project retired factory-config aliases from initial-structure events", () => {
    const legacyInitialStructure = event(
      "event-legacy-factory-1",
      1,
      FACTORY_EVENT_TYPES.initialStructureRequest,
      {
        factory: {
          workers: [
            {
              model_provider: "CODEX",
              name: "processor",
              provider: "SCRIPT_WRAP",
              type: "MODEL_WORKER",
            },
          ],
          work_types: [
            {
              name: "thoughts",
              states: [
                { name: "init", type: "INITIAL" },
                { name: "complete", type: "TERMINAL" },
              ],
            },
          ],
          workstations: [
            {
              id: "ideafy",
              inputs: [{ state: "init", work_type: "thoughts" }],
              kind: "STANDARD",
              name: "ideafy",
              on_failure: { state: "complete", work_type: "thoughts" },
              outputs: [{ state: "complete", work_type: "thoughts" }],
              worker: "processor",
            },
          ],
        },
      },
    );

    const projected = buildFactoryTimelineSnapshot([legacyInitialStructure], 1);

    expect(projected.dashboard.topology.submit_work_types).toEqual([]);
    expect(projected.dashboard.topology.workstation_nodes_by_id?.ideafy).toMatchObject({
      input_place_ids: [":init"],
      input_work_type_ids: [],
      output_place_ids: [":complete"],
      output_work_type_ids: [],
    });
  });

  it("replays the resource-count smoke fixture across idle, active, and released ticks", () => {
    const idle = buildFactoryTimelineSnapshot(resourceCountTimelineEvents, 1);
    const active = buildFactoryTimelineSnapshot(resourceCountTimelineEvents, 3);
    const released = buildFactoryTimelineSnapshot(
      resourceCountTimelineEvents,
      4,
    );

    expect(
      idle.dashboard.runtime.place_token_counts?.[
        resourceCountAvailablePlaceID
      ],
    ).toBe(2);
    expect(
      active.dashboard.runtime.place_token_counts?.[
        resourceCountAvailablePlaceID
      ],
    ).toBe(1);
    expect(active.dashboard.runtime.in_flight_dispatch_count).toBe(1);
    expect(
      released.dashboard.runtime.place_token_counts?.[
        resourceCountAvailablePlaceID
      ],
    ).toBe(2);
    expect(released.dashboard.runtime.in_flight_dispatch_count).toBe(0);
  });

  it("preserves batch membership and dependency relations from canonical events", () => {
    const tickTwo = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workRequest, relationshipChangeRequest],
      2,
    );

    expect(
      tickTwo.workRequestsByID["request-batch-1"].work_items?.map(
        (item) => item.id,
      ),
    ).toEqual(["work-batch-first", "work-batch-second"]);
    expect(tickTwo.relationsByWorkID["work-batch-second"]).toEqual([
      {
        request_id: "request-batch-1",
        required_state: "done",
        source_work_id: "work-batch-second",
        source_work_name: "second",
        target_work_id: "work-batch-first",
        target_work_name: "first",
        trace_id: "trace-batch",
        type: "DEPENDS_ON",
      },
    ]);
    expect(tickTwo.tracesByWorkID["work-batch-second"].request_ids).toEqual([
      "request-batch-1",
    ]);
    expect(
      tickTwo.tracesByWorkID["work-batch-second"].relations?.[0]
        ?.target_work_id,
    ).toBe("work-batch-first");
  });

  it("retains explicit chaining predecessors through active and completed timeline projections", () => {
    const chainingWorkRequest = event(
      "event-chaining-work-request",
      2,
      FACTORY_EVENT_TYPES.workRequest,
      {
        source: "api",
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Fan In A",
            trace_id: "chain-a",
            work_id: "work-chain-a",
            work_type_id: "story",
          },
          {
            current_chaining_trace_id: "chain-b",
            name: "Fan In B",
            trace_id: "chain-b",
            work_id: "work-chain-b",
            work_type_id: "story",
          },
        ],
      },
    );
    chainingWorkRequest.context.requestId = "request-chain";
    chainingWorkRequest.context.traceIds = ["chain-a", "chain-b"];
    chainingWorkRequest.context.workIds = ["work-chain-a", "work-chain-b"];

    const chainingDispatchRequest = event(
      "event-chaining-dispatch-request",
      3,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-chain",
        inputs: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Fan In A",
            trace_id: "chain-a",
            work_id: "work-chain-a",
            work_type_id: "story",
          },
          {
            current_chaining_trace_id: "chain-b",
            name: "Fan In B",
            trace_id: "chain-b",
            work_id: "work-chain-b",
            work_type_id: "story",
          },
        ],
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        transitionId: "complete",
        workstation: {
          id: "complete",
          inputs: [{ state: "review", workType: "story" }],
          name: "Complete",
          outputs: [{ state: "done", workType: "story" }],
          worker: "completer",
        },
      },
    );
    chainingDispatchRequest.context.dispatchId = "dispatch-chain";
    chainingDispatchRequest.context.traceIds = ["chain-a", "chain-b"];
    chainingDispatchRequest.context.workIds = ["work-chain-a", "work-chain-b"];

    const chainingDispatchResponse = event(
      "event-chaining-dispatch-response",
      4,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-chain",
        durationMillis: 980,
        outcome: "ACCEPTED",
        outputWork: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Fan In Result",
            previous_chaining_trace_ids: ["chain-a", "chain-b"],
            trace_id: "chain-a",
            work_id: "work-chain-result",
            work_type_id: "story",
          },
        ],
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        transitionId: "complete",
        workstation: {
          id: "complete",
          inputs: [{ state: "review", workType: "story" }],
          name: "Complete",
          outputs: [{ state: "done", workType: "story" }],
          worker: "completer",
        },
      },
    );
    chainingDispatchResponse.context.dispatchId = "dispatch-chain";
    chainingDispatchResponse.context.traceIds = ["chain-a", "chain-b"];
    chainingDispatchResponse.context.workIds = ["work-chain-a", "work-chain-b"];

    const tickThree = buildFactoryTimelineSnapshot(
      [initialStructureRequest, chainingWorkRequest, chainingDispatchRequest],
      3,
    );
    const tickFour = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        chainingWorkRequest,
        chainingDispatchRequest,
        chainingDispatchResponse,
      ],
      4,
    );

    expect(
      tickThree.dashboard.runtime.active_executions_by_dispatch_id?.[
        "dispatch-chain"
      ]?.work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Fan In A",
        trace_id: "chain-a",
        work_id: "work-chain-a",
        work_type_id: "story",
      },
      {
        current_chaining_trace_id: "chain-b",
        display_name: "Fan In B",
        trace_id: "chain-b",
        work_id: "work-chain-b",
        work_type_id: "story",
      },
    ]);
    expect(
      tickThree.workstationRequestsByDispatchID["dispatch-chain"]?.request_view
        ?.input_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Fan In A",
        trace_id: "chain-a",
        work_id: "work-chain-a",
        work_type_id: "story",
      },
      {
        current_chaining_trace_id: "chain-b",
        display_name: "Fan In B",
        trace_id: "chain-b",
        work_id: "work-chain-b",
        work_type_id: "story",
      },
    ]);
    expect(
      tickFour.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-chain"
      ]?.request?.input_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Fan In A",
        trace_id: "chain-a",
        work_id: "work-chain-a",
        work_type_id: "story",
      },
      {
        current_chaining_trace_id: "chain-b",
        display_name: "Fan In B",
        trace_id: "chain-b",
        work_id: "work-chain-b",
        work_type_id: "story",
      },
    ]);
    expect(
      tickFour.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-chain"
      ]?.response?.output_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Fan In Result",
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        trace_id: "chain-a",
        work_id: "work-chain-result",
        work_type_id: "story",
      },
    ]);
    expect(
      tickFour.workstationRequestsByDispatchID["dispatch-chain"]?.response_view
        ?.output_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Fan In Result",
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        trace_id: "chain-a",
        work_id: "work-chain-result",
        work_type_id: "story",
      },
    ]);
    expect(tickFour.workstationRequestsByDispatchID["dispatch-chain"]?.work_items)
      .toEqual(
        expect.arrayContaining([
          {
            current_chaining_trace_id: "chain-a",
            display_name: "Fan In Result",
            previous_chaining_trace_ids: ["chain-a", "chain-b"],
            trace_id: "chain-a",
            work_id: "work-chain-result",
            work_type_id: "story",
          },
        ]),
      );
    expect(
      tickFour.tracesByWorkID["work-chain-result"].dispatches[0],
    ).toMatchObject({
      current_chaining_trace_id: "chain-a",
      dispatch_id: "dispatch-chain",
      input_items: [
        {
          current_chaining_trace_id: "chain-a",
          work_id: "work-chain-a",
        },
        {
          current_chaining_trace_id: "chain-b",
          work_id: "work-chain-b",
        },
      ],
      output_items: [
        {
          current_chaining_trace_id: "chain-a",
          previous_chaining_trace_ids: ["chain-a", "chain-b"],
          work_id: "work-chain-result",
        },
      ],
      previous_chaining_trace_ids: ["chain-a", "chain-b"],
      trace_id: "chain-a",
      trace_ids: ["chain-a", "chain-b"],
    });
    expect(tickFour.tracesByWorkID["work-chain-result"].work_items).toEqual(
      expect.arrayContaining([
        {
          current_chaining_trace_id: "chain-a",
          display_name: "Fan In Result",
          previous_chaining_trace_ids: ["chain-a", "chain-b"],
          trace_id: "chain-a",
          work_id: "work-chain-result",
          work_type_id: "story",
        },
      ]),
    );
  });

  it("prefers context dispatch chaining lineage over deprecated payload copies", () => {
    const chainingWorkRequest = event(
      "event-context-chaining-work-request",
      2,
      FACTORY_EVENT_TYPES.workRequest,
      {
        source: "api",
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Fan In A",
            trace_id: "chain-a",
            work_id: "work-chain-a",
            work_type_id: "story",
          },
          {
            current_chaining_trace_id: "chain-b",
            name: "Fan In B",
            trace_id: "chain-b",
            work_id: "work-chain-b",
            work_type_id: "story",
          },
        ],
      },
    );
    chainingWorkRequest.context.requestId = "request-chain";
    chainingWorkRequest.context.traceIds = ["chain-a", "chain-b"];
    chainingWorkRequest.context.workIds = ["work-chain-a", "work-chain-b"];

    const chainingDispatchRequest = event(
      "event-context-chaining-dispatch-request",
      3,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        current_chaining_trace_id: "payload-chain-stale",
        dispatchId: "dispatch-chain",
        inputs: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Fan In A",
            trace_id: "chain-a",
            work_id: "work-chain-a",
            work_type_id: "story",
          },
          {
            current_chaining_trace_id: "chain-b",
            name: "Fan In B",
            trace_id: "chain-b",
            work_id: "work-chain-b",
            work_type_id: "story",
          },
        ],
        previous_chaining_trace_ids: ["payload-chain-z", "payload-chain-y"],
        transitionId: "complete",
        workstation: {
          id: "complete",
          inputs: [{ state: "review", work_type: "story" }],
          name: "Complete",
          outputs: [{ state: "done", work_type: "story" }],
          worker: "completer",
        },
      },
    );
    chainingDispatchRequest.context.currentChainingTraceId = "chain-a";
    chainingDispatchRequest.context.previousChainingTraceIds = [
      "chain-a",
      "chain-b",
    ];
    chainingDispatchRequest.context.dispatchId = "dispatch-chain";
    chainingDispatchRequest.context.traceIds = ["chain-a", "chain-b"];
    chainingDispatchRequest.context.workIds = ["work-chain-a", "work-chain-b"];

    const chainingDispatchResponse = event(
      "event-context-chaining-dispatch-response",
      4,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        current_chaining_trace_id: "payload-chain-stale",
        dispatchId: "dispatch-chain",
        durationMillis: 980,
        outcome: "ACCEPTED",
        outputWork: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Fan In Result",
            previous_chaining_trace_ids: ["chain-a", "chain-b"],
            trace_id: "chain-a",
            work_id: "work-chain-result",
            work_type_id: "story",
          },
        ],
        previous_chaining_trace_ids: ["payload-chain-z", "payload-chain-y"],
        transitionId: "complete",
        workstation: {
          id: "complete",
          inputs: [{ state: "review", work_type: "story" }],
          name: "Complete",
          outputs: [{ state: "done", work_type: "story" }],
          worker: "completer",
        },
      },
    );
    chainingDispatchResponse.context.currentChainingTraceId = "chain-a";
    chainingDispatchResponse.context.previousChainingTraceIds = [
      "chain-a",
      "chain-b",
    ];
    chainingDispatchResponse.context.dispatchId = "dispatch-chain";
    chainingDispatchResponse.context.traceIds = ["chain-a", "chain-b"];
    chainingDispatchResponse.context.workIds = ["work-chain-a", "work-chain-b"];

    const tickThree = buildFactoryTimelineSnapshot(
      [initialStructureRequest, chainingWorkRequest, chainingDispatchRequest],
      3,
    );
    const tickFour = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        chainingWorkRequest,
        chainingDispatchRequest,
        chainingDispatchResponse,
      ],
      4,
    );

    expect(
      tickThree.workstationRequestsByDispatchID["dispatch-chain"]?.request_view
        ?.current_chaining_trace_id,
    ).toBe("chain-a");
    expect(
      tickThree.workstationRequestsByDispatchID["dispatch-chain"]?.request_view
        ?.previous_chaining_trace_ids,
    ).toEqual(["chain-a", "chain-b"]);
    expect(
      tickFour.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-chain"
      ]?.request?.current_chaining_trace_id,
    ).toBe("chain-a");
    expect(
      tickFour.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-chain"
      ]?.request?.previous_chaining_trace_ids,
    ).toEqual(["chain-a", "chain-b"]);
    expect(
      tickFour.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-chain"
      ]?.response?.output_work_items,
    ).toEqual([
      {
        current_chaining_trace_id: "chain-a",
        display_name: "Fan In Result",
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        trace_id: "chain-a",
        work_id: "work-chain-result",
        work_type_id: "story",
      },
    ]);
    expect(
      tickFour.tracesByWorkID["work-chain-result"].dispatches[0],
    ).toMatchObject({
      current_chaining_trace_id: "chain-a",
      previous_chaining_trace_ids: ["chain-a", "chain-b"],
    });
  });

  it("retains failed completion details in fixed timeline reconstruction", () => {
    const events = [
      initialStructureRequest,
      failedWorkInput,
      failedRequest,
      failedResponse,
    ];
    const activeTick = buildFactoryTimelineSnapshot(events, 3);
    const failedTick = buildFactoryTimelineSnapshot(events, 4);

    expect(
      activeTick.dashboard.runtime.session.failed_work_details_by_work_id?.[
        "work-failed"
      ],
    ).toBeUndefined();
    expect(activeTick.dashboard.runtime.session.provider_sessions).toEqual([]);

    const detail =
      failedTick.dashboard.runtime.session.failed_work_details_by_work_id?.[
        "work-failed"
      ];
    expect(detail).toMatchObject({
      dispatch_id: "dispatch-failed",
      failure_message: "Provider rate limit exceeded.",
      failure_reason: "throttled",
      transition_id: "review",
      workstation_name: "Review",
      work_item: {
        display_name: "Blocked Timeline Story",
        trace_id: "trace-failed",
        work_id: "work-failed",
        work_type_id: "story",
      },
    });
    expect(failedTick.dashboard.runtime.session.failed_work_labels).toEqual([
      "Blocked Timeline Story",
    ]);
    expect(
      failedTick.dashboard.runtime.session.provider_sessions?.[0],
    ).toMatchObject({
      dispatch_id: "dispatch-failed",
      failure_message: "Provider rate limit exceeded.",
      failure_reason: "throttled",
      outcome: "FAILED",
    });
    expect(
      failedTick.tracesByWorkID["work-failed"].dispatches[0],
    ).toMatchObject({
      dispatch_id: "dispatch-failed",
      failure_message: "Provider rate limit exceeded.",
      failure_reason: "throttled",
      outcome: "FAILED",
    });
  });

  it("counts failed work items for failed_count and failed_by_work_type in fixed timeline reconstruction", () => {
    const failedBatchWorkInput = event(
      "event-failed-batch-1",
      2,
      FACTORY_EVENT_TYPES.workRequest,
      {
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "Blocked Story",
            trace_id: "trace-failed-batch",
            work_id: "work-failed-batch-1",
            work_type_id: "story",
          },
          {
            name: "Rejected Story",
            trace_id: "trace-failed-batch",
            work_id: "work-failed-batch-2",
            work_type_id: "story",
          },
          {
            name: "Reworked Story",
            trace_id: "trace-failed-batch",
            work_id: "work-failed-batch-3",
            work_type_id: "story",
          },
        ],
      },
    );
    failedBatchWorkInput.context.requestId = "request-work-failed-batch";
    failedBatchWorkInput.context.traceIds = ["trace-failed-batch"];
    failedBatchWorkInput.context.workIds = [
      "work-failed-batch-1",
      "work-failed-batch-2",
      "work-failed-batch-3",
    ];

    const failedBatchRequest = event(
      "event-failed-batch-2",
      3,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        dispatchId: "dispatch-failed-batch",
        inputs: [
          { workId: "work-failed-batch-1" },
          { workId: "work-failed-batch-2" },
          { workId: "work-failed-batch-3" },
        ],
        transitionId: "review",
      },
    );
    failedBatchRequest.context.dispatchId = "dispatch-failed-batch";
    failedBatchRequest.context.traceIds = ["trace-failed-batch"];
    failedBatchRequest.context.workIds = [
      "work-failed-batch-1",
      "work-failed-batch-2",
      "work-failed-batch-3",
    ];

    const failedBatchResponse = event(
      "event-failed-batch-3",
      4,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        dispatchId: "dispatch-failed-batch",
        durationMillis: 600,
        failureMessage: "Provider rate limit exceeded.",
        failureReason: "throttled",
        outcome: "FAILED",
        outputWork: [
          {
            name: "Blocked Story",
            state: "failed",
            trace_id: "trace-failed-batch",
            work_id: "work-failed-batch-1",
            work_type_id: "story",
          },
          {
            name: "Rejected Story",
            state: "failed",
            trace_id: "trace-failed-batch",
            work_id: "work-failed-batch-2",
            work_type_id: "story",
          },
          {
            name: "Reworked Story",
            state: "failed",
            trace_id: "trace-failed-batch",
            work_id: "work-failed-batch-3",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: {
          id: "review",
          inputs: [{ state: "new", workType: "story" }],
          name: "Review",
          onFailure: { state: "failed", workType: "story" },
          outputs: [{ state: "done", workType: "story" }],
          worker: "reviewer",
        },
      },
    );
    failedBatchResponse.context.dispatchId = "dispatch-failed-batch";
    failedBatchResponse.context.traceIds = ["trace-failed-batch"];
    failedBatchResponse.context.workIds = [
      "work-failed-batch-1",
      "work-failed-batch-2",
      "work-failed-batch-3",
    ];

    const failedTick = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        failedBatchWorkInput,
        failedBatchRequest,
        failedBatchResponse,
      ],
      4,
    );

    expect(failedTick.dashboard.runtime.session.dispatched_count).toBe(1);
    expect(failedTick.dashboard.runtime.session.completed_count).toBe(0);
    expect(failedTick.dashboard.runtime.session.failed_count).toBe(3);
    expect(failedTick.dashboard.runtime.session.failed_by_work_type).toEqual({
      story: 3,
    });
    expect(failedTick.dashboard.runtime.session.failed_work_labels).toEqual([
      "Blocked Story",
      "Rejected Story",
      "Reworked Story",
    ]);
  });

  it("excludes system-time failures from replay failed_count while preserving customer failures", () => {
    const rawSystemTime = "__system_time";
    const systemTimeStructure = event(
      "event-system-time-failed-1",
      1,
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
                { name: "failed", type: "FAILED" },
              ],
            },
            {
              name: rawSystemTime,
              states: [{ name: "pending", type: "PROCESSING" }],
            },
          ],
          workstations: [
            {
              id: "review",
              inputs: [{ state: "new", workType: "story" }],
              name: "Review",
              onFailure: { state: "failed", workType: "story" },
              outputs: [{ state: "done", workType: "story" }],
              worker: "reviewer",
            },
            {
              id: `${rawSystemTime}:expire`,
              inputs: [{ state: "pending", workType: rawSystemTime }],
              name: `${rawSystemTime}:expire`,
              outputs: [],
              worker: "reviewer",
            },
          ],
        },
      },
    );
    const customerFailedWorkInput = event(
      "event-system-time-failed-2",
      2,
      FACTORY_EVENT_TYPES.workRequest,
      {
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "Customer Story",
            trace_id: "trace-customer-failed",
            work_id: "work-customer-failed",
            work_type_id: "story",
          },
        ],
      },
    );
    customerFailedWorkInput.context.requestId = "request-customer-failed";
    customerFailedWorkInput.context.traceIds = ["trace-customer-failed"];
    customerFailedWorkInput.context.workIds = ["work-customer-failed"];
    const customerFailedRequest = event(
      "event-system-time-failed-3",
      3,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        dispatchId: "dispatch-customer-failed",
        inputs: [{ workId: "work-customer-failed" }],
        transitionId: "review",
      },
    );
    customerFailedRequest.context.dispatchId = "dispatch-customer-failed";
    customerFailedRequest.context.traceIds = ["trace-customer-failed"];
    customerFailedRequest.context.workIds = ["work-customer-failed"];
    const customerFailedResponse = event(
      "event-system-time-failed-4",
      4,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        dispatchId: "dispatch-customer-failed",
        durationMillis: 100,
        failureMessage: "Customer work failed.",
        failureReason: "validation_error",
        outcome: "FAILED",
        outputWork: [
          {
            name: "Customer Story",
            state: "failed",
            trace_id: "trace-customer-failed",
            work_id: "work-customer-failed",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
      },
    );
    customerFailedResponse.context.dispatchId = "dispatch-customer-failed";
    customerFailedResponse.context.traceIds = ["trace-customer-failed"];
    customerFailedResponse.context.workIds = ["work-customer-failed"];
    const systemTimeWorkInput = event(
      "event-system-time-failed-5",
      5,
      FACTORY_EVENT_TYPES.workRequest,
      {
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "expiry tick",
            trace_id: "trace-system-time-failed",
            work_id: "work-system-time-failed",
            work_type_id: rawSystemTime,
          },
        ],
      },
    );
    systemTimeWorkInput.context.requestId = "request-system-time-failed";
    systemTimeWorkInput.context.traceIds = ["trace-system-time-failed"];
    systemTimeWorkInput.context.workIds = ["work-system-time-failed"];
    const systemTimeFailedRequest = event(
      "event-system-time-failed-6",
      6,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        dispatchId: "dispatch-system-time-failed",
        inputs: [
          {
            name: "expiry tick",
            trace_id: "trace-system-time-failed",
            work_id: "work-system-time-failed",
            work_type_id: rawSystemTime,
          },
        ],
        transitionId: `${rawSystemTime}:expire`,
      },
    );
    systemTimeFailedRequest.context.dispatchId = "dispatch-system-time-failed";
    systemTimeFailedRequest.context.traceIds = ["trace-system-time-failed"];
    systemTimeFailedRequest.context.workIds = ["work-system-time-failed"];
    const systemTimeFailedResponse = event(
      "event-system-time-failed-7",
      7,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        dispatchId: "dispatch-system-time-failed",
        durationMillis: 20,
        failureMessage: "System timer expired.",
        failureReason: "timeout",
        outcome: "FAILED",
        outputWork: [
          {
            name: "expiry tick",
            state: "failed",
            trace_id: "trace-system-time-failed",
            work_id: "work-system-time-failed",
            work_type_id: rawSystemTime,
          },
        ],
        transitionId: `${rawSystemTime}:expire`,
      },
    );
    systemTimeFailedResponse.context.dispatchId = "dispatch-system-time-failed";
    systemTimeFailedResponse.context.traceIds = ["trace-system-time-failed"];
    systemTimeFailedResponse.context.workIds = ["work-system-time-failed"];

    const failedTick = buildFactoryTimelineSnapshot(
      [
        systemTimeStructure,
        customerFailedWorkInput,
        customerFailedRequest,
        customerFailedResponse,
        systemTimeWorkInput,
        systemTimeFailedRequest,
        systemTimeFailedResponse,
      ],
      7,
    );

    expect(failedTick.dashboard.runtime.session.dispatched_count).toBe(1);
    expect(failedTick.dashboard.runtime.session.completed_count).toBe(0);
    expect(failedTick.dashboard.runtime.session.failed_count).toBe(1);
    expect(failedTick.dashboard.runtime.session.failed_by_work_type).toEqual({
      story: 1,
    });
  });

  it("reduces inference request and response events into dispatch-keyed attempt details", () => {
    const events = [
      initialStructureRequest,
      workInput,
      request,
      inferenceRequest,
      inferenceResponse,
      failedInferenceRequest,
      failedInferenceResponse,
      response,
    ];
    const pendingTick = buildFactoryTimelineSnapshot(events, 4);
    const completedTick = buildFactoryTimelineSnapshot(events, 5);

    const pendingAttempt =
      pendingTick.dashboard.runtime.inference_attempts_by_dispatch_id?.[
        "dispatch-1"
      ]?.["dispatch-1/inference-request/1"];
    expect(pendingAttempt).toMatchObject({
      attempt: 1,
      dispatch_id: "dispatch-1",
      inference_request_id: "dispatch-1/inference-request/1",
      prompt: "Review this timeline story.",
      request_time: "2026-04-16T12:00:04Z",
      transition_id: "review",
      working_directory: "/work/project",
      worktree: "/work/project/.worktrees/story",
    });
    expect(pendingAttempt?.outcome).toBeUndefined();

    const completedAttempt =
      completedTick.dashboard.runtime.inference_attempts_by_dispatch_id?.[
        "dispatch-1"
      ]?.["dispatch-1/inference-request/1"];
    expect(completedAttempt).toMatchObject({
      duration_millis: 1250,
      outcome: "SUCCEEDED",
      response: "The story is ready for review.",
      response_time: "2026-04-16T12:00:05Z",
    });

    const failedAttempt = buildFactoryTimelineSnapshot(events, 7).dashboard
      .runtime.inference_attempts_by_dispatch_id?.["dispatch-1"]?.[
      "dispatch-1/inference-request/2"
    ];
    expect(failedAttempt).toMatchObject({
      attempt: 2,
      duration_millis: 875,
      error_class: "rate_limited",
      exit_code: 1,
      outcome: "FAILED",
      prompt: "Retry the timeline story.",
      response_time: "2026-04-16T12:00:07Z",
    });
  });

  it("reduces script request and response events into script-aware workstation state", () => {
    const events = [
      initialStructureRequest,
      workInput,
      request,
      inferenceRequest,
      inferenceResponse,
      response,
      scriptPendingWorkInput,
      scriptPendingDispatchRequest,
      scriptPendingRequest,
      scriptSuccessWorkInput,
      scriptSuccessDispatchRequest,
      scriptSuccessRequest,
      scriptSuccessResponse,
      scriptSuccessDispatchResponse,
      scriptFailedWorkInput,
      scriptFailedDispatchRequest,
      scriptFailedRequest,
      scriptFailedResponse,
      scriptFailedDispatchResponse,
    ];
    const mixedTick = buildFactoryTimelineSnapshot(events, 20);

    expect(
      mixedTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-script-pending"
      ],
    ).toMatchObject({
      counts: {
        dispatched_count: 1,
        errored_count: 0,
        responded_count: 0,
      },
      request: {
        script_request: {
          args: ["--work", "work-script-pending"],
          attempt: 1,
          command: "script-tool",
          script_request_id: "dispatch-script-pending/script-request/1",
        },
      },
    });
    expect(
      mixedTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-script-pending"
      ]?.request.prompt,
    ).toBeUndefined();
    expect(
      mixedTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-script-pending"
      ]?.response,
    ).toBeUndefined();

    expect(
      mixedTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-script-success"
      ],
    ).toMatchObject({
      counts: {
        dispatched_count: 1,
        errored_count: 0,
        responded_count: 1,
      },
      request: {
        script_request: {
          args: ["--work", "work-script-success"],
          attempt: 1,
          command: "script-tool",
          script_request_id: "dispatch-script-success/script-request/1",
        },
      },
      response: {
        duration_millis: 222,
        outcome: "ACCEPTED",
        script_response: {
          duration_millis: 222,
          outcome: "SUCCEEDED",
          script_request_id: "dispatch-script-success/script-request/1",
          stderr: "",
          stdout: "script success stdout\n",
        },
      },
    });
    expect(
      mixedTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-script-success"
      ]?.response?.response_text,
    ).toBeUndefined();
    expect(
      mixedTick.dashboard.runtime.inference_attempts_by_dispatch_id?.[
        "dispatch-script-success"
      ],
    ).toBeUndefined();

    expect(
      mixedTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-script-failed"
      ],
    ).toMatchObject({
      counts: {
        dispatched_count: 1,
        errored_count: 1,
        responded_count: 0,
      },
      response: {
        failure_message: "Script timed out.",
        failure_reason: "script_timeout",
        outcome: "FAILED",
        script_response: {
          duration_millis: 500,
          failure_type: "TIMEOUT",
          outcome: "TIMED_OUT",
          script_request_id: "dispatch-script-failed/script-request/1",
          stderr: "script timed out\n",
          stdout: "",
        },
      },
    });
    expect(
      mixedTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-script-failed"
      ]?.response?.response_text,
    ).toBeUndefined();

    expect(
      mixedTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-1"
      ],
    ).toMatchObject({
      counts: {
        dispatched_count: 1,
        errored_count: 0,
        responded_count: 1,
      },
      request: {
        prompt: "Review this timeline story.",
      },
      response: {
        response_text: "The story is ready for review.",
      },
    });
  });

  it("projects keyed workstation-request views for in-flight, success, and error dispatches", () => {
    const requestOnlyTick = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workInput, request, inferenceRequest],
      4,
    );

    expect(
      requestOnlyTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-1"
      ],
    ).toMatchObject({
      counts: {
        dispatched_count: 1,
        errored_count: 0,
        responded_count: 0,
      },
      dispatch_id: "dispatch-1",
      request: {
        input_work_items: [
          {
            display_name: "Timeline Story",
            trace_id: "trace-1",
            work_id: "work-1",
            work_type_id: "story",
          },
        ],
        input_work_type_ids: ["story"],
        model: "gpt-5.4",
        prompt: "Review this timeline story.",
        provider: "openai",
        request_time: "2026-04-16T12:00:04Z",
        trace_ids: ["trace-1"],
        working_directory: "/work/project",
        worktree: "/work/project/.worktrees/story",
      },
      transition_id: "review",
      workstation_name: "Review",
    });
    expect(
      requestOnlyTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-1"
      ]?.response,
    ).toBeUndefined();

    const successTick = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        workInput,
        request,
        inferenceRequest,
        inferenceResponse,
        response,
      ],
      5,
    );

    expect(
      successTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-1"
      ],
    ).toMatchObject({
      request: {
        model: "gpt-5.4",
        prompt: "Review this timeline story.",
        provider: "openai",
        request_metadata: {
          prompt_source: "factory-renderer",
          session_id: "session-1",
        },
        request_time: "2026-04-16T12:00:04Z",
        working_directory: "/work/project",
        worktree: "/work/project/.worktrees/story",
      },
      counts: {
        dispatched_count: 1,
        errored_count: 0,
        responded_count: 1,
      },
      response: {
        duration_millis: 1250,
        response_metadata: {
          provider_session_id: "session-1",
          retry_count: "0",
        },
        response_text: "The story is ready for review.",
        outcome: "ACCEPTED",
        output_work_items: [
          {
            display_name: "Timeline Story",
            trace_id: "trace-1",
            work_id: "work-1",
            work_type_id: "story",
          },
        ],
        provider_session: {
          id: "session-1",
        },
      },
    });

    const errorInferenceRequest = event(
      "event-failed-inference-request",
      4,
      FACTORY_EVENT_TYPES.inferenceRequest,
      {
        attempt: 1,
        dispatchId: "dispatch-failed",
        inferenceRequestId: "dispatch-failed/inference-request/1",
        prompt: "Retry the blocked story.",
        transitionId: "review",
        workingDirectory: "/work/error",
        worktree: "/work/error/.worktrees/story",
      },
    );
    errorInferenceRequest.context.dispatchId = "dispatch-failed";
    errorInferenceRequest.context.traceIds = ["trace-failed"];
    errorInferenceRequest.context.workIds = ["work-failed"];
    errorInferenceRequest.context.eventTime = "2026-04-16T12:00:04Z";

    const errorInferenceResponse = event(
      "event-failed-inference-response",
      5,
      FACTORY_EVENT_TYPES.inferenceResponse,
      {
        attempt: 1,
        dispatchId: "dispatch-failed",
        durationMillis: 600,
        errorClass: "rate_limited",
        inferenceRequestId: "dispatch-failed/inference-request/1",
        outcome: "FAILED",
        transitionId: "review",
      },
    );
    errorInferenceResponse.context.dispatchId = "dispatch-failed";
    errorInferenceResponse.context.traceIds = ["trace-failed"];
    errorInferenceResponse.context.workIds = ["work-failed"];
    errorInferenceResponse.context.eventTime = "2026-04-16T12:00:05Z";

    const errorTick = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        failedWorkInput,
        failedRequest,
        errorInferenceRequest,
        errorInferenceResponse,
        failedResponse,
      ],
      5,
    );

    expect(
      errorTick.dashboard.runtime.workstation_requests_by_dispatch_id?.[
        "dispatch-failed"
      ],
    ).toMatchObject({
      request: {
        model: "claude-3.7",
        prompt: "Retry the blocked story.",
        provider: "anthropic",
        request_metadata: {
          prompt_source: "factory-renderer",
        },
        working_directory: "/work/error",
        worktree: "/work/error/.worktrees/story",
      },
      counts: {
        dispatched_count: 1,
        errored_count: 1,
        responded_count: 0,
      },
      response: {
        error_class: "rate_limited",
        failure_message: "Provider rate limit exceeded.",
        failure_reason: "throttled",
        outcome: "FAILED",
        response_metadata: {
          retry_count: "1",
        },
      },
    });
  });

  it("projects dispatch-keyed workstation requests from canonical dispatch and inference data", () => {
    const inferenceResponseWithDiagnostics = {
      ...inferenceResponse,
      payload: {
        ...inferenceResponse.payload,
        diagnostics: {
          provider: {
            model: "gpt-5.4",
            provider: "openai",
            requestMetadata: {
              prompt_source: "review-template",
            },
            responseMetadata: {
              response_status: "ok",
            },
          },
        },
      },
    } satisfies FactoryEvent;
    const activeProjected = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workInput, request, inferenceRequest],
      4,
    );
    expect(activeProjected.workstationRequestsByDispatchID["dispatch-1"]).toMatchObject({
      counts: {
        dispatched_count: 1,
        errored_count: 0,
        responded_count: 0,
      },
      dispatch_id: "dispatch-1",
      request_view: {
        input_work_items: [
          {
            display_name: "Timeline Story",
            trace_id: "trace-1",
            work_id: "work-1",
            work_type_id: "story",
          },
        ],
        input_work_type_ids: ["story"],
        model: "gpt-5.4",
        prompt: "Review this timeline story.",
        provider: "openai",
        request_time: "2026-04-16T12:00:04Z",
        trace_ids: ["trace-1"],
        working_directory: "/work/project",
        worktree: "/work/project/.worktrees/story",
      },
    });
    expect(
      activeProjected.workstationRequestsByDispatchID["dispatch-1"].response_view,
    ).toBeUndefined();

    const successfulProjected = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        workInput,
        request,
        inferenceRequest,
        inferenceResponseWithDiagnostics,
        response,
      ],
      5,
    );

    expect(Object.keys(successfulProjected.workstationRequestsByDispatchID)).toEqual(["dispatch-1"]);
    expect(
      successfulProjected.workstationRequestsByDispatchID["dispatch-1"],
    ).toMatchObject({
      counts: {
        dispatched_count: 1,
        errored_count: 0,
        responded_count: 1,
      },
      dispatch_id: "dispatch-1",
      dispatched_request_count: 1,
      errored_request_count: 0,
      model: "gpt-5.4",
      prompt: "Review this timeline story.",
      provider: "openai",
      request_view: {
        input_work_items: [
          {
            display_name: "Timeline Story",
            trace_id: "trace-1",
            work_id: "work-1",
            work_type_id: "story",
          },
        ],
        input_work_type_ids: ["story"],
        model: "gpt-5.4",
        prompt: "Review this timeline story.",
        provider: "openai",
        request_metadata: {
          prompt_source: "review-template",
        },
        request_time: "2026-04-16T12:00:04Z",
        trace_ids: ["trace-1"],
        working_directory: "/work/project",
        worktree: "/work/project/.worktrees/story",
      },
      request_id: "request-work-1",
      request_metadata: {
        prompt_source: "review-template",
      },
      responded_request_count: 1,
      response: "The story is ready for review.",
      response_view: {
        duration_millis: 1250,
        outcome: "ACCEPTED",
        output_work_items: [
          {
            display_name: "Timeline Story",
            trace_id: "trace-1",
            work_id: "work-1",
            work_type_id: "story",
          },
        ],
        provider_session: {
          id: "session-1",
        },
        response_metadata: {
          response_status: "ok",
        },
        response_text: "The story is ready for review.",
      },
      response_metadata: {
        response_status: "ok",
      },
      total_duration_millis: 1250,
      transition_id: "review",
      working_directory: "/work/project",
      workstation_name: "Review",
      workstation_node_id: "review",
      worktree: "/work/project/.worktrees/story",
    });
    const thinSuccessfulProjected = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        workInput,
        request,
        inferenceRequest,
        inferenceResponseWithDiagnostics,
        response,
      ],
      5,
    );
    expect(
      thinSuccessfulProjected.workstationRequestsByDispatchID["dispatch-1"],
    ).toMatchObject({
      outcome: "ACCEPTED",
      response: "The story is ready for review.",
      workstation_name: "Review",
      work_items: [
        {
          display_name: "Timeline Story",
          trace_id: "trace-1",
          work_id: "work-1",
          work_type_id: "story",
        },
      ],
    });
    expect(
      successfulProjected.workstationRequestsByDispatchID["dispatch-1"].inference_attempts.map(
        (attempt) => attempt.inference_request_id,
      ),
    ).toEqual(["dispatch-1/inference-request/1"]);

    const rejectedProjected = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        rejectedWorkInput,
        rejectedRequest,
        rejectedInferenceRequest,
        rejectedInferenceResponse,
        rejectedResponse,
      ],
      6,
    );
    expect(
      rejectedProjected.workstationRequestsByDispatchID["dispatch-rejected"],
    ).toMatchObject({
      counts: {
        dispatched_count: 1,
        errored_count: 0,
        responded_count: 1,
      },
      dispatch_id: "dispatch-rejected",
      outcome: "REJECTED",
      request_view: {
        input_work_items: [
          {
            display_name: "Rejected Timeline Story",
            trace_id: "trace-rejected",
            work_id: "work-rejected",
            work_type_id: "story",
          },
        ],
        prompt: "Review the story and explain why it needs more work.",
        provider: "codex",
      },
      response: "The story needs another pass before approval.",
      response_view: {
        feedback: "Please fix the missing acceptance test.",
        outcome: "REJECTED",
        output_work_items: [
          {
            display_name: "Rejected Timeline Story",
            trace_id: "trace-rejected",
            work_id: "work-rejected",
            work_type_id: "story",
          },
        ],
        provider_session: {
          id: "session-rejected",
        },
        response_text: "The story needs another pass before approval.",
      },
    });

    const failedDispatchInferenceRequest = event(
      "event-failed-dispatch-inference-request",
      4,
      FACTORY_EVENT_TYPES.inferenceRequest,
      {
        attempt: 1,
        dispatchId: "dispatch-failed",
        inferenceRequestId: "dispatch-failed/inference-request/1",
        prompt: "Retry the blocked story.",
        transitionId: "review",
        workingDirectory: "/work/error",
        worktree: "/work/error/.worktrees/story",
      },
    );
    failedDispatchInferenceRequest.context.dispatchId = "dispatch-failed";
    failedDispatchInferenceRequest.context.traceIds = ["trace-failed"];
    failedDispatchInferenceRequest.context.workIds = ["work-failed"];
    failedDispatchInferenceRequest.context.eventTime = "2026-04-16T12:00:04Z";

    const failedDispatchInferenceResponse = event(
      "event-failed-dispatch-inference-response",
      5,
      FACTORY_EVENT_TYPES.inferenceResponse,
      {
        attempt: 1,
        dispatchId: "dispatch-failed",
        durationMillis: 600,
        errorClass: "rate_limited",
        inferenceRequestId: "dispatch-failed/inference-request/1",
        outcome: "FAILED",
        transitionId: "review",
      },
    );
    failedDispatchInferenceResponse.context.dispatchId = "dispatch-failed";
    failedDispatchInferenceResponse.context.traceIds = ["trace-failed"];
    failedDispatchInferenceResponse.context.workIds = ["work-failed"];
    failedDispatchInferenceResponse.context.eventTime = "2026-04-16T12:00:05Z";

    const failedProjected = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        failedWorkInput,
        failedRequest,
        failedDispatchInferenceRequest,
        failedDispatchInferenceResponse,
        failedResponse,
      ],
      5,
    );
    expect(failedProjected.workstationRequestsByDispatchID["dispatch-failed"]).toMatchObject({
      counts: {
        dispatched_count: 1,
        errored_count: 1,
        responded_count: 0,
      },
      dispatch_id: "dispatch-failed",
      failure_message: "Provider rate limit exceeded.",
      failure_reason: "throttled",
      request_view: {
        prompt: "Retry the blocked story.",
        provider: "anthropic",
      },
      response_view: {
        error_class: "rate_limited",
        failure_message: "Provider rate limit exceeded.",
        failure_reason: "throttled",
        outcome: "FAILED",
      },
    });
  });

  it("projects script-aware workstation requests by dispatch id without inference-shaped fallbacks", () => {
    const projected = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        workInput,
        request,
        inferenceRequest,
        inferenceResponse,
        response,
        scriptPendingWorkInput,
        scriptPendingDispatchRequest,
        scriptPendingRequest,
        scriptSuccessWorkInput,
        scriptSuccessDispatchRequest,
        scriptSuccessRequest,
        scriptSuccessResponse,
        scriptSuccessDispatchResponse,
        scriptFailedWorkInput,
        scriptFailedDispatchRequest,
        scriptFailedRequest,
        scriptFailedResponse,
        scriptFailedDispatchResponse,
      ],
      20,
    );

    expect(
      projected.workstationRequestsByDispatchID["dispatch-script-pending"],
    ).toMatchObject({
      dispatch_id: "dispatch-script-pending",
      dispatched_request_count: 1,
      errored_request_count: 0,
      responded_request_count: 0,
      script_request: {
        args: ["--work", "work-script-pending"],
        attempt: 1,
        command: "script-tool",
        script_request_id: "dispatch-script-pending/script-request/1",
      },
      workstation_node_id: "review",
    });
    expect(
      projected.workstationRequestsByDispatchID["dispatch-script-pending"].response,
    ).toBeUndefined();
    expect(
      projected.workstationRequestsByDispatchID["dispatch-script-pending"]
        .inference_attempts,
    ).toEqual([]);

    expect(
      projected.workstationRequestsByDispatchID["dispatch-script-success"],
    ).toMatchObject({
      dispatch_id: "dispatch-script-success",
      dispatched_request_count: 1,
      errored_request_count: 0,
      responded_request_count: 1,
      script_request: {
        args: ["--work", "work-script-success"],
        attempt: 1,
        command: "script-tool",
        script_request_id: "dispatch-script-success/script-request/1",
      },
      script_response: {
        duration_millis: 222,
        outcome: "SUCCEEDED",
        script_request_id: "dispatch-script-success/script-request/1",
        stderr: "",
        stdout: "script success stdout\n",
      },
    });
    expect(
      projected.workstationRequestsByDispatchID["dispatch-script-success"].response,
    ).toBeUndefined();

    expect(
      projected.workstationRequestsByDispatchID["dispatch-script-failed"],
    ).toMatchObject({
      dispatch_id: "dispatch-script-failed",
      dispatched_request_count: 1,
      errored_request_count: 1,
      failure_message: "Script timed out.",
      failure_reason: "script_timeout",
      responded_request_count: 0,
      script_request: {
        args: ["--work", "work-script-failed"],
        attempt: 1,
        command: "script-tool",
        script_request_id: "dispatch-script-failed/script-request/1",
      },
      script_response: {
        duration_millis: 500,
        failure_type: "TIMEOUT",
        outcome: "TIMED_OUT",
        script_request_id: "dispatch-script-failed/script-request/1",
        stderr: "script timed out\n",
        stdout: "",
      },
    });
    expect(
      projected.workstationRequestsByDispatchID["dispatch-script-failed"].response,
    ).toBeUndefined();

    expect(projected.workstationRequestsByDispatchID["dispatch-1"]).toMatchObject({
      dispatched_request_count: 1,
      errored_request_count: 0,
      responded_request_count: 1,
      prompt: "Review this timeline story.",
      response: "The story is ready for review.",
    });
  });

  it("resolves thin dispatch request workstation names from topology", () => {
    const thinRequest = event(
      "event-thin-dispatch-request",
      3,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        dispatchId: "dispatch-thin-1",
        inputs: [{ workId: "work-1" }],
        transitionId: "review",
        worker: {
          model: "gpt-5.4",
          modelProvider: "openai",
        },
      },
    );
    thinRequest.context.dispatchId = "dispatch-thin-1";
    thinRequest.context.traceIds = ["trace-1"];
    thinRequest.context.workIds = ["work-1"];

    const projected = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workInput, thinRequest],
      3,
    );

    expect(
      projected.dashboard.runtime.active_executions_by_dispatch_id?.[
        "dispatch-thin-1"
      ],
    ).toMatchObject({
      transition_id: "review",
      workstation_name: "Review",
    });
    expect(
      projected.workstationRequestsByDispatchID["dispatch-thin-1"],
    ).toMatchObject({
      request_view: {
        input_work_items: [
          {
            display_name: "Timeline Story",
            trace_id: "trace-1",
            work_id: "work-1",
            work_type_id: "story",
          },
        ],
      },
      transition_id: "review",
      workstation_name: "Review",
    });
  });

  it("ignores undefined trace identifiers in thin event context arrays", () => {
    const malformedThinResponse = {
      ...response,
      payload: {
        ...response.payload,
        workstation: undefined,
      },
    } satisfies FactoryEvent;
    malformedThinResponse.context.traceIds = [
      "trace-1",
      undefined as unknown as string,
    ];
    malformedThinResponse.context.workIds = [
      "work-1",
      undefined as unknown as string,
    ];

    const projected = buildFactoryTimelineSnapshot(
      [initialStructureRequest, workInput, request, malformedThinResponse],
      4,
    );

    expect(
      projected.workstationRequestsByDispatchID["dispatch-1"],
    ).toMatchObject({
      trace_ids: ["trace-1"],
      workstation_name: "Review",
    });
  });

  it("does not count failed inference attempts as responded dispatch-keyed workstation requests", () => {
    const errorInferenceRequest = event(
      "event-failed-inference-request",
      4,
      FACTORY_EVENT_TYPES.inferenceRequest,
      {
        attempt: 1,
        dispatchId: "dispatch-failed",
        inferenceRequestId: "dispatch-failed/inference-request/1",
        prompt: "Retry the blocked story.",
        transitionId: "review",
        workingDirectory: "/work/error",
        worktree: "/work/error/.worktrees/story",
      },
    );
    errorInferenceRequest.context.dispatchId = "dispatch-failed";
    errorInferenceRequest.context.traceIds = ["trace-failed"];
    errorInferenceRequest.context.workIds = ["work-failed"];
    errorInferenceRequest.context.eventTime = "2026-04-16T12:00:04Z";

    const errorInferenceResponse = event(
      "event-failed-inference-response",
      5,
      FACTORY_EVENT_TYPES.inferenceResponse,
      {
        attempt: 1,
        dispatchId: "dispatch-failed",
        durationMillis: 600,
        errorClass: "rate_limited",
        inferenceRequestId: "dispatch-failed/inference-request/1",
        outcome: "FAILED",
        transitionId: "review",
      },
    );
    errorInferenceResponse.context.dispatchId = "dispatch-failed";
    errorInferenceResponse.context.traceIds = ["trace-failed"];
    errorInferenceResponse.context.workIds = ["work-failed"];
    errorInferenceResponse.context.eventTime = "2026-04-16T12:00:05Z";

    const projected = buildFactoryTimelineSnapshot(
      [
        initialStructureRequest,
        failedWorkInput,
        failedRequest,
        errorInferenceRequest,
        errorInferenceResponse,
        failedResponse,
      ],
      5,
    );

    expect(projected.workstationRequestsByDispatchID["dispatch-failed"]).toMatchObject({
      dispatched_request_count: 1,
      errored_request_count: 1,
      responded_request_count: 0,
    });
  });

  it("replays the failure-analysis smoke fixture across current work and failed terminal details", () => {
    const responseEvent = failureAnalysisTimelineEvents.find(
      (event) => event.type === FACTORY_EVENT_TYPES.dispatchResponse,
    );
    expect(responseEvent?.payload).toMatchObject({
      failureMessage:
        "Provider rate limit exceeded while generating the analysis.",
      failureReason: "provider_rate_limit",
      outcome: "FAILED",
    });

    const activeTick = buildFactoryTimelineSnapshot(
      failureAnalysisTimelineEvents,
      3,
    );
    expect(
      activeTick.dashboard.runtime.session.failed_work_details_by_work_id?.[
        "work-blocked-analysis"
      ],
    ).toBeUndefined();
    expect(
      activeTick.dashboard.runtime.current_work_items_by_place_id?.[
        "story:new"
      ],
    ).toEqual([
      {
        display_name: "Queued Analysis Story",
        trace_id: "trace-queued-analysis",
        work_id: "work-queued-analysis",
        work_type_id: "story",
      },
    ]);

    const failedTick = buildFactoryTimelineSnapshot(
      failureAnalysisTimelineEvents,
      4,
    );
    expect(
      failedTick.dashboard.runtime.current_work_items_by_place_id?.[
        "story:new"
      ],
    ).toEqual([
      {
        display_name: "Queued Analysis Story",
        trace_id: "trace-queued-analysis",
        work_id: "work-queued-analysis",
        work_type_id: "story",
      },
    ]);
    expect(failedTick.dashboard.runtime.session.failed_work_labels).toEqual([
      "Blocked Analysis Story",
    ]);
    expect(
      failedTick.dashboard.runtime.session.failed_work_details_by_work_id?.[
        "work-blocked-analysis"
      ],
    ).toMatchObject({
      dispatch_id: "dispatch-blocked-analysis",
      failure_message:
        "Provider rate limit exceeded while generating the analysis.",
      failure_reason: "provider_rate_limit",
      transition_id: "review",
      workstation_name: "Review",
      work_item: {
        display_name: "Blocked Analysis Story",
        trace_id: "trace-blocked-analysis",
        work_id: "work-blocked-analysis",
        work_type_id: "story",
      },
    });
  });

  it("hides raw system time topology and labels expiry dispatches for dashboard projection", () => {
    const rawSystemTime = "__system_time";
    const systemTimeInitialStructure = event(
      "event-time-1",
      1,
      FACTORY_EVENT_TYPES.initialStructureRequest,
      {
        factory: {
          workTypes: [
            {
              name: "story",
              states: [
                { name: "new", type: "INITIAL" },
                { name: "done", type: "TERMINAL" },
              ],
            },
            {
              name: rawSystemTime,
              states: [{ name: "pending", type: "PROCESSING" }],
            },
          ],
          workstations: [
            {
              id: "daily-refresh",
              inputs: [
                { state: "new", workType: "story" },
                { state: "pending", workType: rawSystemTime },
              ],
              name: "Daily refresh",
              outputs: [{ state: "done", workType: "story" }],
              behavior: "CRON",
              worker: "refresh-worker",
            },
            {
              id: `${rawSystemTime}:expire`,
              inputs: [{ state: "pending", workType: rawSystemTime }],
              name: `${rawSystemTime}:expire`,
              outputs: [],
              worker: "",
            },
          ],
        },
      },
    );
    const systemTimeRequest = event(
      "event-time-2",
      2,
      FACTORY_EVENT_TYPES.workRequest,
      {
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "daily-refresh tick",
            tags: {
              "agent_factory.cron.workstation": "daily-refresh",
              "agent_factory.time.expires_at": "2026-04-16T12:01:00Z",
            },
            trace_id: "trace-time",
            work_id: "time-daily-refresh",
            work_type_id: rawSystemTime,
          },
        ],
      },
    );
    const systemTimeExpiryRequest = event(
      "event-time-3",
      3,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        dispatchId: "dispatch-expire",
        inputs: [
          {
            name: "daily-refresh tick",
            trace_id: "trace-time",
            work_id: "time-daily-refresh",
            work_type_id: rawSystemTime,
          },
        ],
        transitionId: `${rawSystemTime}:expire`,
        workstation: {
          id: `${rawSystemTime}:expire`,
          inputs: [{ state: "pending", workType: rawSystemTime }],
          name: `${rawSystemTime}:expire`,
          outputs: [],
          worker: "",
        },
      },
    );
    const systemTimeExpiryResponse = event(
      "event-time-4",
      4,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        dispatchId: "dispatch-expire",
        durationMillis: 10,
        outcome: "ACCEPTED",
        outputWork: [],
        transitionId: `${rawSystemTime}:expire`,
        workstation: {
          id: `${rawSystemTime}:expire`,
          inputs: [{ state: "pending", workType: rawSystemTime }],
          name: `${rawSystemTime}:expire`,
          outputs: [],
          worker: "",
        },
      },
    );
    systemTimeRequest.context.traceIds = ["trace-time"];
    systemTimeRequest.context.workIds = ["time-daily-refresh"];
    systemTimeExpiryRequest.context.dispatchId = "dispatch-expire";
    systemTimeExpiryRequest.context.traceIds = ["trace-time"];
    systemTimeExpiryRequest.context.workIds = ["time-daily-refresh"];
    systemTimeExpiryResponse.context.dispatchId = "dispatch-expire";
    systemTimeExpiryResponse.context.traceIds = ["trace-time"];
    systemTimeExpiryResponse.context.workIds = ["time-daily-refresh"];

    const activeTick = buildFactoryTimelineSnapshot(
      [
        systemTimeInitialStructure,
        systemTimeRequest,
        systemTimeExpiryRequest,
        systemTimeExpiryResponse,
      ],
      3,
    );
    const completedTick = buildFactoryTimelineSnapshot(
      [
        systemTimeInitialStructure,
        systemTimeRequest,
        systemTimeExpiryRequest,
        systemTimeExpiryResponse,
      ],
      4,
    );

    expect(activeTick.dashboard.topology.submit_work_types).toEqual([
      { work_type_name: "story" },
    ]);
    expect(activeTick.dashboard.topology.workstation_node_ids).toEqual([
      "daily-refresh",
    ]);
    expect(
      activeTick.dashboard.topology.workstation_nodes_by_id["daily-refresh"],
    ).toMatchObject({
      input_place_ids: ["story:new"],
      input_work_type_ids: ["story"],
      workstation_name: "Daily refresh",
    });
    expect(
      activeTick.dashboard.runtime.place_token_counts?.[
        "__system_time:pending"
      ],
    ).toBeUndefined();
    expect(
      activeTick.dashboard.runtime.current_work_items_by_place_id?.[
        "__system_time:pending"
      ],
    ).toBeUndefined();
    expect(activeTick.dashboard.runtime.active_workstation_node_ids).toEqual([
      "time:expire",
    ]);
    expect(activeTick.dashboard.runtime.active_dispatch_ids).toEqual([]);
    expect(
      activeTick.dashboard.runtime.active_executions_by_dispatch_id,
    ).toEqual({});
    expect(activeTick.dashboard.runtime.in_flight_dispatch_count).toBe(0);
    expect(
      activeTick.dashboard.runtime.workstation_activity_by_node_id,
    ).toEqual({});
    expect(activeTick.dashboard.runtime.session.completed_count).toBe(0);
    expect(activeTick.dashboard.runtime.session.dispatched_count).toBe(0);
    expect(activeTick.dashboard.runtime.session.has_data).toBe(false);
    expect(completedTick.dashboard.runtime.session.completed_count).toBe(0);
    expect(completedTick.dashboard.runtime.session.dispatched_count).toBe(0);
    expect(completedTick.dashboard.runtime.session.has_data).toBe(false);
    expect(completedTick.tracesByWorkID["time-daily-refresh"]).toBeUndefined();
    expect(JSON.stringify(activeTick.dashboard)).not.toContain(rawSystemTime);
    expect(JSON.stringify(completedTick.dashboard)).not.toContain(
      rawSystemTime,
    );
  });

  it("follows latest tick in current mode and preserves selected tick in fixed mode", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEvent(initialStructureRequest);
    store.appendEvent(workInput);
    store.appendEvent(request);

    expect(useFactoryTimelineStore.getState().selectedTick).toBe(3);
    useFactoryTimelineStore.getState().selectTick(2);
    expect(useFactoryTimelineStore.getState().mode).toBe("fixed");
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(2);

    useFactoryTimelineStore.getState().appendEvent(response);
    useFactoryTimelineStore.getState().appendEvent(lifecycle);
    expect(useFactoryTimelineStore.getState().latestTick).toBe(5);
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(2);

    useFactoryTimelineStore.getState().setCurrentMode();
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(5);
    expect(
      useFactoryTimelineStore.getState().worldViewCache[5].dashboard
        .factory_state,
    ).toBe("PAUSED");
  });

  it("supports batched event appends while preserving current-mode tick tracking", () => {
    const store = useFactoryTimelineStore.getState();

    store.appendEvents([initialStructureRequest, workInput, request]);

    expect(useFactoryTimelineStore.getState().selectedTick).toBe(3);
    expect(useFactoryTimelineStore.getState().events).toHaveLength(3);

    useFactoryTimelineStore.getState().selectTick(2);
    useFactoryTimelineStore.getState().appendEvents([response, lifecycle]);

    expect(useFactoryTimelineStore.getState().latestTick).toBe(5);
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(2);
    expect(
      useFactoryTimelineStore.getState().worldViewCache[2].dashboard.tick_count,
    ).toBe(2);
  });

  it("preserves script-backed workstation-request details in cached snapshots", () => {
    const store = useFactoryTimelineStore.getState();

    store.appendEvents([
      initialStructureRequest,
      scriptSuccessWorkInput,
      scriptSuccessDispatchRequest,
      scriptSuccessRequest,
      scriptSuccessResponse,
      scriptSuccessDispatchResponse,
    ]);

    const cachedSnapshot =
      useFactoryTimelineStore.getState().worldViewCache[
        useFactoryTimelineStore.getState().latestTick
      ];
    expect(
      cachedSnapshot.workstationRequestsByDispatchID["dispatch-script-success"],
    ).toMatchObject({
      script_request: {
        args: ["--work", "work-script-success"],
        command: "script-tool",
        script_request_id: "dispatch-script-success/script-request/1",
      },
      script_response: {
        outcome: "SUCCEEDED",
        script_request_id: "dispatch-script-success/script-request/1",
        stdout: "script success stdout\n",
      },
    });
  });
});
