import type { DashboardSnapshot } from "../api/dashboard";
import {
  buildDashboardSnapshotFixture,
  mediumBranchingDashboardTopology,
} from "../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../components/dashboard/test-fixtures";

const terminalBaseSnapshot = semanticWorkflowDashboardSnapshot;

export const baselineSnapshot = buildDashboardSnapshotFixture(
  mediumBranchingDashboardTopology,
);

export const activeSnapshot = semanticWorkflowDashboardSnapshot;

export const terminalSnapshot = {
  ...terminalBaseSnapshot,
  tick_count: 4,
  runtime: {
    ...terminalBaseSnapshot.runtime,
    place_occupancy_work_items_by_place_id: {
      ...(terminalBaseSnapshot.runtime.place_occupancy_work_items_by_place_id ??
        {}),
      "story:blocked": [
        {
          display_name: "Failed Story",
          trace_id: "trace-failed-story",
          work_id: "work-failed-story",
          work_type_id: "story",
        },
      ],
      "story:complete": [
        {
          display_name: "Done Story",
          trace_id: "trace-done-story",
          work_id: "work-complete",
          work_type_id: "story",
        },
      ],
    },
    place_token_counts: {
      ...(terminalBaseSnapshot.runtime.place_token_counts ?? {}),
      "story:blocked": 1,
      "story:complete": 1,
    },
    session: {
      ...terminalBaseSnapshot.runtime.session,
      completed_count: 1,
      completed_work_labels: ["Done Story"],
      provider_sessions: [
        ...(terminalBaseSnapshot.runtime.session.provider_sessions ?? []),
        {
          dispatch_id: "dispatch-complete",
          outcome: "ACCEPTED",
          provider_session: {
            id: "sess-done-story",
            kind: "session_id",
            provider: "codex",
          },
          transition_id: "complete",
          workstation_name: "Complete",
          work_items: [
            {
              display_name: "Done Story",
              trace_id: "trace-done-story",
              work_id: "work-complete",
              work_type_id: "story",
            },
          ],
        },
      ],
    },
  },
} satisfies DashboardSnapshot;

export const importedFactorySnapshot = (() => {
  const snapshot = structuredClone(semanticWorkflowDashboardSnapshot);

  snapshot.factory_state = "Imported factory active";
  snapshot.tick_count = semanticWorkflowDashboardSnapshot.tick_count + 1;

  return snapshot;
})();
