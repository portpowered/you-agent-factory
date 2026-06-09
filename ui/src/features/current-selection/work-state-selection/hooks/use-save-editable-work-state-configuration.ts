import { useCallback, useMemo } from "react";

import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { useScopedFactoryDocumentSave } from "../../base/hooks/useScopedFactoryDocumentSave";
import type {
  EditableWorkStateConfigurationState,
  EditableWorkStateSaveState,
  EditableWorkStateSaveValidationErrors,
} from "../lib/detail-card-types";
import { getWorkStateDetailMessages } from "../messages/work-state-detail";

interface UseSaveEditableWorkStateConfigurationOptions {
  editableConfigurationState?: EditableWorkStateConfigurationState;
  locale?: string | null;
  onWorkStateRenamed?: (placeId: string) => void;
  scopeKey: string | null;
}

export interface UseSaveEditableWorkStateConfigurationResult {
  canSave: boolean;
  lastSuccessfulSaveWasTopologyAffecting: boolean;
  save: () => Promise<void>;
  saveAttemptRevision: number;
  saveMutationError: CurrentFactoryDefinitionError | null;
  saveState: EditableWorkStateSaveState;
}

export function useSaveEditableWorkStateConfiguration({
  editableConfigurationState,
  locale,
  onWorkStateRenamed,
  scopeKey,
}: UseSaveEditableWorkStateConfigurationOptions): UseSaveEditableWorkStateConfigurationResult {
  const messages = getWorkStateDetailMessages(locale);
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
  } = useScopedFactoryDocumentSave<EditableWorkStateSaveValidationErrors>({
    fallbackErrorMessage: messages.editableConfigurationSaveFallbackError,
    isDirty,
    mapSaveErrorToFieldErrors: resolveSaveFieldErrors,
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
      scopeKey == null ||
      !editableConfigurationState.canSave
    ) {
      return;
    }

    const { originalStateName, workTypeName } = editableConfigurationState;

    await saveNow({
      baseVersion: editableConfigurationState.baseVersion,
      factory: editableConfigurationState.pendingFactoryDefinition,
      previousFactory: editableConfigurationState.savedFactoryDefinition,
      onSaved: () => {
        editableConfigurationState.markChangesSaved();
        const savedStateName = editableConfigurationState.draft.name.trim();
        if (savedStateName.length > 0 && savedStateName !== originalStateName) {
          onWorkStateRenamed?.(`${workTypeName}:${savedStateName}`);
        }
      },
      scopeKey,
    });
  }, [editableConfigurationState, onWorkStateRenamed, saveNow, scopeKey]);

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

function resolveSaveFieldErrors(
  error: Pick<CurrentFactoryDefinitionError, "message" | "targets">,
): EditableWorkStateSaveValidationErrors | undefined {
  const fieldErrors: EditableWorkStateSaveValidationErrors = {};

  for (const target of error.targets ?? []) {
    const fieldName = resolveTargetFieldName(target);
    if (fieldName === null) {
      continue;
    }
    fieldErrors[fieldName] ??= error.message;
  }

  return Object.keys(fieldErrors).length > 0 ? fieldErrors : undefined;
}

function resolveTargetFieldName(
  target: NonNullable<CurrentFactoryDefinitionError["targets"]>[number],
): keyof EditableWorkStateSaveValidationErrors | null {
  const subjectID = target.subject.id.trim().toLowerCase();
  const subjectType = target.subject.type;

  if (subjectType === "WORK_STATE" && subjectID === "name") {
    return "name";
  }

  if (subjectType === "WORK_TYPE" && subjectID === "name") {
    return "name";
  }

  if (
    /factory\.workTypes\[\d+\]\.states\[\d+\]\.name/.test(target.code) ||
    /factory\.workTypes\[\d+\]\.name/.test(target.code)
  ) {
    return "name";
  }

  return null;
}
