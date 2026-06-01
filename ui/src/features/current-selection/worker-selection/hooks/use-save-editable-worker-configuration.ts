import { useCallback, useMemo } from "react";
import { useScopedFactoryDocumentSave } from "../../base/public";
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

interface UseSaveEditableWorkerConfigurationResult {
  canSave: boolean;
  save: () => Promise<void>;
  saveAttemptRevision: number;
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

  const { isPending, saveAttemptRevision, saveNow, saveState } =
    useScopedFactoryDocumentSave<EditableWorkerSaveValidationErrors>({
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
      save,
      saveAttemptRevision,
      saveState,
    }),
    [canSave, save, saveAttemptRevision, saveState],
  );
}
