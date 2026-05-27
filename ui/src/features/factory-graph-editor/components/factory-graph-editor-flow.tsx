import type { Edge, NodeProps } from "@xyflow/react";

import { cn } from "../../../lib/cn";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import { ActivityGraphNodeShell } from "../../flowchart/components/current-activity-node-shell";
import {
  GraphSemanticIcon,
  type GraphSemanticIconKind,
} from "../../flowchart/components/graph-semantic-icon";
import type {
  FactoryGraphNodeKind,
  FactoryGraphTopology,
} from "../lib/factory-graph-draft-types";
import type { FactoryGraphConnectionEndpoint } from "../lib/factory-graph-editor-connections";
import type { FactoryGraphWorkerRuntimeStatus } from "../lib/factory-graph-editor-runtime";
import {
  type FactoryGraphReactFlowNode,
  projectFactoryGraphToReactFlow,
} from "../lib/factory-graph-react-flow-projection";
import { FACTORY_GRAPH_EDITOR_EDGE_TYPES } from "./factory-graph-editor-edge";

type FactoryGraphEditorNode = FactoryGraphReactFlowNode;

const KIND_CLASS: Record<FactoryGraphNodeKind, string> = {
  resource: "border-af-success-border bg-af-success-surface",
  worker: "border-af-info-border bg-af-info-surface",
  workstation: "border-af-accent-border bg-af-accent-surface",
  "work-type": "border-af-border bg-af-surface-subtle",
  "work-state": "border-af-border-strong bg-af-surface-raised",
};

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
  const projection = projectFactoryGraphToReactFlow({
    editor: {
      canEditConnections: input.canEditConnections,
      onConnectionAnchorClick: input.onConnectionAnchorClick,
      pendingAdditionEdgeIds: input.pendingAdditionEdgeIds,
      pendingAdditionNodeIds: input.pendingAdditionNodeIds,
      pendingConnectionSource: input.pendingConnectionSource,
      pendingRemovalEdgeIds: input.pendingRemovalEdgeIds,
      pendingRemovalNodeIds: input.pendingRemovalNodeIds,
    },
    layoutPositionsByNodeId: input.layoutPositionsByNodeId,
    locale: input.locale,
    mode: "editor",
    runtime: {
      workerStatusByName: input.workerStatusByName,
    },
    topology: input.topology,
  });

  return projection;
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
        data.draftStatus === "addition" && "ring-2 ring-af-warning-border",
        data.draftStatus === "removal" &&
          "border-af-danger-border bg-af-danger-surface ring-2 ring-af-danger-border",
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
                className={cn("h-4 w-4", semanticIconClassName(data.kind))}
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
          <p className="m-0 text-[0.65rem] leading-5 text-af-text-subtle">
            {data.connectionHint}
          </p>
        ) : null}
      </div>
    </ActivityGraphNodeShell>
  );
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
      return "text-af-success";
    case "worker":
      return "text-af-info";
    case "workstation":
      return "text-af-text";
    case "work-type":
      return "text-af-info";
    case "work-state":
      return "text-af-text-muted";
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
