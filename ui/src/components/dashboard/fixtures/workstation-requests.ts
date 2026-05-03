import type {
  DashboardInferenceAttempt,
  DashboardWorkstationRequest,
} from "../../../api/dashboard/types";

const READY_DISPATCH_ID = "dispatch-review-ready";
const PENDING_DISPATCH_ID = "dispatch-review-pending";
const REJECTED_DISPATCH_ID = "dispatch-review-rejected";
const ERRORED_DISPATCH_ID = "dispatch-review-errored";
const REVIEW_REQUEST_TIME = "2026-04-08T12:00:01Z";
const REVIEW_RESPONSE_TIME = "2026-04-08T12:00:04Z";
const REVIEW_WORKING_DIRECTORY = "C:\\work\\portos";
const REVIEW_WORKTREE = "C:\\work\\portos\\.worktrees\\active-story";

export function buildDashboardInferenceAttemptFixture(
  dispatchID: string,
  overrides: Partial<DashboardInferenceAttempt> = {},
): DashboardInferenceAttempt {
  return {
    attempt: 1,
    dispatch_id: dispatchID,
    inference_request_id: `${dispatchID}/inference-request/1`,
    prompt: "Review the active story and return a concise result.",
    request_time: REVIEW_REQUEST_TIME,
    transition_id: "review",
    working_directory: REVIEW_WORKING_DIRECTORY,
    worktree: REVIEW_WORKTREE,
    ...overrides,
  };
}

export function buildDashboardWorkstationRequestFixture(
  dispatchID: string,
  overrides: Partial<DashboardWorkstationRequest> = {},
): DashboardWorkstationRequest {
  const workItems = [
    {
      display_name: "Active Story",
      trace_id: "trace-active-story",
      work_id: "work-active-story",
      work_type_id: "story",
    },
  ];

  return {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 0,
    },
    dispatch_id: dispatchID,
    dispatched_request_count: 1,
    errored_request_count: 0,
    inference_attempts: [],
    request_view: {
      input_work_items: workItems,
      input_work_type_ids: ["story"],
      started_at: REVIEW_REQUEST_TIME,
      trace_ids: ["trace-active-story"],
    },
    responded_request_count: 0,
    started_at: REVIEW_REQUEST_TIME,
    transition_id: "review",
    work_items: workItems,
    workstation_name: "Review",
    workstation_node_id: "review",
    ...overrides,
  };
}

export const readyWorkstationRequestFixture = buildDashboardWorkstationRequestFixture(
  READY_DISPATCH_ID,
  {
    counts: {
      dispatched_count: 2,
      errored_count: 1,
      responded_count: 1,
    },
    dispatched_request_count: 2,
    errored_request_count: 1,
    inference_attempts: [
      buildDashboardInferenceAttemptFixture(READY_DISPATCH_ID, {
        inference_request_id: `${READY_DISPATCH_ID}/inference-request/1`,
        outcome: "FAILED",
        response_time: "2026-04-08T12:00:02Z",
      }),
      buildDashboardInferenceAttemptFixture(READY_DISPATCH_ID, {
        attempt: 2,
        duration_millis: 740,
        inference_request_id: `${READY_DISPATCH_ID}/inference-request/2`,
        outcome: "SUCCEEDED",
        prompt: "Retry the review with the latest context.",
        response: "Ready for the next workstation.",
        response_time: REVIEW_RESPONSE_TIME,
      }),
    ],
    model: "gpt-5.4",
    outcome: "ACCEPTED",
    prompt: "Review the active story and decide whether it is ready.",
    provider: "codex",
    provider_session: {
      id: "sess-ready-request",
      kind: "session_id",
      provider: "codex",
    },
    request_view: {
      input_work_items: [
        {
          display_name: "Active Story",
          trace_id: "trace-active-story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      model: "gpt-5.4",
      prompt: "Review the active story and decide whether it is ready.",
      provider: "codex",
      request_metadata: {
        prompt_source: "factory-renderer",
        source: "dispatch-history",
      },
      request_time: REVIEW_REQUEST_TIME,
      started_at: REVIEW_REQUEST_TIME,
      trace_ids: ["trace-active-story"],
      working_directory: REVIEW_WORKING_DIRECTORY,
      worktree: REVIEW_WORKTREE,
    },
    request_id: "request-ready-story",
    request_metadata: {
      prompt_source: "factory-renderer",
      source: "dispatch-history",
    },
    responded_request_count: 1,
    response: "Ready for the next workstation.",
    response_view: {
      duration_millis: 63_000,
      end_time: REVIEW_RESPONSE_TIME,
      outcome: "ACCEPTED",
      output_work_items: [
        {
          display_name: "Active Story",
          trace_id: "trace-active-story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      provider_session: {
        id: "sess-ready-request",
        kind: "session_id",
        provider: "codex",
      },
      response_metadata: {
        finish_reason: "stop",
        session_source: "codex",
      },
      response_text: "Ready for the next workstation.",
    },
    response_metadata: {
      finish_reason: "stop",
      session_source: "codex",
    },
    total_duration_millis: 63_000,
    trace_ids: ["trace-active-story"],
    working_directory: REVIEW_WORKING_DIRECTORY,
    worktree: REVIEW_WORKTREE,
  },
);

export const noResponseWorkstationRequestFixture = buildDashboardWorkstationRequestFixture(
  PENDING_DISPATCH_ID,
  {
    prompt: "Review the active story while the provider response is still pending.",
    request_view: {
      input_work_items: [
        {
          display_name: "Active Story",
          trace_id: "trace-active-story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      prompt: "Review the active story while the provider response is still pending.",
      request_metadata: {
        prompt_source: "factory-renderer",
      },
      request_time: REVIEW_REQUEST_TIME,
      started_at: REVIEW_REQUEST_TIME,
      trace_ids: ["trace-active-story"],
      working_directory: REVIEW_WORKING_DIRECTORY,
      worktree: "C:\\work\\portos\\.worktrees\\pending-story",
    },
    request_id: "request-pending-story",
    request_metadata: {
      prompt_source: "factory-renderer",
    },
    working_directory: REVIEW_WORKING_DIRECTORY,
    worktree: "C:\\work\\portos\\.worktrees\\pending-story",
  },
);

export const rejectedWorkstationRequestFixture = buildDashboardWorkstationRequestFixture(
  REJECTED_DISPATCH_ID,
  {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 1,
    },
    dispatched_request_count: 1,
    inference_attempts: [
      buildDashboardInferenceAttemptFixture(REJECTED_DISPATCH_ID, {
        duration_millis: 920,
        inference_request_id: `${REJECTED_DISPATCH_ID}/inference-request/1`,
        outcome: "SUCCEEDED",
        response: "The active story needs revision before it can continue.",
        response_time: "2026-04-08T12:00:03Z",
      }),
    ],
    model: "gpt-5.4",
    outcome: "REJECTED",
    prompt: "Review the active story and explain what needs to change before approval.",
    provider: "codex",
    provider_session: {
      id: "sess-rejected-story",
      kind: "session_id",
      provider: "codex",
    },
    request_view: {
      input_work_items: [
        {
          display_name: "Active Story",
          trace_id: "trace-active-story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      model: "gpt-5.4",
      prompt: "Review the active story and explain what needs to change before approval.",
      provider: "codex",
      request_metadata: {
        prompt_source: "factory-renderer",
        source: "dispatch-history",
      },
      request_time: REVIEW_REQUEST_TIME,
      started_at: REVIEW_REQUEST_TIME,
      trace_ids: ["trace-active-story"],
      working_directory: REVIEW_WORKING_DIRECTORY,
      worktree: REVIEW_WORKTREE,
    },
    request_id: "request-rejected-story",
    request_metadata: {
      prompt_source: "factory-renderer",
      source: "dispatch-history",
    },
    responded_request_count: 1,
    response: "The active story needs revision before it can continue.",
    response_metadata: {
      finish_reason: "rejected",
      session_source: "codex",
    },
    response_view: {
      duration_millis: 2_000,
      end_time: "2026-04-08T12:00:03Z",
      outcome: "REJECTED",
      output_work_items: [
        {
          display_name: "Active Story",
          trace_id: "trace-active-story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      provider_session: {
        id: "sess-rejected-story",
        kind: "session_id",
        provider: "codex",
      },
      response_metadata: {
        finish_reason: "rejected",
        session_source: "codex",
      },
      response_text: "The active story needs revision before it can continue.",
    },
    total_duration_millis: 2_000,
    trace_ids: ["trace-active-story"],
    working_directory: REVIEW_WORKING_DIRECTORY,
    worktree: REVIEW_WORKTREE,
  },
);

export const erroredWorkstationRequestFixture = buildDashboardWorkstationRequestFixture(
  ERRORED_DISPATCH_ID,
  {
    counts: {
      dispatched_count: 1,
      errored_count: 1,
      responded_count: 0,
    },
    errored_request_count: 1,
    failure_message: "Provider rate limit exceeded while reviewing the story.",
    failure_reason: "provider_rate_limit",
    inference_attempts: [
      buildDashboardInferenceAttemptFixture(ERRORED_DISPATCH_ID, {
        error_class: "provider_rate_limit",
        inference_request_id: `${ERRORED_DISPATCH_ID}/inference-request/1`,
        outcome: "FAILED",
        response_time: "2026-04-08T12:00:02Z",
      }),
    ],
    outcome: "FAILED",
    prompt: "Review the blocked story and explain the failure.",
    request_view: {
      input_work_items: [
        {
          display_name: "Active Story",
          trace_id: "trace-active-story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      prompt: "Review the blocked story and explain the failure.",
      request_time: REVIEW_REQUEST_TIME,
      started_at: REVIEW_REQUEST_TIME,
      trace_ids: ["trace-active-story"],
    },
    request_id: "request-error-story",
    responded_request_count: 0,
    response_view: {
      error_class: "provider_rate_limit",
      failure_message: "Provider rate limit exceeded while reviewing the story.",
      failure_reason: "provider_rate_limit",
      outcome: "FAILED",
    },
  },
);

export const scriptPendingWorkstationRequestFixture = buildDashboardWorkstationRequestFixture(
  "dispatch-review-script-pending",
  {
    prompt: undefined,
    request_id: "request-script-pending-story",
    responded_request_count: 0,
    script_request: {
      args: ["--work", "work-active-story"],
      attempt: 1,
      command: "script-tool",
      script_request_id: "dispatch-review-script-pending/script-request/1",
    },
  },
);

export const scriptSuccessWorkstationRequestFixture = buildDashboardWorkstationRequestFixture(
  "dispatch-review-script-success",
  {
    request_id: "request-script-success-story",
    responded_request_count: 1,
    script_request: {
      args: ["--work", "work-active-story"],
      attempt: 1,
      command: "script-tool",
      script_request_id: "dispatch-review-script-success/script-request/1",
    },
    script_response: {
      duration_millis: 222,
      outcome: "SUCCEEDED",
      script_request_id: "dispatch-review-script-success/script-request/1",
      stderr: "",
      stdout: "script success stdout\n",
    },
  },
);

export const scriptFailedWorkstationRequestFixture = buildDashboardWorkstationRequestFixture(
  "dispatch-review-script-failed",
  {
    errored_request_count: 1,
    failure_message: "Script timed out.",
    failure_reason: "script_timeout",
    request_id: "request-script-failed-story",
    responded_request_count: 0,
    script_request: {
      args: ["--work", "work-active-story"],
      attempt: 1,
      command: "script-tool",
      script_request_id: "dispatch-review-script-failed/script-request/1",
    },
    script_response: {
      duration_millis: 500,
      failure_type: "TIMEOUT",
      outcome: "TIMED_OUT",
      script_request_id: "dispatch-review-script-failed/script-request/1",
      stderr: "script timed out\n",
      stdout: "",
    },
  },
);

export const dashboardWorkstationRequestFixtures = {
  noResponse: noResponseWorkstationRequestFixture,
  ready: readyWorkstationRequestFixture,
  rejected: rejectedWorkstationRequestFixture,
  errored: erroredWorkstationRequestFixture,
  scriptFailed: scriptFailedWorkstationRequestFixture,
  scriptPending: scriptPendingWorkstationRequestFixture,
  scriptSuccess: scriptSuccessWorkstationRequestFixture,
} satisfies Record<string, DashboardWorkstationRequest>;

