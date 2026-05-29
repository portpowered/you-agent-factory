import { type Edge, MarkerType, type Node } from "@xyflow/react";

import type { ActivityGraphNodeHandle } from "../../flowchart/components/current-activity-node-shell";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import type {
  FactoryGraphDraftValidationError,
  FactoryGraphEdge,
  FactoryGraphNode,
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";
import type {
  FactoryGraphConnectionEndpoint,
  FactoryGraphConnectionResolver,
} from "./factory-graph-editor-connections";
import {
  getFactoryGraphConnectionAnchors,
  isValidFactoryGraphConnection,
  resolveFactoryGraphConnectionAnchorContext,
} from "./factory-graph-editor-connections";
import type { FactoryGraphWorkerRuntimeStatus } from "./factory-graph-editor-runtime";
import { filterFactoryGraphTopologyForCustomerDisplay } from "./factory-graph-customer-display";

export type FactoryGraphReactFlowMode = "editor" | "observer";

export type FactoryGraphReactFlowNode = Node<
  {
    active: boolean;
    activeFlow: boolean;
    activeTool: "add" | "connect" | "delete" | null;
    canEditConnections: boolean;
    connectionAnchors: ActivityGraphNodeHandle[];
    connectionHint: string;
    draftStatus: "addition" | "none" | "removal";
    focused: boolean;
    kind: FactoryGraphNodeKind;
    kindLabel: string;
    label: string;
    muted: boolean;
    pendingLabel: string;
    removingLabel: string;
    selectedWorkId: string | null;
    tokenCount: number | null;
    validationMessage: string | null;
    workerStatus?: FactoryGraphWorkerRuntimeStatus;
    workerStatusLabel?: string;
  },
  "factoryEntity"
>;

export type FactoryGraphReactFlowEdge = Edge<{
  active: boolean;
  alwaysShowLabel: boolean;
  kind: FactoryGraphEdge["kind"];
  label: string;
  pendingStatus: "addition" | "none" | "removal";
}>;

export interface FactoryGraphReactFlowProjection {
  edges: FactoryGraphReactFlowEdge[];
  nodes: FactoryGraphReactFlowNode[];
}

export interface FactoryGraphReactFlowRuntimeOverlay {
  activeEdgeIds?: ReadonlySet<string>;
  activeNodeIds?: ReadonlySet<string>;
  focusedNodeIds?: ReadonlySet<string>;
  mutedNodeIds?: ReadonlySet<string>;
  placeTokenCountsByNodeId?: ReadonlyMap<string, number>;
  selectedWorkId?: string | null;
  workerStatusByName?: ReadonlyMap<string, FactoryGraphWorkerRuntimeStatus>;
}

export interface FactoryGraphReactFlowEditorOverlay {
  activeTool?: "add" | "connect" | "delete" | null;
  canEditConnections: boolean;
  onConnectionAnchorClick?: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingAdditionEdgeIds: ReadonlySet<string>;
  pendingAdditionNodeIds: ReadonlySet<string>;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  pendingRemovalEdgeIds: ReadonlySet<string>;
  pendingRemovalNodeIds: ReadonlySet<string>;
  validationErrors?: readonly FactoryGraphDraftValidationError[];
}

export interface ProjectFactoryGraphToReactFlowOptions {
  editor?: FactoryGraphReactFlowEditorOverlay;
  layoutPositionsByNodeId?: ReadonlyMap<string, { x: number; y: number }>;
  locale?: string;
  mode?: FactoryGraphReactFlowMode;
  runtime?: FactoryGraphReactFlowRuntimeOverlay;
  topology: FactoryGraphTopology;
  workstationResolver?: FactoryGraphConnectionResolver;
}

const COLUMN_BY_KIND: Record<FactoryGraphNodeKind, number> = {
  resource: 0,
  worker: 1,
  workstation: 2,
  "work-type": 3,
  "work-state": 4,
};
const EDGE_COLOR_BY_KIND = {
  "worker-assignment": "var(--color-af-info)",
  "worker-resource": "var(--color-af-success)",
  "work-type-state": "var(--color-af-overlay)",
  "workstation-input": "var(--color-af-accent)",
  "workstation-on-continue": "var(--color-af-info)",
  "workstation-on-failure": "var(--color-af-danger-text)",
  "workstation-on-rejection": "var(--color-af-warning-text)",
  "workstation-output": "var(--color-af-accent)",
  "workstation-resource": "var(--color-af-success)",
} as const;
const COLUMN_X = 232;
const ROW_Y = 118;

const EMPTY_SET: ReadonlySet<string> = new Set();

export function projectFactoryGraphToReactFlow(
  options: FactoryGraphTopology | ProjectFactoryGraphToReactFlowOptions,
): FactoryGraphReactFlowProjection {
  const input = normalizeProjectionOptions(options);
  const displayTopology = filterFactoryGraphTopologyForCustomerDisplay(
    input.topology,
  );
  const messages = getFactoryGraphEditorMessages(input.locale);
  const rowCounts = new Map<number, number>();
  const validationMessages = validationMessagesByNodeId(
    input.editor?.validationErrors ?? [],
  );

  const projectionInput: ProjectFactoryGraphToReactFlowOptions = {
    ...input,
    topology: displayTopology,
  };

  return {
    edges: displayTopology.edges.map((edge) =>
      buildFactoryGraphReactFlowEdge(edge, projectionInput),
    ),
    nodes: [...displayTopology.nodes].sort(sortFactoryGraphNodes).map((node) => {
      const column = COLUMN_BY_KIND[node.kind];
      const row = rowCounts.get(column) ?? 0;
      rowCounts.set(column, row + 1);
      const workerStatus =
        node.kind === "worker"
          ? (input.runtime?.workerStatusByName?.get(node.label) ?? "idle")
          : undefined;

      return {
        className: nodeClassName(node.id, input),
        data: {
          active: input.runtime?.activeNodeIds?.has(node.id) ?? false,
          activeFlow: input.runtime?.activeNodeIds?.has(node.id) ?? false,
          activeTool: input.editor?.activeTool ?? null,
          canEditConnections: input.editor?.canEditConnections ?? false,
          connectionAnchors: buildNodeHandles({
            editor: input.editor,
            locale: input.locale,
            node,
            topology: displayTopology,
            workstationResolver: input.workstationResolver,
          }),
          connectionHint: messages.flowConnectionHint,
          draftStatus: draftStatusForNode(node.id, input.editor),
          focused: input.runtime?.focusedNodeIds?.has(node.id) ?? false,
          kind: node.kind,
          kindLabel: messages.kindLabel(node.kind),
          label: node.label,
          muted: input.runtime?.mutedNodeIds?.has(node.id) ?? false,
          pendingLabel: messages.flowPendingLabel,
          removingLabel: messages.flowRemovingLabel,
          selectedWorkId: input.runtime?.selectedWorkId ?? null,
          tokenCount:
            input.runtime?.placeTokenCountsByNodeId?.get(node.id) ?? null,
          validationMessage: validationMessages.get(node.id) ?? null,
          workerStatus,
          workerStatusLabel: workerStatus
            ? messages.workerStatusLabel(workerStatus)
            : undefined,
        },
        draggable: true,
        id: node.id,
        position: input.layoutPositionsByNodeId?.get(node.id) ?? {
          x: column * COLUMN_X,
          y: row * ROW_Y,
        },
        type: "factoryEntity",
      } satisfies FactoryGraphReactFlowNode;
    }),
  };
}

function normalizeProjectionOptions(
  options: FactoryGraphTopology | ProjectFactoryGraphToReactFlowOptions,
): ProjectFactoryGraphToReactFlowOptions {
  if ("topology" in options) {
    return options;
  }

  return {
    topology: options,
  };
}

function sortFactoryGraphNodes(
  left: FactoryGraphNode,
  right: FactoryGraphNode,
) {
  const leftColumn = COLUMN_BY_KIND[left.kind];
  const rightColumn = COLUMN_BY_KIND[right.kind];
  if (leftColumn !== rightColumn) {
    return leftColumn - rightColumn;
  }
  return left.label.localeCompare(right.label);
}

function buildFactoryGraphReactFlowEdge(
  edge: FactoryGraphEdge,
  input: ProjectFactoryGraphToReactFlowOptions,
) {
  const messages = getFactoryGraphEditorMessages(input.locale);
  const color = EDGE_COLOR_BY_KIND[edge.kind];
  const pendingAddition =
    input.editor?.pendingAdditionEdgeIds.has(edge.id) ?? false;
  const pendingRemoval =
    input.editor?.pendingRemovalEdgeIds.has(edge.id) ?? false;
  const active = input.runtime?.activeEdgeIds?.has(edge.id) ?? false;
  const edgeLabel = messages.edgeKindLabel(edge.kind);
  const handleAssignment = getEdgeHandleAssignment(
    edge,
    input.topology,
    input.workstationResolver,
  );

  return {
    animated:
      active ||
      edge.kind === "workstation-on-continue" ||
      edge.kind === "workstation-on-failure" ||
      edge.kind === "workstation-on-rejection",
    className: [
      active ? "agent-factory-editor-edge--active" : "",
      pendingAddition ? "agent-factory-editor-edge--pending-addition" : "",
      pendingRemoval ? "agent-factory-editor-edge--pending-removal" : "",
    ]
      .filter(Boolean)
      .join(" "),
    data: {
      active,
      alwaysShowLabel:
        input.editor?.canEditConnections === true ||
        input.editor?.pendingConnectionSource !== null,
      kind: edge.kind,
      label: edgeLabel,
      pendingStatus: pendingRemoval
        ? "removal"
        : pendingAddition
          ? "addition"
          : "none",
    },
    ariaLabel: messages.edgeAriaLabel(
      edgeLabel,
      describeNodeKey(edge.source),
      describeNodeKey(edge.target),
    ),
    ariaRole: "button",
    focusable: true,
    id: edge.id,
    interactionWidth: 24,
    markerEnd: {
      color: pendingRemoval
        ? "var(--color-af-danger-text)"
        : pendingAddition
          ? "var(--color-af-warning-text)"
          : active
            ? "var(--color-af-success)"
            : color,
      type: MarkerType.ArrowClosed,
    },
    source: edge.sourceId,
    sourceHandle: handleAssignment?.sourceHandle,
    style: {
      opacity: pendingRemoval ? 0.48 : undefined,
      stroke: pendingRemoval
        ? "var(--color-af-danger-text)"
        : pendingAddition
          ? "var(--color-af-warning-text)"
          : active
            ? "var(--color-af-success)"
            : color,
      strokeDasharray: pendingRemoval
        ? "7 5"
        : pendingAddition
          ? "9 4"
          : edge.kind === "worker-resource" ||
              edge.kind === "workstation-resource"
            ? "4 5"
            : undefined,
      strokeWidth: pendingRemoval || pendingAddition || active ? 2 : 1.7,
    },
    target: edge.targetId,
    targetHandle: handleAssignment?.targetHandle,
    type: "factoryEditorEdge",
  } satisfies FactoryGraphReactFlowEdge;
}

function describeNodeKey(key: FactoryGraphEdge["source"]) {
  return key.kind === "work-state"
    ? `${key.workTypeName}:${key.stateName}`
    : key.name;
}

function getEdgeHandleAssignment(
  edge: FactoryGraphEdge,
  topology: FactoryGraphTopology,
  workstationResolver?: FactoryGraphConnectionResolver,
): { sourceHandle: string; targetHandle: string } | null {
  const sourceNode = findTopologyNode(topology, edge.sourceId);
  const targetNode = findTopologyNode(topology, edge.targetId);
  const sourceHandle = getNodeHandleId(
    sourceNode,
    edge.kind,
    "source",
    workstationResolver,
  );
  const targetHandle = getNodeHandleId(
    targetNode,
    edge.kind,
    "target",
    workstationResolver,
  );
  if (!sourceHandle || !targetHandle) {
    return null;
  }
  return { sourceHandle, targetHandle };
}

function getNodeHandleId(
  node: FactoryGraphNode,
  edgeKind: FactoryGraphEdge["kind"],
  role: "source" | "target",
  workstationResolver?: FactoryGraphConnectionResolver,
) {
  if (edgeKind === "work-type-state") {
    return null;
  }

  const anchorContext = resolveFactoryGraphConnectionAnchorContext(
    node,
    workstationResolver,
  );

  return (
    getFactoryGraphConnectionAnchors(node.kind, anchorContext).find(
      (anchor) =>
        anchor.role === role &&
        (anchor.edgeKinds ?? [anchor.edgeKind]).includes(edgeKind),
    )?.id ?? null
  );
}

function buildNodeHandles(input: {
  editor?: FactoryGraphReactFlowEditorOverlay;
  locale?: string;
  node: FactoryGraphNode;
  topology: FactoryGraphTopology;
  workstationResolver?: FactoryGraphConnectionResolver;
}): ActivityGraphNodeHandle[] {
  const messages = getFactoryGraphEditorMessages(input.locale);
  const anchorContext = resolveFactoryGraphConnectionAnchorContext(
    input.node,
    input.workstationResolver,
  );
  const selectedSource =
    input.editor?.pendingConnectionSource?.nodeId === input.node.id
      ? input.editor.pendingConnectionSource
      : null;
  const nodeIsPendingRemoval =
    input.editor?.pendingRemovalNodeIds.has(input.node.id) ?? false;
  const canEditConnections = input.editor?.canEditConnections ?? false;
  const pendingSourceNode =
    input.editor?.pendingConnectionSource !== null &&
    input.editor?.pendingConnectionSource !== undefined
      ? findNode(input.topology, input.editor.pendingConnectionSource.nodeId)
      : null;
  const pendingSourceWorkstation =
    pendingSourceNode === null
      ? undefined
      : resolveFactoryGraphConnectionAnchorContext(
          pendingSourceNode,
          input.workstationResolver,
        )?.workstation;

  return getFactoryGraphConnectionAnchors(input.node.kind, anchorContext).map(
    (anchor) => {
    const selected =
      selectedSource?.anchorId === anchor.id && anchor.role === "source";
    const compatible =
      input.editor?.pendingConnectionSource !== null &&
      input.editor?.pendingConnectionSource !== undefined &&
      input.editor.pendingConnectionSource.nodeId !== input.node.id &&
      anchor.role === "target" &&
      isValidFactoryGraphConnection({
        sourceAnchorId: input.editor.pendingConnectionSource.anchorId,
        sourceNodeKind: pendingSourceNode?.kind ?? input.node.kind,
        sourceWorkstation: pendingSourceWorkstation,
        targetAnchorId: anchor.id,
        targetNodeKind: input.node.kind,
        targetWorkstation: anchorContext?.workstation,
      });

    return {
      buttonAriaLabel:
        anchor.role === "source"
          ? `${messages.toolbarConnectLabel}: ${input.node.label} ${anchor.label}`
          : `${messages.toolbarConnectLabel}: ${input.node.label} ${anchor.label}`,
      buttonDisabled: !canEditConnections || nodeIsPendingRemoval,
      buttonPressed: selected || undefined,
      buttonTitle: anchor.description,
      connectable: canEditConnections && !nodeIsPendingRemoval,
      id: anchor.id,
      label: anchor.label,
      onButtonClick:
        input.editor?.onConnectionAnchorClick &&
        canEditConnections &&
        !nodeIsPendingRemoval
          ? () =>
              input.editor?.onConnectionAnchorClick?.({
                anchorId: anchor.id,
                nodeId: input.node.id,
              })
          : undefined,
      side: anchor.side,
      type: anchor.role,
      variant: selected
        ? "selected"
        : compatible
          ? "valid-target"
          : canEditConnections
            ? "default"
            : "muted",
    } satisfies ActivityGraphNodeHandle;
    },
  );
}

function findTopologyNode(topology: FactoryGraphTopology, nodeId: string) {
  const node = topology.nodes.find((entry) => entry.id === nodeId);
  if (!node) {
    throw new Error(
      `Expected graph node "${nodeId}" to exist in editor topology.`,
    );
  }
  return node;
}

function findNode(topology: FactoryGraphTopology, nodeId: string) {
  const node = topology.nodes.find((entry) => entry.id === nodeId);
  if (!node) {
    throw new Error(
      `Expected graph node "${nodeId}" to exist in editor topology.`,
    );
  }
  return node;
}

function draftStatusForNode(
  nodeId: string,
  editor: FactoryGraphReactFlowEditorOverlay | undefined,
) {
  if (editor?.pendingRemovalNodeIds.has(nodeId)) {
    return "removal";
  }
  if (editor?.pendingAdditionNodeIds.has(nodeId)) {
    return "addition";
  }
  return "none";
}

function nodeClassName(
  nodeId: string,
  input: ProjectFactoryGraphToReactFlowOptions,
) {
  return [
    input.runtime?.activeNodeIds?.has(nodeId)
      ? "agent-factory-editor-node--active"
      : "",
    input.runtime?.focusedNodeIds?.has(nodeId)
      ? "agent-factory-editor-node--focused"
      : "",
    input.runtime?.mutedNodeIds?.has(nodeId)
      ? "agent-factory-editor-node--muted"
      : "",
    input.editor?.pendingAdditionNodeIds.has(nodeId)
      ? "agent-factory-editor-node--pending-addition"
      : "",
    input.editor?.pendingRemovalNodeIds.has(nodeId)
      ? "agent-factory-editor-node--pending-removal"
      : "",
  ]
    .filter(Boolean)
    .join(" ");
}

function validationMessagesByNodeId(
  errors: readonly FactoryGraphDraftValidationError[],
) {
  const messages = new Map<string, string>();
  for (const error of errors) {
    if (error.target.kind !== "node") {
      continue;
    }
    messages.set(error.target.id, error.message);
  }
  return messages;
}

export function emptyFactoryGraphReactFlowEditorOverlay(): FactoryGraphReactFlowEditorOverlay {
  return {
    canEditConnections: false,
    pendingAdditionEdgeIds: EMPTY_SET,
    pendingAdditionNodeIds: EMPTY_SET,
    pendingConnectionSource: null,
    pendingRemovalEdgeIds: EMPTY_SET,
    pendingRemovalNodeIds: EMPTY_SET,
  };
}
