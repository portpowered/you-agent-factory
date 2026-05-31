import { useCallback, useMemo } from "react";

import {
  isFactoryDocumentSaveSubmitting,
  useScopedFactoryDocumentSave,
} from "../../current-selection/base/public";
import { createEmptyFactoryGraphDraft } from "../lib/factory-graph-draft-types";
import {
  applyFactoryGraphPendingEdits,
  type FactoryGraphOperationResult,
} from "../lib/factory-graph-operations";
import { mapGraphSaveOutcomeToDocumentSaveState } from "../lib/graph-document-save-state";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import type { useFactoryGraphDraftState } from "./factory-graph-draft-hook";

const FACTORY_GRAPH_SAVE_FALLBACK_ERROR = "Factory graph save failed.";

export function useEditableFactoryGraphSaveController({
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
