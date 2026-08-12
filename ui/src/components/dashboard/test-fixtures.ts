import type { DashboardSnapshot } from "../../api/dashboard/types";
import {
  activeWorkRuntimeOverlay,
  activeWorkWithMultimodalPayloadRuntimeOverlay,
  buildDashboardSnapshotFixture,
  buildEmptyDashboardRuntimeFixture,
  factoryFromDashboardTopology,
  failedOutcomeRuntimeOverlay,
  rejectedOutcomeRuntimeOverlay,
  retryAttemptRuntimeOverlay,
} from "./fixtures/runtime";
import {
  mediumBranchingDashboardTopology,
  oneNodeDashboardTopology,
  twentyNodeDashboardTopology as twentyNodeTopologyFixture,
  workstationKindParityDashboardTopology,
} from "./fixtures/topologies";
import type { WorkstationIconMetadata } from "../../features/flowchart/lib/workstation-icon-metadata";
import { workstationIconMetadata } from "../../features/flowchart/lib/workstation-icon-metadata";

export interface WorkstationKindParityExpectation {
  buttonName: string;
  metadata: Pick<WorkstationIconMetadata, "iconKind" | "label">;
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

export const currentSelectionWorkContentsDashboardSnapshot: DashboardSnapshot =
  buildDashboardSnapshotFixture(mediumBranchingDashboardTopology, [
    activeWorkWithMultimodalPayloadRuntimeOverlay,
  ]);

export const workstationKindParityDashboardSnapshot: DashboardSnapshot = {
  factory: factoryFromDashboardTopology(workstationKindParityDashboardTopology),
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

const mixedFactorySemanticsDefinition = {
  ...workstationKindParityDashboardSnapshot.factory,
  name: "mixed-workstation-semantics",
  workers: [
    { model: "gpt-5-mini", name: "agent", type: "MODEL_WORKER" },
    { model: "gpt-5-mini", name: "classifier", type: "MODEL_WORKER" },
    { model: "gpt-5-mini", name: "inference", type: "MODEL_WORKER" },
    { model: "gpt-5-mini", name: "poller", type: "HOSTED_WORKER" },
    { model: "gpt-5-mini", name: "script", type: "SCRIPT_WORKER" },
  ],
  workstations: [
    {
      behavior: "STANDARD",
      id: "classifier",
      inputs: [{ state: "init", workType: "story" }],
      name: "Classifier route",
      outputs: [{ state: "planned", workType: "story" }],
      type: "CLASSIFIER_WORKSTATION",
      worker: "classifier",
    },
    {
      behavior: "STANDARD",
      id: "logical-router",
      inputs: [{ state: "planned", workType: "story" }],
      name: "Logical route",
      outputs: [{ state: "ready", workType: "story" }],
      type: "LOGICAL_MOVE",
      worker: "",
    },
    {
      behavior: "STANDARD",
      guards: [
        {
          maxVisits: 3,
          type: "VISIT_COUNT",
          workstation: "execute-goal",
        },
      ],
      id: "goal-loop-breaker",
      inputs: [{ state: "implemented", workType: "story" }],
      name: "goal-loop-breaker",
      outputs: [{ state: "planned", workType: "story" }],
      type: "LOGICAL_MOVE",
      worker: "",
    },
    {
      behavior: "STANDARD",
      id: "long-inference",
      inputs: [{ state: "ready", workType: "story" }],
      name: "Inference workstation with a deliberately long authored title",
      outputs: [{ state: "implemented", workType: "story" }],
      type: "INFERENCE_RUN",
      worker: "inference",
    },
    {
      behavior: "STANDARD",
      id: "agent",
      inputs: [{ state: "implemented", workType: "story" }],
      name: "Agent worker",
      outputs: [{ state: "complete", workType: "story" }],
      type: "AGENT_RUN",
      worker: "agent",
    },
    {
      behavior: "REPEATER",
      id: "execute-goal",
      inputs: [{ state: "ready", workType: "story" }],
      name: "execute-goal",
      outputs: [{ state: "implemented", workType: "story" }],
      type: "AGENT_RUN",
      worker: "agent",
    },
    {
      behavior: "CRON",
      id: "script-cron",
      inputs: [{ state: "tick", workType: "schedule" }],
      name: "Script cron",
      outputs: [{ state: "scheduled", workType: "story" }],
      type: "SCRIPT_RUN",
      worker: "script",
    },
    {
      behavior: "POLLER",
      id: "poller",
      inputs: [],
      name: "Poller source",
      outputs: [{ state: "scheduled", workType: "story" }],
      type: "POLLER_RUN",
      worker: "poller",
    },
  ],
} satisfies NonNullable<DashboardSnapshot["factory"]>;

export const mixedFactorySemanticsDashboardSnapshot: DashboardSnapshot = {
  ...workstationKindParityDashboardSnapshot,
  factory: mixedFactorySemanticsDefinition,
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

export const twentyNodeDashboardSnapshot: DashboardSnapshot =
  buildDashboardSnapshotFixture(twentyNodeTopologyFixture, [
    activeWorkRuntimeOverlay,
    failedOutcomeRuntimeOverlay,
  ]);
