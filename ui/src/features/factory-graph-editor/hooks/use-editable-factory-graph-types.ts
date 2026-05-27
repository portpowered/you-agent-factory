import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDocument,
  DashboardTopology,
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

export interface EditableFactoryGraphSaveInput {
  baseVersion?: CurrentFactoryDocument["version"];
  factoryDefinition: CanonicalFactoryDefinition;
}

export interface UseEditableFactoryGraphOptions {
  activeWorkCount?: number;
  currentFactoryDocument?: CurrentFactoryDocument;
  projectedFactory?: CanonicalFactoryDefinition;
  projectedTopology?: DashboardTopology;
  saveFactoryDefinition?: (
    input: EditableFactoryGraphSaveInput,
  ) => Promise<unknown>;
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
    isSaving: boolean;
    isStale: boolean;
    lastError: string | null;
    lastSuccess: boolean;
  };
  validationState: {
    errors: FactoryGraphDraftValidationError[];
    isValid: boolean;
  };
}
