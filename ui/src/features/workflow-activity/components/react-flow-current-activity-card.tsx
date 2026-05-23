import "@xyflow/react/dist/style.css";

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
import type { FactoryValue } from "../../../api/named-factory";
import { DASHBOARD_PANEL_SHELL_CLASS } from "../../../components/ui/dashboard-shell";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import { FactoryGraphEditorDraftActions } from "../../factory-graph-editor/components/factory-graph-editor-draft-actions";
import type { CurrentActivityNode } from "../../flowchart/public";
import { buildGraphLayout, type GraphLayout } from "../../flowchart/lib/layout";
import type {
  FactoryPngImportValue,
  ReadFactoryImportFile,
} from "../../import/public";
import {
  type CurrentActivityImportController,
  useCurrentActivityImportController,
} from "../hooks/current-activity-import-controller";
import {
  groupActiveExecutionsByWorkstationNodeID,
  useActiveExecutions,
} from "../hooks/react-flow-current-activity-card-active-executions";
import { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import {
  createWorkflowTopologyAsyncCache,
  useWorkflowTopologyAsyncCache,
} from "../hooks/workflow-topology-async-cache";
import { CurrentActivityGraphHeaderActions } from "./react-flow-current-activity-card-editor-chrome";
import { CurrentActivityGraphEditorDialogs } from "./react-flow-current-activity-card-editor-dialogs";
import { useFactoryGraphEditorViewModel } from "../hooks/react-flow-current-activity-card-editor-graph";
import {
  buildGraphEdges,
  initialFocusNodes,
} from "../lib/react-flow-current-activity-card-edges";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_GRAPH_LAYOUT,
  EMPTY_NODE_POSITIONS,
} from "../lib/react-flow-current-activity-card-graph";
import {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "../lib/react-flow-current-activity-card-keys";
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { useCurrentActivityGraphStore } from "../state/currentActivityGraphStore";

export {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "../lib/react-flow-current-activity-card-keys";

const GRAPH_LAYOUT_CACHE = createWorkflowTopologyAsyncCache<GraphLayout>();
const CURRENT_ACTIVITY_CARD_CLASS = cn(
  DASHBOARD_PANEL_SHELL_CLASS,
  "relative flex h-full min-h-0 min-w-0 flex-col p-4 md:p-5",
);
const CURRENT_ACTIVITY_HEADER_CLASS = "mb-4";
const CURRENT_ACTIVITY_TITLE_CLASS = cn("m-0", DASHBOARD_SECTION_HEADING_CLASS);

export type CurrentActivitySelection =
  | { kind: "node"; nodeId: string }
  | { kind: "state-node"; placeId: string }
  | { kind: "work-item"; dispatchId: string; nodeId: string; workID: string };

function CurrentActivityCardHeading({
  hidden = false,
  locale,
}: {
  hidden?: boolean;
  locale?: string;
}) {
  const messages = getWorkflowActivityShellMessages(locale);

  if (hidden) {
    return (
      <span className="sr-only" id="workflow-graph-heading">
        {messages.title}
      </span>
    );
  }

  return (
    <div>
      <h2 className={CURRENT_ACTIVITY_TITLE_CLASS} id="workflow-graph-heading">
        {messages.title}
      </h2>
    </div>
  );
}
interface ReactFlowCurrentActivityCardProps {
  activateFactory?: (value: FactoryValue) => Promise<FactoryValue>;
  importController?: CurrentActivityImportController;
  locale?: string;
  now: number;
  onFactoryActivated?: () => void;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectWorkstation: (nodeId: string) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
}

function useGraphLayout(snapshot: DashboardSnapshot) {
  const topologyKey = useMemo(
    () => currentActivityTopologyKey(snapshot.topology),
    [snapshot.topology],
  );
  const layoutTopology = useMemo(() => snapshot.topology, [snapshot.topology]);

  return useWorkflowTopologyAsyncCache({
    cache: GRAPH_LAYOUT_CACHE,
    dependencies: [layoutTopology],
    fallbackValue: EMPTY_GRAPH_LAYOUT,
    initialValue: EMPTY_GRAPH_LAYOUT,
    loadLayout: () => buildGraphLayout(layoutTopology),
    mapResolvedLayout: identityGraphLayout,
    topologyKey,
  });
}

function identityGraphLayout(layout: GraphLayout) {
  return layout;
}

function useCurrentActivityBaseNodes({
  activeExecutionsByWorkstationNodeID,
  activeGraphHighlights,
  activeItemLabelsByPlaceId,
  graphLayout,
  handleAssignments,
  now,
  onSelectStateNode,
  onSelectWorkID,
  onSelectWorkstation,
  selection,
  snapshot,
  storedNodePositions,
}: Pick<
  ReactFlowCurrentActivityCardProps,
  | "now"
  | "onSelectStateNode"
  | "onSelectWorkID"
  | "onSelectWorkstation"
  | "selection"
  | "snapshot"
> & {
  activeExecutionsByWorkstationNodeID: Record<
    string,
    DashboardActiveExecution[]
  >;
  activeGraphHighlights: ReturnType<typeof buildActiveGraphHighlights>;
  activeItemLabelsByPlaceId: ReturnType<typeof buildActiveItemLabelsByPlaceId>;
  graphLayout: GraphLayout;
  handleAssignments: ReturnType<typeof buildHandleAssignments>;
  storedNodePositions: typeof EMPTY_NODE_POSITIONS;
}) {
  return useMemo<CurrentActivityNode[]>(
    () =>
      buildCurrentActivityNodes({
        activeExecutionsByWorkstationNodeID,
        activeGraphHighlights,
        activeItemLabelsByPlaceId,
        graphLayout,
        handleAssignments,
        now,
        onSelectStateNode,
        onSelectWorkID,
        onSelectWorkstation,
        selection,
        snapshot,
        storedNodePositions,
      }),
    [
      activeExecutionsByWorkstationNodeID,
      activeGraphHighlights,
      activeItemLabelsByPlaceId,
      graphLayout,
      handleAssignments,
      now,
      onSelectStateNode,
      onSelectWorkID,
      onSelectWorkstation,
      selection,
      snapshot,
      storedNodePositions,
    ],
  );
}

export function useCurrentActivityGraphViewModel({
  now,
  onSelectStateNode,
  onSelectWorkID,
  onSelectWorkstation,
  selection,
  snapshot,
}: Pick<
  ReactFlowCurrentActivityCardProps,
  | "now"
  | "onSelectStateNode"
  | "onSelectWorkID"
  | "onSelectWorkstation"
  | "selection"
  | "snapshot"
>) {
  const activeExecutions = useActiveExecutions(snapshot);
  const activeExecutionsByWorkstationNodeID = useMemo(
    () => groupActiveExecutionsByWorkstationNodeID(activeExecutions),
    [activeExecutions],
  );
  const graphLayout = useGraphLayout(snapshot);
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
  const visibleGraphEdges = useMemo(
    () => buildVisibleGraphEdges(graphLayout),
    [graphLayout],
  );
  const handleAssignments = useMemo(
    () => buildHandleAssignments(visibleGraphEdges),
    [visibleGraphEdges],
  );
  const activeGraphHighlights = useMemo(
    () => buildActiveGraphHighlights(activeExecutions, visibleGraphEdges),
    [activeExecutions, visibleGraphEdges],
  );
  const activeItemLabelsByPlaceId = useMemo(
    () => buildActiveItemLabelsByPlaceId(activeExecutions),
    [activeExecutions],
  );
  const baseNodes = useCurrentActivityBaseNodes({
    activeExecutionsByWorkstationNodeID,
    activeGraphHighlights,
    activeItemLabelsByPlaceId,
    graphLayout,
    handleAssignments,
    now,
    onSelectStateNode,
    onSelectWorkID,
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
        visibleGraphEdges,
      ),
    [activeGraphHighlights, handleAssignments, visibleGraphEdges],
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

export function ReactFlowCurrentActivityCard(
  props: ReactFlowCurrentActivityCardProps,
) {
  const editor = useCurrentActivityGraphEditor(props.snapshot);
  return (
    <ReactFlowCurrentActivityCardView
      {...props}
      editor={editor}
      showHeaderActions
    />
  );
}

export function ReactFlowCurrentActivityCardView(
  props: ReactFlowCurrentActivityCardProps & {
    editor: ReturnType<typeof useCurrentActivityGraphEditor>;
    showHeaderActions?: boolean;
  },
) {
  const { editor, showHeaderActions = false } = props;
  const graph = useCurrentActivityGraphViewModel(props);
  const editorGraph = useFactoryGraphEditorViewModel(
    editor,
    props.snapshot,
    props.locale,
  );
  const fallbackImportController = useCurrentActivityImportController({
    activateFactory: props.activateFactory,
    onFactoryActivated: props.onFactoryActivated,
    onFactoryImportReady: props.onFactoryImportReady,
    readFactoryImportFile: props.readFactoryImportFile,
  });
  const imports = props.importController ?? fallbackImportController;
  const shouldRenderImportPreviewDialog = props.importController === undefined;
  const readyImportPreviewState =
    imports.importPreviewState.status === "ready"
      ? imports.importPreviewState
      : null;

  return (
    <section
      aria-labelledby="workflow-graph-heading"
      className={CURRENT_ACTIVITY_CARD_CLASS}
    >
      {showHeaderActions ? (
        <div className={CURRENT_ACTIVITY_HEADER_CLASS}>
          <CurrentActivityGraphHeaderActions
            editorMode={editor.editorMode}
            editorUnavailableClassifierWorkstationName={
              editor.editorUnavailableClassifierWorkstationName
            }
            hasChanges={editor.draftState.hasChanges}
            isDefinitionLoading={
              editor.editableDefinitionQuery.status === "pending"
            }
            loadErrorMessage={editor.editableDefinitionQuery.error?.message}
            locale={props.locale}
            onToggle={editor.handleEditorModeToggle}
          />
        </div>
      ) : null}
      {showHeaderActions ? (
        <CurrentActivityCardHeading locale={props.locale} />
      ) : (
        <CurrentActivityCardHeading hidden locale={props.locale} />
      )}
      <FactoryGraphEditorDraftActions
        canSave={editor.canSaveDraft}
        description={editor.saveSummary.description}
        isSaving={editor.saveEditableDefinition.status === "pending"}
        locale={props.locale}
        onDiscard={editor.handleDiscardPendingChanges}
        onSave={() => {
          editor.setIsConfirmingSave(true);
        }}
        saveDisabledReason={editor.saveBlockedReason}
        visible={editor.editorMode && editor.draftState.hasChanges}
      />
      <CurrentActivityGraphSurface
        editor={editor}
        editorGraph={editorGraph}
        graph={graph}
        imports={imports}
        locale={props.locale}
        snapshot={props.snapshot}
      />
      <CurrentActivityGraphEditorDialogs
        editor={editor}
        imports={imports}
        locale={props.locale}
        readyImportPreviewState={readyImportPreviewState}
        shouldRenderImportPreviewDialog={shouldRenderImportPreviewDialog}
      />
    </section>
  );
}
