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

type FactoryGraphEditorNode = Node<
  {
    canEditConnections: boolean;
    connectionAnchors: ActivityGraphNodeHandle[];
    draftStatus: "addition" | "none" | "removal";
    kind: FactoryGraphNodeKind;
    label: string;
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
const KIND_LABEL: Record<FactoryGraphNodeKind, string> = {
  resource: "Resource",
  worker: "Worker",
  workstation: "Workstation",
  "work-type": "Work type",
  "work-state": "Work state",
};
const KIND_CLASS: Record<FactoryGraphNodeKind, string> = {
  resource: "border-af-success/24 bg-af-success/6",
  worker: "border-af-info/24 bg-af-info/6",
  workstation: "border-af-accent/28 bg-af-accent/6",
  "work-type": "border-af-overlay/16 bg-af-overlay/4",
  "work-state": "border-af-overlay/18 bg-af-surface/92",
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
  onConnectionAnchorClick?: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  pendingAdditionNodeIds: ReadonlySet<string>;
  pendingRemovalEdgeIds: ReadonlySet<string>;
  pendingRemovalNodeIds: ReadonlySet<string>;
  topology: FactoryGraphTopology;
}): {
  edges: Edge[];
  nodes: FactoryGraphEditorNode[];
} {
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
            node,
            onConnectionAnchorClick: input.onConnectionAnchorClick,
            pendingConnectionSource: input.pendingConnectionSource,
            pendingRemovalNodeIds: input.pendingRemovalNodeIds,
            topology: input.topology,
          }),
          draftStatus: input.pendingRemovalNodeIds.has(node.id)
            ? "removal"
            : input.pendingAdditionNodeIds.has(node.id)
              ? "addition"
              : "none",
          kind: node.kind,
          label: node.label,
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
      const pendingRemoval = input.pendingRemovalEdgeIds.has(edge.id);
      return {
        animated:
          edge.kind === "workstation-on-continue" ||
          edge.kind === "workstation-on-failure" ||
          edge.kind === "workstation-on-rejection",
        id: edge.id,
        label: edge.kind,
        markerEnd: {
          color,
          type: MarkerType.ArrowClosed,
        },
        source: edge.sourceId,
        style: {
          opacity: pendingRemoval ? 0.48 : 1,
          stroke: pendingRemoval ? "var(--color-af-danger-ink)" : color,
          strokeDasharray: pendingRemoval
            ? "7 5"
            : edge.kind === "worker-resource" || edge.kind === "workstation-resource"
              ? "4 5"
              : undefined,
          strokeWidth: pendingRemoval ? 2 : 1.7,
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
            {KIND_LABEL[data.kind]}
          </span>
          {data.draftStatus === "addition" ? (
            <span className="rounded-full border border-af-warning/24 bg-af-warning/10 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.08em] text-af-warning-ink">
              Pending
            </span>
          ) : null}
          {data.draftStatus === "removal" ? (
            <span className="rounded-full border border-af-danger/24 bg-af-danger/10 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.08em] text-af-danger-ink">
              Removing
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
            Use labeled anchors for compatible connections.
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
  node: FactoryGraphTopology["nodes"][number];
  onConnectionAnchorClick?: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  pendingRemovalNodeIds: ReadonlySet<string>;
  topology: FactoryGraphTopology;
}): ActivityGraphNodeHandle[] {
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
          ? `Choose ${input.node.label} ${anchor.label} connection source`
          : `Connect to ${input.node.label} ${anchor.label} anchor`,
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
