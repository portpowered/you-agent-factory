import {
  applyNodeChanges,
  type FitViewOptions,
  type NodeChange,
} from "@xyflow/react";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useState,
} from "react";

import type {
  DashboardActiveExecution,
  DashboardSnapshot,
} from "../../../api/dashboard/types";
import type { GraphLayout } from "../../flowchart/lib/layout";
import type { CurrentActivityNode } from "../../flowchart/public";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { resolveStoredNodePositionsForGraphKey } from "../lib/bridge-graph-layout-positions";
import { mergeDocNodesIntoGraphLayout } from "../lib/current-activity-doc-graph-layout";
import { buildVisibleGraphEdgesWithDraft } from "../lib/react-flow-current-activity-card-draft-edges";
import {
  buildGraphEdges,
  initialFocusNodes,
} from "../lib/react-flow-current-activity-card-edges";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  EMPTY_NODE_POSITIONS,
} from "../lib/react-flow-current-activity-card-graph";
import { currentActivityGraphKey } from "../lib/react-flow-current-activity-card-keys";
import type { CurrentActivitySelection } from "../lib/react-flow-current-activity-card-types";
import { useCurrentActivityGraphStore } from "../state/currentActivityGraphStore";
import { preserveExistingBundledFilesWhenAbsent } from "../../../api/factory-definition";
import { resolveObserveModeFactoryDefinition } from "./observe-mode-factory-definition";
import {
  groupActiveExecutionsByWorkstationNodeID,
  useActiveExecutions,
} from "./react-flow-current-activity-card-active-executions";
import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";
import { useCurrentActivityGraphLayoutForFactory } from "./react-flow-current-activity-card-graph-layout";
import { useTopologyStableFactoryForLayout } from "./use-topology-stable-factory-for-layout";

export type CurrentActivityGraphViewModelInput = {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
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
  storedNodePositions,
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
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  factoryDefinition?: DashboardSnapshot["factory"];
  graphLayout: GraphLayout;
  storedNodePositions: typeof EMPTY_NODE_POSITIONS;
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
          validationTargets: editor.structuralValidation.targets,
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
        storedNodePositions,
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
      storedNodePositions,
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

export function currentActivityCardFactoryDefinition(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
  snapshot: DashboardSnapshot,
  timelineMode: ReturnType<typeof useFactoryTimelineStore.getState>["mode"],
): DashboardSnapshot["factory"] | null | undefined {
  if (!editor.editorMode) {
    if (editor.editableDefinitionQuery?.status !== "success") {
      return snapshot.factory ?? null;
    }

    return observeModeFactoryDefinition(editor, snapshot, timelineMode);
  }

  if (editor.editableDefinitionQuery?.status !== "success") {
    return null;
  }

  return editorModeFactoryDefinition(editor) ?? null;
}

function observeModeFactoryDefinition(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
  snapshot: DashboardSnapshot,
  timelineMode: ReturnType<typeof useFactoryTimelineStore.getState>["mode"],
): DashboardSnapshot["factory"] | undefined {
  const document = editor.editableDefinitionQuery?.data;
  if (!document) {
    return snapshot.factory;
  }

  const resolvedFactory = resolveObserveModeFactoryDefinition({
    document,
    snapshotFactory: snapshot.factory,
    timelineMode,
  });

  return preserveExistingBundledFilesWhenAbsent(resolvedFactory, document);
}

function editorModeFactoryDefinition(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
) {
  return (
    editor.draftState.pendingFactoryDefinition ??
    editor.draftState.latestDocument ??
    editor.draftState.baseDocument ??
    undefined
  );
}

function useCurrentActivityGraphNodePresentation(
  baseNodes: CurrentActivityNode[],
) {
  const [nodes, setNodes] = useState<CurrentActivityNode[]>([]);

  useEffect(() => {
    setNodes((currentNodes) => {
      const currentPositions = new Map(
        currentNodes.map((node) => [node.id, node.position]),
      );
      return baseNodes.map((node) => ({
        ...node,
        position: currentPositions.get(node.id) ?? node.position,
      }));
    });
  }, [baseNodes]);

  const handleNodesChange = useCallback((changes: NodeChange[]) => {
    setNodes(
      (currentNodes) =>
        applyNodeChanges(changes, currentNodes) as CurrentActivityNode[],
    );
  }, []);
  const displayNodes = useMemo(() => {
    const positionOverrides = new Map(
      nodes.map((node) => [node.id, node.position] as const),
    );

    return baseNodes.map((node) => ({
      ...node,
      position: positionOverrides.get(node.id) ?? node.position,
    }));
  }, [baseNodes, nodes]);

  return { displayNodes, handleNodesChange };
}

function useStableCurrentActivityGraphLayout(
  snapshot: DashboardSnapshot,
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
) {
  const timelineMode = useFactoryTimelineStore((state) => state.mode);
  const displayFactoryDefinition = currentActivityCardFactoryDefinition(
    editor,
    snapshot,
    timelineMode,
  );
  const layoutFactoryDefinition = useTopologyStableFactoryForLayout(
    displayFactoryDefinition,
  );
  const graphLayout = useCurrentActivityGraphLayoutForFactory(
    snapshot,
    layoutFactoryDefinition,
    editor.hiddenNodeClasses,
  );

  return { displayFactoryDefinition, graphLayout };
}

function useCurrentActivityGraphEdges({
  activeGraphHighlights,
  displayNodes,
  handleAssignments,
  pendingAdditionEdgeIds,
  visibleGraphEdges,
}: {
  activeGraphHighlights: ReturnType<typeof buildActiveGraphHighlights>;
  displayNodes: CurrentActivityNode[];
  handleAssignments: ReturnType<typeof buildHandleAssignments>;
  pendingAdditionEdgeIds: ReadonlySet<string>;
  visibleGraphEdges: GraphLayout["edges"];
}) {
  return useMemo(
    () =>
      buildGraphEdges(
        activeGraphHighlights,
        handleAssignments,
        pendingAdditionEdgeIds,
        visibleGraphEdges,
        displayNodes,
      ),
    [
      activeGraphHighlights,
      displayNodes,
      handleAssignments,
      pendingAdditionEdgeIds,
      visibleGraphEdges,
    ],
  );
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: graph view-model keeps observe/editor layout, doc projection, and node presentation wiring together.
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
}: CurrentActivityGraphViewModelInput) {
  const activeExecutions = useActiveExecutions(snapshot);
  const activeExecutionsByWorkstationNodeID = useMemo(
    () => groupActiveExecutionsByWorkstationNodeID(activeExecutions),
    [activeExecutions],
  );
  const { displayFactoryDefinition, graphLayout: topologyGraphLayout } =
    useStableCurrentActivityGraphLayout(snapshot, editor);
  const graphLayout = useMemo(
    () =>
      mergeDocNodesIntoGraphLayout(
        topologyGraphLayout,
        displayFactoryDefinition,
      ),
    [displayFactoryDefinition, topologyGraphLayout],
  );
  const graphKey = useMemo(
    () => currentActivityGraphKey(graphLayout),
    [graphLayout],
  );
  const layoutNodeIds = useMemo(
    () => graphLayout.nodes.map((node) => node.nodeId),
    [graphLayout.nodes],
  );
  const positionsByGraphKey = useCurrentActivityGraphStore(
    (state) => state.positionsByGraphKey,
  );
  const bridgePositionsToGraphKey = useCurrentActivityGraphStore(
    (state) => state.bridgePositionsToGraphKey,
  );
  const storedNodePositions = useMemo(
    () =>
      graphKey
        ? resolveStoredNodePositionsForGraphKey(
            positionsByGraphKey,
            graphKey,
            layoutNodeIds,
          )
        : EMPTY_NODE_POSITIONS,
    [graphKey, layoutNodeIds, positionsByGraphKey],
  );
  const setStoredNodePosition = useCurrentActivityGraphStore(
    (state) => state.setNodePosition,
  );

  useLayoutEffect(() => {
    if (!graphKey || layoutNodeIds.length === 0) {
      return;
    }

    bridgePositionsToGraphKey(graphKey, layoutNodeIds);
  }, [bridgePositionsToGraphKey, graphKey, layoutNodeIds]);
  const { pendingAdditionEdgeIds, visibleGraphEdges } = useMemo(
    () =>
      buildVisibleGraphEdgesWithDraft({
        draft: editor.draftState.draft,
        graphLayout,
      }),
    [editor.draftState.draft, graphLayout],
  );
  const handleAssignments = useMemo(
    () => buildHandleAssignments(visibleGraphEdges, graphLayout.nodes),
    [graphLayout.nodes, visibleGraphEdges],
  );
  const activeGraphHighlights = useActiveGraphHighlights({
    activeExecutions,
    graphLayout,
    visibleGraphEdges,
  });
  const activeItemLabelsByPlaceId = useMemo(
    () => buildActiveItemLabelsByPlaceId(activeExecutions),
    [activeExecutions],
  );
  const baseNodes = useCurrentActivityBaseNodes({
    activeExecutionsByWorkstationNodeID,
    activeGraphHighlights,
    activeItemLabelsByPlaceId,
    editor,
    factoryDefinition: displayFactoryDefinition ?? undefined,
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
    storedNodePositions,
  });
  const { displayNodes, handleNodesChange } =
    useCurrentActivityGraphNodePresentation(baseNodes);
  const edges = useCurrentActivityGraphEdges({
    activeGraphHighlights,
    displayNodes,
    handleAssignments,
    pendingAdditionEdgeIds,
    visibleGraphEdges,
  });
  const initialFitViewOptions = useInitialFitViewOptions(graphLayout);

  return {
    edges,
    graphKey,
    handleNodesChange,
    initialFitViewKey:
      initialFitViewOptions.nodes?.map((node) => node.id).join(":") ||
      "full-graph",
    initialFitViewOptions,
    nodes: displayNodes,
    setStoredNodePosition,
    storedNodePositions,
  };
}
