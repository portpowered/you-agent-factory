import type {
  DashboardSnapshot,
  DashboardTrace,
  DashboardWorkstationRequest,
} from "../api/dashboard";
import { dashboardWorkstationRequestFixtures } from "../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../components/dashboard/test-fixtures";
export const activeStoryTrace: DashboardTrace = {
  trace_id: "trace-active-story",
  work_ids: ["work-active-story"],
  transition_ids: ["plan", "review"],
  workstation_sequence: ["Plan", "Review"],
  dispatches: [
    {
      dispatch_id: "dispatch-review-active",
      transition_id: "review",
      workstation_name: "Review",
      outcome: "ACCEPTED",
      start_time: "2026-04-08T12:00:00Z",
      end_time: "2026-04-08T12:00:01Z",
      duration_millis: 1000,
      consumed_tokens: [],
      output_mutations: [],
    },
  ],
};

export const historicalWorkOutcomeSnapshot = workOutcomeSnapshot(
  semanticWorkflowDashboardSnapshot,
  2,
  {
    completed: 2,
    completedLabels: ["Historical Story"],
    dispatched: 3,
    failed: 1,
    failedByWorkType: { story: 1 },
    failedLabels: ["Historical Failure"],
    inFlight: 1,
    queued: 2,
  },
);
export const liveWorkOutcomeSnapshot = workOutcomeSnapshot(
  semanticWorkflowDashboardSnapshot,
  5,
  {
    completed: 11,
    completedLabels: ["Historical Story", "Live Story"],
    dispatched: 14,
    failed: 4,
    failedByWorkType: { story: 3, task: 1 },
    failedLabels: ["Historical Failure", "Live Failure"],
    inFlight: 2,
    queued: 3,
  },
);
export const inferenceDetailsSnapshot = withInferenceDetails(
  semanticWorkflowDashboardSnapshot,
);
export const markdownReadyWorkstationRequest: DashboardWorkstationRequest = {
  ...dashboardWorkstationRequestFixtures.ready,
  inference_attempts: dashboardWorkstationRequestFixtures.ready.inference_attempts.map(
    (attempt) =>
      attempt.attempt === 2
        ? {
            ...attempt,
            prompt: [
              "## Review checklist",
              "",
              "- Check the latest diff",
              "- Run `bun test` before approval",
              "",
              "```text",
              "bun test",
              "```",
            ].join("\n"),
            response: [
              "### Reviewer response",
              "",
              "1. Run `bun run lint`",
              "2. Confirm the diff is limited",
              "",
              "```text",
              "bun run lint",
              "```",
            ].join("\n"),
          }
        : attempt,
  ),
};
export interface WorkOutcomeCounts {
  completed: number;
  failed: number;
  inFlight: number;
  queued: number;
}

export interface WorkOutcomeSnapshotOptions extends WorkOutcomeCounts {
  completedLabels: string[];
  dispatched: number;
  failedByWorkType: Record<string, number>;
  failedLabels: string[];
}

export function workOutcomeSnapshot(
  source: DashboardSnapshot,
  tickCount: number,
  options: WorkOutcomeSnapshotOptions,
): DashboardSnapshot {
  return {
    ...source,
    tick_count: tickCount,
    runtime: {
      ...source.runtime,
      in_flight_dispatch_count: options.inFlight,
      place_token_counts: {
        ...(source.runtime.place_token_counts ?? {}),
        "story:init": options.queued,
      },
      session: {
        ...source.runtime.session,
        completed_count: options.completed,
        completed_work_labels: options.completedLabels,
        dispatched_count: options.dispatched,
        failed_by_work_type: options.failedByWorkType,
        failed_count: options.failed,
        failed_work_labels: options.failedLabels,
      },
    },
  };
}

export function withInferenceDetails(source: DashboardSnapshot): DashboardSnapshot {
  return {
    ...source,
    runtime: {
      ...source.runtime,
      inference_attempts_by_dispatch_id: {
        ...(source.runtime.inference_attempts_by_dispatch_id ?? {}),
        "dispatch-review-active": {
          "dispatch-review-active/inference-request/1": {
            attempt: 1,
            dispatch_id: "dispatch-review-active",
            duration_millis: 520,
            error_class: "provider_rate_limit",
            inference_request_id: "dispatch-review-active/inference-request/1",
            outcome: "FAILED",
            prompt: "Review Active Story and return a decision.",
            request_time: "2026-04-08T12:00:01Z",
            response_time: "2026-04-08T12:00:02Z",
            transition_id: "review",
            working_directory: "C:\\work\\portos",
            worktree: "C:\\work\\portos\\.worktrees\\active-story",
          },
          "dispatch-review-active/inference-request/2": {
            attempt: 2,
            dispatch_id: "dispatch-review-active",
            duration_millis: 740,
            inference_request_id: "dispatch-review-active/inference-request/2",
            outcome: "SUCCEEDED",
            prompt: "Retry Active Story after provider recovery.",
            request_time: "2026-04-08T12:00:03Z",
            response: "Active Story is ready for the next workstation.",
            response_time: "2026-04-08T12:00:04Z",
            transition_id: "review",
            working_directory: "C:\\work\\portos",
            worktree: "C:\\work\\portos\\.worktrees\\active-story",
          },
        },
      },
      session: {
        ...source.runtime.session,
        provider_sessions: (source.runtime.session.provider_sessions ?? []).map(
          (attempt) =>
            attempt.dispatch_id === "dispatch-review-active"
              ? {
                  ...attempt,
                  diagnostics: {
                    provider: {
                      model: "gpt-5.4",
                      provider: "codex",
                      request_metadata: {
                        prompt_source: "factory-renderer",
                      },
                    },
                    rendered_prompt: {
                      system_prompt_hash: "sha256:system-runtime",
                      user_message_hash: "sha256:user-runtime",
                    },
                  },
                }
              : attempt,
        ),
      },
    },
  };
}

export const failedStoryTrace: DashboardTrace = {
  trace_id: "trace-failed-story",
  work_ids: ["work-failed-story"],
  transition_ids: ["repair"],
  workstation_sequence: ["Repair"],
  dispatches: [
    {
      dispatch_id: "dispatch-repair-failed",
      transition_id: "repair",
      workstation_name: "Repair",
      outcome: "FAILED",
      failure_message:
        "Provider rate limit exceeded while generating the repair.",
      failure_reason: "provider_rate_limit",
      start_time: "2026-04-08T12:00:00Z",
      end_time: "2026-04-08T12:00:01Z",
      duration_millis: 1000,
      consumed_tokens: [],
      output_mutations: [],
    },
  ],
};
