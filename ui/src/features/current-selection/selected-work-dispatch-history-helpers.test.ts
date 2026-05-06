import { describe, expect, it } from "vitest";

import type {
  DashboardRuntimeWorkstationRequest,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";
import type { SelectedWorkRequestHistoryItem } from "./detail-card-types";
import {
  dedupeWorkItems,
  hasResponseDetails,
  isProjectedWorkstationRequest,
  requestCounts,
  requestDurationMillis,
  requestFailureMessage,
  requestFailureReason,
  requestInferenceAttempts,
  requestInputWorkItems,
  requestModel,
  requestOutcome,
  requestOutputWorkItems,
  requestPrompt,
  requestProvider,
  requestProviderSession,
  requestResponseText,
  requestScriptRequest,
  requestScriptResponse,
  requestStartedAt,
  requestTraceIDs,
  requestWorkingDirectory,
  requestWorktree,
  scriptAttemptNumber,
  scriptRequestID,
  scriptResponseDurationMillis,
  scriptResponseExitCode,
  scriptResponseFailureType,
} from "./selected-work-dispatch-history-helpers";

const inputWorkItem: DashboardWorkItemRef = {
  display_name: "Input Story",
  trace_id: "trace-input",
  work_id: "work-input",
  work_type_id: "story",
};

const outputWorkItem: DashboardWorkItemRef = {
  display_name: "Output Story",
  trace_id: "trace-output",
  work_id: "work-output",
  work_type_id: "story",
};

function buildProjectedRequest(
  overrides: Partial<DashboardWorkstationRequest> = {},
): DashboardWorkstationRequest {
  return {
    counts: {
      dispatched_count: 2,
      errored_count: 1,
      responded_count: 1,
    },
    dispatch_id: "dispatch-projected",
    dispatched_request_count: 2,
    errored_request_count: 1,
    inference_attempts: [],
    request_view: {
      input_work_items: [inputWorkItem],
      started_at: "2026-04-08T12:00:00Z",
      trace_ids: ["trace-input", "trace-request-view"],
    },
    responded_request_count: 1,
    response_view: {
      duration_millis: 1250,
      failure_message: "Dispatch failed after response view fallback.",
      failure_reason: "response_view_failed",
      outcome: "FAILED",
      output_work_items: [outputWorkItem],
    },
    started_at: "2026-04-08T12:00:01Z",
    transition_id: "review",
    work_items: [inputWorkItem, outputWorkItem],
    workstation_name: "Review",
    workstation_node_id: "review",
    ...overrides,
  };
}

function buildRuntimeRequest(
  overrides: Partial<DashboardRuntimeWorkstationRequest> = {},
): DashboardRuntimeWorkstationRequest {
  return {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 1,
    },
    dispatch_id: "dispatch-runtime",
    request: {
      input_work_items: [inputWorkItem],
      started_at: "2026-04-08T12:00:02Z",
      trace_ids: ["trace-input", "trace-runtime"],
    },
    response: {
      duration_millis: 900,
      failure_message: "Runtime failure message",
      failure_reason: "runtime_failed",
      outcome: "ACCEPTED",
      output_work_items: [outputWorkItem],
    },
    transition_id: "review",
    workstation_name: "Review",
    ...overrides,
  };
}

describe("selected-work-dispatch-history-helpers", () => {
  it("distinguishes projected requests from runtime requests", () => {
    expect(isProjectedWorkstationRequest(buildProjectedRequest())).toBe(true);
    expect(isProjectedWorkstationRequest(buildRuntimeRequest())).toBe(false);
  });

  it("reads counts, work items, trace ids, and timestamps from the correct owner surface", () => {
    const projected = buildProjectedRequest({
      trace_ids: ["trace-top-level", "", "trace-request-view"],
    });
    const runtime = buildRuntimeRequest();

    expect(requestCounts(projected)).toEqual({
      dispatchedCount: 2,
      erroredCount: 1,
      respondedCount: 1,
    });
    expect(requestCounts(runtime)).toEqual({
      dispatchedCount: 1,
      erroredCount: 0,
      respondedCount: 1,
    });

    expect(requestInputWorkItems(projected)).toEqual([inputWorkItem]);
    expect(requestInputWorkItems(runtime)).toEqual([inputWorkItem]);
    expect(requestOutputWorkItems(projected)).toEqual([outputWorkItem]);
    expect(requestOutputWorkItems(runtime)).toEqual([outputWorkItem]);

    expect(requestTraceIDs(projected)).toEqual([
      "trace-top-level",
      "trace-request-view",
      "trace-input",
    ]);
    expect(requestTraceIDs(runtime)).toEqual(["trace-input", "trace-runtime"]);

    expect(requestStartedAt(projected)).toBe("2026-04-08T12:00:01Z");
    expect(
      requestStartedAt(
        buildProjectedRequest({
          started_at: undefined,
        }),
      ),
    ).toBe("2026-04-08T12:00:00Z");
    expect(requestStartedAt(runtime)).toBe("2026-04-08T12:00:02Z");
  });

  it("keeps prompt, provider, model, working directory, worktree, session, and response text on projected requests only", () => {
    const projected = buildProjectedRequest({
      model: "gpt-5.4",
      prompt: "Review this story.",
      provider: "codex",
      provider_session: {
        id: "session-1",
        kind: "session_id",
        provider: "codex",
      },
      response: "Projected response text",
      working_directory: "/repo/worktree",
      worktree: "/repo/.worktrees/story",
    });
    const runtime = buildRuntimeRequest();

    expect(requestPrompt(projected)).toBe("Review this story.");
    expect(requestProvider(projected)).toBe("codex");
    expect(requestModel(projected)).toBe("gpt-5.4");
    expect(requestWorkingDirectory(projected)).toBe("/repo/worktree");
    expect(requestWorktree(projected)).toBe("/repo/.worktrees/story");
    expect(requestProviderSession(projected)).toEqual({
      id: "session-1",
      kind: "session_id",
      provider: "codex",
    });
    expect(requestResponseText(projected)).toBe("Projected response text");

    expect(requestPrompt(runtime)).toBeUndefined();
    expect(requestProvider(runtime)).toBeUndefined();
    expect(requestModel(runtime)).toBeUndefined();
    expect(requestWorkingDirectory(runtime)).toBeUndefined();
    expect(requestWorktree(runtime)).toBeUndefined();
    expect(requestProviderSession(runtime)).toBeUndefined();
    expect(requestResponseText(runtime)).toBeUndefined();
  });

  it("uses response and script fallbacks for durations, outcomes, failures, and script attempt data", () => {
    const projectedFromResponseView = buildProjectedRequest();
    const projectedFromScript = buildProjectedRequest({
      outcome: undefined,
      response_view: {
        script_response: {
          attempt: 3,
          duration_millis: 321,
          exit_code: 17,
          failure_type: "TIMEOUT",
          outcome: "TIMED_OUT",
          script_request_id: "script-response-view",
        },
      },
      script_request: {
        args: ["--work", "work-input"],
        attempt: 2,
        command: "script-tool",
        script_request_id: "script-request-top-level",
      },
      script_response: undefined,
      total_duration_millis: undefined,
    });
    const projectedFromTopLevelScript = buildProjectedRequest({
      outcome: undefined,
      response_view: undefined,
      script_response: {
        attempt: 4,
        duration_millis: 654,
        exit_code: 9,
        failure_type: "PROCESS_ERROR",
        outcome: "FAILED_EXIT_CODE",
        script_request_id: "script-response-top-level",
      },
      total_duration_millis: undefined,
    });
    const runtime = buildRuntimeRequest({
      request: {
        input_work_items: [inputWorkItem],
        script_request: {
          args: ["--runtime"],
          attempt: 5,
          command: "runtime-script",
          script_request_id: "runtime-script-request",
        },
        started_at: "2026-04-08T12:00:02Z",
        trace_ids: ["trace-runtime"],
      },
      response: {
        duration_millis: 900,
        failure_message: "Runtime failure message",
        failure_reason: "runtime_failed",
        outcome: "ACCEPTED",
        output_work_items: [outputWorkItem],
        script_response: {
          attempt: 5,
          duration_millis: 222,
          exit_code: 0,
          failure_type: "TIMEOUT",
          outcome: "SUCCEEDED",
          script_request_id: "runtime-script-request",
        },
      },
    });

    expect(requestDurationMillis(projectedFromResponseView)).toBe(1250);
    expect(requestDurationMillis(projectedFromScript)).toBe(321);
    expect(requestDurationMillis(projectedFromTopLevelScript)).toBe(654);
    expect(requestDurationMillis(runtime)).toBe(900);

    expect(requestOutcome(projectedFromResponseView)).toBe("FAILED");
    expect(requestOutcome(projectedFromScript)).toBe("TIMED_OUT");
    expect(requestOutcome(projectedFromTopLevelScript)).toBe("FAILED_EXIT_CODE");
    expect(requestOutcome(runtime)).toBe("ACCEPTED");

    expect(requestFailureReason(projectedFromResponseView)).toBe("response_view_failed");
    expect(
      requestFailureMessage(projectedFromResponseView),
    ).toBe("Dispatch failed after response view fallback.");
    expect(requestFailureReason(runtime)).toBe("runtime_failed");
    expect(requestFailureMessage(runtime)).toBe("Runtime failure message");

    expect(requestScriptRequest(projectedFromScript)?.script_request_id).toBe(
      "script-request-top-level",
    );
    expect(
      requestScriptRequest(
        buildProjectedRequest({
          request_view: {
            input_work_items: [inputWorkItem],
            script_request: {
              args: ["--fallback"],
              attempt: 6,
              command: "script-fallback",
              script_request_id: "script-request-view",
            },
            started_at: "2026-04-08T12:00:00Z",
            trace_ids: ["trace-request-view"],
          },
          script_request: undefined,
        }),
      )?.script_request_id,
    ).toBe("script-request-view");
    expect(requestScriptRequest(runtime)?.script_request_id).toBe(
      "runtime-script-request",
    );

    expect(requestScriptResponse(projectedFromTopLevelScript)?.script_request_id).toBe(
      "script-response-top-level",
    );
    expect(requestScriptResponse(projectedFromScript)?.script_request_id).toBe(
      "script-response-view",
    );
    expect(requestScriptResponse(runtime)?.script_request_id).toBe(
      "runtime-script-request",
    );

    expect(scriptAttemptNumber(projectedFromScript.script_request)).toBe(2);
    expect(scriptRequestID(projectedFromScript.script_request)).toBe(
      "script-request-top-level",
    );
    expect(
      scriptRequestID({
        attempt: 1,
        command: "legacy-script",
        scriptRequestId: "legacy-script-id",
      }),
    ).toBe("legacy-script-id");
    expect(
      scriptResponseDurationMillis(projectedFromTopLevelScript.script_response),
    ).toBe(654);
    expect(
      scriptResponseDurationMillis({
        attempt: 1,
        durationMillis: 77,
        outcome: "SUCCEEDED",
        scriptRequestId: "legacy-script-id",
      }),
    ).toBe(77);
    expect(scriptResponseExitCode(projectedFromTopLevelScript.script_response)).toBe(9);
    expect(
      scriptResponseExitCode({
        attempt: 1,
        exitCode: 3,
        outcome: "FAILED_EXIT_CODE",
        scriptRequestId: "legacy-script-id",
      }),
    ).toBe(3);
    expect(
      scriptResponseFailureType(projectedFromTopLevelScript.script_response),
    ).toBe("PROCESS_ERROR");
    expect(
      scriptResponseFailureType({
        attempt: 1,
        failureType: "TIMEOUT",
        outcome: "TIMED_OUT",
        scriptRequestId: "legacy-script-id",
      }),
    ).toBe("TIMEOUT");
  });

  it("sorts inference attempts and deduplicates work items by work id", () => {
    const projected = buildProjectedRequest({
      inference_attempts: [
        {
          attempt: 2,
          inference_request_id: "dispatch-projected/inference-request/2",
        },
        {
          attempt: 1,
          inference_request_id: "dispatch-projected/inference-request/b",
        },
        {
          attempt: 1,
          inference_request_id: "dispatch-projected/inference-request/a",
        },
      ],
    });

    expect(
      requestInferenceAttempts(projected).map((attempt) => attempt.inference_request_id),
    ).toEqual([
      "dispatch-projected/inference-request/a",
      "dispatch-projected/inference-request/b",
      "dispatch-projected/inference-request/2",
    ]);
    expect(requestInferenceAttempts(buildRuntimeRequest())).toEqual([]);

    expect(
      dedupeWorkItems([
        inputWorkItem,
        { ...inputWorkItem, display_name: "Input Story Replacement" },
        outputWorkItem,
      ]),
    ).toEqual([
      { ...inputWorkItem, display_name: "Input Story Replacement" },
      outputWorkItem,
    ]);
  });

  it("reports whether a request has any response details on the surviving owner surface", () => {
    const emptyProjected = buildProjectedRequest({
      outcome: undefined,
      provider_session: undefined,
      response: undefined,
      response_view: undefined,
      script_response: undefined,
    });
    const projectedWithResponseText = buildProjectedRequest({
      outcome: undefined,
      provider_session: undefined,
      response: "Projected response text",
      response_view: undefined,
      script_response: undefined,
    });
    const projectedWithProviderSession = buildProjectedRequest({
      outcome: undefined,
      provider_session: {
        id: "session-2",
        kind: "session_id",
        provider: "codex",
      },
      response: undefined,
      response_view: undefined,
      script_response: undefined,
    });
    const runtimeWithResponse = buildRuntimeRequest({
      response: {
        output_work_items: [outputWorkItem],
      },
    });

    expect(hasResponseDetails(emptyProjected as SelectedWorkRequestHistoryItem)).toBe(false);
    expect(
      hasResponseDetails(projectedWithResponseText as SelectedWorkRequestHistoryItem),
    ).toBe(true);
    expect(
      hasResponseDetails(projectedWithProviderSession as SelectedWorkRequestHistoryItem),
    ).toBe(true);
    expect(hasResponseDetails(runtimeWithResponse as SelectedWorkRequestHistoryItem)).toBe(
      true,
    );
  });
});
