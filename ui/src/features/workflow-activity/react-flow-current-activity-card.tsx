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
import { FactoryGraphEditorAddEntityDialog } from "../factory-graph-editor/factory-graph-editor-add-dialog";
import {
  FactoryGraphEditorConfirmationDialog,
  FactoryGraphEditorLeaveDialog,
} from "../factory-graph-editor/factory-graph-editor-controls";
import type { CurrentActivityNode } from "../flowchart/current-activity-nodes";
import { buildGraphLayout, type GraphLayout } from "../flowchart/layout";
import {
  FactoryImportPreviewDialog,
  type FactoryPngImportValue,
  type ReadFactoryImportFile,
} from "../import";
import {
  type CurrentActivityImportController,
  useCurrentActivityImportController,
} from "./current-activity-import-controller";
import {
  groupActiveExecutionsByWorkstationNodeID,
  useActiveExecutions,
} from "./react-flow-current-activity-card-active-executions";
import {
  CurrentActivityGraphEditorHeader,
  useCurrentActivityGraphEditor,
} from "./react-flow-current-activity-card-editor";
import { useFactoryGraphEditorViewModel } from "./react-flow-current-activity-card-editor-graph";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";
import {
  buildActiveGraphHighlights,
  buildActiveItemLabelsByPlaceId,
  buildCurrentActivityNodes,
  buildGraphEdges,
  buildHandleAssignments,
  buildVisibleGraphEdges,
  EMPTY_GRAPH_LAYOUT,
  EMPTY_NODE_POSITIONS,
  initialFocusNodes,
} from "./react-flow-current-activity-card-graph";
import { GraphImportErrorPanel } from "./react-flow-current-activity-card-import";
import {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "./react-flow-current-activity-card-keys";
import { useCurrentActivityGraphStore } from "./state/currentActivityGraphStore";

export {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "./react-flow-current-activity-card-keys";

const GRAPH_LAYOUT_CACHE = new Map<string, GraphLayout>();
const GRAPH_LAYOUT_PROMISE_CACHE = new Map<string, Promise<GraphLayout>>();
const CURRENT_ACTIVITY_CARD_CLASS =
  "relative flex h-full min-h-0 min-w-0 flex-col rounded-3xl border border-af-overlay/10 bg-af-surface/72 p-[1.2rem] shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4";
const CURRENT_ACTIVITY_HEADER_CLASS = "mb-4";
const CURRENT_ACTIVITY_EYEBROW_CLASS =
  "mb-[0.65rem] text-xs font-bold uppercase tracking-[0.16em] text-af-accent";
const CURRENT_ACTIVITY_TITLE_CLASS = cx("m-0", DASHBOARD_SECTION_HEADING_CLASS);
const CURRENT_ACTIVITY_HEADER_TEXT_CLASS = "grid gap-2";

export type CurrentActivitySelection =
  | { kind: "node"; nodeId: string }
  | { kind: "state-node"; placeId: string }
  | { kind: "work-item"; dispatchId: string; nodeId: string; workID: string };

function CurrentActivityCardHeading() {
  return (
    <div className={CURRENT_ACTIVITY_HEADER_TEXT_CLASS}>
      <p className={CURRENT_ACTIVITY_EYEBROW_CLASS}>Operator View</p>
      <h2 className={CURRENT_ACTIVITY_TITLE_CLASS} id="workflow-graph-heading">
        Current activity
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
  const editor = useCurrentActivityGraphEditor(props.snapshot.topology);
  const graph = useCurrentActivityGraphViewModel(props);
  const editorGraph = useFactoryGraphEditorViewModel(editor);
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
          onToggle={editor.handleEditorModeToggle}
          title={<CurrentActivityCardHeading />}
        />
      </div>
      <CurrentActivityGraphSurface
        editor={editor}
        editorGraph={editorGraph}
        graph={graph}
        imports={imports}
        locale={props.locale}
        snapshot={props.snapshot}
      />
      {shouldRenderImportPreviewDialog && readyImportPreviewState ? (
        <FactoryImportPreviewDialog
          activationState={imports.activationState}
          locale={props.locale}
          onCancel={() => {
            imports.clearActivationError();
            imports.closeImportPreview();
          }}
          onConfirm={() => {
            void imports.activateImport(readyImportPreviewState.value);
          }}
          previewState={readyImportPreviewState}
        />
      ) : null}
      {imports.dropState.status === "error" ? (
        <GraphImportErrorPanel
          error={imports.dropState.error}
          fileName={imports.dropState.fileName}
          locale={props.locale}
          onDismiss={imports.clearError}
        />
      ) : null}
      <FactoryGraphEditorLeaveDialog
        canSave={editor.canSaveDraft}
        isOpen={editor.leaveDialogOpen}
        isSaving={editor.saveEditableDefinition.status === "pending"}
        onCancel={() => {
          if (editor.saveEditableDefinition.status !== "pending") {
            editor.setIsConfirmingLeaveEditor(false);
          }
        }}
        onDiscard={editor.handleDiscardEditorChanges}
        onSave={() => {
          void editor.handleSaveBeforeLeavingEditor();
        }}
      />
      <FactoryGraphEditorConfirmationDialog
        cancelLabel="Cancel removal"
        confirmLabel={editor.pendingRemovalIntent?.confirmLabel ?? "Delete entity"}
        confirmTone="destructive"
        description={
          editor.pendingRemovalIntent?.confirmDescription ??
          "Remove this graph entity from the current draft."
        }
        isOpen={editor.pendingRemovalIntent !== null}
        onCancel={() => {
          editor.setPendingRemovalEdgeId(null);
          editor.setPendingRemovalNodeId(null);
        }}
        onConfirm={editor.handleConfirmRemoval}
        title={editor.pendingRemovalIntent?.title ?? "Remove graph entity?"}
      />
      <FactoryGraphEditorAddEntityDialog
        currentFactoryDefinition={editor.currentFactoryDefinition}
        draft={editor.addEntityDraft}
        errors={editor.addEntityErrors}
        isOpen={editor.addEntityDraft !== null}
        onChange={(draft) => {
          editor.setAddEntityDraft(draft);
          editor.setAddEntityErrors({});
        }}
        onClose={() => {
          editor.setAddEntityDraft(null);
          editor.setAddEntityErrors({});
        }}
        onSubmit={editor.handleAddEntitySubmit}
      />
    </section>
  );
}
