import { useCallback, useMemo } from "react";

import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { useScopedFactoryDocumentSave } from "../../base/hooks/useScopedFactoryDocumentSave";
import type {
  EditableWorkTypeConfigurationState,
  EditableWorkTypeSaveState,
  EditableWorkTypeSaveValidationErrors,
} from "../lib/detail-card-types";
import { getWorkTypeDetailMessages } from "../messages/work-type-detail";

interface UseSaveEditableWorkTypeConfigurationOptions {
  editableConfigurationState?: EditableWorkTypeConfigurationState;
  locale?: string | null;
  onWorkTypeRenamed?: (workTypeName: string) => void;
  scopeKey: string | null;
}

export interface UseSaveEditableWorkTypeConfigurationResult {
  beginSaveConfirmation: () => void;
  canSave: boolean;
  cancelSaveConfirmation: () => void;
  confirmSave: () => Promise<void>;
  saveState: EditableWorkTypeSaveState;
}

export function useSaveEditableWorkTypeConfiguration({
  editableConfigurationState,
  locale,
  onWorkTypeRenamed,
  scopeKey,
}: UseSaveEditableWorkTypeConfigurationOptions): UseSaveEditableWorkTypeConfigurationResult {
  const messages = getWorkTypeDetailMessages(locale);
  const isDirty =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.isDirty;

  const {
    beginConfirmation,
    cancelConfirmation,
    confirmSave: confirmScopedSave,
    isPending,
    saveState,
  } = useScopedFactoryDocumentSave<EditableWorkTypeSaveValidationErrors>({
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
        const savedWorkTypeName = editableConfigurationState.draft.name.trim();
        if (
          scopeKey != null &&
          savedWorkTypeName.length > 0 &&
          savedWorkTypeName !== scopeKey
        ) {
          onWorkTypeRenamed?.(savedWorkTypeName);
        }
      },
      scopeKey,
    });
  }, [
    confirmScopedSave,
    editableConfigurationState,
    onWorkTypeRenamed,
    scopeKey,
  ]);

  return useMemo(
    () => ({
      beginSaveConfirmation: beginConfirmation,
      canSave,
      cancelSaveConfirmation: cancelConfirmation,
      confirmSave,
      saveState,
    }),
    [
      beginConfirmation,
      canSave,
      cancelConfirmation,
      confirmSave,
      saveState,
    ],
  );
}

function resolveSaveFieldErrors(
  error: Pick<CurrentFactoryDefinitionError, "message" | "targets">,
): EditableWorkTypeSaveValidationErrors | undefined {
  const fieldErrors: EditableWorkTypeSaveValidationErrors = {};

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
): keyof EditableWorkTypeSaveValidationErrors | null {
  if (target.subject.type !== "WORK_TYPE") {
    return null;
  }

  const subjectID = target.subject.id.trim().toLowerCase();

  if (subjectID === "name") {
    return "name";
  }
  if (subjectID === "handlingbehavior") {
    return "handlingBehavior";
  }

  return null;
}
