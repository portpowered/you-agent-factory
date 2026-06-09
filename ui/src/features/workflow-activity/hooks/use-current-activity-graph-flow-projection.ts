import { useMemo } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { FactoryGraphEditorVisibilityPreset } from "../../factory-graph-editor/components/controls/factory-graph-editor-controls";
import type { useFactoryGraphDraftState } from "../../factory-graph-editor/hooks/factory-graph-draft-hook";
import type { useFactoryGraphLayoutDraftState } from "../../factory-graph-editor/hooks/layout/factory-graph-layout-draft-hook";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  createDefaultFactoryLayout,
  factoryLayoutFromDefinition,
} from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { pruneFactoryLayoutEdgesForTopology } from "../../factory-graph-editor/lib/layout/factory-graph-layout-validation";
import { mergeDocNodesIntoGraphLayout } from "../lib/current-activity-doc-graph-layout";
import { graphNodePositionsFromCanonicalLayout } from "../lib/layout/factory-graph-canonical-layout-positions";
import { buildVisibleGraphEdgesWithDraft } from "../lib/react-flow-current-activity-card-draft-edges";
import { useCurrentActivityGraphLayoutForFactory } from "./react-flow-current-activity-card-graph-layout";
import type { CurrentActivityFactoryGraphViewState } from "./use-current-activity-factory-graph-view-state";
import { useTopologyStableFactoryForLayout } from "./use-topology-stable-factory-for-layout";

export function useCurrentActivityGraphFlowProjection({
  draftState,
  hiddenNodeClasses,
  layoutDraftState,
  snapshot,
  viewState,
  visibilityPreset,
}: {
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  hiddenNodeClasses: ReadonlySet<FactoryGraphNodeKind>;
  layoutDraftState: ReturnType<typeof useFactoryGraphLayoutDraftState>;
  snapshot: DashboardSnapshot;
  viewState: CurrentActivityFactoryGraphViewState;
  visibilityPreset: FactoryGraphEditorVisibilityPreset;
}) {
  const displayFactoryDefinition = viewState.displayFactoryDefinition;
  const layoutFactoryDefinition = useTopologyStableFactoryForLayout(
    displayFactoryDefinition,
  );
  const topologyGraphLayout = useCurrentActivityGraphLayoutForFactory(
    snapshot,
    layoutFactoryDefinition,
    hiddenNodeClasses,
    visibilityPreset,
  );
  const graphLayout = useMemo(
    () =>
      mergeDocNodesIntoGraphLayout(
        topologyGraphLayout,
        displayFactoryDefinition,
      ),
    [displayFactoryDefinition, topologyGraphLayout],
  );
  const savedFactoryLayout = useMemo(
    () => factoryLayoutFromDefinition(displayFactoryDefinition),
    [displayFactoryDefinition],
  );
  const hasDocumentBackedLayoutDraft = draftState.source === "current-factory";
  const canonicalLayout = useMemo(
    () =>
      hasDocumentBackedLayoutDraft
        ? (layoutDraftState.layout ?? createDefaultFactoryLayout())
        : savedFactoryLayout,
    [hasDocumentBackedLayoutDraft, layoutDraftState.layout, savedFactoryLayout],
  );
  const canonicalPositions = useMemo(
    () => graphNodePositionsFromCanonicalLayout(graphLayout, canonicalLayout),
    [canonicalLayout, graphLayout],
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
    () =>
      buildVisibleGraphEdgesWithDraft({
        draft: draftState.draft,
        graphLayout: positionedGraphLayout,
      }),
    [draftState.draft, positionedGraphLayout],
  );
  const renderedLayout = useMemo(() => {
    const visibleLayoutEdgeIds = new Set(
      visibleGraphEdges.map((edge) => edge.canonicalEdgeId ?? edge.edgeId),
    );

    return pruneFactoryLayoutEdgesForTopology(
      canonicalLayout,
      visibleLayoutEdgeIds,
    ).layout;
  }, [canonicalLayout, visibleGraphEdges]);

  return useMemo(
    () => ({
      canonicalLayout,
      canonicalLayoutViewport: canonicalLayout.viewport ?? null,
      displayFactoryDefinition,
      graphLayout,
      pendingAdditionEdgeIds,
      positionedGraphLayout,
      renderedLayout,
      topologyGraphLayout,
      visibleGraphEdges,
    }),
    [
      canonicalLayout,
      displayFactoryDefinition,
      graphLayout,
      pendingAdditionEdgeIds,
      positionedGraphLayout,
      renderedLayout,
      topologyGraphLayout,
      visibleGraphEdges,
    ],
  );
}

export type CurrentActivityGraphFlowProjection = ReturnType<
  typeof useCurrentActivityGraphFlowProjection
>;
