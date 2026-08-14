// biome-ignore lint/style/noExcessiveLinesPerFile: editor handle and resize contracts remain colocated with the shared graph interaction mapping.
import type {
  FactoryGraphNodeDimensions,
  FactoryGraphNodeFamily,
  FactoryGraphNodeResizeLabels,
} from "@you-agent-factory/factory-graph";
import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type { FactoryValidationTarget } from "../../../api/factory-validation";
import {
  workstationHasZAxisIncompleteForConnections,
  workstationSupportsProgressOutcomeFailureRoute,
  workstationSupportsProgressOutcomeRoutes,
} from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import type {
  FactoryGraphEdgeKind,
  FactoryGraphNodeKind,
  WorkstationToWorkStateRouteKind,
} from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  type FactoryGraphConnectionAnchorContext,
  type FactoryGraphConnectionEndpoint,
  factoryGraphConnectionAnchorContext,
  getFactoryGraphConnectionAnchors,
  getLocalizedFactoryGraphConnectionAnchors,
  mergeAuthoredProgressOutcomeConnectionAnchors,
  PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS,
} from "../../factory-graph-editor/lib/editor/factory-graph-editor-connections";
import {
  workstationRendersProgressOutcomeHandleValidation,
  workstationRendersProgressOutcomeZAxisHintAnchors,
} from "../../factory-graph-editor/lib/projection/factory-graph-progress-outcome-handle-visibility";
import type { FactoryValidationGraphProjection } from "../../factory-graph-editor/lib/projection/factory-validation-graph-projection";
import { validationHandleErrorsForNode } from "../../factory-graph-editor/lib/projection/factory-validation-graph-projection";
import { getFactoryGraphEditorMessages } from "../../factory-graph-editor/messages/editor";
import type {
  ActivityGraphNodeHandle,
  ZAxisIncompleteHints,
} from "../../flowchart/components/current-activity-node-shell";
import type {
  PositionedEdge,
  PositionedNode,
} from "../../flowchart/lib/layout";

export interface CurrentActivityEditorState {
  activeTool: "add" | "connect" | "delete" | null;
  canInteractWithEditor: boolean;
  editorMode: boolean;
  nodeResizeControls?: CurrentActivityNodeResizeController;
  onConnectionAnchorClick: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
  validationTargets?: readonly FactoryValidationTarget[];
}

export interface CurrentActivityNodeResizeTarget {
  family: FactoryGraphNodeFamily;
  nodeId: string;
  position: { x: number; y: number };
}

export interface CurrentActivityNodeResizeController {
  enabled: boolean;
  labels: FactoryGraphNodeResizeLabels;
  onFitToContent: (
    target: CurrentActivityNodeResizeTarget,
    dimensions: FactoryGraphNodeDimensions,
  ) => void;
  onResetSize: (target: CurrentActivityNodeResizeTarget) => void;
  onResizeEnd: (
    target: CurrentActivityNodeResizeTarget,
    dimensions: FactoryGraphNodeDimensions,
  ) => void;
}

/** Graph node header selection competes with delete-tool onNodeClick when wired. */
export function shouldWireGraphNodeSelectionHandlers(
  editor?: CurrentActivityEditorState,
): boolean {
  return !(editor?.editorMode === true && editor.activeTool === "delete");
}

type CurrentActivityEndpointKind = Extract<
  FactoryGraphNodeKind,
  "resource" | "worker" | "work-state" | "workstation"
>;

export function supportedSemanticHandleIdsForEdge(
  edge: PositionedEdge,
  nodeKindsById?: ReadonlyMap<string, PositionedNode["nodeKind"]>,
) {
  const edgeKind = edgeKindForPositionedEdge(edge);
  const sourceKind = endpointNodeKind(
    edge.fromNodeId,
    edge.sourcePlaceKind,
    nodeKindsById,
  );
  const targetKind = endpointNodeKind(
    edge.toNodeId,
    edge.targetPlaceKind,
    nodeKindsById,
  );
  if (!edgeKind || !sourceKind || !targetKind) {
    return null;
  }

  if (edgeKind === "work-state-visibility-bypass") {
    const outcomeRouteKind = visibilityBypassOutcomeRouteKind(edge.edgeId);
    if (!outcomeRouteKind) {
      return null;
    }

    return {
      sourceHandleId: visibilityBypassSourceHandleId(outcomeRouteKind),
      targetHandleId: "workstation-input-target",
    };
  }

  const sourceHandleId = connectionAnchorId(sourceKind, edgeKind, "source");
  const targetHandleId = connectionAnchorId(targetKind, edgeKind, "target");
  if (!sourceHandleId || !targetHandleId) {
    return null;
  }

  return {
    sourceHandleId,
    targetHandleId,
  };
}

export const supportedEditorHandleIdsForEdge =
  supportedSemanticHandleIdsForEdge;

export function authoredProgressOutcomeSourceHandlesByWorkstationNodeId(
  edges: readonly PositionedEdge[],
  nodes: readonly PositionedNode[],
): ReadonlyMap<string, ReadonlySet<string>> {
  const nodeKindsById = new Map(
    nodes.map((node) => [node.nodeId, node.nodeKind]),
  );
  const handlesByWorkstationNodeId = new Map<string, Set<string>>();

  for (const edge of edges) {
    const supportedHandles = supportedSemanticHandleIdsForEdge(
      edge,
      nodeKindsById,
    );
    if (
      !supportedHandles ||
      !PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS.has(supportedHandles.sourceHandleId)
    ) {
      continue;
    }

    if (!edge.fromNodeId.startsWith("workstation:")) {
      continue;
    }

    const handleIds =
      handlesByWorkstationNodeId.get(edge.fromNodeId) ?? new Set<string>();
    handleIds.add(supportedHandles.sourceHandleId);
    handlesByWorkstationNodeId.set(edge.fromNodeId, handleIds);
  }

  return handlesByWorkstationNodeId;
}

export function resolveWorkstationConnectionAnchorContext(
  factory: CanonicalFactoryDefinition | undefined,
  factoryGraphNodeId: string,
): FactoryGraphConnectionAnchorContext | undefined {
  const workstationName = factoryGraphNodeId.startsWith("workstation:")
    ? factoryGraphNodeId.slice("workstation:".length)
    : factoryGraphNodeId;
  const workstation = (factory?.workstations ?? []).find(
    (candidate) =>
      candidate.name === workstationName || candidate.id === workstationName,
  );

  return workstation
    ? factoryGraphConnectionAnchorContext(workstation, factory?.workers)
    : undefined;
}

export function resolveZAxisIncompleteHints(args: {
  connectionAnchorContext?: FactoryGraphConnectionAnchorContext;
  editor?: CurrentActivityEditorState;
  locale?: string | null;
  nodeKind: FactoryGraphNodeKind;
}): ZAxisIncompleteHints | null {
  if (
    args.nodeKind !== "workstation" ||
    !args.connectionAnchorContext ||
    args.editor?.editorMode !== true ||
    args.editor.activeTool === "delete" ||
    !args.editor.canInteractWithEditor
  ) {
    return null;
  }

  if (
    !workstationHasZAxisIncompleteForConnections(
      args.connectionAnchorContext.workstation,
    ) ||
    !workstationRendersProgressOutcomeZAxisHintAnchors(
      args.connectionAnchorContext,
    )
  ) {
    return null;
  }

  const hint = getFactoryGraphEditorMessages(
    args.locale,
  ).zAxisIncompleteConnectionHint;
  return {
    accessibleLabel: hint,
    title: hint,
  };
}

export function buildSemanticGraphHandles(args: {
  authoredProgressOutcomeSourceHandleIds?: ReadonlySet<string>;
  connectionAnchorContext?: FactoryGraphConnectionAnchorContext;
  editor?: CurrentActivityEditorState;
  locale?: string | null;
  nodeId: string;
  nodeKind: FactoryGraphNodeKind;
  validationProjection?: FactoryValidationGraphProjection;
}) {
  const validationHandleErrors =
    args.nodeKind === "workstation" && args.validationProjection
      ? validationHandleErrorsForNode(args.validationProjection, args.nodeId)
      : undefined;
  const connectable =
    args.editor?.editorMode === true &&
    args.editor.canInteractWithEditor &&
    args.editor.activeTool !== "delete";

  const workstation = args.connectionAnchorContext?.workstation;
  const supportsProgressOutcomeRoutes =
    args.nodeKind !== "workstation" ||
    !workstation ||
    workstationSupportsProgressOutcomeRoutes(workstation);
  const supportsProgressOutcomeFailureRoute =
    args.nodeKind !== "workstation" ||
    !workstation ||
    workstationSupportsProgressOutcomeFailureRoute(workstation);

  let anchors = getLocalizedFactoryGraphConnectionAnchors(
    args.nodeKind,
    args.locale,
    args.nodeKind === "workstation" ? args.connectionAnchorContext : undefined,
  );
  if (args.nodeKind === "workstation") {
    anchors = mergeAuthoredProgressOutcomeConnectionAnchors(
      anchors,
      args.authoredProgressOutcomeSourceHandleIds,
    );
  }

  const handles: ActivityGraphNodeHandle[] = anchors.map((anchor) => {
    const isAuthoredOnlyProgressOutcomeHandle =
      args.nodeKind === "workstation" &&
      ((!supportsProgressOutcomeRoutes &&
        PROGRESS_OUTCOME_SOURCE_ANCHOR_IDS.has(anchor.id)) ||
        (!supportsProgressOutcomeFailureRoute &&
          anchor.id === "workstation-on-failure-source"));
    const handleValidation = validationHandleErrors?.get(anchor.id);
    const rendersHandleValidation =
      args.nodeKind !== "workstation" ||
      !args.connectionAnchorContext ||
      workstationRendersProgressOutcomeHandleValidation(
        args.connectionAnchorContext,
        anchor.id,
      );
    const showHandleValidation =
      handleValidation !== undefined && rendersHandleValidation;
    const selected =
      args.editor?.pendingConnectionSource?.nodeId === args.nodeId &&
      args.editor.pendingConnectionSource.anchorId === anchor.id;
    const validTarget =
      connectable &&
      !isAuthoredOnlyProgressOutcomeHandle &&
      args.editor?.pendingConnectionSource !== null &&
      args.editor?.pendingConnectionSource?.nodeId !== args.nodeId &&
      anchor.role === "target";
    const visible = args.editor?.editorMode === true;

    return {
      buttonAriaLabel: showHandleValidation
        ? handleValidation.message
        : anchor.description,
      buttonPressed: selected || undefined,
      buttonDisabled:
        !visible || isAuthoredOnlyProgressOutcomeHandle || undefined,
      buttonTitle: showHandleValidation
        ? handleValidation.message
        : anchor.description,
      connectable: connectable && !isAuthoredOnlyProgressOutcomeHandle,
      hidden: !visible || isAuthoredOnlyProgressOutcomeHandle || undefined,
      id: anchor.id,
      label: anchor.label,
      onButtonClick: () =>
        args.editor?.onConnectionAnchorClick({
          anchorId: anchor.id,
          nodeId: args.nodeId,
        }),
      side: anchor.side,
      type: anchor.role,
      validationError: showHandleValidation,
      validationMessage: showHandleValidation
        ? handleValidation.message
        : undefined,
      variant: showHandleValidation
        ? "error"
        : selected
          ? "selected"
          : validTarget
            ? "valid-target"
            : "default",
    } satisfies ActivityGraphNodeHandle;
  });

  return handles;
}

function endpointNodeKind(
  nodeId: string,
  placeKind:
    | PositionedEdge["sourcePlaceKind"]
    | PositionedEdge["targetPlaceKind"],
  nodeKindsById?: ReadonlyMap<string, PositionedNode["nodeKind"]>,
): CurrentActivityEndpointKind | null {
  const nodeKind = nodeKindsById?.get(nodeId);
  if (nodeKind === "workstation") {
    return "workstation";
  }
  if (nodeKind === "resource") {
    return "resource";
  }
  if (nodeKind === "state_position") {
    return "work-state";
  }
  if (nodeKind === "constraint" && nodeId.startsWith("place:worker:")) {
    return "worker";
  }

  if (nodeId.startsWith("workstation:")) {
    return "workstation";
  }
  if (nodeId.startsWith("resource:")) {
    return "resource";
  }
  if (nodeId.startsWith("worker:")) {
    return "worker";
  }
  if (nodeId.startsWith("work-state:")) {
    return "work-state";
  }
  if (placeKind === "resource") {
    return "resource";
  }
  if (placeKind === "work_state") {
    return "work-state";
  }
  if (nodeId.startsWith("place:worker:")) {
    return "worker";
  }
  if (nodeId.startsWith("place:") && !nodeId.endsWith(":available")) {
    return "work-state";
  }
  return null;
}

function edgeKindForPositionedEdge(
  edge: PositionedEdge,
): FactoryGraphEdgeKind | null {
  const edgeKind = edge.edgeId.split(":")[0];
  if (isFactoryGraphEdgeKind(edgeKind)) {
    return edgeKind;
  }

  const sourceKind = endpointNodeKind(
    edge.fromNodeId,
    edge.sourcePlaceKind,
    undefined,
  );
  const targetKind = endpointNodeKind(
    edge.toNodeId,
    edge.targetPlaceKind,
    undefined,
  );

  if (sourceKind === "resource" && targetKind === "worker") {
    return "worker-resource";
  }
  if (sourceKind === "worker" && targetKind === "workstation") {
    return "worker-assignment";
  }
  if (sourceKind === "resource" && targetKind === "workstation") {
    return "workstation-resource";
  }
  if (sourceKind === "work-state" && targetKind === "workstation") {
    return "workstation-input";
  }
  if (sourceKind === "workstation" && targetKind === "work-state") {
    if (edge.outcomeKind === "failed" || edge.stateCategory === "FAILED") {
      return "workstation-on-failure";
    }
    if (edge.outcomeKind === "rejected") {
      return "workstation-on-rejection";
    }
    if (edge.outcomeKind === "continue") {
      return "workstation-on-continue";
    }
    return "workstation-output";
  }

  return null;
}

function isFactoryGraphEdgeKind(value: string): value is FactoryGraphEdgeKind {
  return (
    value === "worker-assignment" ||
    value === "worker-resource" ||
    value === "workstation-input" ||
    value === "workstation-on-continue" ||
    value === "workstation-on-failure" ||
    value === "workstation-on-rejection" ||
    value === "workstation-output" ||
    value === "workstation-resource" ||
    value === "work-state-visibility-bypass" ||
    value === "work-type-state"
  );
}

function visibilityBypassOutcomeRouteKind(
  edgeId: string,
): WorkstationToWorkStateRouteKind | null {
  if (!edgeId.startsWith("work-state-visibility-bypass:")) {
    return null;
  }

  const outcomeRouteKind = edgeId.split(":")[1];
  return isWorkstationToWorkStateRouteKind(outcomeRouteKind)
    ? outcomeRouteKind
    : null;
}

function isWorkstationToWorkStateRouteKind(
  value: string | undefined,
): value is WorkstationToWorkStateRouteKind {
  return (
    value === "workstation-on-continue" ||
    value === "workstation-on-failure" ||
    value === "workstation-on-rejection" ||
    value === "workstation-output"
  );
}

function visibilityBypassSourceHandleId(
  outcomeRouteKind: WorkstationToWorkStateRouteKind,
): string {
  switch (outcomeRouteKind) {
    case "workstation-on-continue":
      return "workstation-on-continue-source";
    case "workstation-on-failure":
      return "workstation-on-failure-source";
    case "workstation-on-rejection":
      return "workstation-on-rejection-source";
    case "workstation-output":
      return "workstation-output-source";
  }
}

function connectionAnchorId(
  nodeKind: CurrentActivityEndpointKind,
  edgeKind: FactoryGraphEdgeKind,
  role: "source" | "target",
) {
  if (
    edgeKind === "work-type-state" ||
    edgeKind === "work-state-visibility-bypass"
  ) {
    return null;
  }

  return (
    getFactoryGraphConnectionAnchors(nodeKind).find(
      (anchor) =>
        anchor.role === role &&
        (anchor.edgeKinds ?? [anchor.edgeKind]).includes(edgeKind),
    )?.id ?? null
  );
}
