import { useCallback, useMemo } from "react";

import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { useScopedFactoryDocumentSave } from "../../base/public";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationSaveState,
  EditableWorkstationSaveValidationErrors,
} from "../lib/detail-card-types";
import { parseWorkstationSaveScopeKey } from "../lib/workstation-save-scope-key";
import { mapWorkstationSaveErrorToFieldErrors } from "../lib/workstation-save-validation-field-mapping";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";

interface UseSaveEditableWorkstationConfigurationOptions {
  editableConfigurationState?: EditableWorkstationConfigurationState;
  locale?: string | null;
  onWorkstationRenamed?: (nodeId: string) => void;
  scopeKey: string | null;
}

export interface UseSaveEditableWorkstationConfigurationResult {
  beginSaveConfirmation: () => void;
  canSave: boolean;
  cancelSaveConfirmation: () => void;
  confirmSave: () => Promise<void>;
  lastSuccessfulSaveWasTopologyAffecting: boolean;
  saveAttemptRevision: number;
  saveMutationError: CurrentFactoryDefinitionError | null;
  saveState: EditableWorkstationSaveState;
}

export function useSaveEditableWorkstationConfiguration({
  editableConfigurationState,
  locale,
  onWorkstationRenamed,
  scopeKey,
}: UseSaveEditableWorkstationConfigurationOptions): UseSaveEditableWorkstationConfigurationResult {
  const messages = getWorkstationDetailMessages(locale);
  const isDirty =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.isDirty;

  const {
    beginConfirmation,
    cancelConfirmation,
    confirmSave: confirmScopedSave,
    error: saveMutationError,
    isPending,
    lastSuccessfulSaveWasTopologyAffecting,
    saveAttemptRevision,
    saveState,
  } = useScopedFactoryDocumentSave<EditableWorkstationSaveValidationErrors>({
    fallbackErrorMessage: messages.editableConfigurationSaveFallbackError,
    isDirty,
    mapSaveErrorToFieldErrors: mapWorkstationSaveErrorToFieldErrors,
    scopeKey,
  });

  const canSave =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.isDirty &&
    !editableConfigurationState.hasValidationErrors &&
    editableConfigurationState.pendingFactoryDefinition != null &&
    !isPending;

  const beginSaveConfirmation = useCallback(() => {
    if (!canSave) {
      return;
    }

    beginConfirmation();
  }, [beginConfirmation, canSave]);

  const confirmSave = useCallback(async () => {
    if (
      editableConfigurationState?.status !== "ready" ||
      editableConfigurationState.pendingFactoryDefinition == null ||
      scopeKey == null
    ) {
      return;
    }

    await confirmScopedSave({
      baseVersion: editableConfigurationState.baseVersion,
      factory: editableConfigurationState.pendingFactoryDefinition,
      onSaved: () => {
        editableConfigurationState.markChangesSaved();
        const savedWorkstationName =
          editableConfigurationState.draft.name.trim();
        const parsedScopeKey =
          scopeKey == null ? null : parseWorkstationSaveScopeKey(scopeKey);
        if (
          parsedScopeKey != null &&
          savedWorkstationName.length > 0 &&
          savedWorkstationName !== parsedScopeKey.workstationName
        ) {
          onWorkstationRenamed?.(parsedScopeKey.nodeId);
        }
      },
      previousFactory: editableConfigurationState.savedFactoryDefinition,
      scopeKey,
    });
  }, [
    confirmScopedSave,
    editableConfigurationState,
    onWorkstationRenamed,
    scopeKey,
  ]);

  return useMemo(
    () => ({
      beginSaveConfirmation,
      canSave,
      cancelSaveConfirmation: cancelConfirmation,
      confirmSave,
      lastSuccessfulSaveWasTopologyAffecting,
      saveAttemptRevision,
      saveMutationError,
      saveState,
    }),
    [
      beginSaveConfirmation,
      canSave,
      cancelConfirmation,
      confirmSave,
      lastSuccessfulSaveWasTopologyAffecting,
      saveAttemptRevision,
      saveMutationError,
      saveState,
    ],
  );
}
