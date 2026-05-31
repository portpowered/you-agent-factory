import { useCallback, useMemo } from "react";

import { mapWorkstationSaveErrorToFieldErrors } from "../lib/workstation-save-validation-field-mapping";
import { useScopedFactoryDocumentSave } from "../../base/public";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationSaveState,
  EditableWorkstationSaveValidationErrors,
} from "../lib/detail-card-types";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";

interface UseSaveEditableWorkstationConfigurationOptions {
  editableConfigurationState?: EditableWorkstationConfigurationState;
  locale?: string | null;
  scopeKey: string | null;
}

export interface UseSaveEditableWorkstationConfigurationResult {
  beginSaveConfirmation: () => void;
  canSave: boolean;
  cancelSaveConfirmation: () => void;
  confirmSave: () => Promise<void>;
  saveState: EditableWorkstationSaveState;
}

export function useSaveEditableWorkstationConfiguration({
  editableConfigurationState,
  locale,
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
    isPending,
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
      onSaved: editableConfigurationState.markChangesSaved,
      scopeKey,
    });
  }, [confirmScopedSave, editableConfigurationState, scopeKey]);

  return useMemo(
    () => ({
      beginSaveConfirmation,
      canSave,
      cancelSaveConfirmation: cancelConfirmation,
      confirmSave,
      saveState,
    }),
    [
      beginSaveConfirmation,
      canSave,
      cancelConfirmation,
      confirmSave,
      saveState,
    ],
  );
}
