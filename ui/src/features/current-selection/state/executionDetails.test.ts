import { describe, expect, it } from "vitest";
import type {
  DashboardActiveExecution,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardTrace,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
} from "../../../api/dashboard";

import { selectWorkItemExecutionDetails } from "./executionDetails";

const selectedWorkItem: DashboardWorkItemRef = {
  display_name: "Runtime Story",
  trace_id: "trace-runtime",
  work_id: "work-runtime",
  work_type_id: "story",
};

const selectedNode: DashboardWorkstationNode = {
  model: "gpt-5.4",
  node_id: "review",
  provider: "codex",
  transition_id: "review",
  worker_type: "reviewer",
  workstation_name: "Review",
};

const activeExecution: DashboardActiveExecution = {
  dispatch_id: "dispatch-runtime",
  model: "gpt-5.4",
  provider: "codex",
  started_at: "2026-04-18T12:00:00Z",
  trace_ids: ["trace-runtime"],
  transition_id: "review",
  work_items: [selectedWorkItem],
  work_type_ids: ["story"],
  workstation_name: "Review",
  workstation_node_id: "review",
};

const completedAttempt: DashboardProviderSessionAttempt = {
  diagnostics: {
    provider: {
      model: "gpt-5.4",
      provider: "codex",
      request_metadata: {
        prompt_source: "factory-renderer",
      },
      response_metadata: {
        retry_count: "1",
      },
    },
    rendered_prompt: {
      system_prompt_hash: "sha256:system-runtime",
      user_message_hash: "sha256:user-runtime",
    },
  },
  dispatch_id: "dispatch-runtime",
  outcome: "ACCEPTED",
  provider_session: {
    id: "session-runtime",
    kind: "session_id",
    provider: "codex",
  },
  transition_id: "review",
  work_items: [selectedWorkItem],
  workstation_name: "Review",
};

const successfulWorkstationRequest: DashboardRuntimeWorkstationRequest = {
  counts: {
    dispatched_count: 2,
    errored_count: 1,
    responded_count: 1,
  },
  dispatch_id: "dispatch-runtime",
  request: {
    input_work_items: [selectedWorkItem],
    input_work_type_ids: ["story"],
    started_at: "2026-04-18T12:00:00Z",
    trace_ids: ["trace-runtime"],
  },
  response: {
    duration_millis: 1200,
    end_time: "2026-04-18T12:00:02Z",
    outcome: "ACCEPTED",
  },
  transition_id: "review",
  workstation_name: "Review",
};

describe("selectWorkItemExecutionDetails", () => {
  it("uses the workstation-request projection as the primary selected-work accessor for successful runs", () => {
    const details = selectWorkItemExecutionDetails({
      activeExecution,
      dispatchID: activeExecution.dispatch_id,
      inferenceAttemptsByDispatchID: {
        "dispatch-runtime": {
          "dispatch-runtime/inference-request/2": {
            attempt: 2,
            dispatch_id: "dispatch-runtime",
            duration_millis: 875,
            error_class: "rate_limited",
            exit_code: 1,
            inference_request_id: "dispatch-runtime/inference-request/2",
            outcome: "FAILED",
            prompt: "Retry the runtime story.",
            request_time: "2026-04-18T12:00:03Z",
            response_time: "2026-04-18T12:00:04Z",
            transition_id: "review",
          },
          "dispatch-runtime/inference-request/1": {
            attempt: 1,
            dispatch_id: "dispatch-runtime",
            duration_millis: 1200,
            inference_request_id: "dispatch-runtime/inference-request/1",
            outcome: "SUCCEEDED",
            prompt: "Review the runtime story.",
            request_time: "2026-04-18T12:00:01Z",
            response: "The story is complete.",
            response_time: "2026-04-18T12:00:02Z",
            transition_id: "review",
            working_directory: "/work/project",
            worktree: "/work/project/.worktrees/story",
          },
        },
      },
      providerSessions: [completedAttempt],
      selectedNode,
      workItem: selectedWorkItem,
      workstationRequestsByDispatchID: {
        "dispatch-runtime": successfulWorkstationRequest,
      },
    });

    expect(details.dispatchID).toBe("dispatch-runtime");
    expect(details.elapsedStartTimestamp).toBe("2026-04-18T12:00:00Z");
    expect(details.workstationName).toBe("Review");
    expect(details.workstationRequest).toEqual(successfulWorkstationRequest);
    expect(details.workstationRequest?.counts).toEqual({
      dispatched_count: 2,
      errored_count: 1,
      responded_count: 1,
    });
    expect(details.workstationRequest?.response?.duration_millis).toBe(1200);
    expect(details.inferenceAttempts.map((attempt) => attempt.inference_request_id)).toEqual([
      "dispatch-runtime/inference-request/1",
      "dispatch-runtime/inference-request/2",
    ]);
    expect(details.inferenceAttempts[0]).toMatchObject({
      outcome: "SUCCEEDED",
      prompt: "Review the runtime story.",
      response: "The story is complete.",
    });
    expect(details.inferenceAttempts[1]).toMatchObject({
      error_class: "rate_limited",
      exit_code: 1,
      outcome: "FAILED",
    });
    expect(details.traceIDs).toEqual(["trace-runtime"]);
  });

  it("preserves request-only workstation-request details for active runs", () => {
    const details = selectWorkItemExecutionDetails({
      activeExecution: {
        ...activeExecution,
        model: undefined,
        provider: undefined,
      },
      dispatchID: activeExecution.dispatch_id,
      selectedNode: {
        ...selectedNode,
        model: undefined,
        provider: undefined,
      },
      workItem: selectedWorkItem,
      workstationRequestsByDispatchID: {
        "dispatch-runtime": {
          counts: {
            dispatched_count: 1,
            errored_count: 0,
            responded_count: 0,
          },
          dispatch_id: "dispatch-runtime",
          request: {
            input_work_items: [selectedWorkItem],
            started_at: "2026-04-18T12:00:00Z",
            trace_ids: ["trace-runtime"],
          },
          transition_id: "review",
          workstation_name: "Review",
        },
      },
    });

    expect(details.dispatchID).toBe("dispatch-runtime");
    expect(details.workstationName).toBe("Review");
    expect(details.workstationRequest?.response).toBeUndefined();
    expect(details.workstationRequest?.request.started_at).toBe("2026-04-18T12:00:00Z");
    expect(details.workstationRequest?.counts).toEqual({
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 0,
    });
  });

  it("preserves response error details from the workstation-request projection", () => {
    const details = selectWorkItemExecutionDetails({
      dispatchID: "dispatch-runtime",
      providerSessions: [completedAttempt],
      selectedNode,
      workItem: selectedWorkItem,
      workstationRequestsByDispatchID: {
        "dispatch-runtime": {
          counts: {
            dispatched_count: 2,
            errored_count: 1,
            responded_count: 0,
          },
          dispatch_id: "dispatch-runtime",
          request: {
            input_work_items: [selectedWorkItem],
            started_at: "2026-04-18T12:00:00Z",
            trace_ids: ["trace-runtime"],
          },
          response: {
            durationMillis: 875,
            end_time: "2026-04-18T12:00:04Z",
            failure_message: "Provider rate limit exceeded.",
            failure_reason: "rate_limited",
            outcome: "FAILED",
          },
          transition_id: "review",
          workstation_name: "Review",
        },
      },
    });

    expect(details.dispatchID).toBe("dispatch-runtime");
    expect(details.workstationRequest?.response).toMatchObject({
      durationMillis: 875,
      failure_message: "Provider rate limit exceeded.",
      failure_reason: "rate_limited",
      outcome: "FAILED",
    });
  });

  it("keeps workstation identity details available when a run has no nested inference attempts yet", () => {
    const details = selectWorkItemExecutionDetails({
      activeExecution: {
        ...activeExecution,
        model: undefined,
        provider: undefined,
      },
      dispatchID: activeExecution.dispatch_id,
      selectedNode,
      workItem: selectedWorkItem,
    });

    expect(details.dispatchID).toBe("dispatch-runtime");
    expect(details.workstationName).toBe("Review");
    expect(details.inferenceAttempts).toEqual([]);
  });

  it("derives completed-run identity details from matching provider session attempts", () => {
    const details = selectWorkItemExecutionDetails({
      providerSessions: [completedAttempt],
      selectedNode,
      workItem: selectedWorkItem,
    });

    expect(details.dispatchID).toBe("dispatch-runtime");
    expect(details.workstationName).toBe("Review");
  });

  it("falls back to workstation identity when provider-session details are absent", () => {
    const details = selectWorkItemExecutionDetails({
      providerSessions: [
        {
          ...completedAttempt,
          diagnostics: {
            provider: {
              provider: "codex",
            },
          },
        },
      ],
      selectedNode,
      workItem: selectedWorkItem,
    });

    expect(details.dispatchID).toBe("dispatch-runtime");
    expect(details.workstationName).toBe("Review");
  });

  it("does not use provider sessions from another work item", () => {
    const unrelatedAttempt: DashboardProviderSessionAttempt = {
      ...completedAttempt,
      dispatch_id: "dispatch-other",
      provider_session: {
        id: "session-other",
        kind: "session_id",
        provider: "codex",
      },
      work_items: [{ ...selectedWorkItem, work_id: "work-other" }],
    };

    const details = selectWorkItemExecutionDetails({
      providerSessions: [unrelatedAttempt],
      workItem: selectedWorkItem,
    });

    expect(details.dispatchID).toBeUndefined();
    expect(details.workstationName).toBeUndefined();
    expect(details.inferenceAttempts).toEqual([]);
  });

  it("combines multiple trace identifiers from active execution, work item, and trace data", () => {
    const trace: DashboardTrace = {
      dispatches: [
        {
          dispatch_id: "dispatch-runtime",
          duration_millis: 1200,
          end_time: "2026-04-18T12:00:02Z",
          outcome: "ACCEPTED",
          start_time: "2026-04-18T12:00:00Z",
          trace_id: "trace-dispatch",
          transition_id: "review",
          work_ids: [selectedWorkItem.work_id],
        },
      ],
      trace_id: "trace-retained",
      transition_ids: ["review"],
      work_ids: [selectedWorkItem.work_id],
      workstation_sequence: ["Review"],
    };

    const details = selectWorkItemExecutionDetails({
      activeExecution: {
        ...activeExecution,
        trace_ids: ["trace-runtime", "trace-secondary"],
      },
      dispatchID: activeExecution.dispatch_id,
      trace,
      workItem: {
        ...selectedWorkItem,
        trace_id: "trace-work-item",
      },
    });

    expect(details.traceIDs).toEqual([
      "trace-dispatch",
      "trace-retained",
      "trace-runtime",
      "trace-secondary",
      "trace-work-item",
    ]);
  });

  it("does not project provider diagnostics onto execution details", () => {
    const details = selectWorkItemExecutionDetails({
      providerSessions: [
        {
          ...completedAttempt,
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
              variables: {
                prompt_source: "factory-renderer",
                system_prompt: "Never expose this raw system prompt.",
                user_message: "Never expose this raw user message.",
              },
            },
          },
        },
      ],
      workItem: selectedWorkItem,
    });

    expect(JSON.stringify(details)).not.toContain("Never expose this raw system prompt.");
    expect(JSON.stringify(details)).not.toContain("Never expose this raw user message.");
    expect(JSON.stringify(details)).not.toContain("factory-renderer");
    expect(JSON.stringify(details)).not.toContain("sha256:system-runtime");
  });
});
