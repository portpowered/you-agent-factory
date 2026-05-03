import type { DashboardRuntimeWorkstationRequest } from "../../../api/dashboard";

import { scriptDashboardIntegrationFixtureIDs } from "./script-dashboard-integration-events";

export const scriptDashboardIntegrationBackendWorkstationRequestsByDispatchID = {
  [scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID]: {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 1,
    },
    dispatch_id: scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID,
    request: {
      input_work_items: [
        {
          display_name: scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkLabel,
          trace_id: scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID,
          work_id: scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID,
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      script_request: {
        args: ["--work", scriptDashboardIntegrationFixtureIDs.scriptSuccessWorkID],
        attempt: 1,
        command: "script-tool",
        script_request_id: `${scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID}/script-request/1`,
      },
      started_at: "2026-04-19T12:00:03Z",
      trace_ids: [scriptDashboardIntegrationFixtureIDs.scriptSuccessTraceID],
    },
    response: {
      duration_millis: 222,
      end_time: "2026-04-19T12:00:06Z",
      outcome: "ACCEPTED",
      script_response: {
        duration_millis: 222,
        outcome: "SUCCEEDED",
        script_request_id: `${scriptDashboardIntegrationFixtureIDs.scriptSuccessDispatchID}/script-request/1`,
        stderr: "",
        stdout: "script success stdout\n",
      },
    },
    transition_id: "review",
    workstation_name: "Review",
  },
  [scriptDashboardIntegrationFixtureIDs.failedDispatchID]: {
    counts: {
      dispatched_count: 1,
      errored_count: 1,
      responded_count: 0,
    },
    dispatch_id: scriptDashboardIntegrationFixtureIDs.failedDispatchID,
    request: {
      input_work_items: [
        {
          display_name: scriptDashboardIntegrationFixtureIDs.failedWorkLabel,
          trace_id: scriptDashboardIntegrationFixtureIDs.failedTraceID,
          work_id: scriptDashboardIntegrationFixtureIDs.failedWorkID,
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      script_request: {
        args: ["--work", scriptDashboardIntegrationFixtureIDs.failedWorkID],
        attempt: 1,
        command: "script-tool",
        script_request_id: `${scriptDashboardIntegrationFixtureIDs.failedDispatchID}/script-request/1`,
      },
      started_at: "2026-04-19T12:00:07Z",
      trace_ids: [scriptDashboardIntegrationFixtureIDs.failedTraceID],
    },
    response: {
      duration_millis: 500,
      end_time: "2026-04-19T12:00:10Z",
      failure_message: scriptDashboardIntegrationFixtureIDs.failedFailureMessage,
      failure_reason: scriptDashboardIntegrationFixtureIDs.failedFailureReason,
      outcome: "FAILED",
      script_response: {
        duration_millis: 500,
        failure_type: "TIMEOUT",
        outcome: "TIMED_OUT",
        script_request_id: `${scriptDashboardIntegrationFixtureIDs.failedDispatchID}/script-request/1`,
        stderr: "script timed out\n",
        stdout: "",
      },
    },
    transition_id: "review",
    workstation_name: "Review",
  },
  [scriptDashboardIntegrationFixtureIDs.inferenceDispatchID]: {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 1,
    },
    dispatch_id: scriptDashboardIntegrationFixtureIDs.inferenceDispatchID,
    request: {
      input_work_items: [
        {
          display_name: scriptDashboardIntegrationFixtureIDs.inferenceWorkLabel,
          trace_id: scriptDashboardIntegrationFixtureIDs.inferenceTraceID,
          work_id: scriptDashboardIntegrationFixtureIDs.inferenceWorkID,
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      model: "gpt-5.4",
      prompt: "Review the inference-backed dashboard story.",
      provider: "codex",
      request_metadata: {
        prompt_source: scriptDashboardIntegrationFixtureIDs.inferencePromptSource,
        source: "script-dashboard-integration-fixture",
      },
      request_time: "2026-04-19T12:00:12Z",
      started_at: "2026-04-19T12:00:11Z",
      trace_ids: [scriptDashboardIntegrationFixtureIDs.inferenceTraceID],
      working_directory: "/work/inference-dashboard",
      worktree: "/work/inference-dashboard/.worktrees/story",
    },
    response: {
      duration_millis: scriptDashboardIntegrationFixtureIDs.inferenceDurationMillis,
      end_time: "2026-04-19T12:00:14Z",
      outcome: "ACCEPTED",
      provider_session: {
        id: scriptDashboardIntegrationFixtureIDs.inferenceProviderSessionID,
        kind: "session_id",
        provider: "codex",
      },
      response_metadata: {
        provider_session_id: scriptDashboardIntegrationFixtureIDs.inferenceProviderSessionID,
        retry_count: "0",
      },
      response_text: scriptDashboardIntegrationFixtureIDs.inferenceResponseText,
    },
    transition_id: "review",
    workstation_name: "Review",
  },
} satisfies Record<string, DashboardRuntimeWorkstationRequest>;

