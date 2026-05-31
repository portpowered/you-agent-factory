// biome-ignore lint/nursery/noExcessiveLinesPerFile: editable graph hook keeps draft, projection, save, and mutation wiring in one view-model seam.
import { useCallback, useMemo, useState } from "react";

import {
  isFactoryDocumentSaveSubmitting,
  useScopedFactoryDocumentSave,
} from "../../current-selection/base/public";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import { mapGraphSaveOutcomeToDocumentSaveState } from "../lib/graph-document-save-state";
import {
  type CanonicalFactoryDefinition,
  createEmptyFactoryGraphDraft,
  type FactoryGraphDraft,
} from "../lib/factory-graph-draft-types";
import type { FactoryGraphAddEntityDraft } from "../lib/factory-graph-editor-additions";
import type { FactoryGraphNodeFieldUpdate } from "../lib/factory-graph-field-operations";
import { updateFactoryGraphNodeField } from "../lib/factory-graph-field-operations";
import { createFactoryGraphWorkstationResolver } from "../lib/factory-graph-editor-connections";
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
    locale: options.locale,
  });
  const projection = useMemo(() => {
    const factoryDefinition =
      draftState.pendingFactoryDefinition ?? draftState.baseDocument ?? null;

    return projectFactoryGraphToReactFlow({
      factoryDefinition,
      locale: options.locale ?? undefined,
      topology: draftState.graph,
      workstationResolver: factoryDefinition
        ? createFactoryGraphWorkstationResolver(factoryDefinition.workstations)
        : undefined,
    });
  }, [
    draftState.baseDocument,
    draftState.graph,
    draftState.pendingFactoryDefinition,
    options.locale,
  ]);
  const saveController = useEditableFactoryGraphSaveController({
    activeWorkCount: options.activeWorkCount ?? 0,
    draftState,
    factoryDocumentScopeKey: options.factoryDocumentScopeKey ?? null,
    locale: options.locale,
    setBlockedOperation,
  });
  const mutationActions = useEditableFactoryGraphMutationActions({
    baseFactoryDefinition,
    draftState,
    locale: options.locale,
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
    documentSaveControls: saveController.documentSaveControls,
    saveMutation: saveController.saveMutation,
    saveState: {
      canSave: saveController.canSave,
      documentSave: saveController.documentSave,
      isStale: saveController.isStale,
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
  locale,
}: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  locale?: string | null;
}) {
  return useMemo(
    () =>
      baseFactoryDefinition
        ? buildFactoryGraphState({
            baseFactoryDefinition,
            draft: draftState.draft,
            locale,
          })
        : null,
    [baseFactoryDefinition, draftState.draft, locale],
  );
}

function useEditableFactoryGraphMutationActions({
  baseFactoryDefinition,
  draftState,
  locale,
  setBlockedOperation,
}: {
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  locale?: string | null;
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
              locale,
              node,
            })
          : missingFactoryResult(
              "Load the current factory before editing graph nodes.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draftState.draft, locale],
  );
  const removeNode = useCallback(
    (nodeId: string) =>
      applyDraftOperation(() =>
        baseFactoryDefinition
          ? removeFactoryGraphNode({
              baseFactoryDefinition,
              draft: draftState.draft,
              locale,
              nodeId,
            })
          : missingFactoryResult(
              "Load the current factory before removing graph nodes.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draftState.draft, locale],
  );
  const connectNodes = useConnectNodesAction({
    applyDraftOperation,
    baseFactoryDefinition,
    draft: draftState.draft,
    locale,
  });
  const disconnectEdge = useDisconnectEdgeAction({
    applyDraftOperation,
    baseFactoryDefinition,
    draft: draftState.draft,
    locale,
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
  locale,
}: {
  applyDraftOperation: (
    operation: () => FactoryGraphOperationResult<FactoryGraphDraft>,
  ) => FactoryGraphOperationResult<FactoryGraphDraft>;
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphDraft;
  locale?: string | null;
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
              locale,
              ...connection,
            })
          : missingFactoryResult(
              "Load the current factory before connecting graph nodes.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draft, locale],
  );
}

function useDisconnectEdgeAction({
  applyDraftOperation,
  baseFactoryDefinition,
  draft,
  locale,
}: {
  applyDraftOperation: (
    operation: () => FactoryGraphOperationResult<FactoryGraphDraft>,
  ) => FactoryGraphOperationResult<FactoryGraphDraft>;
  baseFactoryDefinition: CanonicalFactoryDefinition | null;
  draft: FactoryGraphDraft;
  locale?: string | null;
}) {
  return useCallback(
    (edgeId: string) =>
      applyDraftOperation(() =>
        baseFactoryDefinition
          ? disconnectFactoryGraphEdge({
              baseFactoryDefinition,
              draft,
              edgeId,
              locale,
            })
          : missingFactoryResult(
              "Load the current factory before disconnecting graph edges.",
            ),
      ),
    [applyDraftOperation, baseFactoryDefinition, draft, locale],
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

const FACTORY_GRAPH_SAVE_FALLBACK_ERROR = "Factory graph save failed.";

function useEditableFactoryGraphSaveController({
  activeWorkCount,
  draftState,
  factoryDocumentScopeKey,
  locale,
  setBlockedOperation,
}: {
  activeWorkCount: number;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  factoryDocumentScopeKey: string | null;
  locale?: string | null;
  setBlockedOperation: (
    result: FactoryGraphOperationResult<never> | null,
  ) => void;
}) {
  const messages = getFactoryGraphEditorMessages(locale);
  const scopedDocumentSave = useScopedFactoryDocumentSave({
    fallbackErrorMessage:
      messages.noticeSaveFailedTitle ?? FACTORY_GRAPH_SAVE_FALLBACK_ERROR,
    isDirty: draftState.hasChanges,
    scopeKey: factoryDocumentScopeKey,
  });
  const isStale = isFactoryGraphDraftStale(draftState);
  const documentSave = useMemo(
    () =>
      isStale
        ? mapGraphSaveOutcomeToDocumentSaveState({
            errorMessage: null,
            isSubmitting: false,
            isStale: true,
          })
        : scopedDocumentSave.saveState,
    [isStale, scopedDocumentSave.saveState],
  );
  const isSaving = isFactoryDocumentSaveSubmitting(documentSave);
  const canSave =
    factoryDocumentScopeKey !== null &&
    draftState.hasChanges &&
    draftState.pendingFactoryDefinition !== null &&
    draftState.validationErrors.length === 0 &&
    draftState.latestDocument !== null &&
    activeWorkCount === 0 &&
    !isStale &&
    !isSaving &&
    !scopedDocumentSave.isPending;
  const resetSaveState = useCallback(() => {
    scopedDocumentSave.reset();
    scopedDocumentSave.clearSaveFeedback();
  }, [scopedDocumentSave.clearSaveFeedback, scopedDocumentSave.reset]);
  const save = useSaveEditableFactoryGraph({
    canSave,
    confirmSave: scopedDocumentSave.confirmSave,
    draftState,
    factoryDocumentScopeKey,
    locale,
    setBlockedOperation,
  });

  return {
    canSave,
    documentSave,
    documentSaveControls: {
      beginConfirmation: scopedDocumentSave.beginConfirmation,
      cancelConfirmation: scopedDocumentSave.cancelConfirmation,
      clearSaveFeedback: scopedDocumentSave.clearSaveFeedback,
    },
    isStale,
    resetSaveState,
    save,
    saveMutation: {
      error: scopedDocumentSave.error,
      isPending: scopedDocumentSave.isPending,
      reset: scopedDocumentSave.reset,
    },
  };
}

function useSaveEditableFactoryGraph({
  canSave,
  confirmSave,
  draftState,
  factoryDocumentScopeKey,
  locale,
  setBlockedOperation,
}: {
  canSave: boolean;
  confirmSave: ReturnType<typeof useScopedFactoryDocumentSave>["confirmSave"];
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  factoryDocumentScopeKey: string | null;
  locale?: string | null;
  setBlockedOperation: (
    result: FactoryGraphOperationResult<never> | null,
  ) => void;
}) {
  return useCallback(async () => {
    if (
      factoryDocumentScopeKey === null ||
      !canSave ||
      !draftState.latestDocument
    ) {
      return false;
    }

    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition: draftState.latestDocument,
      draft: draftState.draft,
      locale,
    });
    if (!saveInput.ok) {
      setBlockedOperation(saveInput as FactoryGraphOperationResult<never>);
      return false;
    }

    let didSave = false;
    await confirmSave({
      baseVersion: draftState.latestDocument.version,
      factory: saveInput.value,
      onSaved: () => {
        draftState.replaceDraft(createEmptyFactoryGraphDraft());
        setBlockedOperation(null);
        didSave = true;
      },
      scopeKey: factoryDocumentScopeKey,
    });
    return didSave;
  }, [
    canSave,
    confirmSave,
    draftState,
    factoryDocumentScopeKey,
    locale,
    setBlockedOperation,
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
