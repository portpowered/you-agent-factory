import { describe, expect, it } from "vitest";

import type {
  DashboardProviderSessionAttempt,
  DashboardRuntimeWorkstationRequest,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";
import { buildEmptyDashboardRuntimeFixture } from "../../components/dashboard/fixtures/runtime";
import {
  buildSelectedWorkDispatchAttempts,
  filterProviderSessionAttempts,
  requestDispatchID,
  requestWorkItems,
  resolveProjectedWorkstationRequestsByDispatchID,
  selectLatestProviderSessionAttemptsByDispatch,
  selectWorkstationRequestsForWork,
  sortWorkstationRequests,
  toDashboardWorkstationRequest,
} from "./useCurrentSelection.request-helpers";

const workAlpha: DashboardWorkItemRef = {
  display_name: "Alpha",
  trace_id: "trace-alpha",
  work_id: "work-alpha",
  work_type_id: "story",
};

const workBeta: DashboardWorkItemRef = {
  display_name: "Beta",
  trace_id: "trace-beta",
  work_id: "work-beta",
  work_type_id: "story",
};

function buildRuntimeRequest(
  dispatchID: string,
  overrides: Partial<DashboardRuntimeWorkstationRequest> = {},
): DashboardRuntimeWorkstationRequest {
  return {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 1,
    },
    dispatch_id: dispatchID,
    request: {
      input_work_items: [workAlpha],
      script_request: {
        args: ["--work", workAlpha.work_id],
        attempt: 1,
        command: "script-tool",
        script_request_id: `${dispatchID}/script-request/1`,
      },
      started_at: "2026-04-08T12:00:00Z",
      trace_ids: ["trace-alpha"],
    },
    response: {
      duration_millis: 640,
      failure_message: "Runtime failure message",
      failure_reason: "runtime_failed",
      outcome: "FAILED",
      output_work_items: [workBeta],
      script_response: {
        attempt: 1,
        duration_millis: 640,
        outcome: "SUCCEEDED",
        script_request_id: `${dispatchID}/script-request/1`,
      },
    },
    transition_id: "review",
    workstation_name: "Review",
    ...overrides,
  };
}

function buildProjectedRequest(
  dispatchID: string,
  overrides: Partial<DashboardWorkstationRequest> = {},
): DashboardWorkstationRequest {
  return {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 1,
    },
    dispatch_id: dispatchID,
    dispatched_request_count: 1,
    errored_request_count: 0,
    inference_attempts: [],
    request_view: {
      input_work_items: [workAlpha],
      started_at: "2026-04-08T12:00:01Z",
      trace_ids: ["trace-alpha"],
    },
    responded_request_count: 1,
    response_view: {
      output_work_items: [workBeta],
    },
    started_at: "2026-04-08T12:00:01Z",
    transition_id: "review",
    work_items: [workAlpha, workBeta],
    workstation_name: "Review",
    workstation_node_id: "review",
    ...overrides,
  };
}

function buildAttempt(
  dispatchID: string,
  overrides: Partial<DashboardProviderSessionAttempt> = {},
): DashboardProviderSessionAttempt {
  return {
    dispatch_id: dispatchID,
    outcome: "ACCEPTED",
    provider_session: {
      id: `${dispatchID}-session`,
      kind: "session_id",
      provider: "codex",
    },
    transition_id: "review",
    work_items: [workAlpha],
    workstation_name: "Review",
    ...overrides,
  };
}

describe("useCurrentSelection.request-helpers", () => {
  it("resolves projected requests from explicit maps or runtime snapshots", () => {
    const explicitProjected = {
      "dispatch-projected": buildProjectedRequest("dispatch-projected"),
    };
    const snapshot: DashboardSnapshot = {
      ...buildEmptyDashboardRuntimeFixture(),
      runtime: {
        ...buildEmptyDashboardRuntimeFixture(),
        workstation_requests_by_dispatch_id: {
          "dispatch-runtime": buildRuntimeRequest("dispatch-runtime"),
        },
      },
    } as DashboardSnapshot;

    expect(
      resolveProjectedWorkstationRequestsByDispatchID(snapshot, explicitProjected),
    ).toBe(explicitProjected);
    expect(
      resolveProjectedWorkstationRequestsByDispatchID(snapshot, undefined),
    ).toEqual({
      "dispatch-runtime": toDashboardWorkstationRequest(
        snapshot.runtime.workstation_requests_by_dispatch_id["dispatch-runtime"],
      ),
    });
    expect(
      resolveProjectedWorkstationRequestsByDispatchID(
        { ...snapshot, runtime: { ...snapshot.runtime, workstation_requests_by_dispatch_id: undefined } },
        undefined,
      ),
    ).toBeUndefined();
  });

  it("filters provider-session attempts and selects the latest session per dispatch in request order", () => {
    const attempts = [
      buildAttempt("dispatch-older", {
        provider_session: {
          id: "older-session",
          kind: "session_id",
          provider: "codex",
        },
      }),
      buildAttempt("dispatch-latest", {
        provider_session: {
          id: "latest-session",
          kind: "session_id",
          provider: "codex",
        },
      }),
      buildAttempt("dispatch-latest", {
        provider_session: {
          id: "latest-session-newer",
          kind: "session_id",
          provider: "codex",
        },
      }),
      buildAttempt("dispatch-missing", {
        provider_session: undefined,
      }),
    ];
    const requests = [
      buildProjectedRequest("dispatch-latest"),
      buildRuntimeRequest("dispatch-older"),
    ];

    expect(
      filterProviderSessionAttempts(attempts, (attempt) =>
        attempt.dispatch_id.startsWith("dispatch-l"),
      ).map((attempt) => attempt.dispatch_id),
    ).toEqual(["dispatch-latest", "dispatch-latest"]);
    expect(filterProviderSessionAttempts(undefined, () => true)).toEqual([]);

    expect(
      selectLatestProviderSessionAttemptsByDispatch(attempts, requests).map(
        (attempt) => attempt.provider_session?.id,
      ),
    ).toEqual(["latest-session-newer", "older-session"]);
  });

  it("sorts and selects workstation requests by started time and related work items", () => {
    const requests = {
      "dispatch-newer": buildProjectedRequest("dispatch-newer", {
        started_at: "2026-04-08T12:00:03Z",
        work_items: [workBeta],
      }),
      "dispatch-older": buildRuntimeRequest("dispatch-older", {
        request: {
          input_work_items: [workAlpha],
          started_at: "2026-04-08T12:00:01Z",
          trace_ids: ["trace-alpha"],
        },
      }),
      "dispatch-same-time": buildProjectedRequest("dispatch-same-time", {
        started_at: "2026-04-08T12:00:03Z",
      }),
    };

    expect(
      sortWorkstationRequests(Object.values(requests)).map((request) =>
        requestDispatchID(request),
      ),
    ).toEqual(["dispatch-newer", "dispatch-same-time", "dispatch-older"]);
    expect(
      selectWorkstationRequestsForWork(requests, workBeta.work_id).map((request) =>
        requestDispatchID(request),
      ),
    ).toEqual(["dispatch-newer", "dispatch-same-time", "dispatch-older"]);
    expect(selectWorkstationRequestsForWork(undefined, workAlpha.work_id)).toEqual([]);
  });

  it("derives selected-work dispatch attempts from requests and merges them with provider attempts", () => {
    const attempts = [
      buildAttempt("dispatch-runtime", {
        failure_message: "Existing provider failure",
        provider_session: {
          id: "runtime-session",
          kind: "session_id",
          provider: "codex",
        },
        workstation_name: "Review Existing",
      }),
    ];
    const workstationRequestsByDispatchID = {
      "dispatch-runtime": buildRuntimeRequest("dispatch-runtime"),
      "dispatch-projected": buildProjectedRequest("dispatch-projected", {
        failure_message: "Projected failure",
        failure_reason: "projected_failed",
        outcome: "FAILED",
        provider_session: {
          id: "projected-session",
          kind: "session_id",
          provider: "codex",
        },
      }),
    };

    expect(
      buildSelectedWorkDispatchAttempts({
        attempts,
        workID: workAlpha.work_id,
        workstationRequestsByDispatchID,
      }),
    ).toEqual([
      {
        diagnostics: undefined,
        dispatch_id: "dispatch-projected",
        failure_message: "Projected failure",
        failure_reason: "projected_failed",
        outcome: "FAILED",
        provider_session: {
          id: "projected-session",
          kind: "session_id",
          provider: "codex",
        },
        transition_id: "review",
        work_items: [workAlpha, workBeta],
        workstation_name: "Review",
      },
      {
        diagnostics: undefined,
        dispatch_id: "dispatch-runtime",
        failure_message: "Existing provider failure",
        failure_reason: "runtime_failed",
        outcome: "ACCEPTED",
        provider_session: {
          id: "runtime-session",
          kind: "session_id",
          provider: "codex",
        },
        transition_id: "review",
        work_items: [workAlpha],
        workstation_name: "Review Existing",
      },
    ]);

    expect(
      buildSelectedWorkDispatchAttempts({
        attempts: undefined,
        workID: workBeta.work_id,
        workstationRequestsByDispatchID: undefined,
      }),
    ).toEqual([]);
  });

  it("converts runtime requests to projected requests and exposes request-owned work items", () => {
    const runtime = buildRuntimeRequest("dispatch-runtime");
    const camelCaseRuntime = buildRuntimeRequest("dispatch-runtime-camel", {
      counts: {
        dispatchedCount: 2,
        erroredCount: 1,
        respondedCount: 1,
      },
      dispatch_id: undefined,
      dispatchId: "dispatch-runtime-camel-id",
      request: {
        inputWorkItems: [workAlpha],
        scriptRequest: {
          args: ["--camel"],
          attempt: 2,
          command: "script-camel",
          scriptRequestId: "dispatch-runtime-camel/script-request/2",
        },
        startedAt: "2026-04-08T12:00:02Z",
        traceIds: ["trace-camel"],
      },
      response: {
        durationMillis: 222,
        failureMessage: "Camel failure message",
        failureReason: "camel_failed",
        outcome: "FAILED",
        outputWorkItems: [workBeta],
        scriptResponse: {
          attempt: 2,
          durationMillis: 222,
          outcome: "SUCCEEDED",
          scriptRequestId: "dispatch-runtime-camel/script-request/2",
        },
      },
      transition_id: undefined,
      transitionId: "repair",
      workstation_name: undefined,
      workstationName: "Repair",
    });

    expect(requestWorkItems(runtime)).toEqual([workAlpha, workBeta]);
    expect(requestWorkItems(buildProjectedRequest("dispatch-projected"))).toEqual([
      workAlpha,
      workBeta,
    ]);
    expect(requestDispatchID(runtime)).toBe("dispatch-runtime");
    expect(
      requestDispatchID({
        ...buildRuntimeRequest("dispatch-legacy"),
        dispatch_id: undefined,
        dispatchId: "dispatch-legacy-id",
      }),
    ).toBe("dispatch-legacy-id");

    expect(toDashboardWorkstationRequest(runtime)).toEqual({
      dispatch_id: "dispatch-runtime",
      dispatched_request_count: 1,
      errored_request_count: 0,
      failure_message: "Runtime failure message",
      failure_reason: "runtime_failed",
      inference_attempts: [],
      outcome: "FAILED",
      responded_request_count: 1,
      script_request: {
        args: ["--work", workAlpha.work_id],
        attempt: 1,
        command: "script-tool",
        script_request_id: "dispatch-runtime/script-request/1",
      },
      script_response: {
        attempt: 1,
        duration_millis: 640,
        outcome: "SUCCEEDED",
        script_request_id: "dispatch-runtime/script-request/1",
      },
      started_at: "2026-04-08T12:00:00Z",
      total_duration_millis: 640,
      trace_ids: ["trace-alpha"],
      transition_id: "review",
      work_items: [workAlpha, workBeta],
      workstation_name: "Review",
      workstation_node_id: "review",
    });
    expect(toDashboardWorkstationRequest(camelCaseRuntime)).toEqual({
      dispatch_id: "dispatch-runtime-camel-id",
      dispatched_request_count: 2,
      errored_request_count: 1,
      failure_message: "Camel failure message",
      failure_reason: "camel_failed",
      inference_attempts: [],
      outcome: "FAILED",
      responded_request_count: 1,
      script_request: {
        args: ["--camel"],
        attempt: 2,
        command: "script-camel",
        scriptRequestId: "dispatch-runtime-camel/script-request/2",
      },
      script_response: {
        attempt: 2,
        durationMillis: 222,
        outcome: "SUCCEEDED",
        scriptRequestId: "dispatch-runtime-camel/script-request/2",
      },
      started_at: "2026-04-08T12:00:02Z",
      total_duration_millis: 222,
      trace_ids: ["trace-camel"],
      transition_id: "repair",
      work_items: [workAlpha, workBeta],
      workstation_name: "Repair",
      workstation_node_id: "repair",
    });
    expect(
      toDashboardWorkstationRequest(buildProjectedRequest("dispatch-projected")),
    ).toEqual(buildProjectedRequest("dispatch-projected"));
  });
});
