import { useCallback, useMemo } from "react";

import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { useScopedFactoryDocumentSave } from "../../base/hooks/useScopedFactoryDocumentSave";
import type {
  EditableWorkstationConfigurationState,
  EditableWorkstationSaveState,
  EditableWorkstationSaveValidationErrors,
} from "../lib/keys/detail-card-types";
import { parseWorkstationSaveScopeKey } from "../lib/keys/workstation-save-scope-key";
import { mapWorkstationSaveErrorToFieldErrors } from "../lib/validation/workstation-save-validation-field-mapping";
import { getWorkstationDetailMessages } from "../messages/workstation-detail";

interface UseSaveEditableWorkstationConfigurationOptions {
  editableConfigurationState?: EditableWorkstationConfigurationState;
  locale?: string | null;
  onWorkstationRenamed?: (nodeId: string) => void;
  scopeKey: string | null;
}

export interface UseSaveEditableWorkstationConfigurationResult {
  canSave: boolean;
  lastSuccessfulSaveWasTopologyAffecting: boolean;
  save: () => Promise<void>;
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
    error: saveMutationError,
    isPending,
    lastSuccessfulSaveWasTopologyAffecting,
    saveAttemptRevision,
    saveNow,
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

  const save = useCallback(async () => {
    if (
      !canSave ||
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
    canSave,
    editableConfigurationState,
    onWorkstationRenamed,
    saveNow,
    scopeKey,
  ]);

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
