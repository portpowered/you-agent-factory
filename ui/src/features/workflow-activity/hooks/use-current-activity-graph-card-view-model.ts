import { useCallback, useMemo, useState } from "react";

import {
  factoryGraphNodeIdForAddEntityDraft,
  type GraphEditorAddNodePlacementViewport,
  resolveInitialPlacementTopLeftForViewport,
} from "../lib/graph-editor-add-node-placement";
import {
  type CurrentActivityGraphRenderProjection,
  type CurrentActivityGraphViewModelInput,
  useCurrentActivityGraphViewModel,
} from "./react-flow-current-activity-card-graph-view-model";
import type { CurrentActivityGraphFlowProjection } from "./use-current-activity-graph-flow-projection";
import type { CurrentActivityGraphState } from "./use-current-activity-graph-state";

type CurrentActivityGraphCardViewModelInput = Omit<
  CurrentActivityGraphViewModelInput,
  "editor"
> & {
  graphController: CurrentActivityGraphState;
};

export function useCurrentActivityGraphCardViewModel(
  input: CurrentActivityGraphCardViewModelInput,
): CurrentActivityGraphCardViewModel {
  const [addNodePlacementViewport, setAddNodePlacementViewport] =
    useState<GraphEditorAddNodePlacementViewport | null>(null);
  const {
    connectionControls,
    graphProjection: editorGraphProjection,
    ...publicEditor
  } = input.graphController;
  const graphProjection = currentActivityGraphRenderProjection(
    editorGraphProjection,
  );
  const graph = useCurrentActivityGraphViewModel({
    ...input,
    editor: {
      activeTool: publicEditor.editorControls.activeTool,
      canInteractWithEditor: publicEditor.editorControls.canInteract,
      editorMode: publicEditor.editorControls.isEditing,
      graphProjection,
      handleConnectionAnchorClick: connectionControls.handleAnchorClick,
      pendingConnectionSource: connectionControls.pendingSource,
      selectedWaypointEdgeId:
        publicEditor.edgeWaypointControls.selectedWaypointEdgeId,
      validationTargets: publicEditor.validationControls.targets,
    },
  });
  const submitAddEntity = useCallback(() => {
    const draft = publicEditor.addControls.draft;
    const topLeft =
      draft && addNodePlacementViewport
        ? resolveInitialPlacementTopLeftForViewport({
            draft,
            nodes: graph.nodes,
            placementViewport: addNodePlacementViewport,
          })
        : null;

    publicEditor.addControls.submit?.(
      draft && topLeft
        ? {
            nodeId: factoryGraphNodeIdForAddEntityDraft(draft),
            position: topLeft,
          }
        : undefined,
    );
  }, [addNodePlacementViewport, graph.nodes, publicEditor.addControls]);
  const addControls = useMemo(
    () => ({
      ...publicEditor.addControls,
      submit: submitAddEntity,
      updatePlacementViewport: setAddNodePlacementViewport,
    }),
    [publicEditor.addControls, submitAddEntity],
  );
  const edgeWaypointControls = publicEditor.edgeWaypointControls;
  const {
    canonicalLayoutViewport: _canonicalLayoutViewport,
    initialFitViewKey,
    initialFitViewOptions,
    ...publicGraph
  } = graph;
  return {
    ...publicEditor,
    addControls,
    ...publicGraph,
    edgeWaypointControls,
    layoutControls: {
      ...publicEditor.layoutControls,
      initialFitViewKey,
      initialFitViewOptions,
    },
  };
}

type CurrentActivityReactFlowRenderModel = Omit<
  ReturnType<typeof useCurrentActivityGraphViewModel>,
  "canonicalLayoutViewport" | "initialFitViewKey" | "initialFitViewOptions"
>;

type CurrentActivityGraphPublicControllerModel = Omit<
  CurrentActivityGraphState,
  "addControls" | "connectionControls" | "graphProjection" | "layoutControls"
>;

export type CurrentActivityGraphCardAddControls = Omit<
  CurrentActivityGraphState["addControls"],
  "submit"
> & {
  submit: () => void;
  updatePlacementViewport: (
    placementViewport: GraphEditorAddNodePlacementViewport,
  ) => void;
};

export type CurrentActivityGraphCardLayoutControls =
  CurrentActivityGraphState["layoutControls"] & {
    initialFitViewKey: string;
    initialFitViewOptions: ReturnType<
      typeof useCurrentActivityGraphViewModel
    >["initialFitViewOptions"];
  };

export interface CurrentActivityGraphCardViewModel
  extends CurrentActivityGraphPublicControllerModel,
    CurrentActivityReactFlowRenderModel {
  addControls: CurrentActivityGraphCardAddControls;
  layoutControls: CurrentActivityGraphCardLayoutControls;
}

function currentActivityGraphRenderProjection(
  flowProjection: CurrentActivityGraphFlowProjection,
): CurrentActivityGraphRenderProjection {
  return {
    canonicalLayoutViewport: flowProjection.canonicalLayoutViewport,
    displayFactoryDefinition: flowProjection.displayFactoryDefinition,
    graphLayout: flowProjection.graphLayout,
    pendingAdditionEdgeIds: flowProjection.pendingAdditionEdgeIds,
    positionedGraphLayout: flowProjection.positionedGraphLayout,
    renderedLayout: flowProjection.renderedLayout,
    visibleGraphEdges: flowProjection.visibleGraphEdges,
  };
}
