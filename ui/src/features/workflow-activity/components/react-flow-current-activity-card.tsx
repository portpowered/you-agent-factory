import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  type FitViewOptions,
  type NodeChange,
} from "@xyflow/react";
import { useCallback, useEffect, useId, useMemo, useState } from "react";

import type {
  DashboardActiveExecution,
  DashboardSnapshot,
} from "../../../api/dashboard/types";
import type { FactoryValue } from "../../../api/named-factory";
import { DASHBOARD_PANEL_SHELL_CLASS } from "../../../components/ui/dashboard-shell";
import { DASHBOARD_SECTION_HEADING_CLASS } from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import type { GraphLayout } from "../../flowchart/lib/layout";
import type { CurrentActivityNode } from "../../flowchart/public";
import type { FactoryPngImportValue } from "../../import/lib/factory-png-import";
import type { ReadFactoryImportFile } from "../../import/public";
import {
  type CurrentActivityImportController,
  useCurrentActivityImportController,
} from "../hooks/current-activity-import-controller";
import {
  groupActiveExecutionsByWorkstationNodeID,
  useActiveExecutions,
} from "../hooks/react-flow-current-activity-card-active-executions";
import { useCurrentActivityGraphEditor } from "../hooks/react-flow-current-activity-card-editor";
import { useCurrentActivityGraphLayoutForFactory } from "../hooks/react-flow-current-activity-card-graph-layout";
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
import { getWorkflowActivityShellMessages } from "../messages/activity-shell";
import { useCurrentActivityGraphStore } from "../state/currentActivityGraphStore";
import { CurrentActivityGraphHeaderActions } from "./react-flow-current-activity-card-editor-chrome";
import { CurrentActivityGraphEditorDialogs } from "./react-flow-current-activity-card-editor-dialogs";
import { CurrentActivityGraphSaveNotifications } from "./react-flow-current-activity-card-save-notifications";
import { CurrentActivityGraphSurface } from "./react-flow-current-activity-card-surface";

export {
  currentActivityGraphKey,
  currentActivityTopologyKey,
} from "../lib/react-flow-current-activity-card-keys";

const CURRENT_ACTIVITY_CARD_CLASS = cn(
  DASHBOARD_PANEL_SHELL_CLASS,
  "relative flex h-full min-h-0 min-w-0 flex-col",
);
const CURRENT_ACTIVITY_HEADER_CLASS = "mb-4";
const CURRENT_ACTIVITY_TITLE_CLASS = cn("m-0", DASHBOARD_SECTION_HEADING_CLASS);

export type CurrentActivitySelection =
  | { kind: "node"; nodeId: string }
  | { kind: "state-node"; placeId: string }
  | { kind: "work-item"; dispatchId: string; nodeId: string; workID: string };

function CurrentActivityCardHeading({
  headingID,
  hidden = false,
  locale,
}: {
  headingID: string;
  hidden?: boolean;
  locale?: string;
}) {
  const messages = getWorkflowActivityShellMessages(locale);

  if (hidden) {
    return (
      <span className="sr-only" id={headingID}>
        {messages.title}
      </span>
    );
  }

  return (
    <div>
      <h2 className={CURRENT_ACTIVITY_TITLE_CLASS} id={headingID}>
        {messages.title}
      </h2>
    </div>
  );
}

function useCurrentActivityAccessibilityIDs(widgetInstanceID?: string) {
  const fallbackID = useId();

  return {
    headingID: `workflow-graph-heading-${widgetInstanceID ?? fallbackID}`,
  };
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
  widgetInstanceID?: string;
}

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
        },
        factoryDefinition,
        graphLayout,
        locale,
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
      editor,
      factoryDefinition,
      graphLayout,
      locale,
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
  editor,
  locale,
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
  | "locale"
  | "snapshot"
> & {
  editor: ReturnType<typeof useCurrentActivityGraphEditor>;
}) {
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
      editor.editorMode && hasPendingGraphEntityShapeChanges(editor)
        ? (editor.draftState.pendingFactoryDefinition ?? undefined)
        : snapshot.factory,
    graphLayout,
    locale,
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

function useEditorCurrentActivityGraphLayout(
  snapshot: DashboardSnapshot,
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
) {
  return useCurrentActivityGraphLayoutForFactory(
    snapshot,
    editor.editorMode && hasPendingGraphEntityShapeChanges(editor)
      ? (editor.draftState.pendingFactoryDefinition ?? undefined)
      : undefined,
  );
}

function hasPendingGraphEntityShapeChanges(
  editor: ReturnType<typeof useCurrentActivityGraphEditor>,
) {
  const { additions, removals } = editor.draftState.draft;
  return (
    additions.resources.length > 0 ||
    additions.workers.length > 0 ||
    additions.workStates.length > 0 ||
    additions.workTypes.length > 0 ||
    additions.workstations.length > 0 ||
    removals.resources.length > 0 ||
    removals.workers.length > 0 ||
    removals.workStates.length > 0 ||
    removals.workTypes.length > 0 ||
    removals.workstations.length > 0
  );
}

export function ReactFlowCurrentActivityCard(
  props: ReactFlowCurrentActivityCardProps,
) {
  const editor = useCurrentActivityGraphEditor(props.snapshot, props.locale);
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
  const { headingID } = useCurrentActivityAccessibilityIDs(
    props.widgetInstanceID,
  );
  const graph = useCurrentActivityGraphViewModel({ ...props, editor });
  const fallbackImportController = useCurrentActivityImportController({
    activateFactory: props.activateFactory,
    locale: props.locale,
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
      aria-labelledby={headingID}
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
        <CurrentActivityCardHeading
          headingID={headingID}
          locale={props.locale}
        />
      ) : (
        <CurrentActivityCardHeading
          headingID={headingID}
          hidden
          locale={props.locale}
        />
      )}
      <CurrentActivityGraphSurface
        editor={editor}
        graph={graph}
        headingID={headingID}
        imports={imports}
        locale={props.locale}
        snapshot={props.snapshot}
      />
      <CurrentActivityGraphSaveNotifications
        editor={editor}
        locale={props.locale}
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
