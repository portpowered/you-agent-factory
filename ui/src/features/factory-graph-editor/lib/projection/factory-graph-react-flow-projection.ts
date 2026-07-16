// biome-ignore lint/style/noExcessiveLinesPerFile: React Flow projection keeps node, edge, and handle mapping together for one adapter seam.
import { type Edge, MarkerType, type Node } from "@xyflow/react";
import { workTypeHasDefaultHandling } from "../../../current-factory-definition/lib/work-type-default-handling";
import { workstationHasZAxisIncompleteForConnections } from "../../../current-factory-definition/lib/workstation-progress-outcome-routes";
import type {
  ActivityGraphNodeHandle,
  ZAxisIncompleteHints,
} from "../../../flowchart/components/current-activity-node-shell";
import { factoryGraphEditorEdgeHoverClassName } from "../../../flowchart/lib/current-activity-graph-hover";
import { workstationGraphPresentationFromBehavior } from "../../../flowchart/lib/workstation-graph-presentation";
import { getFactoryGraphEditorMessages } from "../../messages/editor";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraftValidationError,
  FactoryGraphEdge,
  FactoryGraphNode,
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "../draft/factory-graph-draft-types";
import type {
  FactoryGraphConnectionAnchor,
  FactoryGraphConnectionEndpoint,
  FactoryGraphConnectionResolver,
} from "../editor/factory-graph-editor-connections";
import {
  getFactoryGraphConnectionAnchors,
  getLocalizedFactoryGraphConnectionAnchors,
  isValidFactoryGraphConnection,
  resolveFactoryGraphConnectionAnchorContext,
} from "../editor/factory-graph-editor-connections";
import type { FactoryGraphWorkerRuntimeStatus } from "../editor-runtime/factory-graph-editor-runtime";
import { filterFactoryGraphTopologyForCustomerDisplay } from "../operations/factory-graph-customer-display";
import {
  workstationRendersProgressOutcomeHandleValidation,
  workstationRendersProgressOutcomeZAxisHintAnchors,
} from "../projection/factory-graph-progress-outcome-handle-visibility";
import {
  type FactoryValidationGraphProjection,
  filterValidationHandleErrorsForWorkstation,
  validationHandleErrorsForNode,
} from "../projection/factory-validation-graph-projection";
import {
  type FactoryGraphWorkStateType,
  resolveWorkStateTypeForGraphNode,
} from "../work-state/factory-graph-work-state-type";

export type FactoryGraphReactFlowMode = "editor" | "observer";

export type FactoryGraphReactFlowNode = Node<
  {
    active: boolean;
    activeFlow: boolean;
    activeTool: "add" | "connect" | "delete" | null;
    canEditConnections: boolean;
    connectionAnchors: ActivityGraphNodeHandle[];
    connectionHint: string;
    defaultWorkTypeLabel?: string;
    draftStatus: "addition" | "none" | "removal";
    focused: boolean;
    isDefaultWorkType?: boolean;
    kind: FactoryGraphNodeKind;
    kindLabel: string;
    label: string;
    muted: boolean;
    pendingLabel: string;
    removingLabel: string;
    selectedWorkId: string | null;
    tokenCount: number | null;
    validationMessage: string | null;
    workStateType?: FactoryGraphWorkStateType;
    workerStatus?: FactoryGraphWorkerRuntimeStatus;
    workerStatusLabel?: string;
    workstationSemanticBorderClassName?: string;
    workstationSemanticIconClassName?: string;
    workstationSemanticIconKind?: string;
    workstationSemanticLabel?: string;
    zAxisIncompleteHints?: ZAxisIncompleteHints | null;
  },
  "factoryEntity"
>;

export type FactoryGraphReactFlowEdge = Edge<{
  active?: boolean;
  alwaysShowLabel?: boolean;
  factoryGraphEdgeId?: string;
  kind?: FactoryGraphEdge["kind"];
  label?: string;
  pendingStatus?: "addition" | "none" | "removal";
  waypoints?: { x: number; y: number }[];
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
  validationProjection?: FactoryValidationGraphProjection;
}

export interface ProjectFactoryGraphToReactFlowOptions {
  editor?: FactoryGraphReactFlowEditorOverlay;
  /** When true, omit edges whose handles are absent from rendered connection anchors. */
  filterEdgesToRenderedHandles?: boolean;
  factoryDefinition?: CanonicalFactoryDefinition | null;
  layoutPositionsByNodeId?: ReadonlyMap<string, { x: number; y: number }>;
  locale?: string;
  mode?: FactoryGraphReactFlowMode;
  runtime?: FactoryGraphReactFlowRuntimeOverlay;
  topology: FactoryGraphTopology;
  workstationResolver?: FactoryGraphConnectionResolver;
}

const COLUMN_BY_KIND: Record<FactoryGraphNodeKind, number> = {
  doc: 5,
  resource: 0,
  worker: 1,
  workstation: 2,
  "work-type": 3,
  "work-state": 4,
};
const EDGE_COLOR_BY_KIND = {
  "worker-assignment": "var(--color-info)",
  "worker-resource": "var(--color-success)",
  "work-type-state": "var(--color-af-overlay)",
  "workstation-input": "var(--color-primary)",
  "workstation-on-continue": "var(--color-info)",
  "workstation-on-failure": "var(--color-on-error-container)",
  "workstation-on-rejection": "var(--color-on-warning-container)",
  "workstation-output": "var(--color-primary)",
  "workstation-resource": "var(--color-success)",
  "work-state-visibility-bypass": "var(--color-primary)",
} as const;
const COLUMN_X = 232;
const ROW_Y = 118;

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

  const nodes = [...displayTopology.nodes]
    .sort(sortFactoryGraphNodes)
    .map((node) =>
      buildFactoryGraphReactFlowNode({
        displayTopology,
        input,
        messages,
        node,
        rowCounts,
        validationMessages,
      }),
    );
  const nodesById = new Map(nodes.map((node) => [node.id, node]));

  const projectedEdges = displayTopology.edges.map((topologyEdge) =>
    buildFactoryGraphReactFlowEdge(topologyEdge, projectionInput),
  );

  return {
    edges: input.filterEdgesToRenderedHandles
      ? projectedEdges.filter((edge) =>
          shouldIncludeFactoryGraphReactFlowEdge(
            edge,
            edge.data?.kind,
            nodesById,
          ),
        )
      : projectedEdges,
    nodes,
  };
}

function buildFactoryGraphReactFlowNode(input: {
  displayTopology: FactoryGraphTopology;
  input: ProjectFactoryGraphToReactFlowOptions;
  messages: ReturnType<typeof getFactoryGraphEditorMessages>;
  node: FactoryGraphNode;
  rowCounts: Map<number, number>;
  validationMessages: Map<string, string>;
}): FactoryGraphReactFlowNode {
  const column = COLUMN_BY_KIND[input.node.kind];
  const row = input.rowCounts.get(column) ?? 0;
  input.rowCounts.set(column, row + 1);
  const workerStatus =
    input.node.kind === "worker"
      ? (input.input.runtime?.workerStatusByName?.get(input.node.label) ??
        "idle")
      : undefined;
  const canEditConnections = input.input.editor?.canEditConnections ?? false;
  const anchorContext = resolveFactoryGraphConnectionAnchorContext(
    input.node,
    input.input.workstationResolver,
  );
  const workStateType =
    input.node.kind === "work-state" && input.node.key.kind === "work-state"
      ? resolveWorkStateTypeForGraphNode(
          input.input.factoryDefinition,
          input.node.key,
        )
      : undefined;
  const isDefaultWorkType =
    input.node.kind === "work-type"
      ? workTypeHasDefaultHandling(
          input.input.factoryDefinition,
          input.node.label,
        )
      : false;
  const workstationPresentation =
    input.node.kind === "workstation" &&
    anchorContext?.workstation?.behavior !== undefined
      ? workstationGraphPresentationFromBehavior(
          anchorContext.workstation.behavior,
          input.input.locale,
        )
      : null;

  return {
    className: nodeClassName(input.node.id, input.input),
    data: {
      active: input.input.runtime?.activeNodeIds?.has(input.node.id) ?? false,
      activeFlow:
        input.input.runtime?.activeNodeIds?.has(input.node.id) ?? false,
      activeTool: input.input.editor?.activeTool ?? null,
      canEditConnections,
      connectionAnchors: buildNodeHandles({
        editor: input.input.editor,
        locale: input.input.locale,
        node: input.node,
        topology: input.displayTopology,
        workstationResolver: input.input.workstationResolver,
      }),
      connectionHint: input.messages.flowConnectionHint,
      ...(isDefaultWorkType
        ? {
            defaultWorkTypeLabel: input.messages.defaultWorkTypeLabel,
            isDefaultWorkType: true,
          }
        : {}),
      draftStatus: draftStatusForNode(input.node.id, input.input.editor),
      focused: input.input.runtime?.focusedNodeIds?.has(input.node.id) ?? false,
      kind: input.node.kind,
      kindLabel: input.messages.kindLabel(input.node.kind),
      label: input.node.label,
      muted: input.input.runtime?.mutedNodeIds?.has(input.node.id) ?? false,
      pendingLabel: input.messages.flowPendingLabel,
      removingLabel: input.messages.flowRemovingLabel,
      selectedWorkId: input.input.runtime?.selectedWorkId ?? null,
      tokenCount:
        input.input.runtime?.placeTokenCountsByNodeId?.get(input.node.id) ??
        null,
      validationMessage: input.validationMessages.get(input.node.id) ?? null,
      ...(input.node.kind === "work-state" ? { workStateType } : {}),
      workerStatus,
      workerStatusLabel: workerStatus
        ? input.messages.workerStatusLabel(workerStatus)
        : undefined,
      ...(workstationPresentation &&
      workstationPresentation.semanticKind !== "STANDARD"
        ? {
            workstationSemanticBorderClassName:
              workstationPresentation.borderClassName,
            workstationSemanticIconClassName: workstationPresentation.className,
            workstationSemanticIconKind: workstationPresentation.iconKind,
            workstationSemanticLabel: workstationPresentation.label,
          }
        : {}),
      zAxisIncompleteHints: resolveFactoryGraphZAxisIncompleteHints({
        anchorContext,
        canEditConnections,
        locale: input.input.locale,
        nodeKind: input.node.kind,
      }),
    },
    draggable: true,
    id: input.node.id,
    position: input.input.layoutPositionsByNodeId?.get(input.node.id) ?? {
      x: column * COLUMN_X,
      y: row * ROW_Y,
    },
    type: "factoryEntity",
  } satisfies FactoryGraphReactFlowNode;
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

function shouldIncludeFactoryGraphReactFlowEdge(
  edge: FactoryGraphReactFlowEdge,
  edgeKind: FactoryGraphEdge["kind"] | undefined,
  nodesById: ReadonlyMap<string, FactoryGraphReactFlowNode>,
): boolean {
  if (edgeKind === "work-type-state") {
    return true;
  }

  const sourceNode = nodesById.get(edge.source);
  const targetNode = nodesById.get(edge.target);
  if (!sourceNode || !targetNode || !edge.sourceHandle || !edge.targetHandle) {
    return false;
  }

  const sourceAnchorIds = new Set(
    sourceNode.data.connectionAnchors.map((anchor) => anchor.id),
  );
  const targetAnchorIds = new Set(
    targetNode.data.connectionAnchors.map((anchor) => anchor.id),
  );

  return (
    sourceAnchorIds.has(edge.sourceHandle) &&
    targetAnchorIds.has(edge.targetHandle)
  );
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
  const edgeLabel =
    edge.kind === "work-state-visibility-bypass" && edge.outcomeRouteKind
      ? messages.edgeKindLabel(edge.outcomeRouteKind)
      : messages.edgeKindLabel(
          edge.kind === "work-state-visibility-bypass"
            ? "workstation-output"
            : edge.kind,
        );
  const visibleEdgeLabel = edge.kind === "work-type-state" ? "" : edgeLabel;
  const handleAssignment = getEdgeHandleAssignment(
    edge,
    input.topology,
    input.workstationResolver,
  );
  const hoverClassName = factoryGraphEditorEdgeHoverClassName({
    active,
    pendingAddition,
    pendingRemoval,
  });
  const resolvedStroke = pendingRemoval
    ? "var(--color-af-danger-text)"
    : pendingAddition
      ? "var(--color-af-warning-text)"
      : active
        ? "var(--color-af-success)"
        : color;
  const resolvedStyle = {
    opacity: pendingRemoval ? 0.48 : undefined,
    stroke: resolvedStroke,
    strokeDasharray: pendingRemoval
      ? "7 5"
      : pendingAddition
        ? "9 4"
        : edge.kind === "worker-resource" ||
            edge.kind === "workstation-resource"
          ? "4 5"
          : undefined,
    strokeWidth: pendingRemoval || pendingAddition || active ? 2 : 1.7,
  } satisfies FactoryGraphReactFlowEdge["style"];
  const style = hoverClassName
    ? ({
        ...resolvedStyle,
        stroke: "var(--af-graph-edge-stroke)",
        "--af-graph-edge-stroke": resolvedStroke,
      } as FactoryGraphReactFlowEdge["style"])
    : resolvedStyle;

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
      hoverClassName ?? "",
    ]
      .filter(Boolean)
      .join(" "),
    data: {
      active,
      alwaysShowLabel:
        input.editor?.canEditConnections === true ||
        input.editor?.pendingConnectionSource !== null,
      kind: edge.kind,
      label: visibleEdgeLabel,
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
        ? "var(--color-on-error-container)"
        : pendingAddition
          ? "var(--color-on-warning-container)"
          : active
            ? "var(--color-success)"
            : color,
      type: MarkerType.ArrowClosed,
    },
    source: edge.sourceId,
    sourceHandle: handleAssignment?.sourceHandle,
    style,
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
  if (edge.kind === "work-state-visibility-bypass" && edge.outcomeRouteKind) {
    const sourceHandle = getNodeHandleId(
      sourceNode,
      edge.outcomeRouteKind,
      "source",
      workstationResolver,
    );
    const targetHandle = getNodeHandleId(
      targetNode,
      "workstation-input",
      "target",
      workstationResolver,
    );
    if (!sourceHandle || !targetHandle) {
      return null;
    }
    return { sourceHandle, targetHandle };
  }

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
  if (
    edgeKind === "work-type-state" ||
    edgeKind === "work-state-visibility-bypass"
  ) {
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

export function resolveFactoryGraphZAxisIncompleteHints(input: {
  anchorContext?: ReturnType<typeof resolveFactoryGraphConnectionAnchorContext>;
  canEditConnections: boolean;
  locale?: string;
  nodeKind: FactoryGraphNodeKind;
}): ZAxisIncompleteHints | null {
  if (
    input.nodeKind !== "workstation" ||
    !input.canEditConnections ||
    !input.anchorContext?.workstation ||
    !workstationHasZAxisIncompleteForConnections(
      input.anchorContext.workstation,
    ) ||
    !workstationRendersProgressOutcomeZAxisHintAnchors(input.anchorContext)
  ) {
    return null;
  }

  const hint = getFactoryGraphEditorMessages(
    input.locale,
  ).zAxisIncompleteConnectionHint;
  return {
    accessibleLabel: hint,
    title: hint,
  };
}

function projectEditorConnectionAnchorHandle(input: {
  anchor: FactoryGraphConnectionAnchor;
  canEditConnections: boolean;
  compatible: boolean;
  editor?: FactoryGraphReactFlowEditorOverlay;
  handleValidation?: { message: string };
  messages: ReturnType<typeof getFactoryGraphEditorMessages>;
  node: FactoryGraphNode;
  nodeIsPendingRemoval: boolean;
  rendersHandleValidation: boolean;
  selected: boolean;
}): ActivityGraphNodeHandle {
  const handleValidation = input.handleValidation;
  const showHandleValidation =
    handleValidation !== undefined && input.rendersHandleValidation;
  const validationMessage = showHandleValidation
    ? handleValidation.message
    : undefined;

  return {
    buttonAriaLabel: showHandleValidation
      ? validationMessage
      : `${input.messages.toolbarConnectLabel}: ${input.node.label} ${input.anchor.label}`,
    buttonDisabled: !input.canEditConnections || input.nodeIsPendingRemoval,
    buttonPressed: input.selected || undefined,
    buttonTitle: showHandleValidation
      ? validationMessage
      : input.anchor.description,
    connectable: input.canEditConnections && !input.nodeIsPendingRemoval,
    id: input.anchor.id,
    label: input.anchor.label,
    onButtonClick:
      input.editor?.onConnectionAnchorClick &&
      input.canEditConnections &&
      !input.nodeIsPendingRemoval
        ? () =>
            input.editor?.onConnectionAnchorClick?.({
              anchorId: input.anchor.id,
              nodeId: input.node.id,
            })
        : undefined,
    side: input.anchor.side,
    type: input.anchor.role,
    validationError: showHandleValidation || undefined,
    validationMessage,
    variant: showHandleValidation
      ? "error"
      : input.selected
        ? "selected"
        : input.compatible
          ? "valid-target"
          : input.canEditConnections
            ? "default"
            : "muted",
  } satisfies ActivityGraphNodeHandle;
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
  const rawValidationHandleErrors =
    input.node.kind === "workstation" && input.editor?.validationProjection
      ? validationHandleErrorsForNode(
          input.editor.validationProjection,
          input.node.id,
        )
      : undefined;
  const validationHandleErrors =
    rawValidationHandleErrors === undefined
      ? undefined
      : filterValidationHandleErrorsForWorkstation(
          rawValidationHandleErrors,
          anchorContext?.workstation,
        );

  return getLocalizedFactoryGraphConnectionAnchors(
    input.node.kind,
    input.locale,
    anchorContext,
  ).map((anchor) => {
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
    const handleValidation = validationHandleErrors?.get(anchor.id);
    const rendersHandleValidation =
      input.node.kind !== "workstation" ||
      !anchorContext ||
      workstationRendersProgressOutcomeHandleValidation(
        anchorContext,
        anchor.id,
      );

    return projectEditorConnectionAnchorHandle({
      anchor,
      canEditConnections,
      compatible,
      editor: input.editor,
      handleValidation,
      messages,
      node: input.node,
      nodeIsPendingRemoval,
      rendersHandleValidation,
      selected,
    });
  });
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
