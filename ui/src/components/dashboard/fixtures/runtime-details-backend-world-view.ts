import type { DashboardRuntimeWorkstationRequest } from "../../../api/dashboard";

import { runtimeDetailsFixtureIDs } from "./runtime-details-events";

export const runtimeDetailsBackendWorkstationRequestsByDispatchID = {
  [runtimeDetailsFixtureIDs.activeDispatchID]: {
    counts: {
      dispatched_count: 0,
      errored_count: 0,
      responded_count: 0,
    },
    dispatch_id: runtimeDetailsFixtureIDs.activeDispatchID,
    request: {
      input_work_items: [
        {
          display_name: runtimeDetailsFixtureIDs.activeWorkLabel,
          trace_id: runtimeDetailsFixtureIDs.activeTraceID,
          work_id: runtimeDetailsFixtureIDs.activeWorkID,
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      provider: "codex",
      started_at: "2026-04-18T12:00:03Z",
      trace_ids: [runtimeDetailsFixtureIDs.activeTraceID],
    },
    transition_id: "review",
    workstation_name: "Review",
  },
  [runtimeDetailsFixtureIDs.completedDispatchID]: {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 1,
    },
    dispatch_id: runtimeDetailsFixtureIDs.completedDispatchID,
    request: {
      input_work_items: [
        {
          display_name: runtimeDetailsFixtureIDs.completedWorkLabel,
          trace_id: runtimeDetailsFixtureIDs.completedTraceID,
          work_id: runtimeDetailsFixtureIDs.completedWorkID,
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      model: "gpt-5.4",
      prompt: "Review the completed runtime story.",
      provider: "codex",
      request_metadata: {
        prompt_source: runtimeDetailsFixtureIDs.completedPromptSource,
        source: "runtime-details-fixture",
      },
      request_time: "2026-04-18T12:00:05Z",
      started_at: "2026-04-18T12:00:04Z",
      trace_ids: [runtimeDetailsFixtureIDs.completedTraceID],
      working_directory: "/work/completed-runtime",
      worktree: "/work/completed-runtime/.worktrees/runtime",
    },
    response: {
      duration_millis: runtimeDetailsFixtureIDs.completedDurationMillis,
      end_time: "2026-04-18T12:00:07Z",
      outcome: "ACCEPTED",
      provider_session: {
        id: runtimeDetailsFixtureIDs.completedProviderSessionID,
        kind: "session_id",
        provider: "codex",
      },
      response_metadata: {
        provider_session_id: runtimeDetailsFixtureIDs.completedProviderSessionID,
        retry_count: "0",
      },
      response_text: runtimeDetailsFixtureIDs.completedResponseText,
    },
    transition_id: "review",
    workstation_name: "Review",
  },
  [runtimeDetailsFixtureIDs.failedDispatchID]: {
    counts: {
      dispatched_count: 1,
      errored_count: 1,
      responded_count: 0,
    },
    dispatch_id: runtimeDetailsFixtureIDs.failedDispatchID,
    request: {
      input_work_items: [
        {
          display_name: runtimeDetailsFixtureIDs.failedWorkLabel,
          trace_id: runtimeDetailsFixtureIDs.failedTraceID,
          work_id: runtimeDetailsFixtureIDs.failedWorkID,
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      model: "claude-3.7",
      prompt: "Retry the failed runtime story.",
      provider: "anthropic",
      request_metadata: {
        prompt_source: runtimeDetailsFixtureIDs.failedPromptSource,
        source: "runtime-details-fixture",
      },
      request_time: "2026-04-18T12:00:09Z",
      started_at: "2026-04-18T12:00:08Z",
      trace_ids: [runtimeDetailsFixtureIDs.failedTraceID],
      working_directory: "/work/failed-runtime",
      worktree: "/work/failed-runtime/.worktrees/runtime",
    },
    response: {
      duration_millis: runtimeDetailsFixtureIDs.failedDurationMillis,
      end_time: "2026-04-18T12:00:11Z",
      error_class: runtimeDetailsFixtureIDs.failedErrorClass,
      failure_message: runtimeDetailsFixtureIDs.failedFailureMessage,
      failure_reason: runtimeDetailsFixtureIDs.failedFailureReason,
      outcome: "FAILED",
      provider_session: {
        id: runtimeDetailsFixtureIDs.failedProviderSessionID,
        kind: "session_id",
        provider: "anthropic",
      },
      response_metadata: {
        provider_session_id: runtimeDetailsFixtureIDs.failedProviderSessionID,
        retry_count: "1",
      },
    },
    transition_id: "review",
    workstation_name: "Review",
  },
} satisfies Record<string, DashboardRuntimeWorkstationRequest>;

