import type { CurrentActivityGraphState } from "./use-current-activity-graph-state";
import {
  type CurrentActivityGraphRenderProjection,
  type CurrentActivityGraphViewModelInput,
  useCurrentActivityGraphViewModel,
} from "./react-flow-current-activity-card-graph-view-model";
import type { CurrentActivityGraphFlowProjection } from "./use-current-activity-graph-flow-projection";

type CurrentActivityGraphCardViewModelInput = Omit<
  CurrentActivityGraphViewModelInput,
  "editor"
> & {
  graphController: CurrentActivityGraphState;
};

export function useCurrentActivityGraphCardViewModel(
  input: CurrentActivityGraphCardViewModelInput,
) {
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
  const edgeWaypointControls = publicEditor.edgeWaypointControls;
  const {
    canonicalLayoutViewport: _canonicalLayoutViewport,
    initialFitViewKey,
    initialFitViewOptions,
    ...publicGraph
  } = graph;
  return {
    ...publicEditor,
    ...publicGraph,
    edgeWaypointControls,
    layoutControls: {
      ...publicEditor.layoutControls,
      initialFitViewKey,
      initialFitViewOptions,
    },
  };
}

export type CurrentActivityGraphCardViewModel = ReturnType<
  typeof useCurrentActivityGraphCardViewModel
>;

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
