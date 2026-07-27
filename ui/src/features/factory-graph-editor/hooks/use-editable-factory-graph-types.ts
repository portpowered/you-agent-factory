import type { useFactoryDocumentSave } from "../../current-factory-definition/hooks/useFactoryDocumentSave";
import type { FactoryDocumentSaveState } from "../../current-selection/base/hooks/factory-document-save-types";

export type EditableFactoryGraphSaveMutation = Pick<
  ReturnType<typeof useFactoryDocumentSave>,
  "error" | "isPending" | "reset"
>;

export interface EditableFactoryGraphDocumentSaveControls {
  beginConfirmation: () => void;
  cancelConfirmation: () => void;
  clearSaveFeedback: () => void;
}

import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDocument,
  FactoryGraphDraft,
  FactoryGraphDraftValidationError,
} from "../lib/draft/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/editor/factory-graph-editor-additions";
import type { FactoryGraphNodeFieldUpdate } from "../lib/editor-runtime/factory-graph-field-operations";
import type {
  FactoryGraphOperationResult,
  FactoryGraphReactFlowProjection,
  FactoryGraphState,
} from "../lib/operations/factory-graph-operations";
import type { useFactoryGraphDraftState } from "./factory-graph-draft-hook";
import type { useFactoryGraphLayoutDraftState } from "./layout/factory-graph-layout-draft-hook";

export interface UseEditableFactoryGraphOptions {
  activeWorkCount?: number;
  currentFactoryDocument?: CurrentFactoryDocument;
  /** Normalized dashboard session id; graph draft resets when this changes. */
  factoryDocumentScopeKey?: string | null;
  locale?: string | null;
}

export interface EditableFactoryGraphViewModel {
  actions: {
    addNode: (
      node: FactoryGraphAddEntityDraft,
    ) => FactoryGraphOperationResult<FactoryGraphDraft>;
    connectNodes: (connection: {
      sourceAnchorId: string;
      sourceNodeId: string;
      targetAnchorId: string;
      targetNodeId: string;
    }) => FactoryGraphOperationResult<FactoryGraphDraft>;
    discard: () => void;
    disconnectEdge: (
      edgeId: string,
    ) => FactoryGraphOperationResult<FactoryGraphDraft>;
    removeNode: (
      nodeId: string,
    ) => FactoryGraphOperationResult<FactoryGraphDraft>;
    removeSelection: (selection: {
      edgeIds: readonly string[];
      nodeIds: readonly string[];
    }) => FactoryGraphOperationResult<FactoryGraphDraft>;
    addEdgeWaypoint: (
      edgeId: string,
      position: { x: number; y: number },
      insertIndex?: number,
    ) => void;
    moveEdgeWaypoint: (
      edgeId: string,
      waypointIndex: number,
      position: { x: number; y: number },
    ) => void;
    removeEdgeWaypoint: (edgeId: string, waypointIndex: number) => void;
    moveLayoutNode: (
      nodeId: string,
      position: { x: number; y: number },
    ) => void;
    moveLayoutNodesByDelta: (
      nodeIds: readonly string[],
      delta: { x: number; y: number },
      resolvedPositionsByNodeId: ReadonlyMap<string, { x: number; y: number }>,
    ) => void;
    resetLayout: (options?: { recordHistory?: boolean }) => void;
    redoLayout: () => void;
    save: () => Promise<boolean>;
    undoLayout: () => void;
    updateLayoutViewport: (viewport: {
      x: number;
      y: number;
      zoom: number;
    }) => void;
    createVisualGroup: (center: {
      x: number;
      y: number;
    }) => { id: string } | null;
    renameVisualGroup: (groupId: string, label: string) => void;
    setVisualGroupColor: (
      groupId: string,
      color: "primary" | "info" | "success" | "warning" | "outline",
    ) => void;
    addNodeToVisualGroup: (groupId: string, nodeId: string) => void;
    removeNodeFromVisualGroup: (groupId: string, nodeId: string) => void;
    moveVisualGroupByDelta: (
      groupId: string,
      delta: { x: number; y: number },
      resolvedNodePositions?: ReadonlyMap<string, { x: number; y: number }>,
    ) => void;
    resizeVisualGroup: (
      groupId: string,
      bounds: { height: number; width: number; x: number; y: number },
    ) => void;
    deleteVisualGroup: (groupId: string) => void;
    updateNodeField: (
      update: FactoryGraphNodeFieldUpdate,
    ) => FactoryGraphOperationResult<CanonicalFactoryDefinition>;
  };
  blockedOperation: FactoryGraphOperationResult<never> | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  graphState: FactoryGraphState | null;
  layoutDraftState: ReturnType<typeof useFactoryGraphLayoutDraftState>;
  pendingState: {
    canRedoLayout: boolean;
    canUndoLayout: boolean;
    dirtyState: {
      layoutDirty: boolean;
      preferencesDirty: boolean;
      topologyDirty: boolean;
    };
    hasChanges: boolean;
    hasLayoutChanges: boolean;
    hasPortableDocumentChanges: boolean;
    hasPreferenceChanges: boolean;
    hasTopologyChanges: boolean;
    layoutDirty: boolean;
    pendingFactoryDefinition: CanonicalFactoryDefinition | null;
    preferencesDirty: boolean;
    topologyDirty: boolean;
  };
  projection: FactoryGraphReactFlowProjection;
  saveState: {
    canSave: boolean;
    documentSave: FactoryDocumentSaveState;
    isStale: boolean;
  };
  documentSaveControls: EditableFactoryGraphDocumentSaveControls;
  saveMutation: EditableFactoryGraphSaveMutation;
  validationState: {
    errors: FactoryGraphDraftValidationError[];
    isValid: boolean;
  };
}
