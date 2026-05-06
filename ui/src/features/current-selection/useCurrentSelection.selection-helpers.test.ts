import { describe, expect, it } from "vitest";

import type {
  DashboardActiveExecution,
  DashboardFailedWorkDetail,
  DashboardPlaceRef,
  DashboardProviderSessionAttempt,
  DashboardSnapshot,
  DashboardWorkItemRef,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";
import { buildEmptyDashboardRuntimeFixture } from "../../components/dashboard/fixtures/runtime";
import {
  activeExecutionsForSelectedWorkstation,
  buildTerminalWorkItems,
  currentWorkItemsForPlace,
  findStatePlace,
  findTerminalWorkItem,
  inferStateWorkTerminalStatus,
  placeNodeID,
  resolveTrackedWorkSelection,
  terminalHistoryItemsForPlace,
} from "./useCurrentSelection.selection-helpers";

const reviewInputPlace: DashboardPlaceRef = {
  kind: "work_state",
  place_id: "story:new",
  state_category: "INITIAL",
  state_name: "new",
  work_type_name: "story",
};

const reviewOutputPlace: DashboardPlaceRef = {
  kind: "work_state",
  place_id: "story:done",
  state_category: "TERMINAL",
  state_name: "done",
  work_type_name: "story",
};

const failedPlace: DashboardPlaceRef = {
  kind: "work_state",
  place_id: "story:failed",
  state_category: "FAILED",
  state_name: "failed",
  work_type_name: "story",
};

const workAlpha: DashboardWorkItemRef = {
  display_name: "Alpha Story",
  trace_id: "trace-alpha",
  work_id: "work-alpha",
  work_type_id: "story",
};

const workBeta: DashboardWorkItemRef = {
  display_name: "Beta Story",
  trace_id: "trace-beta",
  work_id: "work-beta",
  work_type_id: "story",
};

function buildSnapshot(): DashboardSnapshot {
  const runtime = buildEmptyDashboardRuntimeFixture();

  return {
    runtime: {
      ...runtime,
      active_executions_by_dispatch_id: {},
      current_work_items_by_place_id: {},
      place_occupancy_work_items_by_place_id: {},
      session: {
        ...runtime.session,
        completed_work_labels: [],
        failed_work_details_by_work_id: {},
        failed_work_labels: [],
        provider_sessions: [],
      },
      workstation_requests_by_dispatch_id: {},
    },
    topology: {
      edges: [],
      workstation_node_ids: ["review", "repair"],
      workstation_nodes_by_id: {
        repair: {
          input_places: [reviewOutputPlace],
          node_id: "repair",
          output_places: [failedPlace],
          transition_id: "repair",
          workstation_name: "Repair",
        },
        review: {
          input_places: [reviewInputPlace],
          node_id: "review",
          output_places: [reviewOutputPlace],
          transition_id: "review",
          workstation_name: "Review",
        },
      },
    },
  };
}

describe("useCurrentSelection.selection-helpers", () => {
  it("builds terminal work items and basic place selectors from attempts and failure details", () => {
    const attempt: DashboardProviderSessionAttempt = {
      dispatch_id: "dispatch-review",
      failure_message: "Attempt failure",
      failure_reason: "attempt_failed",
      outcome: "FAILED",
      transition_id: "review",
      work_items: [workAlpha],
      workstation_name: "Review",
    };
    const failureDetail: DashboardFailedWorkDetail = {
      dispatch_id: "dispatch-review",
      failure_message: "Failed after dispatch",
      failure_reason: "dispatch_failed",
      transition_id: "review",
      work_item: workAlpha,
      workstation_name: "Review",
    };
    const snapshot = buildSnapshot();
    snapshot.runtime.current_work_items_by_place_id = {
      [reviewInputPlace.place_id]: [workAlpha],
    };
    snapshot.runtime.place_occupancy_work_items_by_place_id = {
      [reviewOutputPlace.place_id]: [workBeta],
    };

    expect(
      buildTerminalWorkItems(["Alpha Story"], [attempt], {
        [workAlpha.work_id]: failureDetail,
      }),
    ).toEqual([
      {
        attempts: [attempt],
        failureMessage: "Failed after dispatch",
        failureReason: "dispatch_failed",
        label: "Alpha Story",
        traceWorkID: workAlpha.work_id,
        workItem: workAlpha,
      },
    ]);
    expect(findStatePlace(snapshot, reviewInputPlace.place_id)).toEqual(reviewInputPlace);
    expect(findStatePlace(snapshot, "missing-place")).toBeNull();
    expect(currentWorkItemsForPlace(snapshot, reviewInputPlace.place_id)).toEqual([workAlpha]);
    expect(terminalHistoryItemsForPlace(snapshot, reviewOutputPlace.place_id)).toEqual([
      workBeta,
    ]);
    expect(currentWorkItemsForPlace(null, reviewInputPlace.place_id)).toEqual([]);
    expect(terminalHistoryItemsForPlace(snapshot, undefined)).toEqual([]);
    expect(placeNodeID(snapshot, reviewInputPlace)).toBe("review");
    expect(placeNodeID(null, reviewInputPlace)).toBeUndefined();
  });

  it("filters active executions for the selected workstation and infers terminal work status", () => {
    const snapshot = buildSnapshot();
    const matchingExecution: DashboardActiveExecution = {
      dispatch_id: "dispatch-review",
      started_at: "2026-04-08T12:00:00Z",
      transition_id: "review",
      work_items: [workAlpha],
      workstation_name: "Review",
      workstation_node_id: "review",
    };
    const nonMatchingExecution: DashboardActiveExecution = {
      dispatch_id: "dispatch-other",
      started_at: "2026-04-08T12:01:00Z",
      transition_id: "other",
      work_items: [workBeta],
      workstation_name: "Other",
      workstation_node_id: "other",
    };
    snapshot.runtime.active_executions_by_dispatch_id = {
      [matchingExecution.dispatch_id]: matchingExecution,
      [nonMatchingExecution.dispatch_id]: nonMatchingExecution,
    };
    snapshot.runtime.session.completed_work_labels = ["Beta Story"];
    snapshot.runtime.session.failed_work_labels = ["work-alpha"];

    expect(
      activeExecutionsForSelectedWorkstation(
        snapshot,
        { kind: "node", nodeId: "review" },
        snapshot.topology.workstation_nodes_by_id.review,
      ),
    ).toEqual([matchingExecution]);
    expect(
      activeExecutionsForSelectedWorkstation(
        snapshot,
        { kind: "work-item", nodeId: "review", workItem: workAlpha },
        snapshot.topology.workstation_nodes_by_id.review,
      ),
    ).toEqual([]);

    expect(inferStateWorkTerminalStatus(snapshot, reviewOutputPlace, workAlpha)).toBe("failed");
    expect(inferStateWorkTerminalStatus(snapshot, reviewOutputPlace, workBeta)).toBe("completed");
    expect(inferStateWorkTerminalStatus(snapshot, failedPlace, {
      ...workBeta,
      display_name: "Gamma Story",
      work_id: "work-gamma",
    })).toBe("failed");
    expect(inferStateWorkTerminalStatus(null, reviewOutputPlace, workAlpha)).toBeNull();
  });

  it("resolves tracked work selections across script-backed, active, provider, failed, retained, and fallback paths", () => {
    const snapshot = buildSnapshot();
    const scriptRequest: DashboardWorkstationRequest = {
      counts: {
        dispatched_count: 1,
        errored_count: 0,
        responded_count: 1,
      },
      dispatch_id: "dispatch-script",
      dispatched_request_count: 1,
      errored_request_count: 0,
      inference_attempts: [],
      request_view: {
        input_work_items: [workAlpha],
        started_at: "2026-04-08T12:00:00Z",
        trace_ids: ["trace-alpha"],
      },
      responded_request_count: 1,
      script_request: {
        args: ["--work", workAlpha.work_id],
        attempt: 1,
        command: "script-tool",
        script_request_id: "dispatch-script/script-request/1",
      },
      started_at: "2026-04-08T12:00:00Z",
      transition_id: "review",
      work_items: [workAlpha],
      workstation_name: "Review",
      workstation_node_id: "review",
    };
    snapshot.runtime.workstation_requests_by_dispatch_id = {
      [scriptRequest.dispatch_id]: scriptRequest,
    };

    expect(
      resolveTrackedWorkSelection({
        snapshot,
        workID: workAlpha.work_id,
      }),
    ).toEqual({
      dispatchId: "dispatch-script",
      kind: "workstation-request",
      nodeId: "review",
      request: scriptRequest,
    });

    snapshot.runtime.workstation_requests_by_dispatch_id = {};
    const activeExecution: DashboardActiveExecution = {
      dispatch_id: "dispatch-active",
      started_at: "2026-04-08T12:00:02Z",
      transition_id: "review",
      work_items: [workAlpha],
      workstation_name: "Review",
      workstation_node_id: "review",
    };
    snapshot.runtime.active_executions_by_dispatch_id = {
      [activeExecution.dispatch_id]: activeExecution,
    };
    expect(
      resolveTrackedWorkSelection({
        snapshot,
        workID: workAlpha.work_id,
      }),
    ).toEqual({
      dispatchId: "dispatch-active",
      execution: activeExecution,
      kind: "work-item",
      nodeId: "review",
      workItem: workAlpha,
    });

    snapshot.runtime.active_executions_by_dispatch_id = {};
    snapshot.runtime.session.provider_sessions = [
      {
        dispatch_id: "dispatch-provider",
        transition_id: "review",
        work_items: [workAlpha],
        workstation_name: "Review",
      },
    ];
    expect(
      resolveTrackedWorkSelection({
        snapshot,
        workID: workAlpha.work_id,
      }),
    ).toEqual({
      dispatchId: "dispatch-provider",
      kind: "work-item",
      nodeId: "review",
      workItem: workAlpha,
    });

    snapshot.runtime.session.provider_sessions = [];
    snapshot.runtime.session.failed_work_details_by_work_id = {
      [workAlpha.work_id]: {
        dispatch_id: "dispatch-failed",
        failure_message: "Failed",
        failure_reason: "failed",
        transition_id: "review",
        work_item: workAlpha,
        workstation_name: "Review",
      },
    };
    expect(
      resolveTrackedWorkSelection({
        snapshot,
        workID: workAlpha.work_id,
      }),
    ).toEqual({
      dispatchId: "dispatch-failed",
      kind: "work-item",
      nodeId: "review",
      workItem: workAlpha,
    });

    snapshot.runtime.session.failed_work_details_by_work_id = {};
    snapshot.runtime.current_work_items_by_place_id = {
      [reviewInputPlace.place_id]: [workAlpha],
    };
    expect(
      resolveTrackedWorkSelection({
        snapshot,
        workID: workAlpha.work_id,
      }),
    ).toEqual({
      kind: "work-item",
      nodeId: "review",
      workItem: workAlpha,
    });

    snapshot.runtime.current_work_items_by_place_id = {};
    expect(
      resolveTrackedWorkSelection({
        nodeID: "repair",
        snapshot,
        terminalWorkDetail: {
          label: "Alpha Story",
          workItem: workAlpha,
        },
        workID: workAlpha.work_id,
      }),
    ).toEqual({
      kind: "work-item",
      nodeId: "repair",
      workItem: workAlpha,
    });

    expect(
      resolveTrackedWorkSelection({
        snapshot,
        workID: "missing-work",
      }),
    ).toBeNull();
  });

  it("finds terminal work items by trace id, work item id, or label", () => {
    const terminalItem = {
      attempts: [],
      label: "Alpha Story",
      traceWorkID: workAlpha.work_id,
      workItem: workAlpha,
    };

    expect(findTerminalWorkItem([terminalItem], workAlpha)).toBe(terminalItem);
    expect(
      findTerminalWorkItem(
        [{ ...terminalItem, traceWorkID: "other-work", workItem: undefined }],
        workAlpha,
      ),
    ).toEqual({ ...terminalItem, traceWorkID: "other-work", workItem: undefined });
  });
});
