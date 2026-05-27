import { useCallback, useMemo, useState } from "react";

import {
  type CanonicalFactoryDefinition,
  createEmptyFactoryGraphDraft,
  type FactoryGraphDraft,
} from "../lib/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/factory-graph-editor-additions";
import type { FactoryGraphNodeFieldUpdate } from "../lib/factory-graph-field-operations";
import { updateFactoryGraphNodeField } from "../lib/factory-graph-field-operations";
import {
  addFactoryGraphNode,
  applyFactoryGraphPendingEdits,
  buildFactoryGraphState,
  connectFactoryGraphNodes,
  disconnectFactoryGraphEdge,
  type FactoryGraphOperationResult,
  projectFactoryGraphToReactFlow,
  removeFactoryGraphNode,
} from "../lib/factory-graph-operations";
import { useFactoryGraphDraftState } from "./factory-graph-draft-hook";
import type {
  EditableFactoryGraphSaveInput,
  EditableFactoryGraphViewModel,
  UseEditableFactoryGraphOptions,
} from "./use-editable-factory-graph-types";

export function useEditableFactoryGraph(
  options: UseEditableFactoryGraphOptions,
): EditableFactoryGraphViewModel {
  const [blockedOperation, setBlockedOperation] =
    useState<FactoryGraphOperationResult<never> | null>(null);
  const draftState = useFactoryGraphDraftState(options);
  const baseFactoryDefinition =
    draftState.latestDocument ?? draftState.baseDocument ?? null;
  const graphState = useEditableFactoryGraphState({
    baseFactoryDefinition,
    draftState,
  });
  const projection = useMemo(
    () => projectFactoryGraphToReactFlow(draftState.graph),
    [draftState.graph],
  );
  const saveController = useEditableFactoryGraphSaveController({
    activeWorkCount: options.activeWorkCount ?? 0,
    draftState,
    saveFactoryDefinition: options.saveFactoryDefinition,
    setBlockedOperation,
  });
  const mutationActions = useEditableFactoryGraphMutationActions({
    baseFactoryDefinition,
    draftState,
    setBlockedOperation,
  });
  const discard = useCallback(() => {
    draftState.resetDraft();
    setBlockedOperation(null);
    saveController.resetSaveState();
  }, [draftState, saveController]);

  return {
    actions: {
      addNode: mutationActions.addNode,
      connectNodes: mutationActions.connectNodes,
      discard,
      disconnectEdge: mutationActions.disconnectEdge,
      removeNode: mutationActions.removeNode,
      save: saveController.save,
      updateNodeField: mutationActions.updateNodeField,
    },
    blockedOperation,
    draftState,
    graphState,
    pendingState: {
      hasChanges: draftState.hasChanges,
      pendingFactoryDefinition: draftState.pendingFactoryDefinition,
    },
    projection,
    saveState: {
      canSave: saveController.canSave,
      isSaving: saveController.isSaving,
      isStale: saveController.isStale,
      lastError: saveController.lastSaveError,
      lastSuccess: saveController.lastSaveSuccess,
    },
    validationState: {
      errors: draftState.validationErrors,
      isValid: draftState.validationErrors.length === 0,
    },
  };
}

function useEditableFactoryGraphState({
  baseFactoryDefinition,
  draftState,
}: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
}) {
  return useMemo(
    () =>
      baseFactoryDefinition
        ? buildFactoryGraphState({
            baseFactoryDefinition,
            draft: draftState.draft,
          })
        : null,
    [baseFactoryDefinition, draftState.draft],
  );
}

function useEditableFactoryGraphMutationActions({
  baseFactoryDefinition,
  draftState,
  setBlockedOperation,
}: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  setBlockedOperation: (
    result: FactoryGraphOperationResult<never> | null,
  ) => void;
}) {
  const applyDraftOperation = useCallback(
    (
      operation: () => FactoryGraphOperationResult<FactoryGraphDraft>,
    ): FactoryGraphOperationResult<FactoryGraphDraft> => {
      const result = operation();
      if (result.ok) {
        draftState.replaceDraft(result.value);
        setBlockedOperation(null);
      } else {
        setBlockedOperation(result as FactoryGraphOperationResult<never>);
      }
      return result;
    },
    [draftState, setBlockedOperation],
  );
  const addNode = useCallback(
    (node: FactoryGraphAddEntityDraft) =>
      applyDraftOperation(() =>
        baseFactoryDefinition
          ? addFactoryGraphNode({
              baseFactoryDefinition,
              draft: draftState.draft,
              node,
            })
          : missingFactoryResult(
              "Load the current factory before editing graph nodes.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draftState.draft],
  );
  const removeNode = useCallback(
    (nodeId: string) =>
      applyDraftOperation(() =>
        baseFactoryDefinition
          ? removeFactoryGraphNode({
              baseFactoryDefinition,
              draft: draftState.draft,
              nodeId,
            })
          : missingFactoryResult(
              "Load the current factory before removing graph nodes.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draftState.draft],
  );
  const connectNodes = useConnectNodesAction({
    applyDraftOperation,
    baseFactoryDefinition,
    draft: draftState.draft,
  });
  const disconnectEdge = useDisconnectEdgeAction({
    applyDraftOperation,
    baseFactoryDefinition,
    draft: draftState.draft,
  });
  const updateNodeField = useUpdateNodeFieldAction({
    baseFactoryDefinition,
    setBlockedOperation,
  });

  return {
    addNode,
    connectNodes,
    disconnectEdge,
    removeNode,
    updateNodeField,
  };
}

function useConnectNodesAction({
  applyDraftOperation,
  baseFactoryDefinition,
  draft,
}: {
  applyDraftOperation: (
    operation: () => FactoryGraphOperationResult<FactoryGraphDraft>,
  ) => FactoryGraphOperationResult<FactoryGraphDraft>;
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphDraft;
}) {
  return useCallback(
    (connection: {
      sourceAnchorId: string;
      sourceNodeId: string;
      targetAnchorId: string;
      targetNodeId: string;
    }) =>
      applyDraftOperation(() =>
        baseFactoryDefinition
          ? connectFactoryGraphNodes({
              baseFactoryDefinition,
              draft,
              ...connection,
            })
          : missingFactoryResult(
              "Load the current factory before connecting graph nodes.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draft],
  );
}

function useDisconnectEdgeAction({
  applyDraftOperation,
  baseFactoryDefinition,
  draft,
}: {
  applyDraftOperation: (
    operation: () => FactoryGraphOperationResult<FactoryGraphDraft>,
  ) => FactoryGraphOperationResult<FactoryGraphDraft>;
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphDraft;
}) {
  return useCallback(
    (edgeId: string) =>
      applyDraftOperation(() =>
        baseFactoryDefinition
          ? disconnectFactoryGraphEdge({
              baseFactoryDefinition,
              draft,
              edgeId,
            })
          : missingFactoryResult(
              "Load the current factory before disconnecting graph edges.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draft],
  );
}

function useUpdateNodeFieldAction({
  baseFactoryDefinition,
  setBlockedOperation,
}: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  setBlockedOperation: (
    result: FactoryGraphOperationResult<never> | null,
  ) => void;
}) {
  return useCallback(
    (update: FactoryGraphNodeFieldUpdate) => {
      if (!baseFactoryDefinition) {
        const result = missingFactoryResult(
          "Load the current factory before editing graph fields.",
        );
        setBlockedOperation(result);
        return result;
      }

      const result = updateFactoryGraphNodeField({
        baseFactoryDefinition,
        update,
      });
      setBlockedOperation(
        result.ok ? null : (result as FactoryGraphOperationResult<never>),
      );
      return result;
    },
    [baseFactoryDefinition, setBlockedOperation],
  );
}

function useEditableFactoryGraphSaveController({
  activeWorkCount,
  draftState,
  saveFactoryDefinition,
  setBlockedOperation,
}: {
  activeWorkCount: number;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  saveFactoryDefinition?: (
    input: EditableFactoryGraphSaveInput,
  ) => Promise<unknown>;
  setBlockedOperation: (
    result: FactoryGraphOperationResult<never> | null,
  ) => void;
}) {
  const [isSaving, setIsSaving] = useState(false);
  const [lastSaveError, setLastSaveError] = useState<string | null>(null);
  const [lastSaveSuccess, setLastSaveSuccess] = useState(false);
  const isStale = isFactoryGraphDraftStale(draftState);
  const canSave =
    Boolean(saveFactoryDefinition) &&
    draftState.hasChanges &&
    draftState.pendingFactoryDefinition !== null &&
    draftState.validationErrors.length === 0 &&
    draftState.latestDocument !== null &&
    activeWorkCount === 0 &&
    !isStale &&
    !isSaving;
  const resetSaveState = useCallback(() => {
    setLastSaveError(null);
    setLastSaveSuccess(false);
  }, []);
  const save = useSaveEditableFactoryGraph({
    canSave,
    draftState,
    resetSaveState,
    saveFactoryDefinition,
    setBlockedOperation,
    setIsSaving,
    setLastSaveError,
    setLastSaveSuccess,
  });

  return {
    canSave,
    isSaving,
    isStale,
    lastSaveError,
    lastSaveSuccess,
    resetSaveState,
    save,
  };
}

function useSaveEditableFactoryGraph({
  canSave,
  draftState,
  resetSaveState,
  saveFactoryDefinition,
  setBlockedOperation,
  setIsSaving,
  setLastSaveError,
  setLastSaveSuccess,
}: {
  canSave: boolean;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  resetSaveState: () => void;
  saveFactoryDefinition?: (
    input: EditableFactoryGraphSaveInput,
  ) => Promise<unknown>;
  setBlockedOperation: (
    result: FactoryGraphOperationResult<never> | null,
  ) => void;
  setIsSaving: (isSaving: boolean) => void;
  setLastSaveError: (error: string | null) => void;
  setLastSaveSuccess: (success: boolean) => void;
}) {
  return useCallback(async () => {
    if (!saveFactoryDefinition || !canSave || !draftState.latestDocument) {
      return false;
    }

    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition: draftState.latestDocument,
      draft: draftState.draft,
    });
    if (!saveInput.ok) {
      setBlockedOperation(saveInput as FactoryGraphOperationResult<never>);
      return false;
    }

    setIsSaving(true);
    resetSaveState();
    try {
      await saveFactoryDefinition({
        baseVersion: draftState.latestDocument.version,
        factoryDefinition: saveInput.value,
      });
      draftState.replaceDraft(createEmptyFactoryGraphDraft());
      setBlockedOperation(null);
      setLastSaveSuccess(true);
      return true;
    } catch (error) {
      setLastSaveError(
        error instanceof Error ? error.message : "Factory graph save failed.",
      );
      return false;
    } finally {
      setIsSaving(false);
    }
  }, [
    canSave,
    draftState,
    resetSaveState,
    saveFactoryDefinition,
    setBlockedOperation,
    setIsSaving,
    setLastSaveError,
    setLastSaveSuccess,
  ]);
}

function isFactoryGraphDraftStale(
  draftState: ReturnType<typeof useFactoryGraphDraftState>,
) {
  return (
    draftState.hasChanges &&
    draftState.baseDocument !== null &&
    draftState.latestDocument !== null &&
    (draftState.baseDocument.version.logical !==
      draftState.latestDocument.version.logical ||
      draftState.baseDocument.version.physical !==
        draftState.latestDocument.version.physical)
  );
}

function missingFactoryResult(
  message: string,
): FactoryGraphOperationResult<never> {
  return {
    message,
    ok: false,
    reason: "INVALID_SAVE",
  };
}
