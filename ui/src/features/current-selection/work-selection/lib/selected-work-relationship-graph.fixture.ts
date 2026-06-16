import type {
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../../../api/dashboard/types";

export const selectedWorkItem: DashboardWorkItemRef = {
  display_name: "Active Story",
  state: "in_progress",
  trace_id: "trace-active-story",
  work_id: "work-active-story",
  work_type_id: "story",
};

export function snapshotFixture(): DashboardSnapshot & {
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
      "work-grandchild-story": [
        {
          source_work_id: "work-child-story",
          sourceWorkName: "Child Story",
          targetWorkId: "work-grandchild-story",
          targetWorkName: "Grandchild Story",
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
            {
              display_name: "Grandchild Story",
              state: "queued",
              trace_id: "trace-grandchild-story",
              work_id: "work-grandchild-story",
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
