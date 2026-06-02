import type { useFactoryDocumentSave } from "../../current-factory-definition/public";
import type { FactoryDocumentSaveState } from "../../current-selection/base/public";

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
} from "../lib/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/factory-graph-editor-additions";
import type { FactoryGraphNodeFieldUpdate } from "../lib/factory-graph-field-operations";
import type {
  FactoryGraphOperationResult,
  FactoryGraphReactFlowProjection,
  FactoryGraphState,
} from "../lib/factory-graph-operations";
import type { useFactoryGraphDraftState } from "./factory-graph-draft-hook";

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
    save: () => Promise<boolean>;
    updateNodeField: (
      update: FactoryGraphNodeFieldUpdate,
    ) => FactoryGraphOperationResult<CanonicalFactoryDefinition>;
  };
  blockedOperation: FactoryGraphOperationResult<never> | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  graphState: FactoryGraphState | null;
  pendingState: {
    hasChanges: boolean;
    pendingFactoryDefinition: CanonicalFactoryDefinition | null;
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
