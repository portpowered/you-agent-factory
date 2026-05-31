import { useCallback, useMemo } from "react";

import type { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { useScopedFactoryDocumentSave } from "../../base/hooks/useScopedFactoryDocumentSave";
import type {
  EditableResourceConfigurationState,
  EditableResourceSaveState,
  EditableResourceSaveValidationErrors,
} from "../lib/detail-card-types";
import { getResourceDetailMessages } from "../messages/resource-detail";

interface UseSaveEditableResourceConfigurationOptions {
  editableConfigurationState?: EditableResourceConfigurationState;
  locale?: string | null;
  onResourceRenamed?: (resourceName: string) => void;
  scopeKey: string | null;
}

interface UseSaveEditableResourceConfigurationResult {
  canSave: boolean;
  save: () => Promise<void>;
  saveState: EditableResourceSaveState;
}

export function useSaveEditableResourceConfiguration({
  editableConfigurationState,
  locale,
  onResourceRenamed,
  scopeKey,
}: UseSaveEditableResourceConfigurationOptions): UseSaveEditableResourceConfigurationResult {
  const messages = getResourceDetailMessages(locale);
  const isDirty =
    editableConfigurationState?.status === "ready" &&
    editableConfigurationState.isDirty;

  const { isPending, saveNow, saveState } =
    useScopedFactoryDocumentSave<EditableResourceSaveValidationErrors>({
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
      scopeKey == null
    ) {
      return;
    }

    await saveNow({
      baseVersion: editableConfigurationState.baseVersion,
      factory: editableConfigurationState.pendingFactoryDefinition,
      onSaved: () => {
        editableConfigurationState.markChangesSaved();
        const savedResourceName = editableConfigurationState.draft.name.trim();
        if (
          scopeKey != null &&
          savedResourceName.length > 0 &&
          savedResourceName !== scopeKey
        ) {
          onResourceRenamed?.(savedResourceName);
        }
      },
      scopeKey,
    });
  }, [editableConfigurationState, onResourceRenamed, saveNow, scopeKey]);

  return useMemo(
    () => ({
      canSave,
      save,
      saveState,
    }),
    [canSave, save, saveState],
  );
}

function resolveSaveFieldErrors(
  error: Pick<CurrentFactoryDefinitionError, "message" | "targets">,
): EditableResourceSaveValidationErrors | undefined {
  const fieldErrors: EditableResourceSaveValidationErrors = {};

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
): keyof EditableResourceSaveValidationErrors | null {
  if (target.subject.type !== "RESOURCE") {
    return null;
  }

  const subjectID = target.subject.id.trim().toLowerCase();

  if (subjectID === "backend") {
    return "backend";
  }
  if (subjectID === "capacity") {
    return "capacity";
  }
  if (subjectID === "loadpolicy") {
    return "loadPolicy";
  }
  if (subjectID === "model") {
    return "model";
  }
  if (subjectID === "name") {
    return "name";
  }
  if (subjectID === "provider") {
    return "provider";
  }
  if (subjectID === "type") {
    return "type";
  }

  return null;
}
