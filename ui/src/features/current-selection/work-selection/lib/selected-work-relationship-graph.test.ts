import { describe, expect, it } from "vitest";

import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../../../api/dashboard/types";
import { buildSelectedWorkRelationshipGraph } from "./selected-work-relationship-graph";

const selectedWorkItem: DashboardWorkItemRef = {
  display_name: "Active Story",
  state: "in_progress",
  trace_id: "trace-active-story",
  work_id: "work-active-story",
  work_type_id: "story",
};

function snapshotFixture(): DashboardSnapshot & {
  relationsByWorkID: Record<string, Array<Record<string, string>>>;
} {
  return {
    factory_state: "RUNNING",
    relationsByWorkID: {
      "work-active-story": [
        {
          source_work_id: "work-active-story",
          sourceWorkName: "Active Story",
          targetWorkId: "work-dependency-story",
          targetWorkName: "Dependency Story",
          type: "DEPENDS_ON",
          requiredState: "ready",
        },
        {
          source_work_id: "work-active-story",
          sourceWorkName: "Active Story",
          targetWorkId: "work-parent-story",
          targetWorkName: "Parent Story",
          type: "PARENT_CHILD",
        },
      ],
      "work-blocked-story": [
        {
          source_work_id: "work-blocked-story",
          sourceWorkName: "Blocked Story",
          targetWorkId: "work-active-story",
          targetWorkName: "Active Story",
          type: "DEPENDS_ON",
          requiredState: "approved",
        },
      ],
      "work-child-story": [
        {
          source_work_id: "work-child-story",
          sourceWorkName: "Child Story",
          targetWorkId: "work-active-story",
          targetWorkName: "Active Story",
          type: "PARENT_CHILD",
        },
      ],
    },
    runtime: {
      active_executions_by_dispatch_id: {
        "dispatch-active-story": {
          dispatch_id: "dispatch-active-story",
          started_at: "2026-05-26T10:00:00Z",
          transition_id: "transition-story",
          work_items: [
            selectedWorkItem,
            {
              display_name: "Dependency Story",
              state: "ready",
              trace_id: "trace-dependency-story",
              work_id: "work-dependency-story",
              work_type_id: "story",
            },
            {
              display_name: "Parent Story",
              state: "queued",
              trace_id: "trace-parent-story",
              work_id: "work-parent-story",
              work_type_id: "epic",
            },
            {
              display_name: "Blocked Story",
              state: "blocked",
              trace_id: "trace-blocked-story",
              work_id: "work-blocked-story",
              work_type_id: "story",
            },
            {
              display_name: "Child Story",
              state: "queued",
              trace_id: "trace-child-story",
              work_id: "work-child-story",
              work_type_id: "task",
            },
          ],
          workstation_node_id: "transition-story",
        },
      },
      in_flight_dispatch_count: 1,
      session: {
        completed_count: 0,
        dispatched_count: 1,
        failed_count: 0,
        has_data: true,
      },
    },
    tick_count: 4,
    topology: {
      edges: [],
      workstation_node_ids: [],
      workstation_nodes_by_id: {},
    },
    uptime_seconds: 42,
  };
}

describe("buildSelectedWorkRelationshipGraph", () => {
  it("builds a typed first-degree graph around the selected work item", () => {
    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot: snapshotFixture(),
    });

    expect(graph.status).toBe("ready");
    if (graph.status !== "ready") {
      throw new Error(`expected ready graph, got ${graph.status}`);
    }

    expect(graph.selectedWork).toMatchObject({
      label: "Active Story",
      state: "in_progress",
      traceID: "trace-active-story",
      workID: "work-active-story",
      workTypeID: "story",
    });
    expect(graph.relatedWork.map((node) => node.label)).toEqual([
      "Blocked Story",
      "Child Story",
      "Dependency Story",
      "Parent Story",
    ]);
    expect(graph.edges).toEqual([
      {
        relationship: "CHILD",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-child-story",
      },
      {
        relationship: "DEPENDS_ON",
        requiredState: "ready",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-dependency-story",
      },
      {
        relationship: "PARENT",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-parent-story",
      },
      {
        relationship: "REQUIRED_BY",
        requiredState: "approved",
        sourceWorkID: "work-active-story",
        targetWorkID: "work-blocked-story",
      },
    ]);
  });

  it("returns an explicit empty graph when no supported relationships exist", () => {
    const snapshot = snapshotFixture();
    snapshot.relationsByWorkID = {};

    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot,
    });

    expect(graph).toMatchObject({
      edges: [],
      relatedWork: [],
      selectedWork: {
        label: "Active Story",
        workID: "work-active-story",
      },
      status: "empty",
    });
  });

  it("returns an explicit error state when relationship data is unavailable", () => {
    const { relationsByWorkID: _relationsByWorkID, ...snapshot } =
      snapshotFixture();

    const graph = buildSelectedWorkRelationshipGraph({
      selectedWorkItem,
      snapshot,
    });

    expect(graph.status).toBe("error");
    if (graph.status !== "error") {
      throw new Error(`expected error graph, got ${graph.status}`);
    }
    expect(graph.message).toContain("unavailable");
  });
});
