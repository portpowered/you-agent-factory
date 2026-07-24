// biome-ignore lint/style/noExcessiveLinesPerFile: graph view-model composes projection, selection bridge, and React Flow presentation in one hook module.
import type {
  EdgeChange,
  FitViewOptions,
  NodeChange,
  OnSelectionChangeFunc,
} from "@xyflow/react";
import { useCallback, useMemo, useState } from "react";

import type {
  DashboardActiveExecution,
  DashboardSnapshot,
} from "../../../api/dashboard/types";
import type { FactoryGraphEditorSelectionController } from "../../factory-graph-editor/hooks/selection/use-factory-graph-editor-selection";
import { useFactoryGraphEditorSelection } from "../../factory-graph-editor/hooks/selection/use-factory-graph-editor-selection";
import type { CanonicalFactoryDefinition } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import type { FactoryLayout } from "../../factory-graph-editor/lib/layout/factory-graph-layout-operations";
import { decorateProjectedEdgesWithWaypoints } from "../../factory-graph-editor/lib/projection/factory-graph-react-flow-edge-waypoint-projection";
import type { FactoryGraphReactFlowEdge } from "../../factory-graph-editor/lib/projection/factory-graph-react-flow-projection";
import type { GraphLayout } from "../../flowchart/lib/layout";
import type { CurrentActivityNode } from "../../flowchart/public";
import {
  buildGraphEdges,
  initialFocusNodes,
} from "../lib/react-flow-current-activity-card-edges";
import type { CurrentActivityEditorState } from "../lib/react-flow-current-activity-card-editor-handles";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
} from "../lib/react-flow-current-activity-card-graph";
import { currentActivityGraphKey } from "../lib/react-flow-current-activity-card-keys";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import { createCurrentActivityGraphSelectionBridgePublisher } from "./current-activity-graph-selection-bridge-publisher";
import {
  groupActiveExecutionsByWorkstationNodeID,
  useActiveExecutions,
} from "./react-flow-current-activity-card-active-executions";
import {
  useCurrentActivityGraphEdgePresentation,
  useCurrentActivityGraphSelectionGestures,
} from "./react-flow-current-activity-card-graph-selection-gestures";

const EMPTY_TRANSIENT_NODE_POSITIONS = new Map<
  string,
  { x: number; y: number }
>();

export type CurrentActivityGraphRenderProjection = {
  canonicalLayoutViewport: { x: number; y: number; zoom: number } | null;
  displayFactoryDefinition: CanonicalFactoryDefinition | null | undefined;
  graphLayout: GraphLayout;
  pendingAdditionEdgeIds: ReadonlySet<string>;
  positionedGraphLayout: GraphLayout;
  renderedLayout: FactoryLayout;
  visibleGraphEdges: GraphLayout["edges"];
};

export type CurrentActivityGraphViewModelEditorInput = Omit<
  CurrentActivityEditorState,
  "onConnectionAnchorClick" | "validationTargets"
> & {
  graphProjection: CurrentActivityGraphRenderProjection;
  handleConnectionAnchorClick: CurrentActivityEditorState["onConnectionAnchorClick"];
  selectedWaypointEdgeId?: string | null;
  validationTargets?: CurrentActivityEditorState["validationTargets"];
};

export type CurrentActivityGraphViewModelInput = {
  editor: CurrentActivityGraphViewModelEditorInput;
  locale?: string;
  now: number;
  onSelectDoc: (targetPath: string) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectResource: (resourceName: string) => void;
  onSelectWorker: (workerName: string) => void;
  onSelectWorkType: (workTypeName: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
};

function useCurrentActivityBaseNodes({
  activeExecutionsByWorkstationNodeID,
  activeGraphHighlights,
  activeItemLabelsByPlaceId,
  editor,
  factoryDefinition,
  graphLayout,
  locale,
  now,
  onSelectDoc,
  onSelectResource,
  onSelectStateNode,
  onSelectWorkID,
  onSelectWorker,
  onSelectWorkType,
  onSelectWorkstation,
  selection,
  snapshot,
}: Pick<
  CurrentActivityGraphViewModelInput,
  | "now"
  | "onSelectDoc"
  | "onSelectResource"
  | "onSelectStateNode"
  | "onSelectWorkID"
  | "onSelectWorker"
  | "onSelectWorkType"
  | "onSelectWorkstation"
  | "selection"
  | "locale"
  | "snapshot"
> & {
  activeExecutionsByWorkstationNodeID: Record<
    string,
    DashboardActiveExecution[]
  >;
  activeGraphHighlights: ReturnType<typeof buildActiveGraphHighlights>;
  activeItemLabelsByPlaceId: ReturnType<typeof buildActiveItemLabelsByPlaceId>;
  editor: CurrentActivityGraphViewModelEditorInput;
  factoryDefinition?: DashboardSnapshot["factory"];
  graphLayout: GraphLayout;
}) {
  return useMemo<CurrentActivityNode[]>(
    () =>
      buildCurrentActivityNodes({
        activeExecutionsByWorkstationNodeID,
        activeGraphHighlights,
        activeItemLabelsByPlaceId,
        editor: {
          activeTool: editor.activeTool,
          canInteractWithEditor: editor.canInteractWithEditor,
          editorMode: editor.editorMode,
          onConnectionAnchorClick: editor.handleConnectionAnchorClick,
          pendingConnectionSource: editor.pendingConnectionSource,
          validationTargets: editor.validationTargets,
        },
        factoryDefinition,
        graphLayout,
        locale,
        now,
        onSelectDoc,
        onSelectResource,
        onSelectStateNode,
        onSelectWorkID,
        onSelectWorker,
        onSelectWorkType,
        onSelectWorkstation,
        selection,
        snapshot,
      }),
    [
      activeExecutionsByWorkstationNodeID,
      activeGraphHighlights,
      activeItemLabelsByPlaceId,
      editor,
      factoryDefinition,
      graphLayout,
      locale,
      now,
      onSelectDoc,
      onSelectResource,
      onSelectStateNode,
      onSelectWorkID,
      onSelectWorker,
      onSelectWorkType,
      onSelectWorkstation,
      selection,
      snapshot,
    ],
  );
}

function useActiveGraphHighlights({
  activeExecutions,
  graphLayout,
  visibleGraphEdges,
}: {
  activeExecutions: DashboardActiveExecution[];
  graphLayout: GraphLayout;
  visibleGraphEdges: GraphLayout["edges"];
}) {
  return useMemo(
    () =>
      buildActiveGraphHighlights(
        activeExecutions,
        visibleGraphEdges,
        graphLayout.nodes,
      ),
    [activeExecutions, graphLayout.nodes, visibleGraphEdges],
  );
}

function useCurrentActivityGraphNodePresentation(
  baseNodes: CurrentActivityNode[],
  graphSelection: FactoryGraphEditorSelectionController,
) {
  const basePositionKey = useMemo(
    () =>
      baseNodes
        .map((node) => `${node.id}:${node.position.x},${node.position.y}`)
        .join("|"),
    [baseNodes],
  );
  const [transientPositionState, setTransientPositionState] = useState<{
    basePositionKey: string;
    positionsByNodeId: ReadonlyMap<string, { x: number; y: number }>;
  }>(() => ({
    basePositionKey,
    positionsByNodeId: EMPTY_TRANSIENT_NODE_POSITIONS,
  }));

  const handleNodesChange = useCallback(
    (changes: NodeChange[]) => {
      const positionChanges: NodeChange[] = [];

      for (const change of changes) {
        if (change.type === "position") {
          positionChanges.push(change);
        }
      }

      if (positionChanges.length === 0) {
        return;
      }

      setTransientPositionState((currentPositionState) => {
        const currentPositionsByNodeId =
          currentPositionState.basePositionKey === basePositionKey
            ? currentPositionState.positionsByNodeId
            : EMPTY_TRANSIENT_NODE_POSITIONS;
        let nextPositionsByNodeId: Map<
          string,
          { x: number; y: number }
        > | null = null;

        for (const change of positionChanges) {
          if (change.type !== "position") {
            continue;
          }

          nextPositionsByNodeId ??= new Map(currentPositionsByNodeId);

          if (!change.position) {
            nextPositionsByNodeId.delete(change.id);
            continue;
          }

          nextPositionsByNodeId.set(change.id, change.position);
        }

        return nextPositionsByNodeId
          ? {
              basePositionKey,
              positionsByNodeId: nextPositionsByNodeId,
            }
          : currentPositionState;
      });
    },
    [basePositionKey],
  );
  const transientPositionsByNodeId =
    transientPositionState.basePositionKey === basePositionKey
      ? transientPositionState.positionsByNodeId
      : EMPTY_TRANSIENT_NODE_POSITIONS;

  const displayNodes = useMemo(
    () =>
      baseNodes.map((node) => ({
        ...node,
        position: transientPositionsByNodeId.get(node.id) ?? node.position,
        selected: graphSelection.isNodeSelected(node.id),
      })),
    [baseNodes, graphSelection, transientPositionsByNodeId],
  );

  return { displayNodes, handleNodesChange };
}

function useCurrentActivityGraphEdges({
  activeGraphHighlights,
  displayNodes,
  graphSelection,
  handleAssignments,
  layout,
  pendingAdditionEdgeIds,
  selectedWaypointEdgeId,
  visibleGraphEdges,
}: {
  activeGraphHighlights: ReturnType<typeof buildActiveGraphHighlights>;
  displayNodes: CurrentActivityNode[];
  graphSelection: FactoryGraphEditorSelectionController;
  handleAssignments: ReturnType<typeof buildHandleAssignments>;
  layout: FactoryLayout;
  pendingAdditionEdgeIds: ReadonlySet<string>;
  selectedWaypointEdgeId?: string | null;
  visibleGraphEdges: GraphLayout["edges"];
}) {
  return useMemo(() => {
    const edges = buildGraphEdges(
      activeGraphHighlights,
      handleAssignments,
      pendingAdditionEdgeIds,
      visibleGraphEdges,
      displayNodes,
    );

    return decorateProjectedEdgesWithWaypoints({
      edges: edges as FactoryGraphReactFlowEdge[],
      layout,
      selectedWaypointEdgeId: selectedWaypointEdgeId ?? null,
    }).map((edge) => {
      const layoutEdgeId =
        (edge.data as { factoryGraphEdgeId?: string } | undefined)
          ?.factoryGraphEdgeId ?? edge.id;
      const selected =
        edge.selected === true ||
        graphSelection.isEdgeSelected(edge.id) ||
        graphSelection.isEdgeSelected(layoutEdgeId);

      if (!selected) {
        return edge;
      }

      return {
        ...edge,
        selected: true,
        type: edge.type ?? "factoryEditorEdge",
      };
    });
  }, [
    activeGraphHighlights,
    displayNodes,
    graphSelection,
    handleAssignments,
    layout,
    pendingAdditionEdgeIds,
    selectedWaypointEdgeId,
    visibleGraphEdges,
  ]);
}

function useInitialFitViewOptions(graphLayout: GraphLayout) {
  return useMemo<FitViewOptions>(
    () => ({
      maxZoom: 1.15,
      minZoom: 0.7,
      nodes: initialFocusNodes(graphLayout),
      padding: 0.18,
    }),
    [graphLayout],
  );
}

export type CurrentActivityGraphViewModelResult = {
  canonicalLayoutViewport: CurrentActivityGraphRenderProjection["canonicalLayoutViewport"];
  clearGraphSelection: () => void;
  edges: FactoryGraphReactFlowEdge[];
  graphKey: string;
  graphSelection: FactoryGraphEditorSelectionController;
  handleEdgesChange: (changes: EdgeChange[]) => void;
  handleGraphSelectionChange: OnSelectionChangeFunc;
  handleGraphSelectionStart: (event: { shiftKey: boolean }) => void;
  handleNodesChange: (changes: NodeChange[]) => void;
  initialFitViewKey: string;
  initialFitViewOptions: FitViewOptions;
  nodes: CurrentActivityNode[];
};

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: composes graph projection, editor-local selection, and React Flow presentation wiring.
export function useCurrentActivityGraphViewModel({
  editor,
  locale,
  now,
  onSelectDoc,
  onSelectResource,
  onSelectStateNode,
  onSelectWorkID,
  onSelectWorker,
  onSelectWorkType,
  onSelectWorkstation,
  selection,
  snapshot,
}: CurrentActivityGraphViewModelInput): CurrentActivityGraphViewModelResult {
  const activeExecutions = useActiveExecutions(snapshot);
  const activeExecutionsByWorkstationNodeID = useMemo(
    () => groupActiveExecutionsByWorkstationNodeID(activeExecutions),
    [activeExecutions],
  );
  const {
    canonicalLayoutViewport,
    displayFactoryDefinition,
    graphLayout,
    pendingAdditionEdgeIds,
    positionedGraphLayout,
    renderedLayout,
    visibleGraphEdges,
  } = editor.graphProjection;
  const graphKey = useMemo(
    () => currentActivityGraphKey(graphLayout),
    [graphLayout],
  );
  const handleAssignments = useMemo(
    () =>
      buildHandleAssignments(visibleGraphEdges, positionedGraphLayout.nodes),
    [positionedGraphLayout.nodes, visibleGraphEdges],
  );
  const activeGraphHighlights = useActiveGraphHighlights({
    activeExecutions,
    graphLayout: positionedGraphLayout,
    visibleGraphEdges,
  });
  const activeItemLabelsByPlaceId = useMemo(
    () => buildActiveItemLabelsByPlaceId(activeExecutions),
    [activeExecutions],
  );
  const publishGraphSelectionBridgeState = useMemo(
    () =>
      createCurrentActivityGraphSelectionBridgePublisher({
        onSelectDoc,
        onSelectResource,
        onSelectStateNode,
        onSelectWorker,
        onSelectWorkType,
        onSelectWorkstation,
      }),
    [
      onSelectDoc,
      onSelectResource,
      onSelectStateNode,
      onSelectWorker,
      onSelectWorkType,
      onSelectWorkstation,
    ],
  );
  const graphSelection = useFactoryGraphEditorSelection({
    onStateChange: publishGraphSelectionBridgeState,
  });
  const graphSelectionEnabled =
    !editor.editorMode || editor.activeTool !== "delete";
  const baseNodes = useCurrentActivityBaseNodes({
    activeExecutionsByWorkstationNodeID,
    activeGraphHighlights,
    activeItemLabelsByPlaceId,
    editor,
    factoryDefinition: displayFactoryDefinition ?? undefined,
    graphLayout: positionedGraphLayout,
    locale,
    now,
    onSelectDoc,
    onSelectResource,
    onSelectStateNode,
    onSelectWorkID,
    onSelectWorker,
    onSelectWorkType,
    onSelectWorkstation,
    selection,
    snapshot,
  });
  const { displayNodes, handleNodesChange } =
    useCurrentActivityGraphNodePresentation(baseNodes, graphSelection);
  const { handleEdgesChange } = useCurrentActivityGraphEdgePresentation(
    graphSelection,
    graphSelectionEnabled,
  );
  const graphSelectionGestures = useCurrentActivityGraphSelectionGestures(
    graphSelection,
    graphSelectionEnabled,
  );
  const edges = useCurrentActivityGraphEdges({
    activeGraphHighlights,
    displayNodes,
    graphSelection,
    handleAssignments,
    layout: renderedLayout,
    pendingAdditionEdgeIds,
    selectedWaypointEdgeId: editor.selectedWaypointEdgeId,
    visibleGraphEdges,
  });
  const initialFitViewOptions = useInitialFitViewOptions(graphLayout);

  return {
    canonicalLayoutViewport,
    clearGraphSelection: graphSelectionGestures.clearGraphSelection,
    edges,
    graphKey,
    graphSelection,
    handleEdgesChange,
    handleGraphSelectionChange:
      graphSelectionGestures.handleGraphSelectionChange,
    handleGraphSelectionStart: graphSelectionGestures.handleGraphSelectionStart,
    handleNodesChange,
    initialFitViewKey:
      initialFitViewOptions.nodes?.map((node) => node.id).join(":") ||
      "full-graph",
    initialFitViewOptions,
    nodes: displayNodes,
  };
}
