import {
  MarkerType,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react";

import { cx } from "../../lib/cx";
import { ActivityGraphNodeShell } from "../flowchart/current-activity-node-shell";
import type {
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "./factory-graph-draft-types";

type FactoryGraphEditorNode = Node<
  {
    kind: FactoryGraphNodeKind;
    label: string;
    pending: boolean;
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
  pendingNodeIds: ReadonlySet<string>;
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
          kind: node.kind,
          label: node.label,
          pending: input.pendingNodeIds.has(node.id),
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
          stroke: color,
          strokeDasharray:
            edge.kind === "worker-resource" || edge.kind === "workstation-resource"
              ? "4 5"
              : undefined,
          strokeWidth: 1.7,
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
        data.pending && "ring-2 ring-af-warning/34",
      )}
      incomingHandleCount={data.incomingHandleCount}
      nodeType={data.kind === "workstation" ? "workstation" : "resource"}
      outgoingHandleCount={data.outgoingHandleCount}
    >
      <div className="grid h-full min-w-0 content-start gap-2">
        <div className="flex items-center justify-between gap-2">
          <span className="rounded-full border border-af-overlay/14 bg-af-overlay/8 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.08em] text-af-ink/64">
            {KIND_LABEL[data.kind]}
          </span>
          {data.pending ? (
            <span className="rounded-full border border-af-warning/24 bg-af-warning/10 px-2 py-1 text-[0.65rem] font-semibold uppercase tracking-[0.08em] text-af-warning-ink">
              Pending
            </span>
          ) : null}
        </div>
        <p
          className="m-0 min-w-0 truncate font-mono text-sm font-bold leading-6 text-af-ink"
          title={data.label}
        >
          {data.label}
        </p>
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
