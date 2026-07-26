import { useCallback, useMemo } from "react";

import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { useScopedFactoryDocumentSave } from "../../base/hooks/useScopedFactoryDocumentSave";
import type {
  EditableWorkerConfigurationState,
  EditableWorkerSaveState,
  EditableWorkerSaveValidationErrors,
} from "../lib/detail-card-types";
import { mapWorkerSaveErrorToFieldErrors } from "../lib/worker-save-validation-field-mapping";
import { getWorkerDetailMessages } from "../messages/worker-detail";

interface UseSaveEditableWorkerConfigurationOptions {
  editableConfigurationState?: EditableWorkerConfigurationState;
  locale?: string | null;
  onWorkerRenamed?: (workerName: string) => void;
  scopeKey: string | null;
}

export interface UseSaveEditableWorkerConfigurationResult {
  canSave: boolean;
  lastSuccessfulSaveWasTopologyAffecting: boolean;
  save: () => Promise<void>;
  saveAttemptRevision: number;
  saveMutationError: CurrentFactoryDefinitionError | null;
  saveState: EditableWorkerSaveState;
}

export function useSaveEditableWorkerConfiguration({
  editableConfigurationState,
  locale,
  onWorkerRenamed,
  scopeKey,
}: UseSaveEditableWorkerConfigurationOptions): UseSaveEditableWorkerConfigurationResult {
  const messages = getWorkerDetailMessages(locale);
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
  } = useScopedFactoryDocumentSave<EditableWorkerSaveValidationErrors>({
    fallbackErrorMessage: messages.editableConfigurationSaveFallbackError,
    isDirty,
    mapSaveErrorToFieldErrors: mapWorkerSaveErrorToFieldErrors,
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

    await saveNow({
      baseVersion: editableConfigurationState.baseVersion,
      factory: editableConfigurationState.pendingFactoryDefinition,
      previousFactory: editableConfigurationState.savedFactoryDefinition,
      onSaved: () => {
        editableConfigurationState.markChangesSaved();
        const savedWorkerName = editableConfigurationState.draft.name.trim();
        if (
          scopeKey != null &&
          savedWorkerName.length > 0 &&
          savedWorkerName !== scopeKey
        ) {
          onWorkerRenamed?.(savedWorkerName);
        }
      },
      scopeKey,
    });
  }, [editableConfigurationState, onWorkerRenamed, saveNow, scopeKey]);

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
