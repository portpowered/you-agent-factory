import type {
  FactoryGraphDraftEdgeChange,
  FactoryGraphNodeKind,
} from "../../factory-graph-editor/lib/factory-graph-draft-types";
import {
  type FactoryGraphConnectionEndpoint,
  getFactoryGraphConnectionAnchors,
} from "../../factory-graph-editor/lib/factory-graph-editor-connections";
import type { ActivityGraphNodeHandle } from "../../flowchart/components/current-activity-node-shell";
import type { PositionedEdge } from "../../flowchart/lib/layout";

export interface CurrentActivityEditorState {
  activeTool: "add" | "connect" | "delete" | null;
  canInteractWithEditor: boolean;
  editorMode: boolean;
  onConnectionAnchorClick: (endpoint: FactoryGraphConnectionEndpoint) => void;
  pendingConnectionSource: FactoryGraphConnectionEndpoint | null;
}

const EDITOR_HANDLE_IDS_BY_EDGE_KIND = {
  "worker-assignment": {
    sourceHandleId: "worker-assignment-source",
    targetHandleId: "worker-assignment-target",
  },
  "worker-resource": {
    sourceHandleId: "worker-resource-source",
    targetHandleId: "worker-resource-target",
  },
  "workstation-input": {
    sourceHandleId: "workstation-input-source",
    targetHandleId: "workstation-input-target",
  },
  "workstation-on-continue": {
    sourceHandleId: "workstation-on-continue-source",
    targetHandleId: "workstation-on-continue-target",
  },
  "workstation-on-failure": {
    sourceHandleId: "workstation-on-failure-source",
    targetHandleId: "workstation-on-failure-target",
  },
  "workstation-on-rejection": {
    sourceHandleId: "workstation-on-rejection-source",
    targetHandleId: "workstation-on-rejection-target",
  },
  "workstation-output": {
    sourceHandleId: "workstation-output-source",
    targetHandleId: "workstation-output-target",
  },
  "workstation-resource": {
    sourceHandleId: "workstation-resource-source",
    targetHandleId: "workstation-resource-target",
  },
} satisfies Record<
  FactoryGraphDraftEdgeChange["kind"],
  { sourceHandleId: string; targetHandleId: string }
>;

export function supportedEditorHandleIdsForEdge(edge: PositionedEdge) {
  const edgeKind = edge.edgeId.split(":", 1)[0];
  if (edgeKind in EDITOR_HANDLE_IDS_BY_EDGE_KIND) {
    return EDITOR_HANDLE_IDS_BY_EDGE_KIND[
      edgeKind as FactoryGraphDraftEdgeChange["kind"]
    ];
  }

  const sourceIsWorkstation = edge.fromNodeId.startsWith("workstation:");
  const targetIsWorkstation = edge.toNodeId.startsWith("workstation:");
  const sourceIsState = edge.sourcePlaceKind === "work_state";
  const targetIsState = edge.targetPlaceKind === "work_state";

  if (sourceIsState && targetIsWorkstation) {
    return {
      sourceHandleId: "workstation-input-source",
      targetHandleId: "workstation-input-target",
    };
  }

  if (!sourceIsWorkstation || !targetIsState) {
    return null;
  }

  if (edge.outcomeKind === "continue") {
    return {
      sourceHandleId: "workstation-on-continue-source",
      targetHandleId: "workstation-on-continue-target",
    };
  }
  if (edge.outcomeKind === "rejected") {
    return {
      sourceHandleId: "workstation-on-rejection-source",
      targetHandleId: "workstation-on-rejection-target",
    };
  }
  if (edge.outcomeKind === "failed" || edge.stateCategory === "FAILED") {
    return {
      sourceHandleId: "workstation-on-failure-source",
      targetHandleId: "workstation-on-failure-target",
    };
  }

  return {
    sourceHandleId: "workstation-output-source",
    targetHandleId: "workstation-output-target",
  };
}

export function buildEditorHandles(args: {
  editor: CurrentActivityEditorState;
  nodeId: string;
  nodeKind: FactoryGraphNodeKind;
}) {
  const connectable =
    args.editor.canInteractWithEditor && args.editor.activeTool === "connect";

  return getFactoryGraphConnectionAnchors(args.nodeKind).map((anchor) => {
    const selected =
      args.editor.pendingConnectionSource?.nodeId === args.nodeId &&
      args.editor.pendingConnectionSource.anchorId === anchor.id;
    const validTarget =
      connectable &&
      args.editor.pendingConnectionSource !== null &&
      args.editor.pendingConnectionSource.nodeId !== args.nodeId &&
      anchor.role === "target";

    return {
      buttonAriaLabel: anchor.description,
      buttonPressed: selected || undefined,
      buttonTitle: anchor.description,
      connectable,
      id: anchor.id,
      label: anchor.label,
      onButtonClick: () =>
        args.editor.onConnectionAnchorClick({
          anchorId: anchor.id,
          nodeId: args.nodeId,
        }),
      side: anchor.side,
      type: anchor.role,
      variant: selected ? "selected" : validTarget ? "valid-target" : "default",
    } satisfies ActivityGraphNodeHandle;
  });
}
