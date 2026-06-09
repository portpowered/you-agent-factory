import { useCallback, useMemo } from "react";

import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { useScopedFactoryDocumentSave } from "../../base/hooks/useScopedFactoryDocumentSave";
import type {
  EditableDocConfigurationState,
  EditableDocSaveState,
  EditableDocSaveValidationErrors,
} from "../lib/detail-card-types";
import { mapDocSaveErrorToFieldErrors } from "../lib/doc-save-validation-field-mapping";
import { getDocDetailMessages } from "../messages/doc-detail";

interface UseSaveEditableDocConfigurationOptions {
  editableConfigurationState?: EditableDocConfigurationState;
  locale?: string | null;
  onDocRenamed?: (targetPath: string) => void;
  scopeKey: string | null;
}

export interface UseSaveEditableDocConfigurationResult {
  canSave: boolean;
  lastSuccessfulSaveWasTopologyAffecting: boolean;
  save: () => Promise<void>;
  saveAttemptRevision: number;
  saveMutationError: CurrentFactoryDefinitionError | null;
  saveState: EditableDocSaveState;
}

export function useSaveEditableDocConfiguration({
  editableConfigurationState,
  locale,
  onDocRenamed,
  scopeKey,
}: UseSaveEditableDocConfigurationOptions): UseSaveEditableDocConfigurationResult {
  const messages = getDocDetailMessages(locale);
  const isDirty =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.isDirty;

  const {
    error: saveMutationError,
    isPending,
    lastSuccessfulSaveWasTopologyAffecting,
    saveAttemptRevision,
    saveNow,
    saveState,
  } = useScopedFactoryDocumentSave<EditableDocSaveValidationErrors>({
    fallbackErrorMessage: messages.editableConfigurationSaveFallbackError,
    isDirty,
    mapSaveErrorToFieldErrors: mapDocSaveErrorToFieldErrors,
    scopeKey,
  });

  const canSave =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.canSave &&
    editableConfigurationState.pendingFactoryDefinition != null &&
    !isPending;

  const save = useCallback(async () => {
    if (
      editableConfigurationState?.status !== "ready" ||
      editableConfigurationState.pendingFactoryDefinition == null ||
      scopeKey == null
    ) {
      return;
    }

    const savedTargetPath = editableConfigurationState.pendingTargetPath;

    await saveNow({
      baseVersion: editableConfigurationState.baseVersion,
      factory: editableConfigurationState.pendingFactoryDefinition,
      previousFactory: editableConfigurationState.savedFactoryDefinition,
      onSaved: () => {
        editableConfigurationState.markChangesSaved();
        if (
          scopeKey != null &&
          savedTargetPath.length > 0 &&
          savedTargetPath !== scopeKey
        ) {
          onDocRenamed?.(savedTargetPath);
        }
      },
      scopeKey,
    });
  }, [editableConfigurationState, onDocRenamed, saveNow, scopeKey]);

  return useMemo(
    () => ({
      canSave,
      lastSuccessfulSaveWasTopologyAffecting,
      save,
      saveAttemptRevision,
      saveMutationError,
      saveState,
    }),
    [
      canSave,
      lastSuccessfulSaveWasTopologyAffecting,
      save,
      saveAttemptRevision,
      saveMutationError,
      saveState,
    ],
  );
}
