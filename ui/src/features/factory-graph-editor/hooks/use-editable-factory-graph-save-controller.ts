import { useCallback, useMemo } from "react";
import { isFactoryDocumentSaveSubmitting } from "../../current-selection/base/hooks/factory-document-save-types";
import { useScopedFactoryDocumentSave } from "../../current-selection/base/hooks/useScopedFactoryDocumentSave";
import { mapGraphSaveOutcomeToDocumentSaveState } from "../lib/document-save/graph-document-save-state";
import type {
  CanonicalFactoryDefinition,
  CurrentFactoryDocument,
} from "../lib/draft/factory-graph-draft-types";
import { factoryLayoutFromDefinition } from "../lib/layout/factory-graph-layout-operations";
import {
  applyFactoryGraphPendingEdits,
  type FactoryGraphOperationResult,
} from "../lib/operations/factory-graph-operations";
import { getFactoryGraphEditorMessages } from "../messages/editor";
import type { useFactoryGraphDraftState } from "./factory-graph-draft-hook";
import type { useFactoryGraphLayoutDraftState } from "./layout/factory-graph-layout-draft-hook";

const FACTORY_GRAPH_SAVE_FALLBACK_ERROR = "Factory graph save failed.";

export function useEditableFactoryGraphSaveController({
  activeWorkCount,
  draftState,
  factoryDocumentScopeKey,
  layoutDraftState,
  locale,
  setBlockedOperation,
}: {
  activeWorkCount: number;
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  factoryDocumentScopeKey: string | null;
  layoutDraftState: ReturnType<typeof useFactoryGraphLayoutDraftState>;
  locale?: string | null;
  setBlockedOperation: (
    result: FactoryGraphOperationResult<never> | null,
  ) => void;
}) {
  const hasPendingEdits = draftState.hasChanges || layoutDraftState.hasChanges;
  const messages = getFactoryGraphEditorMessages(locale);
  const scopedDocumentSave = useScopedFactoryDocumentSave({
    fallbackErrorMessage:
      messages.noticeSaveFailedTitle ?? FACTORY_GRAPH_SAVE_FALLBACK_ERROR,
    isDirty: hasPendingEdits,
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
    hasPendingEdits &&
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
    layoutDraftState,
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
  layoutDraftState,
  locale,
  setBlockedOperation,
}: {
  canSave: boolean;
  confirmSave: ReturnType<typeof useScopedFactoryDocumentSave>["confirmSave"];
  draftState: ReturnType<typeof useFactoryGraphDraftState>;
  factoryDocumentScopeKey: string | null;
  layoutDraftState: ReturnType<typeof useFactoryGraphLayoutDraftState>;
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
      pendingLayout: layoutDraftState.layout,
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
      previousFactory: draftState.latestDocument,
      onSaved: (savedDocument) => {
        draftState.adoptSavedFactoryDocument(
          savedFactoryGraphDocument(saveInput.value, savedDocument),
        );
        layoutDraftState.adoptSavedLayout(
          factoryLayoutFromDefinition(saveInput.value),
        );
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
    layoutDraftState,
    locale,
    setBlockedOperation,
  ]);
}

function savedFactoryGraphDocument(
  savedFactory: CanonicalFactoryDefinition,
  savedDocument: CurrentFactoryDocument,
): CurrentFactoryDocument {
  return {
    ...savedFactory,
    version: savedDocument.version,
  };
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
