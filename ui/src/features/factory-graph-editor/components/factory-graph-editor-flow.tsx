import type { Edge, NodeProps } from "@xyflow/react";

import { cn } from "../../../lib/cn";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeSurfaceClassName,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import { ActivityGraphNodeShell } from "../../flowchart/components/current-activity-node-shell";
import {
  GraphSemanticIcon,
  type GraphSemanticIconKind,
} from "../../flowchart/components/graph-semantic-icon";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphNodeKind,
  FactoryGraphTopology,
  FactoryWorkstation,
} from "../lib/factory-graph-draft-types";
import type { FactoryGraphConnectionEndpoint } from "../lib/factory-graph-editor-connections";
import { createFactoryGraphWorkstationResolver } from "../lib/factory-graph-editor-connections";
import type { FactoryGraphWorkerRuntimeStatus } from "../lib/factory-graph-editor-runtime";
import {
  type FactoryGraphReactFlowNode,
  projectFactoryGraphToReactFlow,
} from "../lib/factory-graph-react-flow-projection";
import { currentActivityGraphNodeHoverClassName } from "../../flowchart/lib/current-activity-graph-hover";
import {
  workStatePhaseSemanticIconClassName,
  workStatePhaseSemanticIconKind,
  workStatePhaseSurfaceClassName,
} from "../lib/factory-graph-work-state-phase-styling";
import type { FactoryGraphWorkStateType } from "../lib/factory-graph-work-state-type";
import type { FactoryValidationGraphProjection } from "../lib/factory-validation-graph-projection";
import { FACTORY_GRAPH_EDITOR_EDGE_TYPES } from "./factory-graph-editor-edge";

type FactoryGraphEditorNode = FactoryGraphReactFlowNode;

const KIND_CLASS: Record<FactoryGraphNodeKind, string> = {
  resource: activityGraphNodeSurfaceClassName("success"),
  worker: activityGraphNodeSurfaceClassName("info"),
  workstation: activityGraphNodeSurfaceClassName("primary"),
  "work-type": "border-outline bg-surface-container-low",
  "work-state": activityGraphNodeSurfaceClassName("neutralHigh"),
};

export const FACTORY_GRAPH_EDITOR_NODE_TYPES = {
  factoryEntity: FactoryGraphEditorNodeView,
};
export { FACTORY_GRAPH_EDITOR_EDGE_TYPES };

export function buildFactoryGraphEditorFlowModel(input: {
  canEditConnections: boolean;
  factoryDefinition?: CanonicalFactoryDefinition | null;
  layoutPositionsByNodeId?: ReadonlyMap<string, { x: number; y: number }>;
  locale?: string;
  onConnectionAnchorClick?: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingAdditionEdgeIds: ReadonlySet<string>;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  pendingAdditionNodeIds: ReadonlySet<string>;
  pendingRemovalEdgeIds: ReadonlySet<string>;
  pendingRemovalNodeIds: ReadonlySet<string>;
  topology: FactoryGraphTopology;
  validationProjection?: FactoryValidationGraphProjection;
  workstations?: readonly FactoryWorkstation[];
  workerStatusByName?: ReadonlyMap<string, FactoryGraphWorkerRuntimeStatus>;
}): {
  edges: Edge[];
  nodes: FactoryGraphEditorNode[];
} {
  const projection = projectFactoryGraphToReactFlow({
    factoryDefinition: input.factoryDefinition,
    filterEdgesToRenderedHandles: true,
    editor: {
      canEditConnections: input.canEditConnections,
      onConnectionAnchorClick: input.onConnectionAnchorClick,
      pendingAdditionEdgeIds: input.pendingAdditionEdgeIds,
      pendingAdditionNodeIds: input.pendingAdditionNodeIds,
      pendingConnectionSource: input.pendingConnectionSource,
      pendingRemovalEdgeIds: input.pendingRemovalEdgeIds,
      pendingRemovalNodeIds: input.pendingRemovalNodeIds,
      validationProjection: input.validationProjection,
    },
    layoutPositionsByNodeId: input.layoutPositionsByNodeId,
    locale: input.locale,
    mode: "editor",
    runtime: {
      workerStatusByName: input.workerStatusByName,
    },
    topology: input.topology,
    workstationResolver: createFactoryGraphWorkstationResolver(
      input.workstations,
      input.factoryDefinition?.workers,
    ),
  });

  return projection;
}

function FactoryGraphEditorNodeView({
  data,
  selected,
}: NodeProps<FactoryGraphEditorNode>) {
  const surfaceClassName =
    data.kind === "work-state"
      ? workStatePhaseSurfaceClassName(data.workStateType)
      : KIND_CLASS[data.kind];
  const semanticIconKind =
    data.workstationSemanticIconKind ??
    semanticIconKindForNodeKind(data.kind, data.workStateType);
  const semanticIconClassName =
    data.workstationSemanticIconClassName ??
    semanticIconClassNameForNodeKind(data.kind, data.workStateType);
  const semanticIconLabel =
    data.workstationSemanticLabel ?? data.kindLabel;

  return (
    <ActivityGraphNodeShell
      className={cn(
        "min-w-0 w-full justify-start overflow-hidden text-left shadow-none",
        surfaceClassName,
        data.workstationSemanticBorderClassName,
        data.draftStatus === "none"
          ? currentActivityGraphNodeHoverClassName({
              activeFlow: data.activeFlow,
              muted: data.muted,
              selected: selected || data.focused,
              validationError: data.validationMessage !== null,
            })
          : undefined,
        data.draftStatus === "addition" && "ring-2 ring-af-warning-border",
        data.draftStatus === "removal" &&
          cn(
            activityGraphNodeSurfaceClassName("danger"),
            "ring-2 ring-af-danger-border",
          ),
      )}
      handles={data.connectionAnchors}
      nodeType={data.kind === "workstation" ? "workstation" : "resource"}
      zAxisIncompleteHints={data.zAxisIncompleteHints}
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
                className={cn("h-4 w-4", semanticIconClassName)}
                kind={semanticIconKind}
                label={semanticIconLabel}
              />
            </span>
            <ActivityGraphNodeBadge weight="label">
              {semanticIconLabel}
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
          {data.kind === "work-type" && data.isDefaultWorkType ? (
            <ActivityGraphNodeBadge
              className="shrink-0"
              role="status"
              tone="info"
              weight="label"
            >
              {data.defaultWorkTypeLabel}
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
          <p className="m-0 text-[0.65rem] leading-5 text-on-surface-subtle">
            {data.connectionHint}
          </p>
        ) : null}
      </div>
    </ActivityGraphNodeShell>
  );
}

function semanticIconKindForNodeKind(
  kind: FactoryGraphNodeKind,
  workStateType?: FactoryGraphWorkStateType,
): GraphSemanticIconKind {
  if (kind === "work-state") {
    return workStatePhaseSemanticIconKind(workStateType);
  }

  switch (kind) {
    case "resource":
      return "resource";
    case "worker":
      return "active-work";
    case "workstation":
      return "workstation";
    case "work-type":
      return "constraint";
  }
}

function semanticIconClassNameForNodeKind(
  kind: FactoryGraphNodeKind,
  workStateType?: FactoryGraphWorkStateType,
) {
  if (kind === "work-state") {
    return workStatePhaseSemanticIconClassName(workStateType);
  }

  switch (kind) {
    case "resource":
      return "text-success";
    case "worker":
      return "text-info";
    case "workstation":
      return "text-on-surface";
    case "work-type":
      return "text-info";
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
