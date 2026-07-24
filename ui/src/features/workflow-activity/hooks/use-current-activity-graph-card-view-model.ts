import { useCallback, useMemo, useState } from "react";

import { useFactoryGraphVisualGroupEditor } from "../../factory-graph-editor/hooks/layout/factory-graph-visual-group-editor-hook";
import { pruneFactoryGraphEditorSelectionAfterRemoval } from "../../factory-graph-editor/lib/selection/factory-graph-editor-selection-batch-delete";
import {
  type FactoryGraphEditorToolbarSelectionState,
  resolveFactoryGraphEditorToolbarSelectionState,
} from "../../factory-graph-editor/lib/selection/factory-graph-editor-toolbar-selection";
import {
  factoryGraphNodeIdForAddEntityDraft,
  type GraphEditorAddNodePlacementViewport,
  resolveInitialPlacementTopLeftForViewport,
  viewportCenterFromPlacementViewport,
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: card view model composes graph, editor, and selection controllers.
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
  const resolveViewportCenter = useCallback(() => {
    if (!addNodePlacementViewport) {
      return null;
    }

    return viewportCenterFromPlacementViewport(addNodePlacementViewport);
  }, [addNodePlacementViewport]);
  const requestSelectionBatchRemoval =
    publicEditor.removalControls.requestSelectionBatchRemoval;
  const canDeleteSelectionFn = publicEditor.removalControls.canDeleteSelection;
  const confirmRemoval = publicEditor.removalControls.confirm;
  const pendingRemovalIntent = publicEditor.removalControls.pendingIntent;
  const selectionState = graph.graphSelection.state;
  const graphSelectionToolbarState =
    resolveFactoryGraphEditorToolbarSelectionState(selectionState);
  const canDeleteGraphSelection = canDeleteSelectionFn({
    edgeIds: [...selectionState.selectedEdgeIds],
    nodeIds: [...selectionState.selectedNodeIds],
  });
  const deleteGraphSelection = useCallback(() => {
    const currentSelection = graph.graphSelection.state;
    const result = requestSelectionBatchRemoval({
      edgeIds: [...currentSelection.selectedEdgeIds],
      nodeIds: [...currentSelection.selectedNodeIds],
    });
    if (result.status === "applied") {
      graph.graphSelection.replaceSelection(
        pruneFactoryGraphEditorSelectionAfterRemoval(
          currentSelection,
          result.removed,
        ),
      );
    }
  }, [graph.graphSelection, requestSelectionBatchRemoval]);
  const confirmSelectionRemoval = useCallback(() => {
    const currentSelection = graph.graphSelection.state;
    const hadPendingRemoval = Boolean(pendingRemovalIntent);
    confirmRemoval();
    if (hadPendingRemoval) {
      graph.graphSelection.replaceSelection(
        pruneFactoryGraphEditorSelectionAfterRemoval(currentSelection, {
          edgeIds: [...currentSelection.selectedEdgeIds],
          nodeIds: [...currentSelection.selectedNodeIds],
        }),
      );
    }
  }, [confirmRemoval, graph.graphSelection, pendingRemovalIntent]);
  const visualGroupControls = useFactoryGraphVisualGroupEditor({
    activeTool: publicEditor.editorControls.activeTool,
    addNodeToVisualGroup: publicEditor.layoutControls.addNodeToVisualGroup,
    canInteractWithEditor: publicEditor.editorControls.canInteract,
    canvasNodeOptions: publicEditor.layoutControls.canvasNodeOptions,
    createVisualGroup: publicEditor.layoutControls.createVisualGroup,
    editorMode: publicEditor.editorControls.isEditing,
    layout: publicEditor.layoutControls.currentLayout,
    locale: input.locale,
    removeNodeFromVisualGroup:
      publicEditor.layoutControls.removeNodeFromVisualGroup,
    moveVisualGroupByDelta: publicEditor.layoutControls.moveVisualGroupByDelta,
    resizeVisualGroup: publicEditor.layoutControls.resizeVisualGroup,
    deleteVisualGroup: publicEditor.layoutControls.deleteVisualGroup,
    renameVisualGroup: publicEditor.layoutControls.renameVisualGroup,
    resolveViewportCenter,
    setVisualGroupColor: publicEditor.layoutControls.setVisualGroupColor,
  });
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
    canDeleteGraphSelection,
    deleteGraphSelection,
    edgeWaypointControls,
    graphSelectionToolbarState,
    removalControls: {
      ...publicEditor.removalControls,
      confirm: confirmSelectionRemoval,
    },
    visualGroupControls,
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
  canDeleteGraphSelection: boolean;
  deleteGraphSelection: () => void;
  graphSelectionToolbarState: FactoryGraphEditorToolbarSelectionState;
  layoutControls: CurrentActivityGraphCardLayoutControls;
  removalControls: CurrentActivityGraphState["removalControls"];
  visualGroupControls: ReturnType<typeof useFactoryGraphVisualGroupEditor>;
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
