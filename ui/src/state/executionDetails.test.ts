import { describe, expect, it } from "vitest";
import type {
  DashboardActiveExecution,
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardTrace,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
} from "../api/dashboard";

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
    model: "gpt-5.4",
    prompt: "Review the runtime story.",
    provider: "codex",
    request_metadata: {
      prompt_source: "factory-renderer",
    },
    request_time: "2026-04-18T12:00:01Z",
    started_at: "2026-04-18T12:00:00Z",
    trace_ids: ["trace-runtime"],
    working_directory: "/work/project",
    worktree: "/work/project/.worktrees/story",
  },
  response: {
    diagnostics: completedAttempt.diagnostics,
    duration_millis: 1200,
    end_time: "2026-04-18T12:00:02Z",
    outcome: "ACCEPTED",
    provider_session: completedAttempt.provider_session,
    response_text: "The story is complete.",
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
    expect(details.provider).toEqual({
      source: "workstation-request",
      status: "available",
      value: "codex",
    });
    expect(details.model).toEqual({
      source: "workstation-request",
      status: "available",
      value: "gpt-5.4",
    });
    expect(details.providerSession).toEqual({
      source: "workstation-request",
      status: "available",
      value: "session-runtime",
    });
    expect(details.prompt).toEqual({
      promptSource: "factory-renderer",
      source: "diagnostics",
      status: "available",
      systemPromptHash: "sha256:system-runtime",
    });
    expect(details.workstationRequest).toEqual(successfulWorkstationRequest);
    expect(details.workstationRequest?.counts).toEqual({
      dispatched_count: 2,
      errored_count: 1,
      responded_count: 1,
    });
    expect(details.workstationRequest?.response?.response_text).toBe(
      "The story is complete.",
    );
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
            model: "gpt-5.4-mini",
            prompt: "Inspect the runtime request before any response arrives.",
            provider: "codex",
            request_metadata: {
              prompt_source: "request-projection",
            },
            request_time: "2026-04-18T12:00:01Z",
            started_at: "2026-04-18T12:00:00Z",
            trace_ids: ["trace-runtime"],
            working_directory: "/work/request-only",
            worktree: "/work/request-only/.worktrees/runtime",
          },
          transition_id: "review",
          workstation_name: "Review",
        },
      },
    });

    expect(details.provider).toEqual({
      source: "workstation-request",
      status: "available",
      value: "codex",
    });
    expect(details.model).toEqual({
      source: "workstation-request",
      status: "available",
      value: "gpt-5.4-mini",
    });
    expect(details.providerSession).toEqual({ status: "pending" });
    expect(details.prompt).toEqual({
      promptSource: "request-projection",
      source: "diagnostics",
      status: "available",
      systemPromptHash: undefined,
    });
    expect(details.workstationRequest?.response).toBeUndefined();
    expect(details.workstationRequest?.request.prompt).toContain("Inspect the runtime request");
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
            model: "gpt-5.4",
            prompt: "Retry the runtime story.",
            provider: "codex",
            request_time: "2026-04-18T12:00:03Z",
            started_at: "2026-04-18T12:00:00Z",
            trace_ids: ["trace-runtime"],
          },
          response: {
            diagnostics: completedAttempt.diagnostics,
            durationMillis: 875,
            end_time: "2026-04-18T12:00:04Z",
            error_class: "rate_limited",
            failure_message: "Provider rate limit exceeded.",
            failure_reason: "rate_limited",
            outcome: "FAILED",
            provider_session: completedAttempt.provider_session,
          },
          transition_id: "review",
          workstation_name: "Review",
        },
      },
    });

    expect(details.provider).toEqual({
      source: "workstation-request",
      status: "available",
      value: "codex",
    });
    expect(details.providerSession).toEqual({
      source: "workstation-request",
      status: "available",
      value: "session-runtime",
    });
    expect(details.workstationRequest?.response).toMatchObject({
      durationMillis: 875,
      error_class: "rate_limited",
      failure_message: "Provider rate limit exceeded.",
      failure_reason: "rate_limited",
      outcome: "FAILED",
    });
  });

  it("keeps active-run model details pending when the workstation has a configured model but the run has none", () => {
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

    expect(details.provider).toEqual({
      source: "workstation",
      status: "available",
      value: "codex",
    });
    expect(details.model).toEqual({ status: "pending" });
    expect(details.providerSession).toEqual({ status: "pending" });
    expect(details.prompt).toEqual({ status: "pending" });
  });

  it("derives completed-run details from matching provider session attempts", () => {
    const details = selectWorkItemExecutionDetails({
      providerSessions: [completedAttempt],
      selectedNode,
      workItem: selectedWorkItem,
    });

    expect(details.dispatchID).toBe("dispatch-runtime");
    expect(details.provider).toEqual({
      source: "provider-diagnostics",
      status: "available",
      value: "codex",
    });
    expect(details.model).toEqual({
      source: "provider-diagnostics",
      status: "available",
      value: "gpt-5.4",
    });
    expect(details.providerSession).toEqual({
      source: "provider-session",
      status: "available",
      value: "session-runtime",
    });
    expect(details.prompt.status).toBe("available");
  });

  it("omits historical model details when no run metadata exists even if the workstation has a configured model", () => {
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

    expect(details.model).toEqual({ status: "omitted" });
    expect(details.provider).toEqual({
      source: "provider-diagnostics",
      status: "available",
      value: "codex",
    });
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
    expect(details.providerSession).toEqual({ status: "unavailable" });
    expect(details.model).toEqual({ status: "omitted" });
    expect(details.prompt).toEqual({ status: "unavailable" });
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

  it("exposes only safe prompt hashes or source metadata from diagnostics", () => {
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

    expect(details.prompt).toEqual({
      promptSource: "factory-renderer",
      source: "diagnostics",
      status: "available",
      systemPromptHash: "sha256:system-runtime",
    });
    expect(JSON.stringify(details)).not.toContain("Never expose this raw system prompt.");
    expect(JSON.stringify(details)).not.toContain("Never expose this raw user message.");
  });
});
