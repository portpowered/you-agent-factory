import "@xyflow/react/dist/style.css";

import {
  applyNodeChanges,
  Background,
  Controls,
  MarkerType,
  ReactFlow,
  type Edge,
  type FitViewOptions,
  type NodeChange,
} from "@xyflow/react";
import type { CSSProperties } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";

import {
  type FactoryValue,
  type NamedFactoryAPIError,
} from "../../api/named-factory";
import {
  DashboardButton,
  DashboardMessagePanel,
  DashboardMutationDialog,
} from "../../components/dashboard";
import { cx } from "../../components/dashboard/classnames";
import { EMPTY_STATE_CLASS } from "../../components/dashboard/widget-board";
import {
  type GraphNodePosition,
  type GraphNodePositions,
  useCurrentActivityGraphStore,
} from "../../state/currentActivityGraphStore";
import {
  type FactoryImportPreviewState,
  type FactoryImportActivationState,
  type FactoryPngDropState,
  type FactoryPngImportValue,
  type ReadFactoryImportFile,
  type ReadFactoryImportPngError,
  useFactoryImportActivation,
  useFactoryImportPreview,
  useFactoryPngDrop,
} from "../import";
import {
  DashboardFlowAxisLegend,
  DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS,
  DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS,
} from "./dashboard-flow-axis-legend";
import {
  CURRENT_ACTIVITY_NODE_TYPES,
  type CurrentActivityNode,
} from "../flowchart/current-activity-nodes";
import { buildGraphLayout } from "../flowchart/layout";
import type {
  GraphLayout,
  PositionedEdge,
  PositionedPlaceNode,
  PositionedWorkstationNode,
} from "../flowchart/layout";
import type {
  DashboardActiveExecution,
  DashboardSnapshot,
  DashboardTopology,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";

const EDGE_STROKE_MUTED = "var(--color-af-edge-muted)";
const EDGE_STROKE_SOFT = "var(--color-af-edge-muted-soft)";
const EDGE_STROKE_DANGER_MUTED = "var(--color-af-edge-danger-muted)";
const EDGE_STROKE_ACTIVE = "var(--color-af-success)";
const GRAPH_BACKGROUND_COLOR = "var(--color-af-edge-muted-soft)";
const GRAPH_BACKGROUND_GAP = 24;
const GRAPH_BACKGROUND_SIZE = 1;
const GRAPH_DROP_HINT = "Drop a Port OS factory PNG onto this graph to start import.";
const GRAPH_IMPORT_ERROR_TITLE = "Factory import failed";
const GRAPH_IMPORT_LOADING_TITLE = "Validating factory PNG";
const GRAPH_IMPORT_PREVIEW_TITLE = "Review factory import";

type CSSPropertiesWithVariables = CSSProperties & Record<`--${string}`, string | number>;

const GRAPH_CONTROLS_STYLE: CSSPropertiesWithVariables = {
  "--xy-controls-box-shadow": "none",
  "--xy-controls-button-background-color-props":
    "rgb(from var(--color-af-surface) r g b / 0.94)",
  "--xy-controls-button-background-color-hover-props":
    "rgb(from var(--color-af-overlay) r g b / 0.1)",
  "--xy-controls-button-border-color-props":
    "rgb(from var(--color-af-overlay) r g b / 0.08)",
  "--xy-controls-button-color-props": "rgb(from var(--color-af-ink) r g b / 0.72)",
  "--xy-controls-button-color-hover-props": "var(--color-af-ink)",
  backgroundColor: "rgb(from var(--color-af-surface) r g b / 0.88)",
  border: "1px solid rgb(from var(--color-af-overlay) r g b / 0.08)",
  borderRadius: 8,
  overflow: "hidden",
};

export type CurrentActivitySelection =
  | { kind: "node"; nodeId: string }
  | { kind: "state-node"; placeId: string }
  | { kind: "work-item"; dispatchId: string; nodeId: string; workID: string };

interface ReactFlowCurrentActivityCardProps {
  activateFactory?: (value: FactoryValue) => Promise<FactoryValue>;
  onFactoryActivated?: () => void;
  now: number;
  onFactoryImportReady?: (value: FactoryPngImportValue, file: File) => void;
  selection: CurrentActivitySelection | null;
  snapshot: DashboardSnapshot;
  onSelectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
  ) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  readFactoryImportFile?: ReadFactoryImportFile;
}

function edgeIsFailure(edge: PositionedEdge): boolean {
  return edge.outcomeKind === "failed" || edge.stateCategory === "FAILED";
}

function edgeTouchesResource(edge: PositionedEdge): boolean {
  return edge.sourcePlaceKind === "resource" || edge.targetPlaceKind === "resource";
}

function edgeReturnsToResource(edge: PositionedEdge): boolean {
  return edge.targetPlaceKind === "resource";
}

function edgeStyle(edge: PositionedEdge, activeFlow: boolean, muted: boolean): Edge["style"] {
  if (activeFlow) {
    return {
      stroke: EDGE_STROKE_ACTIVE,
      strokeDasharray: "8 6",
      strokeWidth: 2.8,
    };
  }

  const opacity = muted ? 0.34 : undefined;

  if (edge.sourcePlaceKind === "resource") {
    return {
      opacity,
      stroke: EDGE_STROKE_SOFT,
      strokeDasharray: "2 7",
      strokeWidth: 1.5,
    };
  }
  if (edge.outcomeKind === "accepted") {
    return { opacity, stroke: EDGE_STROKE_MUTED, strokeWidth: 1.6 };
  }
  if (edgeIsFailure(edge)) {
    return {
      opacity: muted ? 0.68 : undefined,
      stroke: EDGE_STROKE_DANGER_MUTED,
      strokeDasharray: "3 6",
      strokeWidth: 1.8,
    };
  }
  return {
    opacity,
    stroke: EDGE_STROKE_MUTED,
    strokeDasharray: "8 8",
    strokeWidth: 1.6,
  };
}

function edgeMarkerColor(edge: PositionedEdge, activeFlow: boolean): string {
  if (activeFlow) {
    return EDGE_STROKE_ACTIVE;
  }
  if (edgeIsFailure(edge)) {
    return EDGE_STROKE_DANGER_MUTED;
  }
  return edge.sourcePlaceKind === "resource" ? EDGE_STROKE_SOFT : EDGE_STROKE_MUTED;
}

function edgeSemantic(edge: PositionedEdge): boolean {
  return edge.outcomeKind !== "accepted" || edgeIsFailure(edge);
}

function edgeLabel(edge: PositionedEdge, activeFlow: boolean): string | undefined {
  if (activeFlow) {
    return edge.label || undefined;
  }
  return undefined;
}

function activeTokenLabel(execution: DashboardActiveExecution, workID: string, fallbackID: string): string {
  const workItem = execution.work_items?.find((item) => item.work_id === workID);
  return workItem?.display_name || workItem?.work_id || fallbackID;
}

const EMPTY_GRAPH_LAYOUT: GraphLayout = { edges: [], height: 0, nodes: [], width: 0 };
const EMPTY_NODE_POSITIONS: GraphNodePositions = {};
const GRAPH_LAYOUT_CACHE = new Map<string, GraphLayout>();
const GRAPH_LAYOUT_PROMISE_CACHE = new Map<string, Promise<GraphLayout>>();

interface HandleAssignments {
  incomingHandleCounts: Map<string, number>;
  outgoingHandleCounts: Map<string, number>;
  sourceHandlesByEdgeId: Map<string, string>;
  targetHandlesByEdgeId: Map<string, string>;
}

interface ActiveGraphHighlights {
  activeEdgeIds: ReadonlySet<string>;
  activePlaceNodeIds: ReadonlySet<string>;
  activeWorkstationNodeIds: ReadonlySet<string>;
  hasActiveFlow: boolean;
  relatedNodeIds: ReadonlySet<string>;
}

function workstationGraphNodeId(nodeId: string): string {
  return `workstation:${nodeId}`;
}

function placeGraphNodeId(placeId: string): string {
  return `place:${placeId}`;
}

function buildHandleAssignments(edges: PositionedEdge[]): HandleAssignments {
  const incomingHandleCounts = new Map<string, number>();
  const outgoingHandleCounts = new Map<string, number>();
  const sourceHandlesByEdgeId = new Map<string, string>();
  const targetHandlesByEdgeId = new Map<string, string>();

  for (const edge of edges) {
    const sourceIndex = outgoingHandleCounts.get(edge.fromNodeId) ?? 0;
    const targetIndex = incomingHandleCounts.get(edge.toNodeId) ?? 0;

    sourceHandlesByEdgeId.set(edge.edgeId, `out-${sourceIndex}`);
    targetHandlesByEdgeId.set(edge.edgeId, `in-${targetIndex}`);
    outgoingHandleCounts.set(edge.fromNodeId, sourceIndex + 1);
    incomingHandleCounts.set(edge.toNodeId, targetIndex + 1);
  }

  return {
    incomingHandleCounts,
    outgoingHandleCounts,
    sourceHandlesByEdgeId,
    targetHandlesByEdgeId,
  };
}

function buildActiveGraphHighlights(
  activeExecutions: DashboardActiveExecution[],
  edges: PositionedEdge[],
): ActiveGraphHighlights {
  const activeEdgeIds = new Set<string>();
  const activePlaceNodeIds = new Set<string>();
  const activeWorkstationNodeIds = new Set<string>();
  const consumedPlaceNodeIds = new Set<string>();
  const relatedNodeIds = new Set<string>();

  for (const execution of activeExecutions) {
    const workstationNodeId = workstationGraphNodeId(execution.workstation_node_id);
    activeWorkstationNodeIds.add(workstationNodeId);
    relatedNodeIds.add(workstationNodeId);

    for (const token of execution.consumed_tokens ?? []) {
      const placeNodeId = placeGraphNodeId(token.place_id);
      consumedPlaceNodeIds.add(placeNodeId);
      relatedNodeIds.add(placeNodeId);
    }
  }

  for (const edge of edges) {
    const resourceEdge = edgeTouchesResource(edge);
    const flowsIntoActiveWorkstation =
      !resourceEdge &&
      activeWorkstationNodeIds.has(edge.toNodeId) &&
      consumedPlaceNodeIds.has(edge.fromNodeId);
    const flowsOutOfActiveWorkstation =
      !resourceEdge &&
      activeWorkstationNodeIds.has(edge.fromNodeId) &&
      !edgeIsFailure(edge);

    if (!flowsIntoActiveWorkstation && !flowsOutOfActiveWorkstation) {
      continue;
    }

    activeEdgeIds.add(edge.edgeId);
    relatedNodeIds.add(edge.fromNodeId);
    relatedNodeIds.add(edge.toNodeId);

    if (flowsOutOfActiveWorkstation) {
      activePlaceNodeIds.add(edge.toNodeId);
    }
  }

  return {
    activeEdgeIds,
    activePlaceNodeIds,
    activeWorkstationNodeIds,
    hasActiveFlow: activeExecutions.length > 0,
    relatedNodeIds,
  };
}

function initialFocusNodes(graphLayout: GraphLayout): FitViewOptions["nodes"] | undefined {
  const initialPlace = graphLayout.nodes
    .filter(
      (node): node is PositionedPlaceNode =>
        node.nodeKind !== "workstation" && node.place.state_category === "INITIAL",
    )
    .sort((left, right) => left.x - right.x || left.y - right.y)[0];

  if (!initialPlace) {
    return undefined;
  }

  const firstConnectedWorkstation = graphLayout.edges
    .filter((edge) => edge.fromNodeId === initialPlace.nodeId)
    .map((edge) => graphLayout.nodes.find((node) => node.nodeId === edge.toNodeId))
    .filter((node): node is PositionedWorkstationNode => node?.nodeKind === "workstation")
    .sort((left, right) => left.x - right.x || left.y - right.y)[0];

  return [initialPlace, firstConnectedWorkstation]
    .filter((node): node is PositionedPlaceNode | PositionedWorkstationNode => node !== undefined)
    .map((node) => ({ id: node.nodeId }));
}

function graphDropStateAttribute(dropState: FactoryPngDropState): string {
  return dropState.status;
}

function graphDropOverlayCopy(dropState: FactoryPngDropState): { message: string; title: string } | null {
  switch (dropState.status) {
    case "drag-active":
      return {
        message: GRAPH_DROP_HINT,
        title: "Import factory PNG",
      };
    case "reading":
      return {
        message: `${dropState.fileName} is being parsed and validated locally before import continues.`,
        title: GRAPH_IMPORT_LOADING_TITLE,
      };
    default:
      return null;
  }
}

function graphImportErrorCopy(error: ReadFactoryImportPngError): string {
  switch (error.code) {
    case "NOT_PNG_FILE":
      return "Drop a PNG image exported by Port OS Agent Factory.";
    case "PNG_METADATA_MISSING":
      return "This PNG does not include the Port OS factory metadata needed for import.";
    case "UNSUPPORTED_SCHEMA_VERSION":
      return error.details?.schemaVersion
        ? `This PNG uses unsupported Port OS factory metadata version ${error.details.schemaVersion}.`
        : "This PNG uses an unsupported Port OS factory metadata version.";
    case "PNG_METADATA_INVALID":
    case "FACTORY_PAYLOAD_INVALID":
      return "The embedded Port OS factory metadata is invalid, so the current factory was left unchanged.";
    case "IMAGE_DECODE_FAILED":
    case "PREVIEW_UNAVAILABLE":
      return "The browser could not validate this PNG for import preview, so the current factory was left unchanged.";
    case "FILE_READ_FAILED":
      return "The browser could not read the dropped file. Try dropping the PNG again.";
    case "PNG_INVALID":
      return "This PNG appears truncated or malformed, so import stopped before any activation request.";
    default:
      return error.message;
  }
}

interface GraphDropOverlayProps {
  dropState: FactoryPngDropState;
}

function GraphDropOverlay({ dropState }: GraphDropOverlayProps) {
  const copy = graphDropOverlayCopy(dropState);
  if (!copy) {
    return null;
  }

  return (
    <div
      className="pointer-events-none absolute inset-4 z-10 grid place-items-center rounded-2xl border border-dashed border-af-accent/45 bg-af-surface/92 p-5 text-center shadow-af-panel backdrop-blur-[18px]"
      data-current-activity-drop-overlay={dropState.status}
    >
      <div className="grid max-w-sm gap-2">
        <p className="mb-0 text-xs font-bold uppercase tracking-[0.16em] text-af-accent">
          {copy.title}
        </p>
        <p className="m-0 text-sm text-af-ink/84">{copy.message}</p>
      </div>
    </div>
  );
}

interface GraphImportErrorPanelProps {
  error: ReadFactoryImportPngError;
  fileName: string;
  onDismiss: () => void;
}

function GraphImportErrorPanel({
  error,
  fileName,
  onDismiss,
}: GraphImportErrorPanelProps) {
  return (
    <DashboardMessagePanel
      action={
        <DashboardButton onClick={onDismiss} tone="secondary" type="button">
          Dismiss
        </DashboardButton>
      }
      ariaLive="assertive"
      className="mt-4 min-h-0 px-5 py-4"
      compact={true}
      role="alert"
      title={GRAPH_IMPORT_ERROR_TITLE}
      tone="error"
    >
      <p className="m-0">
        <span className="font-semibold">{fileName}</span>
        {" "}
        {graphImportErrorCopy(error)}
      </p>
    </DashboardMessagePanel>
  );
}

type ReadyFactoryImportPreviewState = Extract<FactoryImportPreviewState, { status: "ready" }>;

interface FactoryImportPreviewDialogProps {
  activationState: FactoryImportActivationState;
  onCancel: () => void;
  onConfirm: () => void;
  previewState: ReadyFactoryImportPreviewState;
}

function FactoryImportPreviewDialog({
  activationState,
  onCancel,
  onConfirm,
  previewState,
}: FactoryImportPreviewDialogProps) {
  const isSubmitting = activationState.status === "submitting";

  return (
    <DashboardMutationDialog
      closeDisabled={isSubmitting}
      closeLabel="Close import preview"
      description={
        <>
          Review the dropped factory before activation. Confirming this import in the next
          step will switch the current factory to{" "}
          <span className="font-semibold text-af-ink">{previewState.value.factoryName}</span>.
        </>
      }
      footer={
        <>
          <DashboardButton
            disabled={isSubmitting}
            onClick={onCancel}
            tone="secondary"
            type="button"
          >
            Cancel import
          </DashboardButton>
          <DashboardButton
            busy={isSubmitting}
            disabled={isSubmitting}
            onClick={onConfirm}
            type="button"
          >
            {isSubmitting ? "Activating factory..." : "Activate factory"}
          </DashboardButton>
        </>
      }
      media={
        <div className="overflow-hidden rounded-[1.25rem] border border-af-overlay/10 bg-af-overlay/4 p-3">
          <img
            alt={`${previewState.value.factoryName} preview image`}
            className="block h-full max-h-[24rem] w-full rounded-[1rem] object-contain"
            src={previewState.value.previewImageSrc}
          />
        </div>
      }
      onClose={onCancel}
      overlayClassName="absolute inset-0 z-20 bg-af-ink/16 backdrop-blur-[6px]"
      title={GRAPH_IMPORT_PREVIEW_TITLE}
    >
      <p className="m-0 text-base font-semibold text-af-ink">{previewState.value.factoryName}</p>

      <dl className="grid gap-3 rounded-[1.1rem] border border-af-overlay/10 bg-af-overlay/4 p-4 text-sm text-af-ink/80">
        <div className="grid gap-1">
          <dt className="text-[0.7rem] font-bold uppercase tracking-[0.14em] text-af-accent">
            Dropped file
          </dt>
          <dd className="m-0 font-semibold text-af-ink">{previewState.file.name}</dd>
        </div>
        <div className="grid gap-1">
          <dt className="text-[0.7rem] font-bold uppercase tracking-[0.14em] text-af-accent">
            Embedded factory
          </dt>
          <dd className="m-0 font-semibold text-af-ink">{previewState.value.factoryName}</dd>
        </div>
      </dl>

      {activationState.status === "error" ? (
        <FactoryImportActivationErrorPanel error={activationState.error} />
      ) : null}
    </DashboardMutationDialog>
  );
}

function factoryImportActivationErrorCopy(error: NamedFactoryAPIError): string {
  switch (error.code) {
    case "FACTORY_ALREADY_EXISTS":
      return "A factory with this name already exists. Rename or remove the existing factory before importing this PNG.";
    case "FACTORY_NOT_IDLE":
      return "The current factory runtime is still active. Wait until it becomes idle before switching factories.";
    case "INVALID_FACTORY":
      return "The dropped factory payload was rejected by the activation API.";
    case "INVALID_FACTORY_NAME":
      return "The embedded factory name is not valid for activation.";
    case "NETWORK_ERROR":
      return "The dashboard could not reach the activation API. Try again once the connection is available.";
    default:
      return error.message;
  }
}

interface FactoryImportActivationErrorPanelProps {
  error: NamedFactoryAPIError;
}

function FactoryImportActivationErrorPanel({
  error,
}: FactoryImportActivationErrorPanelProps) {
  return (
    <DashboardMessagePanel ariaLive="assertive" role="alert" title="Activation failed" tone="error">
      <p className="m-0">{factoryImportActivationErrorCopy(error)}</p>
    </DashboardMessagePanel>
  );
}

export function currentActivityGraphKey(graphLayout: GraphLayout): string {
  if (graphLayout.nodes.length === 0) {
    return "";
  }

  const nodeIds = graphLayout.nodes.map((node) => node.nodeId).sort().join("|");
  const edgeIds = graphLayout.edges.map((edge) => edge.edgeId).sort().join("|");
  return `${nodeIds}::${edgeIds}`;
}

export function currentActivityTopologyKey(topology: DashboardTopology): string {
  return JSON.stringify({
    edges: [...(topology.edges ?? [])]
      .map((edge) => ({
        from_node_id: edge.from_node_id,
        outcome_kind: edge.outcome_kind ?? "accepted",
        state_category: edge.state_category ?? "",
        state_value: edge.state_value ?? "",
        to_node_id: edge.to_node_id,
        via_place_id: edge.via_place_id,
        work_type_id: edge.work_type_id ?? "",
      }))
      .sort((left, right) =>
        JSON.stringify(left).localeCompare(JSON.stringify(right)),
      ),
    workstations: [...topology.workstation_node_ids]
      .sort()
      .map((nodeId) => {
        const workstation = topology.workstation_nodes_by_id[nodeId];
        return {
          input_places: [...(workstation?.input_places ?? [])]
            .map((place) => ({
              kind: place.kind,
              place_id: place.place_id,
              state_category: place.state_category ?? "",
              state_value: place.state_value ?? "",
              type_id: place.type_id ?? "",
            }))
            .sort((left, right) => left.place_id.localeCompare(right.place_id)),
          node_id: workstation?.node_id ?? nodeId,
          output_places: [...(workstation?.output_places ?? [])]
            .map((place) => ({
              kind: place.kind,
              place_id: place.place_id,
              state_category: place.state_category ?? "",
              state_value: place.state_value ?? "",
              type_id: place.type_id ?? "",
            }))
            .sort((left, right) => left.place_id.localeCompare(right.place_id)),
          transition_id: workstation?.transition_id ?? "",
          workstation_kind: workstation?.workstation_kind ?? "",
          workstation_name: workstation?.workstation_name ?? "",
        };
      }),
  });
}

function finitePosition(position: GraphNodePosition | undefined): position is GraphNodePosition {
  return (
    position !== undefined &&
    Number.isFinite(position.x) &&
    Number.isFinite(position.y)
  );
}

function nodePosition(
  nodeId: string,
  fallback: GraphNodePosition,
  storedPositions: GraphNodePositions,
): GraphNodePosition {
  const storedPosition = storedPositions[nodeId];
  return finitePosition(storedPosition) ? storedPosition : fallback;
}

export function ReactFlowCurrentActivityCard({
  activateFactory,
  onFactoryActivated,
  now,
  onFactoryImportReady,
  selection,
  snapshot,
  onSelectWorkItem,
  onSelectStateNode,
  onSelectWorkstation,
  readFactoryImportFile,
}: ReactFlowCurrentActivityCardProps) {
  const {
    closePreview: closeImportPreview,
    openPreview,
    previewState: importPreviewState,
  } = useFactoryImportPreview({
    onPreviewReady: onFactoryImportReady,
  });
  const handleFactoryActivated = useCallback(() => {
    closeImportPreview();
    onFactoryActivated?.();
  }, [closeImportPreview, onFactoryActivated]);
  const {
    activateImport,
    activationState,
    clearActivationError,
  } = useFactoryImportActivation({
    activateFactory,
    onActivated: handleFactoryActivated,
  });
  const handleImportPreviewReady = useCallback((value: FactoryPngImportValue, file: File) => {
    clearActivationError();
    openPreview(value, file);
  }, [clearActivationError, openPreview]);
  const {
    clearError: clearImportError,
    dropState,
    onDragEnter,
    onDragLeave,
    onDragOver,
    onDrop,
  } = useFactoryPngDrop({
    onImportReady: handleImportPreviewReady,
    readFactoryImportFile,
  });
  const activeExecutions = useMemo(
    () =>
      snapshot.runtime.active_dispatch_ids
        ?.map((dispatchId) => snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId])
        .filter((execution): execution is DashboardActiveExecution => execution !== undefined) ??
      [],
    [snapshot.runtime.active_dispatch_ids, snapshot.runtime.active_executions_by_dispatch_id],
  );
  const activeExecutionsByWorkstationNodeID = useMemo(
    () =>
      activeExecutions.reduce<Record<string, DashboardActiveExecution[]>>((accumulator, execution) => {
        const executions = accumulator[execution.workstation_node_id] ?? [];
        accumulator[execution.workstation_node_id] = [...executions, execution];
        return accumulator;
      }, {}),
    [activeExecutions],
  );
  const topologyKey = useMemo(
    () => currentActivityTopologyKey(snapshot.topology),
    [snapshot.topology],
  );
  const layoutTopology = useMemo(() => snapshot.topology, [topologyKey]);

  const [graphLayout, setGraphLayout] = useState<GraphLayout>(EMPTY_GRAPH_LAYOUT);

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
      GRAPH_LAYOUT_PROMISE_CACHE.get(topologyKey) ?? buildGraphLayout(layoutTopology);
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
  const graphKey = useMemo(() => currentActivityGraphKey(graphLayout), [graphLayout]);
  const storedNodePositions = useCurrentActivityGraphStore(
    (state) => state.positionsByGraphKey[graphKey] ?? EMPTY_NODE_POSITIONS,
  );
  const setStoredNodePosition = useCurrentActivityGraphStore((state) => state.setNodePosition);
  const visibleGraphEdges = useMemo(
    () => graphLayout.edges.filter((edge) => !edgeReturnsToResource(edge)),
    [graphLayout.edges],
  );
  const handleAssignments = useMemo(
    () => buildHandleAssignments(visibleGraphEdges),
    [visibleGraphEdges],
  );
  const activeGraphHighlights = useMemo(
    () => buildActiveGraphHighlights(activeExecutions, visibleGraphEdges),
    [activeExecutions, visibleGraphEdges],
  );

  const activeItemLabelsByPlaceId = useMemo(() => {
    const labelsByPlaceId = new Map<string, string[]>();
    const seenByPlaceId = new Map<string, Set<string>>();

    for (const execution of activeExecutions) {
      for (const token of execution.consumed_tokens ?? []) {
        const label = token.name || activeTokenLabel(execution, token.work_id, token.token_id);
        const placeLabels = labelsByPlaceId.get(token.place_id) ?? [];
        const seenLabels = seenByPlaceId.get(token.place_id) ?? new Set<string>();
        if (seenLabels.has(label)) {
          continue;
        }

        seenLabels.add(label);
        placeLabels.push(label);
        seenByPlaceId.set(token.place_id, seenLabels);
        labelsByPlaceId.set(token.place_id, placeLabels);
      }
    }

    return labelsByPlaceId;
  }, [activeExecutions]);

  const baseNodes = useMemo<CurrentActivityNode[]>(
    () => {
      const nextNodes: CurrentActivityNode[] = [];

      for (const positionedNode of graphLayout.nodes) {
        if (positionedNode.nodeKind !== "workstation") {
          const placeNode = positionedNode as PositionedPlaceNode;
          const place = placeNode.place;
          const position = nodePosition(
            placeNode.nodeId,
            { x: placeNode.x, y: placeNode.y },
            storedNodePositions,
          );
          const basePlaceNode = {
            id: placeNode.nodeId,
            position,
            initialHeight: placeNode.height,
            initialWidth: placeNode.width,
            measured: { height: placeNode.height, width: placeNode.width },
            height: placeNode.height,
            width: placeNode.width,
            draggable: true,
            className: "border-0 bg-transparent p-0 text-af-ink",
          };
          const basePlaceData = {
            activeFlow: activeGraphHighlights.activePlaceNodeIds.has(placeNode.nodeId),
            activeItemLabels: activeItemLabelsByPlaceId.get(place.place_id) ?? [],
            incomingHandleCount:
              handleAssignments.incomingHandleCounts.get(placeNode.nodeId) ?? 1,
            muted:
              place.kind !== "resource" &&
              activeGraphHighlights.hasActiveFlow &&
              !activeGraphHighlights.relatedNodeIds.has(placeNode.nodeId),
            outgoingHandleCount:
              handleAssignments.outgoingHandleCounts.get(placeNode.nodeId) ?? 1,
            selectedStateNode:
              selection?.kind === "state-node" &&
              selection.placeId === place.place_id,
            tokenCount: snapshot.runtime.place_token_counts?.[place.place_id] ?? 0,
          };

          if (place.kind === "work_state") {
            nextNodes.push({
              ...basePlaceNode,
              type: "statePosition",
              data: {
                ...basePlaceData,
                onSelectStateNode,
                place,
              },
              selectable: true,
            });
            continue;
          }

          if (place.kind === "resource") {
            nextNodes.push({
              ...basePlaceNode,
              type: "resource",
              data: {
                ...basePlaceData,
                place,
              },
              selectable: false,
            });
            continue;
          }

          nextNodes.push({
            ...basePlaceNode,
            type: "constraint",
            data: {
              ...basePlaceData,
              place,
            },
            selectable: false,
          });
          continue;
        }

        const workstationNode = positionedNode as PositionedWorkstationNode;
        const workstation =
          snapshot.topology.workstation_nodes_by_id[workstationNode.workstationNodeId];
        if (!workstation) {
          continue;
        }
        const executions = activeExecutionsByWorkstationNodeID[workstation.node_id] ?? [];
        const position = nodePosition(
          workstationNode.nodeId,
          { x: workstationNode.x, y: workstationNode.y },
          storedNodePositions,
        );

        nextNodes.push({
          id: workstationNode.nodeId,
          type: "workstation",
          position,
          initialHeight: workstationNode.height,
          initialWidth: workstationNode.width,
          measured: { height: workstationNode.height, width: workstationNode.width },
          height: workstationNode.height,
          width: workstationNode.width,
          data: {
            active: executions.length > 0,
            activeFlow: activeGraphHighlights.activeWorkstationNodeIds.has(workstationNode.nodeId),
            executions,
            incomingHandleCount:
              handleAssignments.incomingHandleCounts.get(workstationNode.nodeId) ?? 1,
            muted:
              activeGraphHighlights.hasActiveFlow &&
              !activeGraphHighlights.relatedNodeIds.has(workstationNode.nodeId),
            now,
            outgoingHandleCount:
              handleAssignments.outgoingHandleCounts.get(workstationNode.nodeId) ?? 1,
            selectedWorkID:
              selection?.kind === "work-item" && selection.nodeId === workstation.node_id
                ? selection.workID
                : null,
            selectedWorkstation:
              selection?.kind === "node" && selection.nodeId === workstation.node_id,
            workstation,
            onSelectWorkstation,
            onSelectWorkItem,
          },
          draggable: true,
          selectable: true,
          className: "border-0 bg-transparent p-0 text-af-ink",
        });
      }

      return nextNodes;
    },
    [
      activeExecutionsByWorkstationNodeID,
      activeGraphHighlights,
      activeItemLabelsByPlaceId,
      graphLayout.nodes,
      handleAssignments,
      now,
      onSelectWorkItem,
      onSelectStateNode,
      onSelectWorkstation,
      selection,
      snapshot.runtime.place_token_counts,
      snapshot.topology.workstation_nodes_by_id,
      storedNodePositions,
    ],
  );
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
    setNodes((currentNodes) => applyNodeChanges(changes, currentNodes) as CurrentActivityNode[]);
  }, []);

  const edges = useMemo<Edge[]>(
    () =>
      visibleGraphEdges.map((edge) => {
        const activeFlow = activeGraphHighlights.activeEdgeIds.has(edge.edgeId);
        const semantic = edgeSemantic(edge);
        const muted =
          activeGraphHighlights.hasActiveFlow &&
          !activeFlow &&
          !semantic &&
          (!activeGraphHighlights.relatedNodeIds.has(edge.fromNodeId) ||
            !activeGraphHighlights.relatedNodeIds.has(edge.toNodeId));

        return {
          animated: activeFlow,
          className: [
            activeFlow ? "agent-flow-edge--active" : "",
            semantic ? "agent-flow-edge--semantic" : "",
            muted ? "agent-flow-edge--muted" : "",
          ].filter(Boolean).join(" "),
          id: edge.edgeId,
          label: edgeLabel(edge, activeFlow),
          labelBgStyle: {
            fill: "var(--color-af-surface)",
            fillOpacity: activeFlow || semantic ? 0.92 : 0,
          },
          labelStyle: { fill: "var(--color-af-ink)" },
          markerEnd: {
            color: edgeMarkerColor(edge, activeFlow),
            type: MarkerType.ArrowClosed,
          },
          source: edge.fromNodeId,
          sourceHandle: handleAssignments.sourceHandlesByEdgeId.get(edge.edgeId),
          target: edge.toNodeId,
          targetHandle: handleAssignments.targetHandlesByEdgeId.get(edge.edgeId),
          style: edgeStyle(edge, activeFlow, muted),
          type: "default",
        };
      }),
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
  const initialFitViewKey =
    initialFitViewOptions.nodes?.map((node) => node.id).join(":") || "full-graph";

  if (snapshot.topology.workstation_node_ids.length === 0) {
    return (
      <section
        className="relative flex h-full min-h-0 min-w-0 flex-col rounded-3xl border border-af-overlay/10 bg-af-surface/72 p-[1.2rem] shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4"
        aria-labelledby="workflow-graph-heading"
      >
        <div className="mb-4 flex items-end justify-between gap-4 max-[720px]:flex-col max-[720px]:items-start [&_h2]:m-0">
          <div>
            <p className="mb-[0.65rem] text-xs font-bold uppercase tracking-[0.16em] text-af-accent">
              Operator View
            </p>
            <h2 id="workflow-graph-heading">Current activity</h2>
          </div>
        </div>
        <div className="grid min-h-60 items-start gap-[0.35rem] rounded-2xl border border-dashed border-af-overlay/15 bg-af-overlay/4 p-5 [&_h3]:m-0">
          <h3>No workflow topology loaded</h3>
          <p>The factory has not published any workstation graph yet.</p>
        </div>
      </section>
    );
  }

  return (
    <section
      className="relative flex h-full min-h-0 min-w-0 flex-col rounded-3xl border border-af-overlay/10 bg-af-surface/72 p-[1.2rem] shadow-af-panel backdrop-blur-[18px] max-[720px]:p-4"
      aria-labelledby="workflow-graph-heading"
    >
      <DashboardFlowAxisLegend
        className="absolute left-7 top-7 max-[720px]:left-4 max-[720px]:right-4"
        defaultExpanded={false}
        edgeItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_EDGE_ITEMS}
        iconItems={DEFAULT_DASHBOARD_FLOW_AXIS_LEGEND_ICON_ITEMS}
      />

      <div className="relative min-h-0 flex-1">
        <div
          aria-label="Work graph viewport"
          className={cx(
            "relative h-full min-h-0 overflow-hidden rounded-[1.4rem] border transition-colors",
            (dropState.status === "drag-active" || dropState.status === "reading") &&
              "border-af-accent/35 bg-af-accent/6",
            dropState.status === "error" && "border-af-danger/18",
            dropState.status === "idle" && "border-transparent",
          )}
          data-current-activity-drop-state={graphDropStateAttribute(dropState)}
          data-current-activity-flow
          onDragEnter={onDragEnter}
          onDragLeave={onDragLeave}
          onDragOver={onDragOver}
          onDrop={onDrop}
          role="region"
        >
          <ReactFlow
            edges={edges}
            fitView
            fitViewOptions={initialFitViewOptions}
            key={initialFitViewKey}
            maxZoom={2}
            minZoom={0.25}
            nodes={nodes}
            nodeTypes={CURRENT_ACTIVITY_NODE_TYPES}
            nodesDraggable={true}
            onNodeDragStop={(_, node) => {
              if (graphKey) {
                setStoredNodePosition(graphKey, node.id, node.position);
              }
            }}
            onNodesChange={handleNodesChange}
            panOnDrag
            proOptions={{ hideAttribution: true }}
            zoomOnScroll
          >
            <Background
              color={GRAPH_BACKGROUND_COLOR}
              gap={GRAPH_BACKGROUND_GAP}
              size={GRAPH_BACKGROUND_SIZE}
            />
            <Controls
              fitViewOptions={{ maxZoom: 1.2, padding: 0.12 }}
              showInteractive={false}
              style={GRAPH_CONTROLS_STYLE}
            />
          </ReactFlow>
          <GraphDropOverlay dropState={dropState} />
        </div>
        {importPreviewState.status === "ready" ? (
          <FactoryImportPreviewDialog
            activationState={activationState}
            onCancel={() => {
              clearActivationError();
              closeImportPreview();
            }}
            onConfirm={() => {
              void activateImport(importPreviewState.value);
            }}
            previewState={importPreviewState}
          />
        ) : null}
        {dropState.status === "error" ? (
          <GraphImportErrorPanel
            error={dropState.error}
            fileName={dropState.fileName}
            onDismiss={clearImportError}
          />
        ) : null}
      </div>
    </section>
  );
}
