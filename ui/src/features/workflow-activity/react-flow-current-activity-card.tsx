import "@xyflow/react/dist/style.css";

import { applyNodeChanges, type FitViewOptions, type NodeChange } from "@xyflow/react";
import { useCallback, useEffect, useMemo, useState } from "react";

import type {
  DashboardActiveExecution,
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import type { FactoryValue } from "../../api/named-factory";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../components/ui/dashboard-typography";
import { cx } from "../../lib/cx";
import { FactoryGraphEditorDraftActions } from "../factory-graph-editor/factory-graph-editor-draft-actions";
import type { CurrentActivityNode } from "../flowchart/current-activity-nodes";
import { buildGraphLayout, type GraphLayout } from "../flowchart/layout";
import type { FactoryPngImportValue, ReadFactoryImportFile } from "../import";
import {
  type CurrentActivityImportController,
  useCurrentActivityImportController,
} from "./current-activity-import-controller";
import {
  groupActiveExecutionsByWorkstationNodeID,
  useActiveExecutions,
} from "./react-flow-current-activity-card-active-executions";
import {
  useCurrentActivityGraphEditor,
} from "./react-flow-current-activity-card-editor";
import { CurrentActivityGraphEditorHeader } from "./react-flow-current-activity-card-editor-chrome";
import { CurrentActivityGraphEditorDialogs } from "./react-flow-current-activity-card-editor-dialogs";
import { useFactoryGraphEditorViewModel } from "./react-flow-current-activity-card-editor-graph";
import {
  buildGraphEdges,
  initialFocusNodes,
} from "./react-flow-current-activity-card-edges";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_GRAPH_LAYOUT,
  EMPTY_NODE_POSITIONS,
} from "./react-flow-current-activity-card-graph";
import {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "./react-flow-current-activity-card-keys";
import { getWorkflowActivityShellMessages } from "./messages/activity-shell";
import { useCurrentActivityGraphStore } from "./state/currentActivityGraphStore";

export {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "./react-flow-current-activity-card-keys";

const GRAPH_LAYOUT_CACHE = new Map<string, GraphLayout>();
const GRAPH_LAYOUT_PROMISE_CACHE = new Map<string, Promise<GraphLayout>>();
const CURRENT_ACTIVITY_CARD_CLASS =
  "relative flex h-full min-h-0 min-w-0 flex-col rounded-3xl border border-af-overlay/10 bg-af-surface/72 p-4 shadow-af-panel backdrop-blur-lg md:p-5";
const CURRENT_ACTIVITY_HEADER_CLASS = "mb-4";
const CURRENT_ACTIVITY_TITLE_CLASS = cx("m-0", DASHBOARD_SECTION_HEADING_CLASS);

export type CurrentActivitySelection =
  | { kind: "node"; nodeId: string }
  | { kind: "state-node"; placeId: string }
  | { kind: "work-item"; dispatchId: string; nodeId: string; workID: string };

function CurrentActivityCardHeading({ locale }: { locale?: string }) {
  const messages = getWorkflowActivityShellMessages(locale);

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
  onSelectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
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
  const [graphLayout, setGraphLayout] =
    useState<GraphLayout>(EMPTY_GRAPH_LAYOUT);

  useEffect(() => {
    let cancelled = false;
    const cachedLayout = GRAPH_LAYOUT_CACHE.get(topologyKey);
    if (cachedLayout) {
      setGraphLayout(cachedLayout);
      return () => {
        cancelled = true;
      };
    }

    const inFlightLayout =
      GRAPH_LAYOUT_PROMISE_CACHE.get(topologyKey) ??
      buildGraphLayout(layoutTopology);
    GRAPH_LAYOUT_PROMISE_CACHE.set(topologyKey, inFlightLayout);

    inFlightLayout
      .then((layout) => {
        GRAPH_LAYOUT_CACHE.set(topologyKey, layout);
        GRAPH_LAYOUT_PROMISE_CACHE.delete(topologyKey);
        if (!cancelled) {
          setGraphLayout(layout);
        }
      })
      .catch(() => {
        GRAPH_LAYOUT_PROMISE_CACHE.delete(topologyKey);
        if (!cancelled) {
          setGraphLayout(EMPTY_GRAPH_LAYOUT);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [layoutTopology, topologyKey]);

  return graphLayout;
}

function useCurrentActivityBaseNodes({
  activeExecutionsByWorkstationNodeID,
  activeGraphHighlights,
  activeItemLabelsByPlaceId,
  graphLayout,
  handleAssignments,
  now,
  onSelectStateNode,
  onSelectWorkItem,
  onSelectWorkstation,
  selection,
  snapshot,
  storedNodePositions,
}: Pick<
  ReactFlowCurrentActivityCardProps,
  | "now"
  | "onSelectStateNode"
  | "onSelectWorkItem"
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
        onSelectWorkItem,
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
      onSelectWorkItem,
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
  onSelectWorkItem,
  onSelectWorkstation,
  selection,
  snapshot,
}: Pick<
  ReactFlowCurrentActivityCardProps,
  | "now"
  | "onSelectStateNode"
  | "onSelectWorkItem"
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
    onSelectWorkItem,
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
      <div className={CURRENT_ACTIVITY_HEADER_CLASS}>
        <CurrentActivityGraphEditorHeader
          editorMode={editor.editorMode}
          hasChanges={editor.draftState.hasChanges}
          isDefinitionLoading={
            editor.editableDefinitionQuery.status === "pending"
          }
          loadErrorMessage={editor.editableDefinitionQuery.error?.message}
          locale={props.locale}
          onToggle={editor.handleEditorModeToggle}
          title={<CurrentActivityCardHeading locale={props.locale} />}
        />
      </div>
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
