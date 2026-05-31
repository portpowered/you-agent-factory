import { useCallback, useMemo } from "react";

import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { useScopedFactoryDocumentSave } from "../../base/hooks/useScopedFactoryDocumentSave";
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

interface UseSaveEditableWorkstationConfigurationResult {
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
    mapSaveErrorToFieldErrors: resolveSaveFieldErrors,
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

function resolveSaveFieldErrors(
  error: Pick<CurrentFactoryDefinitionError, "message" | "targets">,
): EditableWorkstationSaveValidationErrors | undefined {
  const fieldErrors: EditableWorkstationSaveValidationErrors = {};

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
): keyof EditableWorkstationSaveValidationErrors | null {
  const subjectID = target.subject.id.trim().toLowerCase();
  const subjectType = target.subject.type;
  const subjectLocation = target.subject.location;

  if (
    target.code === "factory.worker.danglingReference" &&
    subjectType === "WORKSTATION"
  ) {
    return "workerName";
  }
  if (
    subjectType === "WORKSTATION" &&
    (subjectLocation === "REFERENCE" || subjectLocation === "DEFINITION")
  ) {
    if (subjectID.endsWith("worker") || subjectID === "worker") {
      return "workerName";
    }
    if (subjectID === "behavior") {
      return "behavior";
    }
    if (subjectID === "body" || subjectID === "prompt") {
      return "prompt";
    }
    if (subjectID === "runner" || subjectID === "runnername") {
      return "runnerName";
    }
  }

  return null;
}
