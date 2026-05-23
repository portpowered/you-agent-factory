import {
  MarkerType,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react";

import { cn } from "../../../lib/cn";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeTitleClassName,
} from "../../flowchart/current-activity-node-chrome";
import {
  ActivityGraphNodeShell,
  type ActivityGraphNodeHandle,
} from "../../flowchart/components/current-activity-node-shell";
import { GraphSemanticIcon, type GraphSemanticIconKind } from "../../flowchart/components/graph-semantic-icon";
import { FACTORY_GRAPH_EDITOR_EDGE_TYPES } from "./factory-graph-editor-edge";
import type {
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "../lib/factory-graph-draft-types";
import {
  getFactoryGraphConnectionAnchors,
  isValidFactoryGraphConnection,
  type FactoryGraphConnectionEndpoint,
} from "../lib/factory-graph-editor-connections";
import type {
  FactoryGraphWorkerRuntimeStatus,
} from "../lib/factory-graph-editor-runtime";
import { getFactoryGraphEditorMessages } from "../messages/editor";

type FactoryGraphEditorNode = Node<
  {
    canEditConnections: boolean;
    connectionAnchors: ActivityGraphNodeHandle[];
    connectionHint: string;
    draftStatus: "addition" | "none" | "removal";
    kind: FactoryGraphNodeKind;
    kindLabel: string;
    label: string;
    pendingLabel: string;
    removingLabel: string;
    workerStatus?: FactoryGraphWorkerRuntimeStatus;
    workerStatusLabel?: string;
  },
  "factoryEntity"
>;

const COLUMN_BY_KIND: Record<FactoryGraphNodeKind, number> = {
  resource: 0,
  worker: 1,
  workstation: 2,
  "work-type": 3,
  "work-state": 4,
};
const KIND_CLASS: Record<FactoryGraphNodeKind, string> = {
  resource: "border-af-overlay/22 bg-af-canvas",
  worker: "border-af-info/24 bg-af-surface/88",
  workstation: "border-2 border-af-info/28 bg-af-surface/88",
  "work-type": "border-af-overlay/18 bg-af-overlay/4",
  "work-state": "border-af-overlay/22 bg-af-canvas",
};
const EDGE_COLOR_BY_KIND = {
  "worker-assignment": "var(--color-af-info)",
  "worker-resource": "var(--color-af-success)",
  "work-type-state": "var(--color-af-overlay)",
  "workstation-input": "var(--color-af-accent)",
  "workstation-on-continue": "var(--color-af-info)",
  "workstation-on-failure": "var(--color-af-danger-ink)",
  "workstation-on-rejection": "var(--color-af-warning-ink)",
  "workstation-output": "var(--color-af-accent)",
  "workstation-resource": "var(--color-af-success)",
} as const;
const COLUMN_X = 232;
const ROW_Y = 118;

export const FACTORY_GRAPH_EDITOR_NODE_TYPES = {
  factoryEntity: FactoryGraphEditorNodeView,
};
export { FACTORY_GRAPH_EDITOR_EDGE_TYPES };

export function buildFactoryGraphEditorFlowModel(input: {
  canEditConnections: boolean;
  layoutPositionsByNodeId?: ReadonlyMap<string, { x: number; y: number }>;
  locale?: string;
  onConnectionAnchorClick?: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingAdditionEdgeIds: ReadonlySet<string>;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  pendingAdditionNodeIds: ReadonlySet<string>;
  pendingRemovalEdgeIds: ReadonlySet<string>;
  pendingRemovalNodeIds: ReadonlySet<string>;
  topology: FactoryGraphTopology;
  workerStatusByName?: ReadonlyMap<string, FactoryGraphWorkerRuntimeStatus>;
}): {
  edges: Edge[];
  nodes: FactoryGraphEditorNode[];
} {
  const messages = getFactoryGraphEditorMessages(input.locale);
  const rowCounts = new Map<number, number>();
  const counts = countHandles(input.topology);

  const nodes = [...input.topology.nodes]
    .sort((left, right) => {
      const leftColumn = COLUMN_BY_KIND[left.kind];
      const rightColumn = COLUMN_BY_KIND[right.kind];
      if (leftColumn !== rightColumn) {
        return leftColumn - rightColumn;
      }
      return left.label.localeCompare(right.label);
    })
    .map((node) => {
      const column = COLUMN_BY_KIND[node.kind];
      const row = rowCounts.get(column) ?? 0;
      rowCounts.set(column, row + 1);

      return {
        data: {
          canEditConnections: input.canEditConnections,
          connectionAnchors: buildNodeHandles({
            canEditConnections: input.canEditConnections,
            locale: input.locale,
            node,
            onConnectionAnchorClick: input.onConnectionAnchorClick,
            pendingConnectionSource: input.pendingConnectionSource,
            pendingRemovalNodeIds: input.pendingRemovalNodeIds,
            topology: input.topology,
          }),
          connectionHint: messages.flowConnectionHint,
          draftStatus: input.pendingRemovalNodeIds.has(node.id)
            ? "removal"
            : input.pendingAdditionNodeIds.has(node.id)
              ? "addition"
              : "none",
          kind: node.kind,
          kindLabel: messages.kindLabel(node.kind),
          label: node.label,
          pendingLabel: messages.flowPendingLabel,
          removingLabel: messages.flowRemovingLabel,
          workerStatus:
            node.kind === "worker"
              ? input.workerStatusByName?.get(node.label) ?? "idle"
              : undefined,
          workerStatusLabel:
            node.kind === "worker"
              ? messages.workerStatusLabel(
                  input.workerStatusByName?.get(node.label) ?? "idle",
                )
              : undefined,
        },
        draggable: true,
        id: node.id,
        position:
          input.layoutPositionsByNodeId?.get(node.id) ?? {
            x: column * COLUMN_X,
            y: row * ROW_Y,
          },
        type: "factoryEntity",
      } satisfies FactoryGraphEditorNode;
    });

  return {
    edges: input.topology.edges.map((edge) => buildFactoryGraphEditorEdge(edge, input)),
    nodes: nodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        incomingHandleCount: counts.incoming.get(node.id) ?? 1,
        outgoingHandleCount: counts.outgoing.get(node.id) ?? 1,
      },
    })) as FactoryGraphEditorNode[],
  };
}

function buildFactoryGraphEditorEdge(
  edge: FactoryGraphTopology["edges"][number],
  input: Pick<
    Parameters<typeof buildFactoryGraphEditorFlowModel>[0],
    | "canEditConnections"
    | "locale"
    | "pendingAdditionEdgeIds"
    | "pendingConnectionSource"
    | "pendingRemovalEdgeIds"
  >,
) {
  const messages = getFactoryGraphEditorMessages(input.locale);
  const color = EDGE_COLOR_BY_KIND[edge.kind];
  const pendingAddition = input.pendingAdditionEdgeIds.has(edge.id);
  const pendingRemoval = input.pendingRemovalEdgeIds.has(edge.id);
  const edgeLabel = messages.edgeKindLabel(edge.kind);
  const handleAssignment = getEdgeHandleAssignment(edge);

  return {
    animated:
      edge.kind === "workstation-on-continue" ||
      edge.kind === "workstation-on-failure" ||
      edge.kind === "workstation-on-rejection",
    data: {
      alwaysShowLabel:
        input.canEditConnections || input.pendingConnectionSource !== null,
      label: edgeLabel,
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
        ? "var(--color-af-danger-ink)"
        : pendingAddition
          ? "var(--color-af-warning-ink)"
          : color,
      type: MarkerType.ArrowClosed,
    },
    source: edge.sourceId,
    sourceHandle: handleAssignment?.sourceHandle,
    style: {
      opacity: pendingRemoval ? 0.48 : 1,
      stroke: pendingRemoval
        ? "var(--color-af-danger-ink)"
        : pendingAddition
          ? "var(--color-af-warning-ink)"
          : color,
      strokeDasharray: pendingRemoval
        ? "7 5"
        : pendingAddition
          ? "9 4"
          : edge.kind === "worker-resource" || edge.kind === "workstation-resource"
            ? "4 5"
            : undefined,
      strokeWidth: pendingRemoval || pendingAddition ? 2 : 1.7,
    },
    target: edge.targetId,
    targetHandle: handleAssignment?.targetHandle,
    type: "factoryEditorEdge",
  } satisfies Edge;
}

function describeNodeKey(key: FactoryGraphTopology["edges"][number]["source"]) {
  return key.kind === "work-state" ? `${key.workTypeName}:${key.stateName}` : key.name;
}

function getEdgeHandleAssignment(
  edge: FactoryGraphTopology["edges"][number],
): { sourceHandle: string; targetHandle: string } | null {
  const sourceHandle = getNodeHandleId(edge.source.kind, edge.kind, "source");
  const targetHandle = getNodeHandleId(edge.target.kind, edge.kind, "target");
  if (!sourceHandle || !targetHandle) {
    return null;
  }
  return { sourceHandle, targetHandle };
}

function getNodeHandleId(
  nodeKind: FactoryGraphNodeKind,
  edgeKind: FactoryGraphTopology["edges"][number]["kind"],
  role: "source" | "target",
) {
  return (
    getFactoryGraphConnectionAnchors(nodeKind).find(
      (anchor) => anchor.role === role && anchor.edgeKind === edgeKind,
    )?.id ?? null
  );
}

function FactoryGraphEditorNodeView({
  data,
}: NodeProps<
  FactoryGraphEditorNode & {
    data: FactoryGraphEditorNode["data"] & {
      incomingHandleCount: number;
      outgoingHandleCount: number;
    };
  }
>) {
  return (
    <ActivityGraphNodeShell
      className={cn(
        "min-w-0 w-full justify-start overflow-hidden text-left shadow-none",
        KIND_CLASS[data.kind],
        data.draftStatus === "addition" && "ring-2 ring-af-warning/34",
        data.draftStatus === "removal" &&
          "border-af-danger/28 bg-af-danger/8 opacity-70 ring-2 ring-af-danger/24",
      )}
      handles={data.connectionAnchors}
      incomingHandleCount={data.incomingHandleCount}
      nodeType={data.kind === "workstation" ? "workstation" : "resource"}
      outgoingHandleCount={data.outgoingHandleCount}
    >
      <div className="grid h-full min-w-0 content-start gap-2.5">
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2 overflow-hidden">
            <span
              className="flex min-h-5 shrink-0 items-center"
              data-factory-entity-semantic-icon
              title={data.kindLabel}
            >
              <GraphSemanticIcon
                className={cn(
                  "h-4 w-4",
                  semanticIconClassName(data.kind),
                )}
                kind={semanticIconKind(data.kind)}
                label={data.kindLabel}
              />
            </span>
            <ActivityGraphNodeBadge weight="label">
              {data.kindLabel}
            </ActivityGraphNodeBadge>
          </div>
          {data.kind === "worker" && data.workerStatus ? (
            <ActivityGraphNodeBadge
              className="shrink-0"
              tone={workerStatusTone(data.workerStatus)}
              weight="label"
            >
              {data.workerStatusLabel}
            </ActivityGraphNodeBadge>
          ) : null}
          {data.draftStatus === "addition" ? (
            <ActivityGraphNodeBadge
              className="shrink-0"
              tone="warning"
              weight="label"
            >
              {data.pendingLabel}
            </ActivityGraphNodeBadge>
          ) : null}
          {data.draftStatus === "removal" ? (
            <ActivityGraphNodeBadge
              className="shrink-0"
              tone="danger"
              weight="label"
            >
              {data.removingLabel}
            </ActivityGraphNodeBadge>
          ) : null}
        </div>
        <p
          className={cn(
            "m-0",
            activityGraphNodeTitleClassName(
              data.kind === "workstation"
                ? "font-mono text-[1rem]"
                : "font-mono text-[0.88rem]",
            ),
          )}
          data-factory-entity-title
          title={data.label}
        >
          {data.label}
        </p>
        {data.canEditConnections ? (
          <p className="m-0 text-[0.68rem] leading-5 text-af-ink/60">
            {data.connectionHint}
          </p>
        ) : null}
      </div>
    </ActivityGraphNodeShell>
  );
}

function countHandles(topology: FactoryGraphTopology) {
  const incoming = new Map<string, number>();
  const outgoing = new Map<string, number>();

  for (const edge of topology.edges) {
    incoming.set(edge.targetId, (incoming.get(edge.targetId) ?? 0) + 1);
    outgoing.set(edge.sourceId, (outgoing.get(edge.sourceId) ?? 0) + 1);
  }

  return { incoming, outgoing };
}

function buildNodeHandles(input: {
  canEditConnections: boolean;
  locale?: string;
  node: FactoryGraphTopology["nodes"][number];
  onConnectionAnchorClick?: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  pendingRemovalNodeIds: ReadonlySet<string>;
  topology: FactoryGraphTopology;
}): ActivityGraphNodeHandle[] {
  const messages = getFactoryGraphEditorMessages(input.locale);
  const selectedSource =
    input.pendingConnectionSource?.nodeId === input.node.id
      ? input.pendingConnectionSource
      : null;
  const nodeIsPendingRemoval = input.pendingRemovalNodeIds.has(input.node.id);

  return getFactoryGraphConnectionAnchors(input.node.kind).map((anchor) => {
    const selected =
      selectedSource?.anchorId === anchor.id && anchor.role === "source";
    const compatible =
      input.pendingConnectionSource !== null &&
      input.pendingConnectionSource.nodeId !== input.node.id &&
      anchor.role === "target" &&
      isValidFactoryGraphConnection({
        sourceAnchorId: input.pendingConnectionSource.anchorId,
        sourceNodeKind: findNode(
          input.topology,
          input.pendingConnectionSource.nodeId,
        ).kind,
        targetAnchorId: anchor.id,
        targetNodeKind: input.node.kind,
      });

    return {
      buttonAriaLabel:
        anchor.role === "source"
          ? `${messages.toolbarConnectLabel}: ${input.node.label} ${anchor.label}`
          : `${messages.toolbarConnectLabel}: ${input.node.label} ${anchor.label}`,
      buttonDisabled: !input.canEditConnections || nodeIsPendingRemoval,
      buttonPressed: selected || undefined,
      buttonTitle: anchor.description,
      connectable: input.canEditConnections && !nodeIsPendingRemoval,
      id: anchor.id,
      label: anchor.label,
      onButtonClick:
        input.onConnectionAnchorClick &&
        input.canEditConnections &&
        !nodeIsPendingRemoval
          ? () =>
              input.onConnectionAnchorClick?.({
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
          : input.canEditConnections
            ? "default"
            : "muted",
    } satisfies ActivityGraphNodeHandle;
  });
}

function findNode(topology: FactoryGraphTopology, nodeId: string) {
  const node = topology.nodes.find((entry) => entry.id === nodeId);
  if (!node) {
    throw new Error(`Expected graph node "${nodeId}" to exist in editor topology.`);
  }
  return node;
}

function semanticIconKind(kind: FactoryGraphNodeKind): GraphSemanticIconKind {
  switch (kind) {
    case "resource":
      return "resource";
    case "worker":
      return "active-work";
    case "workstation":
      return "workstation";
    case "work-type":
      return "constraint";
    case "work-state":
      return "queue";
  }
}

function semanticIconClassName(kind: FactoryGraphNodeKind) {
  switch (kind) {
    case "resource":
      return "text-af-success-ink/76";
    case "worker":
      return "text-af-info/78";
    case "workstation":
      return "text-af-ink/62";
    case "work-type":
      return "text-af-info/74";
    case "work-state":
      return "text-af-ink/58";
  }
}

function workerStatusTone(
  status: FactoryGraphWorkerRuntimeStatus,
): "danger" | "neutral" | "success" | "warning" {
  switch (status) {
    case "active":
      return "success";
    case "errored":
      return "danger";
    case "idle":
      return "neutral";
    case "unavailable":
      return "warning";
  }
}
