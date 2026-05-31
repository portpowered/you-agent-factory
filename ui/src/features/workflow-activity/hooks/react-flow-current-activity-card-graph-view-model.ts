import {
  applyNodeChanges,
  type FitViewOptions,
  type NodeChange,
} from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type {
  DashboardActiveExecution,
  DashboardSnapshot,
} from "../../../api/dashboard/types";
import type { GraphLayout } from "../../flowchart/lib/layout";
import type { CurrentActivityNode } from "../../flowchart/public";
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
import {
  groupActiveExecutionsByWorkstationNodeID,
  useActiveExecutions,
} from "./react-flow-current-activity-card-active-executions";
import type { useCurrentActivityGraphEditor } from "./react-flow-current-activity-card-editor";
import { useCurrentActivityGraphLayoutForFactory } from "./react-flow-current-activity-card-graph-layout";

export type CurrentActivityGraphViewModelInput = {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
  locale?: string;
  now: number;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
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
  _snapshot: DashboardSnapshot,
): DashboardSnapshot["factory"] | null | undefined {
  if (editor.editableDefinitionQuery?.status !== "success") {
    return null;
  }

  if (!editor.editorMode) {
    return undefined;
  }

  return editorModeFactoryDefinition(editor) ?? null;
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

function useEditorCurrentActivityGraphLayout(
  snapshot: DashboardSnapshot,
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
) {
  return useCurrentActivityGraphLayoutForFactory(
    snapshot,
    currentActivityCardFactoryDefinition(editor, snapshot),
  );
}

export function useCurrentActivityGraphViewModel({
  editor,
  locale,
  now,
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
  const graphLayout = useEditorCurrentActivityGraphLayout(snapshot, editor);
  const graphKey = useMemo(
    () => currentActivityGraphKey(graphLayout),
    [graphLayout],
  );
  const storedNodePositions = useCurrentActivityGraphStore(
    (state) => state.positionsByGraphKey[graphKey] ?? EMPTY_NODE_POSITIONS,
  );
  const setStoredNodePosition = useCurrentActivityGraphStore(
    (state) => state.setNodePosition,
  );
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
    factoryDefinition:
      currentActivityCardFactoryDefinition(editor, snapshot) ?? undefined,
    graphLayout,
    locale,
    now,
    onSelectStateNode,
    onSelectWorkID,
    onSelectWorker,
    onSelectWorkType,
    onSelectWorkstation,
    selection,
    snapshot,
    storedNodePositions,
  });
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
  const edges = useMemo(
    () =>
      buildGraphEdges(
        activeGraphHighlights,
        handleAssignments,
        pendingAdditionEdgeIds,
        visibleGraphEdges,
      ),
    [
      activeGraphHighlights,
      handleAssignments,
      pendingAdditionEdgeIds,
      visibleGraphEdges,
    ],
  );
  const initialFitViewOptions = useMemo<FitViewOptions>(
    () => ({
      maxZoom: 1.15,
      minZoom: 0.7,
      nodes: initialFocusNodes(graphLayout),
      padding: 0.18,
    }),
    [graphLayout],
  );

  return {
    edges,
    graphKey,
    handleNodesChange,
    initialFitViewKey:
      initialFitViewOptions.nodes?.map((node) => node.id).join(":") ||
      "full-graph",
    initialFitViewOptions,
    nodes,
    setStoredNodePosition,
  };
}
