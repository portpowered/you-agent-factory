import type { FitViewOptions, NodeChange } from "@xyflow/react";
import { useCallback, useMemo, useState } from "react";

import type {
  DashboardActiveExecution,
  DashboardSnapshot,
} from "../../../api/dashboard/types";
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
import {
  groupActiveExecutionsByWorkstationNodeID,
  useActiveExecutions,
} from "./react-flow-current-activity-card-active-executions";
import type { CurrentActivityGraphFlowProjection } from "./use-current-activity-graph-flow-projection";

export type CurrentActivityGraphViewModelEditorInput = Omit<
  CurrentActivityEditorState,
  "onConnectionAnchorClick" | "validationTargets"
> & {
  graphState: CurrentActivityGraphFlowProjection;
  handleConnectionAnchorClick: CurrentActivityEditorState["onConnectionAnchorClick"];
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
) {
  const [selectedNodeIds, setSelectedNodeIds] = useState<ReadonlySet<string>>(
    () => new Set(),
  );

  const handleNodesChange = useCallback((changes: NodeChange[]) => {
    setSelectedNodeIds((currentSelectedNodeIds) => {
      let nextSelectedNodeIds: Set<string> | null = null;

      for (const change of changes) {
        if (
          change.type !== "select" &&
          change.type !== "remove" &&
          change.type !== "add" &&
          change.type !== "replace"
        ) {
          continue;
        }

        nextSelectedNodeIds ??= new Set(currentSelectedNodeIds);

        if (change.type === "select") {
          if (change.selected) {
            nextSelectedNodeIds.add(change.id);
          } else {
            nextSelectedNodeIds.delete(change.id);
          }
          continue;
        }

        if (change.type === "remove") {
          nextSelectedNodeIds.delete(change.id);
          continue;
        }

        const selected = change.item.selected === true;
        if (selected) {
          nextSelectedNodeIds.add(change.item.id);
        } else {
          nextSelectedNodeIds.delete(change.item.id);
        }
      }

      return nextSelectedNodeIds ?? currentSelectedNodeIds;
    });
  }, []);
  const displayNodes = useMemo(
    () =>
      baseNodes.map((node) => ({
        ...node,
        selected: selectedNodeIds.has(node.id),
      })),
    [baseNodes, selectedNodeIds],
  );

  return { displayNodes, handleNodesChange };
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
  const {
    canonicalLayoutViewport,
    displayFactoryDefinition,
    graphLayout,
    pendingAdditionEdgeIds,
    positionedGraphLayout,
    visibleGraphEdges,
  } = editor.graphState;
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
    canonicalLayoutViewport,
    edges,
    graphKey,
    handleNodesChange,
    initialFitViewKey:
      initialFitViewOptions.nodes?.map((node) => node.id).join(":") ||
      "full-graph",
    initialFitViewOptions,
    nodes: displayNodes,
  };
}
