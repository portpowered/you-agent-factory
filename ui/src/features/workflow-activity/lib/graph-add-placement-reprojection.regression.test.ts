import { describe, expect, it } from "vitest";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../../api/factory-definition";
import { baseFactoryDefinition } from "../../factory-graph-editor/lib/factory-graph-draft.test-helpers";
import { resolveStoredNodePositionsForGraphKey } from "./bridge-graph-layout-positions";
import {
  buildCurrentActivityGraphLayoutFromFactory,
  dashboardWorkstationFromFactory,
} from "./current-activity-factory-graph-layout";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildVisibleGraphEdges,
} from "./react-flow-current-activity-card-graph";
import { currentActivityGraphKey } from "./react-flow-current-activity-card-keys";

const factoryWithReviewWorkstation: CanonicalFactoryDefinition = {
  ...baseFactoryDefinition,
  workstations: [
    ...(baseFactoryDefinition.workstations ?? []),
    {
      body: "Review the story.",
      inputs: [{ state: "queued", workType: "story" }],
      name: "review",
      outputs: [{ state: "done", workType: "story" }],
      resources: [{ capacity: 2, name: "gpu" }],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

function buildSnapshot(factory: CanonicalFactoryDefinition): DashboardSnapshot {
  const workstations = (factory.workstations ?? []).map(
    dashboardWorkstationFromFactory,
  );

  return {
    factory,
    factory_state: "IDLE",
    runtime: {
      active_executions_by_dispatch_id: {},
      current_work_items_by_place_id: {},
      place_occupancy_work_items_by_place_id: {},
      place_token_counts: {},
      session: {
        completed_count: 0,
        dispatched_count: 0,
        failed_count: 0,
        has_data: true,
        provider_sessions: [],
      },
      workstation_requests_by_dispatch_id: {},
    },
    tick_count: 0,
    topology: {
      edges: [],
      workstation_node_ids: workstations.map(
        (workstation) => workstation.node_id,
      ),
      workstation_nodes_by_id: Object.fromEntries(
        workstations.map((workstation) => [workstation.node_id, workstation]),
      ),
    },
    uptime_seconds: 0,
  };
}

describe("graph add placement reprojection regression", () => {
  it("keeps a newly added workstation stored position after topology graph key changes", async () => {
    const snapshot = buildSnapshot(factoryWithReviewWorkstation);
    const graphLayout = await buildCurrentActivityGraphLayoutFromFactory(
      factoryWithReviewWorkstation,
    );
    const layoutNode = graphLayout.nodes.find(
      (node) => node.nodeId === "workstation:review",
    );
    expect(layoutNode).toBeDefined();

    const layoutFallback = { x: layoutNode?.x ?? 0, y: layoutNode?.y ?? 0 };
    const storedPosition = { x: 512, y: 288 };
    expect(storedPosition).not.toEqual(layoutFallback);

    const beforeSaveKey = currentActivityGraphKey({
      ...graphLayout,
      edges: [
        {
          edgeId: "edge-before-save",
          fromNodeId: "workstation:draft",
          toNodeId: "worker:writer",
        },
      ],
    });
    const afterSaveKey = currentActivityGraphKey({
      ...graphLayout,
      edges: [
        {
          edgeId: "edge-after-save",
          fromNodeId: "workstation:draft",
          toNodeId: "worker:writer",
        },
      ],
    });
    expect(beforeSaveKey).not.toEqual(afterSaveKey);

    const positionsByGraphKey = {
      [beforeSaveKey]: {
        "workstation:review": storedPosition,
      },
    };
    const nodeIds = graphLayout.nodes.map((node) => node.nodeId);
    const storedNodePositions = resolveStoredNodePositionsForGraphKey(
      positionsByGraphKey,
      afterSaveKey,
      nodeIds,
    );

    expect(storedNodePositions["workstation:review"]).toEqual(storedPosition);

    const visibleGraphEdges = buildVisibleGraphEdges(graphLayout);
    const nodes = buildCurrentActivityNodes({
      activeExecutionsByWorkstationNodeID: {},
      activeGraphHighlights: buildActiveGraphHighlights(
        [],
        visibleGraphEdges,
        graphLayout.nodes,
      ),
      activeItemLabelsByPlaceId: buildActiveItemLabelsByPlaceId([]),
      graphLayout,
      now: Date.parse("2026-06-04T00:00:00Z"),
      onSelectDoc: vi.fn(),
      onSelectResource: vi.fn(),
      onSelectStateNode: vi.fn(),
      onSelectWorkID: vi.fn(),
      onSelectWorker: vi.fn(),
      onSelectWorkType: vi.fn(),
      onSelectWorkstation: vi.fn(),
      selection: null,
      snapshot,
      storedNodePositions,
    });

    const reviewNode = nodes.find((node) => node.id === "workstation:review");
    expect(reviewNode?.position).toEqual(storedPosition);
    expect(reviewNode?.position).not.toEqual(layoutFallback);
  });
});
