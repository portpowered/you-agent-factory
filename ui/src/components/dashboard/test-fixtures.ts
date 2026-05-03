import type { DashboardSnapshot } from "../../api/dashboard/types";
import type { WorkstationIconMetadata } from "../../features/flowchart";
import { workstationIconMetadata } from "../../features/flowchart";
import { buildFactoryTimelineSnapshot } from "../../features/timeline/state/factoryTimelineStore";
import {
  activeWorkRuntimeOverlay,
  buildDashboardSnapshotFixture,
  buildEmptyDashboardRuntimeFixture,
  failedOutcomeRuntimeOverlay,
  rejectedOutcomeRuntimeOverlay,
  retryAttemptRuntimeOverlay,
} from "./fixtures/runtime";
import { resourceCountTimelineEvents } from "./fixtures/resource-count-events";
import {
  mediumBranchingDashboardTopology,
  oneNodeDashboardTopology,
  twentyNodeDashboardTopology as twentyNodeTopologyFixture,
  workstationKindParityDashboardTopology,
} from "./fixtures/topologies";

export interface WorkstationKindParityExpectation {
  buttonName: string;
  metadata: WorkstationIconMetadata;
  nodeID: string;
  workstationName: string;
}

// Storybook and Vitest own this scenario catalog. Keep it out of production runtime
// imports and the dashboard barrel so representative operator data never becomes
// reachable through the app bundle.
export const singleNodeDashboardSnapshot: DashboardSnapshot =
  buildDashboardSnapshotFixture(oneNodeDashboardTopology);

export const semanticWorkflowDashboardSnapshot: DashboardSnapshot =
  buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
    activeWorkRuntimeOverlay,
    retryAttemptRuntimeOverlay,
    failedOutcomeRuntimeOverlay,
    rejectedOutcomeRuntimeOverlay,
  ]);

export const workstationKindParityDashboardSnapshot: DashboardSnapshot = {
  factory_state: "IDLE",
  tick_count: 42,
  topology: workstationKindParityDashboardTopology,
  uptime_seconds: 61,
  runtime: {
    ...buildEmptyDashboardRuntimeFixture(),
    place_token_counts: {
      "schedule:tick": 1,
      "story:complete": 0,
      "story:planned": 1,
      "story:scheduled": 1,
    },
  },
};

export const workstationKindParityExpectations: WorkstationKindParityExpectation[] =
  workstationKindParityDashboardTopology.workstation_node_ids.map((nodeID) => {
    const workstation =
      workstationKindParityDashboardTopology.workstation_nodes_by_id[
        nodeID as keyof typeof workstationKindParityDashboardTopology.workstation_nodes_by_id
      ];

    return {
      buttonName: `Select ${workstation.workstation_name} workstation`,
      metadata: workstationIconMetadata(workstation),
      nodeID,
      workstationName: workstation.workstation_name,
    };
  });

export const twentyNodeDashboardSnapshot: DashboardSnapshot = buildDashboardSnapshotFixture(
  twentyNodeTopologyFixture,
  [activeWorkRuntimeOverlay, failedOutcomeRuntimeOverlay],
);

export const twentyNodeDashboardTopology = twentyNodeDashboardSnapshot.topology;

export function resourceOccupancySnapshotForTick(tick: number): DashboardSnapshot {
  return buildFactoryTimelineSnapshot(resourceCountTimelineEvents, tick).dashboard;
}

