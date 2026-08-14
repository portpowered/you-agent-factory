import type { Edge, NodeProps } from "@xyflow/react";
import {
  FactoryGraphWorkstationGuardedControlCard,
  factoryGraphNodeFamilyForShellType,
  factoryGraphNodeVisualIconClassName,
  factoryGraphNodeWrappedTextClassName,
  factoryGraphWorkstationPresentation,
  resolveFactoryGraphVisualState,
} from "@you-agent-factory/factory-graph";

import { cn } from "../../../../lib/cn";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeSurfaceClassName,
  activityGraphNodeTitleClassName,
} from "../../../flowchart/components/current-activity-node-chrome";
import { ActivityGraphNodeShell } from "../../../flowchart/components/current-activity-node-shell";
import {
  GraphSemanticIcon,
  type GraphSemanticIconKind,
} from "../../../flowchart/components/graph-semantic-icon";
import { currentActivityGraphNodeHoverClassName } from "../../../flowchart/lib/current-activity-graph-hover";
import { FACTORY_GRAPH_EDGE_TYPES } from "../../../graphs/components/factory-graph-edge";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphNodeKind,
  FactoryGraphTopology,
  FactoryWorkstation,
} from "../../lib/draft/factory-graph-draft-types";
import type { FactoryLayout } from "../../lib/layout/factory-graph-layout-operations";
import type { FactoryGraphConnectionEndpoint } from "../../lib/editor/factory-graph-editor-connections";
import { createFactoryGraphWorkstationResolver } from "../../lib/editor/factory-graph-editor-connections";
import type { FactoryGraphWorkerRuntimeStatus } from "../../lib/editor-runtime/factory-graph-editor-runtime";
import {
  type FactoryGraphReactFlowNode,
  projectFactoryGraphToReactFlow,
} from "../../lib/projection/factory-graph-react-flow-projection";
import type { FactoryValidationGraphProjection } from "../../lib/projection/factory-validation-graph-projection";
import {
  workStatePhaseSemanticIconClassName,
  workStatePhaseSemanticIconKind,
  workStatePhaseSurfaceClassName,
} from "../../lib/work-state/factory-graph-work-state-phase-styling";
import type { FactoryGraphWorkStateType } from "../../lib/work-state/factory-graph-work-state-type";

type FactoryGraphEditorNode = FactoryGraphReactFlowNode;

const KIND_CLASS: Record<FactoryGraphNodeKind, string> = {
  doc: activityGraphNodeSurfaceClassName("neutral"),
  resource: activityGraphNodeSurfaceClassName("resource"),
  worker: activityGraphNodeSurfaceClassName("info"),
  workstation: activityGraphNodeSurfaceClassName("workstation"),
  "work-type": activityGraphNodeSurfaceClassName("info"),
  "work-state": activityGraphNodeSurfaceClassName("workState"),
};

export const FACTORY_GRAPH_EDITOR_NODE_TYPES = {
  factoryEntity: FactoryGraphEditorNodeView,
};
export {
  FACTORY_GRAPH_EDGE_TYPES,
  FACTORY_GRAPH_EDGE_TYPES as FACTORY_GRAPH_EDITOR_EDGE_TYPES,
};

export function buildFactoryGraphEditorFlowModel(input: {
  canEditConnections: boolean;
  factoryDefinition?: CanonicalFactoryDefinition | null;
  layout?: FactoryLayout | null;
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
    layout: input.layout,
    layoutPositionsByNodeId: input.layoutPositionsByNodeId,
    locale: input.locale,
    mode: "editor",
    runtime: {
      workerStatusByName: input.workerStatusByName,
    },
    topology: input.topology,
    workstations: input.workstations,
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
  if (data.kind === "worker") {
    return <FactoryGraphEditorWorkerNodeView data={data} selected={selected} />;
  }

  return <FactoryGraphEditorSemanticNodeView data={data} selected={selected} />;
}

function FactoryGraphEditorSemanticNodeView({
  data,
  selected,
}: {
  data: FactoryGraphEditorNode["data"];
  selected: boolean;
}) {
  const shellNodeType = editorShellNodeType(data.kind);
  const visualState = resolveFactoryGraphVisualState({
    activeFlow: data.activeFlow,
    family: factoryGraphNodeFamilyForShellType(shellNodeType),
    focused: data.focused,
    lifecycle:
      data.kind === "work-state"
        ? data.workStateType
        : data.kind === "workstation" && data.active
          ? "PROCESSING"
          : undefined,
    muted: data.muted,
    selected,
    validation: data.validationMessage !== null ? "error" : undefined,
  });
  const surfaceClassName =
    data.kind === "work-state"
      ? workStatePhaseSurfaceClassName(data.workStateType)
      : KIND_CLASS[data.kind];
  const workstationPresentation =
    data.kind === "workstation"
      ? factoryGraphWorkstationPresentation(
          data.workstationSemantics,
          data.locale,
        )
      : undefined;
  const semanticIconKind =
    workstationPresentation?.iconKind ??
    semanticIconKindForNodeKind(data.kind, data.workStateType);
  const semanticIconClassName =
    workstationPresentation?.className ??
    semanticIconClassNameForNodeKind(data.kind, data.workStateType);
  const semanticIconLabel = workstationPresentation?.label ?? data.kindLabel;

  return (
    <ActivityGraphNodeShell
      className={cn(
        "min-w-0 w-full justify-start overflow-hidden text-left shadow-none",
        surfaceClassName,
        workstationPresentation?.borderClassName,
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
      nodeType={shellNodeType}
      visualState={{
        activeFlow: data.activeFlow,
        focused: data.focused,
        lifecycle: visualState.lifecycle,
        muted: data.muted,
        selected,
        validation: visualState.validation,
      }}
      zAxisIncompleteHints={data.zAxisIncompleteHints}
    >
      <FactoryGraphEditorNodeContent
        data={data}
        semanticIconClassName={semanticIconClassName}
        semanticIconKind={semanticIconKind}
        semanticIconLabel={semanticIconLabel}
        visualState={visualState}
        workstationPresentation={workstationPresentation}
      />
    </ActivityGraphNodeShell>
  );
}

function FactoryGraphEditorNodeContent({
  data,
  semanticIconClassName,
  semanticIconKind,
  semanticIconLabel,
  visualState,
  workstationPresentation,
}: {
  data: FactoryGraphEditorNode["data"];
  semanticIconClassName: string;
  semanticIconKind: GraphSemanticIconKind;
  semanticIconLabel: string;
  visualState: ReturnType<typeof resolveFactoryGraphVisualState>;
  workstationPresentation?: ReturnType<
    typeof factoryGraphWorkstationPresentation
  >;
}) {
  return (
    <div className="grid h-full min-w-0 content-start gap-2.5">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2 overflow-hidden">
          <span
            className="flex min-h-5 shrink-0 items-center"
            data-factory-entity-semantic-icon
            title={data.kindLabel}
          >
            <GraphSemanticIcon
              className={cn(
                "h-4 w-4",
                factoryGraphNodeVisualIconClassName(
                  visualState,
                  semanticIconClassName,
                ),
              )}
              kind={semanticIconKind}
              label={semanticIconLabel}
            />
          </span>
          <ActivityGraphNodeBadge
            className={factoryGraphNodeWrappedTextClassName()}
            weight="label"
          >
            {semanticIconLabel}
          </ActivityGraphNodeBadge>
        </div>
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
        {workstationPresentation?.schedulingLabel ? (
          <ActivityGraphNodeBadge
            className="shrink-0"
            tone="neutral"
            weight="label"
          >
            {workstationPresentation.schedulingLabel}
          </ActivityGraphNodeBadge>
        ) : null}
        {data.draftStatus === "addition" ? (
          <ActivityGraphNodeBadge
            className="shrink-0"
            role="status"
            tone="warning"
            weight="label"
          >
            {data.pendingLabel}
          </ActivityGraphNodeBadge>
        ) : null}
        {data.draftStatus === "removal" ? (
          <ActivityGraphNodeBadge
            className="shrink-0"
            role="status"
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
      {workstationPresentation ? (
        <FactoryGraphWorkstationGuardedControlCard
          locale={data.locale}
          presentation={workstationPresentation}
        />
      ) : null}
      {data.canEditConnections ? (
        <p className="m-0 text-[0.65rem] leading-5 text-on-surface-subtle">
          {data.connectionHint}
        </p>
      ) : null}
    </div>
  );
}

function FactoryGraphEditorWorkerNodeView({
  data,
  selected,
}: {
  data: FactoryGraphEditorNode["data"];
  selected: boolean;
}) {
  const visualState = resolveFactoryGraphVisualState({
    activeFlow: data.activeFlow,
    family: "worker",
    focused: data.focused,
    muted: data.muted,
    selected,
    validation: data.validationMessage !== null ? "error" : undefined,
  });
  return (
    <ActivityGraphNodeShell
      className={cn(
        "min-w-0 w-full justify-center overflow-hidden text-left shadow-none",
        KIND_CLASS.worker,
        currentActivityGraphNodeHoverClassName({
          activeFlow: data.activeFlow,
          muted: data.muted,
          selected: selected || data.focused,
          validationError: data.validationMessage !== null,
        }),
        data.draftStatus === "addition" && "ring-2 ring-af-warning-border",
        data.draftStatus === "removal" &&
          cn(
            activityGraphNodeSurfaceClassName("danger"),
            "ring-2 ring-af-danger-border",
          ),
      )}
      handles={data.connectionAnchors}
      nodeType="worker"
      visualState={{
        activeFlow: data.activeFlow,
        focused: data.focused,
        muted: data.muted,
        selected,
        validation: visualState.validation,
      }}
      zAxisIncompleteHints={data.zAxisIncompleteHints}
    >
      <div className="flex min-w-0 items-center justify-between gap-2 overflow-hidden">
        <div className="flex min-w-0 flex-wrap items-start gap-1.5 overflow-hidden">
          <span
            className="flex shrink-0 items-center"
            data-factory-entity-semantic-icon
            title={data.kindLabel}
          >
            <GraphSemanticIcon
              className={cn(
                "h-3.5 w-3.5",
                factoryGraphNodeVisualIconClassName(visualState, "text-info"),
              )}
              kind="active-work"
              label={data.kindLabel}
            />
          </span>
          <div className="grid min-w-0 gap-px overflow-hidden">
            <span
              className={factoryGraphNodeWrappedTextClassName(
                "block overflow-hidden text-[0.62rem] font-bold uppercase leading-none text-info",
              )}
            >
              {data.kindLabel}
            </span>
            <p
              className={cn(
                "m-0 min-w-0",
                activityGraphNodeTitleClassName("font-mono text-[0.8rem]"),
              )}
              data-factory-entity-title
              title={data.label}
            >
              {data.label}
            </p>
          </div>
        </div>
        {data.workerStatus ? (
          <ActivityGraphNodeBadge
            className="shrink-0"
            tone={workerStatusTone(data.workerStatus)}
            weight="label"
          >
            {data.workerStatusLabel}
          </ActivityGraphNodeBadge>
        ) : null}
      </div>
    </ActivityGraphNodeShell>
  );
}

function editorShellNodeType(
  kind: FactoryGraphNodeKind,
):
  | "constraint"
  | "doc"
  | "resource"
  | "statePosition"
  | "worker"
  | "workType"
  | "workstation" {
  switch (kind) {
    case "work-state":
      return "statePosition";
    case "work-type":
      return "workType";
    default:
      return kind;
  }
}

function semanticIconKindForNodeKind(
  kind: FactoryGraphNodeKind,
  workStateType?: FactoryGraphWorkStateType,
): GraphSemanticIconKind {
  if (kind === "work-state") {
    return workStatePhaseSemanticIconKind(workStateType);
  }

  switch (kind) {
    case "doc":
      return "doc";
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
    case "doc":
      return "text-on-surface-variant";
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
