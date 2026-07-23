import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { FactoryGraphEditorVisibilityPreset } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type { FactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { pruneFactoryLayoutEdgesForTopology } from "../../factory-graph-editor/lib/layout/factory-graph-layout-validation";
import type { GraphLayout, PositionedEdge } from "../../flowchart/lib/layout";
import { graphNodePositionsFromCanonicalLayout } from "../lib/layout/factory-graph-canonical-layout-positions";
import { useCurrentActivityGraphLayoutForFactory } from "./react-flow-current-activity-card-graph-layout";
import { useTopologyStableFactoryForLayout } from "./use-topology-stable-factory-for-layout";

export type CurrentActivityGraphResolvedEdges = {
  pendingAdditionEdgeIds: ReadonlySet<string>;
  visibleGraphEdges: PositionedEdge[];
};

export interface CurrentActivityGraphProjectionState {
  displayFactoryDefinition: DashboardSnapshot["factory"] | null | undefined;
  computedLayout: FactoryLayout;
  resolveGraphEdges: (
    positionedGraphLayout: GraphLayout,
  ) => CurrentActivityGraphResolvedEdges;
}

export function useCurrentActivityGraphFlowProjection({
  hiddenNodeClasses,
  projectionState,
  snapshot,
  visibilityPreset,
}: {
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  projectionState: CurrentActivityGraphProjectionState;
  snapshot: DashboardSnapshot;
  visibilityPreset: FactoryGraphEditorVisibilityPreset;
}) {
  const { computedLayout, displayFactoryDefinition, resolveGraphEdges } =
    projectionState;
  const layoutFactoryDefinition = useTopologyStableFactoryForLayout(
    displayFactoryDefinition,
  );
  const graphLayout = useCurrentActivityGraphLayoutForFactory(
    snapshot,
    layoutFactoryDefinition,
    hiddenNodeClasses,
    visibilityPreset,
  );
  const canonicalPositions = useMemo(
    () => graphNodePositionsFromCanonicalLayout(graphLayout, computedLayout),
    [computedLayout, graphLayout],
  );
  const positionedGraphLayout = useMemo(
    () => ({
      ...graphLayout,
      nodes: graphLayout.nodes.map((node) => {
        const position = canonicalPositions[node.nodeId];
        return position ? { ...node, x: position.x, y: position.y } : node;
      }),
    }),
    [canonicalPositions, graphLayout],
  );
  const { pendingAdditionEdgeIds, visibleGraphEdges } = useMemo(
    () => resolveGraphEdges(positionedGraphLayout),
    [positionedGraphLayout, resolveGraphEdges],
  );
  const renderedLayout = useMemo(() => {
    const visibleLayoutEdgeIds = new Set(
      visibleGraphEdges.map((edge) => edge.canonicalEdgeId ?? edge.edgeId),
    );

    return pruneFactoryLayoutEdgesForTopology(
      computedLayout,
      visibleLayoutEdgeIds,
    ).layout;
  }, [computedLayout, visibleGraphEdges]);

  return useMemo(
    () => ({
      canonicalLayout: computedLayout,
      canonicalLayoutViewport: computedLayout.viewport ?? null,
      displayFactoryDefinition,
      graphLayout,
      pendingAdditionEdgeIds,
      positionedGraphLayout,
      renderedLayout,
      visibleGraphEdges,
    }),
    [
      computedLayout,
      displayFactoryDefinition,
      graphLayout,
      pendingAdditionEdgeIds,
      positionedGraphLayout,
      renderedLayout,
      visibleGraphEdges,
    ],
  );
}

export type CurrentActivityGraphFlowProjection = ReturnType<
  typeof useCurrentActivityGraphFlowProjection
>;
