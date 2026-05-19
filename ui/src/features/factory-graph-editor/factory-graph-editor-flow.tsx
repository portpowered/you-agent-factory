import {
  MarkerType,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react";

import { cx } from "../../lib/cx";
import {
  ActivityGraphNodeShell,
  type ActivityGraphNodeHandle,
} from "../flowchart/current-activity-node-shell";
import type {
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";
import {
  getFactoryGraphConnectionAnchors,
  isValidFactoryGraphConnection,
  type FactoryGraphConnectionEndpoint,
} from "./factory-graph-editor-connections";
import type {
  FactoryGraphWorkerRuntimeStatus,
} from "./factory-graph-editor-runtime";
import { getFactoryGraphEditorMessages } from "./messages/editor";

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
  resource: "border-af-success/24 bg-af-success/6",
  worker: "border-af-info/24 bg-af-info/6",
  workstation: "border-af-accent/28 bg-af-accent/6",
  "work-type": "border-af-overlay/16 bg-af-overlay/4",
  "work-state": "border-af-overlay/18 bg-af-surface/92",
};
const WORKER_STATUS_CLASS: Record<FactoryGraphWorkerRuntimeStatus, string> = {
  active: "border-af-success/24 bg-af-success/10 text-af-success-ink",
  errored: "border-af-danger/24 bg-af-danger/10 text-af-danger-ink",
  idle: "border-af-overlay/14 bg-af-overlay/6 text-af-ink/68",
  unavailable: "border-af-warning/24 bg-af-warning/10 text-af-warning-ink",
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

export function buildFactoryGraphEditorFlowModel(input: {
  canEditConnections: boolean;
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
        position: {
          x: column * COLUMN_X,
          y: row * ROW_Y,
        },
        type: "factoryEntity",
      } satisfies FactoryGraphEditorNode;
    });

  return {
    edges: input.topology.edges.map((edge) => {
      const color = EDGE_COLOR_BY_KIND[edge.kind];
      const pendingAddition = input.pendingAdditionEdgeIds.has(edge.id);
      const pendingRemoval = input.pendingRemovalEdgeIds.has(edge.id);
      const edgeLabel = describeEdgeKind(edge.kind);
      return {
        animated:
          edge.kind === "workstation-on-continue" ||
          edge.kind === "workstation-on-failure" ||
          edge.kind === "workstation-on-rejection",
        ariaLabel: `${edgeLabel} from ${describeNodeKey(edge.source)} to ${describeNodeKey(edge.target)}`,
        ariaRole: "button",
        focusable: true,
        id: edge.id,
        interactionWidth: 24,
        label: edgeLabel,
        markerEnd: {
          color: pendingRemoval
            ? "var(--color-af-danger-ink)"
            : pendingAddition
              ? "var(--color-af-warning-ink)"
              : color,
          type: MarkerType.ArrowClosed,
        },
        source: edge.sourceId,
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
      } satisfies Edge;
    }),
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

function describeEdgeKind(kind: FactoryGraphTopology["edges"][number]["kind"]) {
  switch (kind) {
    case "worker-assignment":
      return "Worker assignment";
    case "worker-resource":
      return "Worker resource";
    case "work-type-state":
      return "State membership";
    case "workstation-input":
      return "Input route";
    case "workstation-on-continue":
      return "Continue route";
    case "workstation-on-failure":
      return "Failure route";
    case "workstation-on-rejection":
      return "Reject route";
    case "workstation-output":
      return "Success route";
    case "workstation-resource":
      return "Station resource";
    default:
      return String(kind);
  }
}

function describeNodeKey(key: FactoryGraphTopology["edges"][number]["source"]) {
  return key.kind === "work-state" ? `${key.workTypeName}:${key.stateName}` : key.name;
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
      className={cx(
        "min-w-0 w-full justify-start border text-left shadow-none",
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
      <div className="grid h-full min-w-0 content-start gap-2">
        <div className="flex items-center justify-between gap-2">
          <span className="rounded-full border border-af-overlay/14 bg-af-overlay/8 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.08em] text-af-ink/64">
            {data.kindLabel}
          </span>
          {data.draftStatus === "addition" ? (
            <span className="rounded-full border border-af-warning/24 bg-af-warning/10 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.08em] text-af-warning-ink">
              {data.pendingLabel}
            </span>
          ) : null}
          {data.draftStatus === "removal" ? (
            <span className="rounded-full border border-af-danger/24 bg-af-danger/10 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.08em] text-af-danger-ink">
              {data.removingLabel}
            </span>
          ) : null}
          {data.kind === "worker" && data.workerStatus ? (
            <span
              className={cx(
                "rounded-full border px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.08em]",
                WORKER_STATUS_CLASS[data.workerStatus],
              )}
            >
              {data.workerStatusLabel}
            </span>
          ) : null}
        </div>
        <p
          className="m-0 min-w-0 truncate font-mono text-sm font-bold leading-6 text-af-ink"
          title={data.label}
        >
          {data.label}
        </p>
        {data.canEditConnections ? (
          <p className="m-0 text-[0.65rem] leading-5 text-af-ink/60">
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
